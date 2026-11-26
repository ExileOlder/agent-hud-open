//go:build linux && cgo

package main

/*
#cgo pkg-config: x11 xrandr xext xft
#cgo LDFLAGS: -lm
#include <stdlib.h>
#include "x11hud.h"
*/
import "C"

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"runtime"
	"time"
	"unsafe"
)

func notifyHook(_ context.Context, code int, session uint32) error {
	if C.hud_post_completion(C.int(code), C.uint(session), nil) != 0 {
		return nil
	}
	return errors.New("start Balance HUD before sending a completion reminder")
}

func notifyApproval(_ context.Context, thread approvalThread) error {
	var route [4]C.uint
	for i := range route {
		route[i] = C.uint(binary.LittleEndian.Uint32(thread[4*i:]))
	}
	if C.hud_post_completion(C.int(codexApprovalCode), 0, &route[0]) == 0 {
		return errors.New("start Balance HUD before sending an approval reminder")
	}
	return nil
}

func runDesktop(fetch func() <-chan result, clients ...settingsTools) error {
	var tools settingsTools
	if len(clients) != 0 {
		tools = clients[0]
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	p := loadPlacement()
	var numbers C.int
	if p.Numbers {
		numbers = 1
	}
	motion := C.int(1)
	if p.ReduceMotion {
		motion = 0
	}
	var alreadyRunning C.int
	h := C.hud_open(C.int(p.Edge), C.double(p.Offset), numbers, motion, &alreadyRunning)
	if alreadyRunning != 0 {
		return nil
	}
	if h == nil {
		return errors.New("cannot open X11 display")
	}
	defer C.hud_close(h)
	C.hud_slots(h, C.int(p.slotCount()))
	set := func(rows []row) {
		for i, r := range rows {
			n := C.CString(r.Number)
			C.hud_number(h, C.int(i), n, C.int(r.quotaTone()))
			C.free(unsafe.Pointer(n))
			items := r.peekLine(i)
			for j, cell := range items {
				label, value := C.CString(cell.Label), C.CString(cell.Value)
				C.hud_cell(h, C.int(i), C.int(j), label, value, C.int(cell.Tone))
				C.free(unsafe.Pointer(label))
				C.free(unsafe.Pointer(value))
			}
			C.hud_peek(h, C.int(i), C.int(len(items)))
		}
	}
	rows := view(nil)
	var panel panelSettings
	drawSettings := func() {
		items := panel.items(p, rows)
		for i, item := range items {
			label, value := C.CString(item.title), C.CString(item.value)
			C.hud_settings_row(h, C.int(i), label, value)
			C.free(unsafe.Pointer(label))
			C.free(unsafe.Pointer(value))
		}
		help := C.CString(panel.help())
		C.hud_settings_sync(h, C.int(panel.page), C.int(len(items)), help)
		C.free(unsafe.Pointer(help))
	}
	set(rows)
	var painter barPainter
	started := time.Now()
	hoverStart := started
	lastHover := -1
	var previousFrames visualFrames
	var looks lookTransition
	animating, dirty := false, true
	var regions [5 * 4]C.int
	paint := func(frames visualFrames, full bool) {
		frame := painter.paintVisuals(rows, p, frames)
		n := 0
		if !full {
			for _, r := range painter.damage {
				if r.Empty() {
					continue
				}
				at := n * 4
				regions[at], regions[at+1] = C.int(r.Min.X), C.int(r.Min.Y)
				regions[at+2], regions[at+3] = C.int(r.Dx()), C.int(r.Dy())
				n++
			}
			if n == 0 {
				return
			}
		}
		C.hud_bitmap(h, C.int(frame.Bounds().Dx()), C.int(frame.Bounds().Dy()),
			(*C.uchar)(unsafe.Pointer(&frame.Pix[0])), &regions[0], C.int(n))
	}
	var ready <-chan result
	pending := false
	refresh := func() {
		if ready != nil {
			pending = true
			return
		}
		ready = fetch()
	}
	refresh()
	mail := restoredInbox()
	revision := -1
	notices := tools.notices
	var work workingMarks
	agyActivity := tools.antigravityActivity
	agyWaits := tools.antigravityWaits
	dshActivity := tools.deepSeekActivity
	renderMail := func(now time.Time) {
		var n [8]C.int
		C.hud_mail_bounds(h, &n[0])
		bar := mailRect{int(n[0]), int(n[1]), int(n[2]), int(n[3])}
		screen := mailRect{int(n[4]), int(n[5]), int(n[6]), int(n[7])}
		tail := inboxTail(bar, screen, p.Edge)
		mail.fitRows(screen.h)
		box := inboxPanel(tail, bar, screen, p.Edge, len(mail.visible()))
		hidden := C.hud_drag_active(h) != 0
		if revision != mail.revision {
			for i, item := range mail.visible() {
				title, value := C.CString(agentNames[item.agent]), C.CString(item.summary())
				C.hud_mail_row(h, C.int(i), title, value, C.int(item.tone()))
				C.free(unsafe.Pointer(title))
				C.free(unsafe.Pointer(value))
			}
			revision = mail.revision
		}
		count, opened := len(mail.items), C.int(0)
		if mail.open {
			opened = 1
		}
		if hidden {
			count, opened = 0, 0
		}
		C.hud_mail_sync(h, C.int(count), opened, C.int(tail.x), C.int(tail.y), C.int(box.x), C.int(box.y), C.int(box.w), C.int(box.h), C.int(len(mail.visible())))
		if len(mail.queue) > 0 && !hidden {
			source := mail.sourceSlot(mail.queue[0], rows)
			if source < 0 || source >= p.slotCount() {
				source = p.slotCount() / 2
			}
			_, _, step := barGeometry(p)
			pt, angle := mail.flightAt(now, flightGeometry{bar, tail, screen, p.Edge, source, step})
			var poly [12]C.int
			for i, q := range paperPoints(angle) {
				poly[2*i], poly[2*i+1] = C.int(q.x+.5), C.int(q.y+.5)
			}
			C.hud_mail_flight(h, 1, C.int(pt.x)-paperSize/2, C.int(pt.y)-paperSize/2, &poly[0])
		} else {
			C.hud_mail_flight(h, 0, 0, 0, nil)
		}
	}
	timer := time.NewTicker(5 * time.Minute)
	defer timer.Stop()
	for {
		timeout := 200
		if animating {
			timeout = 32
		}
		if len(mail.queue) > 0 {
			timeout = 16
		}
		action := int(C.hud_step(h, C.int(timeout)))
		switch {
		case action == 1:
			return nil
		case action == 2:
			p.Edge, p.Offset = int(C.hud_edge(h)), float64(C.hud_offset(h))
			p.Numbers = C.hud_numbers(h) != 0
			dirty = true
			if e := p.save(); e != nil {
				fmt.Fprintln(os.Stderr, "Cannot save preferences:", e)
			}
			drawSettings()
		case action == 3:
			refresh()
		case action == 4:
			drawSettings()
			C.hud_settings_show(h)
		case action >= 10 && action <= 11:
			panel.page, panel.message = action-10, ""
			drawSettings()
		case (action >= 90 && action <= 94) || action >= 300:
			if item, ok := mail.action(action); ok {
				openNotice(item)
			}
		case action == 200+codexApprovalCode:
			var thread approvalThread
			for i := range 4 {
				binary.LittleEndian.PutUint32(thread[4*i:], uint32(C.hud_hook_route(h, C.int(i))))
			}
			mail.codexApproval(thread, time.Now(), !p.ReduceMotion)
		case action >= 200:
			event := decodeHook(action-200, uint32(C.hud_hook_session(h)))
			work.hook(event, &mail, time.Now(), !p.ReduceMotion)
		case action >= 100:
			openProvider(action-100, "")
		case action >= 20:
			changed, reload := panel.apply(&p, action-20, rows, tools)
			if changed {
				C.hud_slots(h, C.int(p.slotCount()))
				if e := p.save(); e != nil {
					panel.message = "Could not save preferences: " + e.Error()
				}
				if panel.page == 0 {
					numbers, motion := C.int(0), C.int(1)
					if p.Numbers {
						numbers = 1
					}
					if p.ReduceMotion {
						motion = 0
					}
					C.hud_appearance(h, C.int(p.Edge), C.double(p.Offset), numbers, motion)
				} else {
					rows = updateView(rows, settingsResults(panel.raw, p))
					set(rows)
				}
				dirty = true
			}
			if reload {
				refresh()
			}
			drawSettings()
		}
		select {
		case incoming, ok := <-ready:
			if !ok {
				ready = nil
				if pending {
					pending = false
					refresh()
				} else if panel.message == "Refreshing balances…" {
					panel.message = "Balances refreshed."
					drawSettings()
				}
				break
			}
			panel.raw = mergeQuota(panel.raw, incoming)
			rows = updateView(rows, settingsResults(panel.raw, p))
			set(rows)
			drawSettings()
			dirty = true
		case <-timer.C:
			refresh()
		default:
		}
		select {
		case state, ok := <-dshActivity:
			if ok {
				work.updateDeepSeek(state, &mail, time.Now(), !p.ReduceMotion)
			} else {
				dshActivity = nil
			}
		case waits, ok := <-agyWaits:
			if ok {
				work.updateAntigravityWaits(waits, &mail, time.Now(), !p.ReduceMotion)
			} else {
				agyWaits = nil
			}
		case active, ok := <-agyActivity:
			if ok {
				work.updateAntigravity(active, time.Now())
			} else {
				agyActivity = nil
			}
		case event, ok := <-notices:
			if ok {
				if event.Activity {
					work.updateCodex(event, &mail, time.Now())
				} else {
					mail.receive(mailItem{agent: 1, event: event}, time.Now(), !p.ReduceMotion)
				}
			} else {
				notices = nil
			}
		default:
		}
		if len(mail.queue) > 0 || len(mail.items) > 0 || revision != mail.revision {
			now := time.Now()
			mail.advance(now, !p.ReduceMotion)
			renderMail(now)
		}
		now, hover := time.Now(), int(C.hud_hover(h))
		painter.avatars.trim(now)
		if hover != lastHover {
			hoverStart, lastHover = now, hover
		}
		frames, active := work.frames(rows, now, started, hoverStart, hover, p)
		frames, active = mail.cueFrames(now, p, rows, frames, active)
		visual, blending := looks.visual(now, frames, p.ReduceMotion)
		animating = active || blending
		if dirty || visual != previousFrames {
			paint(visual, dirty)
			previousFrames, dirty = visual, false
		}
	}
}
