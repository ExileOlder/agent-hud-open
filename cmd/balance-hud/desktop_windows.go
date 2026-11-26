package main

import (
	"fmt"
	"image"
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
	var bar, tip, notice uintptr
	var messages []int
	var noticeID int
	var noticeUntil time.Time
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
	var settings bool
	var hideAt time.Time
	hide := func() {
		hover = -1
		settings = false
		hideAt = time.Time{}
		if tip != 0 {
			u("ShowWindow", tip, 0)
		}
	}
	dimensions := func() (int32, int32, int32) { w, h, step := barGeometry(p); return int32(w), int32(h), int32(step) }
	shape := func(w uintptr, width, height, r int32) {
		region := g("CreateRoundRectRgn", 0, 0, uintptr(width+1), uintptr(height+1), uintptr(r), uintptr(r))
		if region != 0 && u("SetWindowRgn", w, region, 1) == 0 {
			g("DeleteObject", region)
		}
	}
	var barImage *image.RGBA
	const peekHeight, peekPad, peekGap, peekSpacing, gear = 64, 30, 12, 30, 45
	measure := func(dc uintptr, s string) int32 {
		t := syscall.StringToUTF16(strings.ReplaceAll(s, "\x00", ""))
		var size point
		g("GetTextExtentPoint32W", dc, uintptr(unsafe.Pointer(&t[0])), uintptr(len(t)-1), uintptr(unsafe.Pointer(&size)))
		return size.X
	}
	font := func(weight uintptr) uintptr {
		name := utf16("Segoe UI")
		return g("CreateFontW", ^uintptr(20), 0, 0, 0, weight, 0, 0, 0, 1, 0, 0, 5, 0, uintptr(unsafe.Pointer(name)))
	}
	labelFont, valueFont := font(400), font(600)
	defer g("DeleteObject", labelFont)
	defer g("DeleteObject", valueFont)
	hoverStart := time.Now()
	var motionAnimating bool

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
		shape(bar, w, h, 100)
		u("SetWindowPos", bar, ^uintptr(0), uintptr(x), uintptr(y), uintptr(w), uintptr(h), 0x10|0x40)
		invalidate(bar)
	}
	details := func(i int) {
		if i >= len(rows) || i < 0 {
			hide()
			return
		}
		if hover != i {
			hoverStart = time.Now()
			if !p.ReduceMotion {
				motionAnimating = true
				u("SetTimer", bar, 1, 40, 0)
				invalidate(bar)
			}
		}
		hover = i
		w, h := int32(2*peekPad+gear), int32(peekHeight)
		if settings {
			w, h = 320, 342
		} else {
			dc := u("GetDC", tip)
			if dc != 0 {
				old := g("SelectObject", dc, labelFont)
				for j, cell := range rows[i].peekLine(i) {
					g("SelectObject", dc, labelFont)
					w += measure(dc, cell.Label) + peekGap
					g("SelectObject", dc, valueFont)
					w += measure(dc, cell.Value)
					if j != 0 {
						w += peekSpacing
					}
				}
				g("SelectObject", dc, old)
				u("ReleaseDC", tip, dc)
			}
		}
		available := area.Right - area.Left
		if p.Edge%2 != 0 {
			available -= frame.Right - frame.Left + 3
		}
		w = min(w, available)
		h = min(h, area.Bottom-area.Top)
		_, _, step := dimensions()
		x, y := frame.Left+int32(i)*step+(step-w)/2, frame.Bottom
		switch p.Edge {
		case 1:
			x, y = frame.Left-w, frame.Top+barPadding+int32(i)*barItem+(barItem-h)/2
		case 2:
			y = frame.Top - h
		case 3:
			x, y = frame.Right, frame.Top+barPadding+int32(i)*barItem+(barItem-h)/2
		}
		x = bound(x, area.Left, area.Right-w)
		y = bound(y, area.Top, area.Bottom-h)
		if settings {
			shape(tip, w, h, 32)
		} else {
			shape(tip, w, h, peekHeight)
		}
		u("SetWindowPos", tip, ^uintptr(0), uintptr(x), uintptr(y), uintptr(w), uintptr(h), 0x10|0x40)
		invalidate(tip)
	}
	bg := g("CreateSolidBrush", 0x090705)
	button := g("CreateSolidBrush", 0x32281d)
	if bg == 0 || button == 0 {
		return fmt.Errorf("cannot create HUD panel brushes")
	}
	defer g("DeleteObject", bg)
	defer g("DeleteObject", button)
	options := func() []string {
		nums, motion := "Show percentages", "Turn off animation"
		if p.Numbers {
			nums = "Hide percentages"
		}
		if p.ReduceMotion {
			motion = "Turn on animation"
		}
		return []string{nums, motion, "Move to top edge", "Move to right edge", "Move to bottom edge", "Move to left edge", "Refresh balances", "Close settings"}
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
		if hwnd == notice {
			if noticeID != 0 {
				write(dc, 12, 8, agentNames[noticeID], 0xbde572)
				write(dc, 12, 28, "Task complete", 0xf5eee7)
			}
			return
		}
		if hwnd == tip {
			if hover < 0 {
				return
			}
			barButton := func(y int32, s string) {
				box := rect{13, y, r.Right - 13, y + 30}
				u("FillRect", dc, uintptr(unsafe.Pointer(&box)), button)
				write(dc, 24, y+6, s, 0xffc990)
			}
			if settings {
				write(dc, 20, 15, "Balance HUD settings", 0xf5eee7)
				write(dc, 20, 41, "Local appearance controls", 0xc8bbae)
				for i, label := range options() {
					barButton(72+int32(i)*32, label)
				}
			} else {
				x := int32(peekPad)
				y := (r.Bottom - 25) / 2
				tones := [...]uintptr{0xf6f4f2, 0x80d92d, 0x5ec5ff, 0x7868ff}
				g("IntersectClipRect", dc, 0, 0, uintptr(r.Right-gear), uintptr(r.Bottom))
				for _, cell := range rows[hover].peekLine(hover) {
					g("SelectObject", dc, labelFont)
					write(dc, x, y, cell.Label, 0x878787)
					x += measure(dc, cell.Label) + peekGap
					g("SelectObject", dc, valueFont)
					write(dc, x, y, cell.Value, tones[cell.Tone])
					x += measure(dc, cell.Value) + peekSpacing
				}
				g("SelectClipRgn", dc, 0)
				write(dc, r.Right-gear+5, y, "⚙", 0xf6f4f2)
			}
			return
		}
		phase := 0.0
		if hover >= 0 && !p.ReduceMotion {
			phase = clamp01(time.Since(hoverStart).Seconds() / hoverSeconds)
		}
		barImage = paintBar(rows, p, hover, phase, barImage)
		w, h := barImage.Bounds().Dx(), barImage.Bounds().Dy()
		info := struct {
			Size                   uint32
			Width, Height          int32
			Planes, Bits           uint16
			Compression, ImageSize uint32
			X, Y                   int32
			Colors, Important      uint32
		}{Size: 40, Width: int32(w), Height: -int32(h), Planes: 1, Bits: 32}
		g("StretchDIBits", dc, 0, 0, uintptr(w), uintptr(h), 0, 0, uintptr(w), uintptr(h),
			uintptr(unsafe.Pointer(&barImage.Pix[0])), uintptr(unsafe.Pointer(&info)), 0, 0x00cc0020)
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
		if i != hover && !settings {
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
	showNotice := func() {
		if notice == 0 || len(messages) == 0 {
			return
		}
		noticeID = messages[0]
		messages = messages[1:]
		noticeUntil = time.Now().Add(5 * time.Second)
		u("SetWindowPos", notice, ^uintptr(0), uintptr(area.Right-265), uintptr(area.Bottom-68), 250, 54, 0x10|0x40)
		invalidate(notice)
	}
	callback := syscall.NewCallback(func(hwnd uintptr, msg uint32, wp, lp uintptr) uintptr {
		if hwnd == tip && tip != 0 {
			switch msg {
			case 0x200:
				hideAt = time.Time{}
				event := track{Flags: 2, Window: tip}
				event.Size = uint32(unsafe.Sizeof(event))
				u("TrackMouseEvent", uintptr(unsafe.Pointer(&event)))
			case 0x2a3:
				if !settings {
					hideAt = time.Now().Add(180 * time.Millisecond)
				}
			case 0x205:
				hide()
				invalidate(bar)
				return 0
			case 0x202:
				m := mouse(lp)
				y := m.Y
				var bounds rect
				u("GetClientRect", tip, uintptr(unsafe.Pointer(&bounds)))
				if !settings && m.X >= bounds.Right-gear {
					settings = true
					details(hover)
					return 0
				}
				if settings && y >= 72 && y < 72+8*32 {
					selection := (y - 72) / 32
					switch selection {
					case 0:
						p.Numbers = !p.Numbers
						place()
						details(hover)
					case 1:
						p.ReduceMotion = !p.ReduceMotion
						invalidate(bar)
						details(hover)
					case 6:
						refresh()
						return 0
					case 7:
						hide()
						invalidate(bar)
						return 0
					default:
						p.Edge = int(selection - 2)
						hide()
						place()
					}
					save()
					return 0
				}
			}
		}
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
			case completionMessage:
				if wp > 0 && wp < uintptr(len(agentNames)) && len(messages) < 32 {
					messages = append(messages, int(wp))
					if noticeID == 0 {
						showNotice()
					}
				}
				return 0
			case 0x113: // WM_TIMER
				if !hideAt.IsZero() && time.Now().After(hideAt) {
					hide()
					invalidate(bar)
				}
				if motionAnimating {
					invalidate(bar)
					if hover < 0 || p.ReduceMotion || time.Since(hoverStart).Seconds() >= hoverSeconds {
						motionAnimating = false
						u("SetTimer", bar, 1, 200, 0)
					}
				}
				if noticeID != 0 && time.Now().After(noticeUntil) {
					noticeID = 0
					u("ShowWindow", notice, 0)
					showNotice()
				}
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
				hideAt = time.Time{}
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
				if !settings {
					hideAt = time.Now().Add(180 * time.Millisecond)
				}
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
	notice = makeWindow("Balance HUD completion")
	if notice == 0 {
		return fmt.Errorf("cannot create Win32 completion popup")
	}
	defer u("DestroyWindow", notice)
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
