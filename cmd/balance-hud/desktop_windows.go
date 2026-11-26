package main

import (
	"fmt"
	"math"
	"os"
	"runtime"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

var user32 = syscall.NewLazyDLL("user32.dll")
var gdi32 = syscall.NewLazyDLL("gdi32.dll")
var kernel32 = syscall.NewLazyDLL("kernel32.dll")
var procedures = map[string]*syscall.LazyProc{} // GUI thread only

//go:uintptrescapes
func call(dll *syscall.LazyDLL, name string, args ...uintptr) uintptr {
	p := procedures[name]
	if p == nil {
		p = dll.NewProc(name)
		procedures[name] = p
	}
	v, _, _ := p.Call(args...)
	return v
}

//go:uintptrescapes
func u(name string, args ...uintptr) uintptr { return call(user32, name, args...) }

//go:uintptrescapes
func g(name string, args ...uintptr) uintptr { return call(gdi32, name, args...) }

type point struct{ X, Y int32 }
type rect struct{ Left, Top, Right, Bottom int32 }
type winClass struct {
	Style                              uint32
	Proc                               uintptr
	ClsExtra, WndExtra                 int32
	Instance, Icon, Cursor, Background uintptr
	Menu, Name                         *uint16
}
type message struct {
	Window         uintptr
	Message        uint32
	WParam, LParam uintptr
	Time           uint32
	Point          point
	Private        uint32
}
type paint struct {
	DC                 uintptr
	Erase              int32
	Rect               rect
	Restore, IncUpdate int32
	Reserved           [32]byte
}
type track struct {
	Size, Flags uint32
	Window      uintptr
	HoverTime   uint32
}

func utf16(s string) *uint16 {
	p, e := syscall.UTF16PtrFromString(strings.ReplaceAll(s, "\x00", ""))
	if e != nil {
		panic(e)
	}
	return p
}
func bound(n, lo, hi int32) int32 { return max(lo, min(n, hi)) }
func mouse(lp uintptr) point      { return point{int32(int16(lp)), int32(int16(lp >> 16))} }

func runDesktop(fetch func() []result) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	p := loadPlacement()
	rows := view(nil)
	var bar, tip uintptr
	var area, frame rect
	var hover = -1
	var pressing, moved bool
	var initial point
	ready := make(chan []result, 1)
	busy := false
	next := time.Time{}
	refresh := func() {
		if busy {
			return
		}
		busy = true
		go func() { ready <- fetch() }()
	}
	hide := func() {
		hover = -1
		if tip != 0 {
			u("ShowWindow", tip, 0)
		}
	}
	dimensions := func() (int32, int32, int32) {
		step := int32(36)
		if p.Numbers && p.Edge%2 == 0 {
			step = 74
		}
		if p.Edge%2 == 0 {
			return 8 + 4*step, 44, step
		}
		if p.Numbers {
			return 82, 152, step
		}
		return 44, 152, step
	}
	invalidate := func(w uintptr) { u("InvalidateRect", w, 0, 0) }
	place := func() {
		if u("SystemParametersInfoW", 48, 0, uintptr(unsafe.Pointer(&area)), 0) == 0 {
			area = rect{0, 0, int32(u("GetSystemMetrics", 0)), int32(u("GetSystemMetrics", 1))}
		}
		w, h, _ := dimensions()
		x := area.Left + int32(float64(area.Right-area.Left-w)*p.Offset)
		y := area.Top + int32(float64(area.Bottom-area.Top-h)*p.Offset)
		switch p.Edge {
		case 0:
			y = area.Top + 3
		case 1:
			x = area.Right - w - 3
		case 2:
			y = area.Bottom - h - 3
		case 3:
			x = area.Left + 3
		}
		frame = rect{x, y, x + w, y + h}
		u("SetWindowPos", bar, ^uintptr(0), uintptr(x), uintptr(y), uintptr(w), uintptr(h), 0x10|0x40)
		invalidate(bar)
	}
	details := func(i int) {
		if i >= len(rows) || i < 0 {
			hide()
			return
		}
		hover = i
		lines := strings.Split(rows[i].Details, "\n")
		w, h := int32(120), int32(len(lines)*18+14)
		for _, s := range lines {
			w = max(w, int32(len([]rune(s))*8+16))
		}
		w = min(w, area.Right-area.Left)
		h = min(h, area.Bottom-area.Top)
		x, y := frame.Left, frame.Bottom+4
		switch p.Edge {
		case 1:
			x, y = frame.Left-w-4, frame.Top
		case 2:
			y = frame.Top - h - 4
		case 3:
			x, y = frame.Right+4, frame.Top
		}
		x = bound(x, area.Left, area.Right-w)
		y = bound(y, area.Top, area.Bottom-h)
		u("SetWindowPos", tip, ^uintptr(0), uintptr(x), uintptr(y), uintptr(w), uintptr(h), 0x10|0x40)
		invalidate(tip)
	}
	ink := []uintptr{0xbde572, 0xffb88f, 0xffb88f, 0x77b8ff}
	bg := g("CreateSolidBrush", 0x231b15)
	if bg == 0 {
		return fmt.Errorf("cannot create HUD background")
	}
	defer g("DeleteObject", bg)
	pens := make([]uintptr, len(ink))
	for i, c := range ink {
		pens[i] = g("CreatePen", 0, 1, c)
		if pens[i] != 0 {
			defer g("DeleteObject", pens[i])
		}
	}
	trackPen := g("CreatePen", 0, 1, 0x635346)
	mutedPen := g("CreatePen", 0, 1, 0xb4a89d)
	if trackPen != 0 {
		defer g("DeleteObject", trackPen)
	}
	if mutedPen != 0 {
		defer g("DeleteObject", mutedPen)
	}
	write := func(dc uintptr, x, y int32, s string, color uintptr) {
		s = strings.ReplaceAll(s, "\x00", "")
		t := utf16(s)
		g("SetTextColor", dc, color)
		g("TextOutW", dc, uintptr(x), uintptr(y), uintptr(unsafe.Pointer(t)), uintptr(len(syscall.StringToUTF16(s))-1))
	}
	render := func(hwnd uintptr) {
		var ps paint
		dc := u("BeginPaint", hwnd, uintptr(unsafe.Pointer(&ps)))
		if dc == 0 {
			return
		}
		defer u("EndPaint", hwnd, uintptr(unsafe.Pointer(&ps)))
		var r rect
		u("GetClientRect", hwnd, uintptr(unsafe.Pointer(&r)))
		u("FillRect", dc, uintptr(unsafe.Pointer(&r)), bg)
		g("SetBkMode", dc, 1)
		font := g("GetStockObject", 17)
		if font != 0 {
			old := g("SelectObject", dc, font)
			defer g("SelectObject", dc, old)
		}
		if hwnd == tip {
			if hover >= 0 {
				for i, s := range strings.Split(rows[hover].Details, "\n") {
					write(dc, 8, 6+int32(i*18), s, 0xf5eee7)
				}
			}
			return
		}
		_, _, step := dimensions()
		old := g("SelectObject", dc, g("GetStockObject", 5))
		defer g("SelectObject", dc, old)
		for i, r := range rows {
			x, y := int32(4)+int32(i)*step, int32(4)
			if p.Edge%2 != 0 {
				x, y = 4, 4+int32(i)*36
			}
			if trackPen != 0 {
				oldPen := g("SelectObject", dc, trackPen)
				g("Ellipse", dc, uintptr(x+4), uintptr(y+4), uintptr(x+32), uintptr(y+32))
				g("SelectObject", dc, oldPen)
			}
			pen, color := pens[i], ink[i]
			if r.Stale {
				pen, color = mutedPen, 0xb4a89d
			}
			if pen != 0 && r.Percent > 0 {
				oldPen := g("SelectObject", dc, pen)
				if r.Percent >= 100 {
					g("Ellipse", dc, uintptr(x+4), uintptr(y+4), uintptr(x+32), uintptr(y+32))
				} else {
					theta := float64(r.Percent) * 2 * math.Pi / 100
					endX := x + 18 + int32(math.Round(14*math.Sin(theta)))
					endY := y + 18 - int32(math.Round(14*math.Cos(theta)))
					g("SetArcDirection", dc, 2)
					g("Arc", dc, uintptr(x+4), uintptr(y+4), uintptr(x+32), uintptr(y+32),
						uintptr(x+18), uintptr(y+4), uintptr(endX), uintptr(endY))
				}
				g("SelectObject", dc, oldPen)
			}
			write(dc, x+18-int32(len(r.Mark)*4), y+10, r.Mark, color)
			if p.Numbers {
				write(dc, x+38, y+10, r.Number, 0xf5eee7)
			}
		}
	}
	save := func() {
		if e := p.save(); e != nil {
			fmt.Fprintln(os.Stderr, "Cannot save appearance:", e)
		}
	}
	pick := func(m point) {
		_, _, step := dimensions()
		v := m.X
		if p.Edge%2 != 0 {
			v = m.Y
		}
		if v < 4 {
			hide()
			return
		}
		i := int((v - 4) / step)
		if i != hover {
			details(i)
		}
	}
	drop := func() {
		var cur point
		if u("GetCursorPos", uintptr(unsafe.Pointer(&cur))) == 0 {
			return
		}
		x, y := bound(cur.X, area.Left, area.Right-1), bound(cur.Y, area.Top, area.Bottom-1)
		d := []int32{y - area.Top, area.Right - 1 - x, area.Bottom - 1 - y, x - area.Left}
		for i, v := range d {
			if v < d[p.Edge] {
				p.Edge = i
			}
		}
		if p.Edge%2 != 0 {
			p.Offset = float64(y-area.Top) / float64(max(1, area.Bottom-area.Top-1))
		} else {
			p.Offset = float64(x-area.Left) / float64(max(1, area.Right-area.Left-1))
		}
		place()
		save()
	}
	callback := syscall.NewCallback(func(hwnd uintptr, msg uint32, wp, lp uintptr) uintptr {
		if msg == 0xf {
			render(hwnd)
			return 0
		}
		if hwnd == bar && bar != 0 {
			switch msg {
			case 0x10: // WM_CLOSE
				u("DestroyWindow", bar)
				return 0
			case 2:
				u("PostQuitMessage", 0)
				return 0
			case 0x113: // WM_TIMER
				// Our non-activating window may not get button-up outside it.
				if pressing {
					key := uintptr(1)
					if u("GetSystemMetrics", 23) != 0 {
						key = 2
					}
					if u("GetAsyncKeyState", key)&0x8000 == 0 {
						pressing = false
						u("ReleaseCapture")
						var cur point
						if u("GetCursorPos", uintptr(unsafe.Pointer(&cur))) != 0 &&
							(moved || math.Abs(float64(cur.X-initial.X))+math.Abs(float64(cur.Y-initial.Y)) >= 6) {
							drop()
						}
					}
				}
				select {
				case incoming := <-ready:
					busy = false
					rows = updateView(rows, incoming)
					invalidate(bar)
					if hover >= 0 {
						details(hover)
					}
				default:
				}
				if time.Now().After(next) {
					refresh()
					next = time.Now().Add(5 * time.Minute)
				}
				return 0
			case 0x200: // WM_MOUSEMOVE
				m := mouse(lp)
				if pressing {
					var cur point
					if u("GetCursorPos", uintptr(unsafe.Pointer(&cur))) != 0 && math.Abs(float64(cur.X-initial.X))+math.Abs(float64(cur.Y-initial.Y)) >= 6 {
						moved = true
					}
				} else {
					event := track{Flags: 2, Window: bar}
					event.Size = uint32(unsafe.Sizeof(event))
					u("TrackMouseEvent", uintptr(unsafe.Pointer(&event)))
					pick(m)
				}
				return 0
			case 0x2a3:
				hide()
				return 0
			case 0x201:
				hide()
				pressing = u("GetCursorPos", uintptr(unsafe.Pointer(&initial))) != 0
				moved = false
				if pressing {
					u("SetCapture", bar)
				}
				return 0
			case 0x202:
				if !pressing {
					return 0
				}
				pressing = false
				u("ReleaseCapture")
				if !moved {
					pick(mouse(lp))
					return 0
				}
				drop()
				return 0
			case 0x205:
				if wp&4 != 0 {
					u("DestroyWindow", bar)
					return 0
				}
				hide()
				p.Numbers = !p.Numbers
				place()
				save()
				return 0
			case 0x208:
				refresh()
				return 0
			case 0x7e:
				hide()
				place()
				return 0 // display change
			}
		}
		return u("DefWindowProcW", hwnd, uintptr(msg), wp, lp)
	})
	cls := utf16("BalanceHUD")
	instance := call(kernel32, "GetModuleHandleW", 0)
	wc := winClass{Proc: callback, Instance: instance, Cursor: u("LoadCursorW", 0, 32512), Name: cls}
	if u("RegisterClassW", uintptr(unsafe.Pointer(&wc))) == 0 {
		return fmt.Errorf("cannot register Win32 HUD window")
	}
	makeWindow := func(title string) uintptr {
		return u("CreateWindowExW", 0x08000088, uintptr(unsafe.Pointer(cls)), uintptr(unsafe.Pointer(utf16(title))), 0x80000000, 0, 0, 1, 1, 0, 0, instance, 0)
	}
	bar = makeWindow("Balance HUD")
	if bar == 0 {
		return fmt.Errorf("cannot create Win32 HUD window")
	}
	defer u("DestroyWindow", bar)
	tip = makeWindow("Balance HUD quota details")
	if tip == 0 {
		return fmt.Errorf("cannot create Win32 quota popup")
	}
	defer u("DestroyWindow", tip)
	place()
	if u("SetTimer", bar, 1, 200, 0) == 0 {
		return fmt.Errorf("cannot start Win32 quota timer")
	}
	defer u("KillTimer", bar, 1)
	for {
		var msg message
		r := int32(u("GetMessageW", uintptr(unsafe.Pointer(&msg)), 0, 0, 0))
		if r == -1 {
			return fmt.Errorf("Win32 message loop failed")
		}
		if r == 0 {
			return nil
		}
		u("TranslateMessage", uintptr(unsafe.Pointer(&msg)))
		u("DispatchMessageW", uintptr(unsafe.Pointer(&msg)))
	}
}
