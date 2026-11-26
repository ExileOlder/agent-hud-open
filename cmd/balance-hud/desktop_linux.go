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

func runDesktop(fetch func() []result) error {
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
		switch C.hud_step(h, C.int(timeout)) {
		case 1:
			return nil
		case 2:
			p = placement{Edge: int(C.hud_edge(h)), Offset: float64(C.hud_offset(h)), Numbers: C.hud_numbers(h) != 0, ReduceMotion: C.hud_motion(h) == 0}
			dirty = true
			if e := p.save(); e != nil {
				fmt.Fprintln(os.Stderr, "Cannot save appearance:", e)
			}
		case 3:
			refresh()
		}
		select {
		case incoming := <-ready:
			inFlight = false
			rows = updateView(rows, incoming)
			set(rows)
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
