//go:build linux && cgo

package main

/*
#cgo pkg-config: x11 xrandr
#include <stdlib.h>
#include "x11hud.h"
*/
import "C"

import (
	"errors"
	"fmt"
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
	h := C.hud_open(C.int(p.Edge), C.double(p.Offset), numbers)
	if h == nil {
		return errors.New("cannot open X11 display")
	}
	defer C.hud_close(h)
	set := func(rows []row) {
		for i, r := range rows {
			mark, num, details := C.CString(r.Mark), C.CString(r.Number), C.CString(r.Details)
			stale := C.int(0)
			if r.Stale {
				stale = 1
			}
			C.hud_row(h, C.int(i), mark, num, details, C.int(r.Percent), stale)
			C.free(unsafe.Pointer(mark))
			C.free(unsafe.Pointer(num))
			C.free(unsafe.Pointer(details))
		}
	}
	rows := view(nil)
	set(rows)
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
		switch C.hud_step(h, 200) {
		case 1:
			return nil
		case 2:
			p = placement{int(C.hud_edge(h)), float64(C.hud_offset(h)), C.hud_numbers(h) != 0}
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
		case <-timer.C:
			refresh()
		default:
		}
	}
}
