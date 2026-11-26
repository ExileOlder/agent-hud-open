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
	"errors"
	"fmt"
	"image"
	"os"
	"runtime"
	"time"
	"unsafe"
)

func runDesktop(fetch func() []result, clients ...settingsTools) error {
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
	h := C.hud_open(C.int(p.Edge), C.double(p.Offset), numbers, motion)
	if h == nil {
		return errors.New("cannot open X11 display")
	}
	defer C.hud_close(h)
	set := func(rows []row) {
		for i, r := range rows {
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
	var frame *image.RGBA
	started := time.Now()
	hoverStart := started
	lastHover := -1
	var previousFrames markFrames
	animating, dirty := false, true
	paint := func(frames markFrames) {
		frame = paintBar(rows, p, frames, frame)
		C.hud_bitmap(h, C.int(frame.Bounds().Dx()), C.int(frame.Bounds().Dy()), (*C.uchar)(unsafe.Pointer(&frame.Pix[0])))
	}
	ready := make(chan []result, 1)
	inFlight := false
	refresh := func() {
		if !inFlight {
			inFlight = true
			go func() { ready <- fetch() }()
		}
	}
	refresh()
	timer := time.NewTicker(5 * time.Minute)
	defer timer.Stop()
	for {
		timeout := 200
		if animating {
			timeout = 32
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
		case action >= 20:
			changed, reload := panel.apply(&p, action-20, rows, tools)
			if changed {
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
		case incoming := <-ready:
			inFlight = false
			panel.raw = incoming
			rows = updateView(rows, settingsResults(incoming, p))
			set(rows)
			if panel.message == "Refreshing balances…" {
				panel.message = "Balances refreshed."
			}
			drawSettings()
			dirty = true
		case <-timer.C:
			refresh()
		default:
		}
		now, hover := time.Now(), int(C.hud_hover(h))
		if hover != lastHover {
			hoverStart, lastHover = now, hover
		}
		frames, active := motionFrames(now.Sub(started), hover, now.Sub(hoverStart), p.ReduceMotion)
		animating = active
		if dirty || frames != previousFrames {
			paint(frames)
			previousFrames, dirty = frames, false
		}
	}
}
