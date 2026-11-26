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

// Paint the gear directly so its shape does not depend on the selected font.
func paintGear(dc uintptr, x, y int32, background uintptr) {
	var teeth [32]point
	for i := range teeth {
		angle := (float64(i) + .5) * 2 * math.Pi / float64(len(teeth))
		radius := 7.0
		if i%4 == 1 || i%4 == 2 {
			radius = 10
		}
		teeth[i] = point{x + int32(math.Round(radius*math.Cos(angle))), y + int32(math.Round(radius*math.Sin(angle)))}
	}
	pen := g("SelectObject", dc, g("GetStockObject", 8))   // NULL_PEN
	brush := g("SelectObject", dc, g("GetStockObject", 0)) // WHITE_BRUSH
	g("Polygon", dc, uintptr(unsafe.Pointer(&teeth[0])), uintptr(len(teeth)))
	g("SelectObject", dc, background)
	g("Ellipse", dc, uintptr(x-4), uintptr(y-4), uintptr(x+4), uintptr(y+4))
	g("SelectObject", dc, brush)
	g("SelectObject", dc, pen)
}

func runDesktop(fetch func() <-chan result, clients ...settingsTools) error {
	var tools settingsTools
	if len(clients) != 0 {
		tools = clients[0]
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	p := loadPlacement()
	rows := view(nil)
	var bar, tip, prefs uintptr
	mail := winMail{mailInbox: restoredInbox()}
	var panel panelSettings
	const prefsW, prefsH, sidebarW, prefsTop, prefsRow = 760, 565, 176, 107, 49
	notices := tools.notices
	var work workingMarks
	agyActivity := tools.antigravityActivity
	agyWaits := tools.antigravityWaits
	dshActivity := tools.deepSeekActivity
	var iconClick time.Time
	lastSlot := -1
	var area, frame rect
	var hover = -1
	var pressing, moved, outlined bool
	var initial point
	var dragOrigin rect
	var downAt time.Time
	var ready <-chan result
	pending := false
	next := time.Time{}
	refresh := func() {
		if ready != nil {
			pending = true
			return
		}
		ready = fetch()
	}
	var hideAt, peekAt time.Time
	var peekProgress float64
	var peekTarget bool
	var tipRect, tipFrom rect
	var tipSlideAt time.Time
	var drawnTipWidth, drawnTipHeight int32
	hide := func() {
		hover = -1
		hideAt, peekAt = time.Time{}, time.Time{}
		peekTarget, peekProgress = false, 0
		tipSlideAt = time.Time{}
		if tip != 0 {
			u("ShowWindow", tip, 0)
			u("KillTimer", tip, 2)
		}
	}
	dimensions := func() (int32, int32, int32) { w, h, step := barGeometry(p); return int32(w), int32(h), int32(step) }
	shape := func(w uintptr, width, height, r int32) {
		region := g("CreateRoundRectRgn", 0, 0, uintptr(width+1), uintptr(height+1), uintptr(r), uintptr(r))
		if region != 0 && u("SetWindowRgn", w, region, 1) == 0 {
			g("DeleteObject", region)
		}
	}
	var painter barPainter
	const peekHeight, peekPad, peekGap, peekSpacing, gear = 64, 30, 12, 30, 45
	extent := func(dc uintptr, s string) point {
		t := syscall.StringToUTF16(strings.ReplaceAll(s, "\x00", ""))
		var size point
		g("GetTextExtentPoint32W", dc, uintptr(unsafe.Pointer(&t[0])), uintptr(len(t)-1), uintptr(unsafe.Pointer(&size)))
		return size
	}
	measure := func(dc uintptr, s string) int32 { return extent(dc, s).X }
	font := func(weight uintptr, size uintptr) uintptr {
		name := utf16("Segoe UI")
		return g("CreateFontW", ^size, 0, 0, 0, weight, 0, 0, 0, 1, 0, 0, 5, 0, uintptr(unsafe.Pointer(name)))
	}
	labelFont, valueFont := font(400, 20), font(600, 20)
	defer g("DeleteObject", labelFont)
	defer g("DeleteObject", valueFont)
	figureFont := font(400, 16)
	defer g("DeleteObject", figureFont)
	mailSmall, mailTitle := font(400, 24), font(600, 32)
	defer g("DeleteObject", mailSmall)
	defer g("DeleteObject", mailTitle)
	prefsFont := font(400, 15)
	defer g("DeleteObject", prefsFont)
	started := time.Now()
	hoverStart := started
	var motionAnimating bool
	var previousFrames visualFrames
	var looks lookTransition

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
	tipGeometry := func(now time.Time) rect {
		if tipSlideAt.IsZero() {
			return tipRect
		}
		k := peekSwitchEase(now.Sub(tipSlideAt))
		if k >= 1 {
			tipSlideAt = time.Time{}
			return tipRect
		}
		mix := func(a, b int32) int32 { return a + int32(math.Round(float64(b-a)*k)) }
		x, y := mix(tipFrom.Left, tipRect.Left), mix(tipFrom.Top, tipRect.Top)
		w, h := mix(tipFrom.Right-tipFrom.Left, tipRect.Right-tipRect.Left), tipRect.Bottom-tipRect.Top
		return rect{x, y, x + w, y + h}
	}
	peekFrame := func(show bool) {
		t := peekProgress
		eased := t * t * (3 - 2*t)
		delta := int32(math.Round(24 * (1 - eased)))
		r := tipGeometry(time.Now())
		x, y := r.Left, r.Top
		w, h := r.Right-r.Left, r.Bottom-r.Top
		if w != drawnTipWidth || h != drawnTipHeight {
			drawnTipWidth, drawnTipHeight = w, h
			shape(tip, w, h, peekHeight)
		}
		switch p.Edge {
		case 0:
			y -= delta
		case 1:
			x += delta
		case 2:
			y += delta
		case 3:
			x -= delta
		}
		u("SetLayeredWindowAttributes", tip, 0, uintptr(math.Round(255*eased)), 2)
		flags := uintptr(0x4 | 0x10)
		if show {
			flags |= 0x40
		}
		u("SetWindowPos", tip, 0, uintptr(x), uintptr(y), uintptr(w), uintptr(h), flags)
	}
	peekGoal := func(shown bool, now time.Time) {
		if hover < 0 || peekTarget == shown {
			return
		}
		peekTarget = shown
		if p.ReduceMotion {
			if !shown {
				hide()
			} else {
				peekProgress = 1
			}
			return
		}
		peekAt = now
		u("SetTimer", tip, 2, 16, 0)
	}
	peekTick := func(now time.Time) {
		if !hideAt.IsZero() && !now.Before(hideAt) {
			hideAt = time.Time{}
			peekGoal(false, now)
		}
		if peekAt.IsZero() {
			if !tipSlideAt.IsZero() {
				peekFrame(false)
			}
			if hideAt.IsZero() && tipSlideAt.IsZero() {
				u("KillTimer", tip, 2)
			}
			return
		}
		elapsed := now.Sub(peekAt)
		peekAt = now
		if peekTarget {
			peekProgress += float64(elapsed) / float64(300*time.Millisecond)
			if peekProgress >= 1 {
				peekProgress = 1
				peekAt = time.Time{}
			}
		} else {
			peekProgress -= float64(elapsed) / float64(235*time.Millisecond)
			if peekProgress <= 0 {
				hide()
				return
			}
		}
		peekFrame(false)
	}
	resumePeek := func() { hideAt = time.Time{}; peekGoal(true, time.Now()) }
	scheduleHide := func() {
		if hover < 0 {
			return
		}
		if p.ReduceMotion {
			hide()
			return
		}
		hideAt = time.Now().Add(120 * time.Millisecond)
		u("SetTimer", tip, 2, 16, 0)
	}
	details := func(i int) {
		if i >= p.slotCount() || i < 0 {
			hide()
			return
		}
		if hover != i {
			hoverStart = time.Now()
			if !p.ReduceMotion {
				motionAnimating = true
				u("SetTimer", bar, 1, 32, 0)
				invalidate(bar)
			}
		}
		fresh := hover < 0
		hover = i
		if fresh {
			resumePeek()
		}
		w, h := int32(2*peekPad+gear), int32(peekHeight)
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
		target := rect{x, y, x + w, y + h}
		if target != tipRect {
			if !fresh && !p.ReduceMotion {
				now := time.Now()
				tipFrom = tipGeometry(now)
				tipSlideAt = now
				u("SetTimer", tip, 2, 16, 0)
			} else {
				tipSlideAt = time.Time{}
			}
			tipRect = target
		}
		if fresh {
			u("SetWindowPos", tip, ^uintptr(0), 0, 0, 0, 0, 0x1|0x2|0x10)
			u("SetWindowPos", bar, ^uintptr(0), 0, 0, 0, 0, 0x1|0x2|0x10)
		}
		peekFrame(true)
		// Publish the new mark's text immediately instead of showing the
		// previously painted mark until the message queue reaches WM_PAINT.
		u("RedrawWindow", tip, 0, 0, 0x101)
	}
	bg := g("CreateSolidBrush", 0x090705)
	if bg == 0 {
		return fmt.Errorf("cannot create HUD panel brushes")
	}
	defer g("DeleteObject", bg)
	prefsBG := g("CreateSolidBrush", 0x292624)
	sideBG := g("CreateSolidBrush", 0x1f1c1a)
	selectedBG := g("CreateSolidBrush", 0xa35d22)
	defer g("DeleteObject", prefsBG)
	defer g("DeleteObject", sideBG)
	defer g("DeleteObject", selectedBG)
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
		if hwnd == mail.badge || hwnd == mail.panel || hwnd == mail.paper {
			mail.renderWindow(hwnd, dc, write, prefsFont, mailSmall, mailTitle, sideBG, extent)
			return
		}
		if hwnd == prefs {
			fill := func(x, y, w, h int32, b uintptr) {
				rc := rect{x, y, x + w, y + h}
				u("FillRect", dc, uintptr(unsafe.Pointer(&rc)), b)
			}
			fill(0, 0, prefsW, prefsH, prefsBG)
			fill(0, 0, sidebarW, prefsH, sideBG)
			g("SelectObject", dc, valueFont)
			write(dc, 20, 20, "Balance HUD", 0xf6f4f2)
			title, desc := "Display", "Choose how the float sits on your desktop."
			if panel.page == 1 {
				title, desc = "Quota accounts", "Choose which subscriptions the float follows."
			}
			write(dc, sidebarW+28, 25, title, 0xf6f4f2)
			g("SelectObject", dc, prefsFont)
			write(dc, sidebarW+28, 60, desc, 0xa9a6a5)
			for i, label := range []string{"Display", "Accounts"} {
				y := int32(68 + i*44)
				if i == panel.page {
					fill(12, y, sidebarW-24, 38, selectedBG)
				}
				write(dc, 23, y+8, label, 0xf6f4f2)
			}
			for i, item := range panel.items(p, rows) {
				y := int32(prefsTop + i*prefsRow)
				fill(sidebarW+28, y, prefsW-sidebarW-56, prefsRow-4, sideBG)
				write(dc, sidebarW+40, y+13, item.title, 0xf6f4f2)
				g("IntersectClipRect", dc, sidebarW+322, uintptr(y), prefsW-42, uintptr(y+prefsRow-4))
				write(dc, prefsW-42-measure(dc, item.value), y+13, item.value, 0x80d92d)
				g("SelectClipRgn", dc, 0)
			}
			write(dc, sidebarW+28, prefsH-51, panel.help(), 0xa9a6a5)
			return
		}
		if hwnd == tip {
			if hover < 0 {
				return
			}

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
			paintGear(dc, r.Right-gear/2-3, r.Bottom/2, bg)
			return
		}
		barImage := painter.paintVisuals(rows, p, previousFrames)
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
		if p.Numbers {
			g("SelectObject", dc, figureFont)
			_, _, step := barGeometry(p)
			for i, r := range rows[:p.slotCount()] {
				x, y := barPadding+i*step+barItem-2, barPadding+barItem/2-10
				if p.Edge%2 != 0 {
					x, y = barItem-2, barPadding+i*barItem+barItem/2-10
				}
				ink := quotaInks[r.quotaTone()]
				rgb := uintptr(ink.R) | uintptr(ink.G)<<8 | uintptr(ink.B)<<16
				if r.Percent < 0 {
					rgb = 0xb5aea5
				}
				write(dc, int32(x), int32(y), r.Number, rgb)
			}
		}
		if outlined {
			pen := g("CreatePen", 1, 1, 0xf6f4f2) // PS_DASH
			oldPen := g("SelectObject", dc, pen)
			oldBrush := g("SelectObject", dc, g("GetStockObject", 5)) // NULL_BRUSH
			g("RoundRect", dc, 2, 2, uintptr(w-1), uintptr(h-1), 92, 92)
			g("SelectObject", dc, oldBrush)
			g("SelectObject", dc, oldPen)
			g("DeleteObject", pen)
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
	drag := func(cur point) {
		moved, outlined = true, true
		x := dragOrigin.Left + cur.X - initial.X
		y := dragOrigin.Top + cur.Y - initial.Y
		u("SetWindowPos", bar, 0, uintptr(x), uintptr(y), 0, 0, 0x1|0x4|0x10)
		invalidate(bar)
	}
	drop := func(cur point) {
		x, y := bound(cur.X, area.Left, area.Right-1), bound(cur.Y, area.Top, area.Bottom-1)
		d := []int32{y - area.Top, area.Right - 1 - x, area.Bottom - 1 - y, x - area.Left}
		for i := range d {
			if d[i] < d[p.Edge] {
				p.Edge = i
			}
		}
		w, h, _ := dimensions()
		centerX := (dragOrigin.Left+dragOrigin.Right)/2 + cur.X - initial.X - area.Left
		centerY := (dragOrigin.Top+dragOrigin.Bottom)/2 + cur.Y - initial.Y - area.Top
		position, travel := centerX-w/2, area.Right-area.Left-w
		if p.Edge%2 != 0 {
			position, travel = centerY-h/2, area.Bottom-area.Top-h
		}
		p.Offset = .5
		if travel > 0 {
			p.Offset = float64(bound(position, 0, travel)) / float64(travel)
		}
		hide()
		place()
		save()
	}
	finishDrag := func() {
		pressing, outlined = false, false
		u("ReleaseCapture")
		var cur point
		if moved && u("GetCursorPos", uintptr(unsafe.Pointer(&cur))) != 0 {
			drop(cur)
		}
		invalidate(bar)
	}

	renderMail := func(now time.Time) { mail.draw(now, frame, area, p, rows, pressing && moved, shape, invalidate) }
	handleIconClick := func(m point) bool {
		_, _, step := dimensions()
		v := m.X
		if p.Edge%2 != 0 {
			v = m.Y
		}
		if v < barPadding {
			return false
		}
		slot := int((v - barPadding) / step)
		if slot >= p.slotCount() {
			return false
		}
		now := time.Now()
		if slot == lastSlot && now.Sub(iconClick) <= 450*time.Millisecond {
			iconClick = time.Time{}
			hide()
			invalidate(bar)
			openProvider(slot, "")
			return true
		}
		lastSlot, iconClick = slot, now
		return false
	}
	callback := syscall.NewCallback(func(hwnd uintptr, msg uint32, wp, lp uintptr) uintptr {
		if action, handled := mail.windowAction(hwnd, msg, wp, lp); handled {
			if item, ok := mail.action(action); ok {
				openNotice(item)
			}
			renderMail(time.Now())
			return 0
		}
		if hwnd == prefs && prefs != 0 {
			switch msg {
			case 0x10:
				u("ShowWindow", prefs, 0)
				return 0
			case 0x100:
				if wp == 27 {
					u("ShowWindow", prefs, 0)
					return 0
				}
			case 0x202:
				m := mouse(lp)
				if m.X < sidebarW && m.Y >= 68 && m.Y < 156 {
					panel.page, panel.message = int((m.Y-68)/44), ""
				} else if m.X >= sidebarW+28 && m.Y >= prefsTop && m.Y < prefsTop+int32(len(panel.items(p, rows)))*prefsRow {
					count := p.slotCount()
					changed, reload := panel.apply(&p, int((m.Y-prefsTop)/prefsRow), rows, tools)
					if changed {
						save()
						if panel.page == 0 || count != p.slotCount() {
							hide()
							place()
						} else {
							rows = updateView(rows, settingsResults(panel.raw, p))
							invalidate(bar)
						}
					}
					if reload {
						refresh()
					}
				}
				invalidate(prefs)
				return 0
			}
		}
		if hwnd == tip && tip != 0 {
			switch msg {
			case 0x113:
				if wp == 2 {
					peekTick(time.Now())
					return 0
				}
			case 0x200:
				resumePeek()
				event := track{Flags: 2, Window: tip}
				event.Size = uint32(unsafe.Sizeof(event))
				u("TrackMouseEvent", uintptr(unsafe.Pointer(&event)))
			case 0x2a3:
				scheduleHide()
			case 0x205:
				hide()
				invalidate(bar)
				return 0
			case 0x202:
				var bounds rect
				u("GetClientRect", tip, uintptr(unsafe.Pointer(&bounds)))
				if mouse(lp).X >= bounds.Right-gear {
					hide()
					u("ShowWindow", prefs, 5)
					u("SetForegroundWindow", prefs)
					invalidate(prefs)
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
			case approvalMessage:
				now := time.Now()
				mail.codexApproval(approvalFromMessage(wp, lp), now, !p.ReduceMotion)
				renderMail(now)
				return 0
			case completionMessage:
				now := time.Now()
				work.hook(decodeHook(int(wp), uint32(lp)), &mail.mailInbox, now, !p.ReduceMotion)
				renderMail(now)
				return 0
			case 0x113: // WM_TIMER
				now := time.Now()
				painter.avatars.trim(now)
				frames, active := work.frames(rows, now, started, hoverStart, hover, p)
				frames, active = mail.cueFrames(now, p, rows, frames, active)
				visual, blending := looks.visual(now, frames, p.ReduceMotion)
				if previousFrames != visual {
					previousFrames = visual
					invalidate(bar)
				}
				select {
				case state, ok := <-dshActivity:
					if ok {
						work.updateDeepSeek(state, &mail.mailInbox, time.Now(), !p.ReduceMotion)
					} else {
						dshActivity = nil
					}
				case waits, ok := <-agyWaits:
					if ok {
						work.updateAntigravityWaits(waits, &mail.mailInbox, time.Now(), !p.ReduceMotion)
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
							work.updateCodex(event, &mail.mailInbox, now)
						} else {
							mail.receive(mailItem{agent: 1, event: event}, now, !p.ReduceMotion)
						}
					} else {
						notices = nil
					}
				default:
				}
				active = active || blending || pressing || len(mail.queue) > 0
				if motionAnimating != active {
					motionAnimating = active
					interval := uintptr(200)
					if active {
						interval = 32
					}
					u("SetTimer", bar, 1, interval, 0)
				}
				renderMail(now)
				if pressing {
					key := uintptr(1)
					if u("GetSystemMetrics", 23) != 0 {
						key = 2
					}
					if u("GetAsyncKeyState", key)&0x8000 == 0 {
						finishDrag()
					} else if !outlined && time.Since(downAt) >= 300*time.Millisecond {
						outlined = true
						invalidate(bar)
					}
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
							invalidate(prefs)
						}
						break
					}
					panel.raw = mergeQuota(panel.raw, incoming)
					rows = updateView(rows, settingsResults(panel.raw, p))
					invalidate(bar)
					invalidate(prefs)
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
				resumePeek()
				if pressing {
					var cur point
					if u("GetCursorPos", uintptr(unsafe.Pointer(&cur))) != 0 && (outlined || math.Hypot(float64(cur.X-initial.X), float64(cur.Y-initial.Y)) >= 6) {
						drag(cur)
					}
				} else {
					event := track{Flags: 2, Window: bar}
					event.Size = uint32(unsafe.Sizeof(event))
					u("TrackMouseEvent", uintptr(unsafe.Pointer(&event)))
					pick(m)
				}
				return 0
			case 0x2a3:
				scheduleHide()
				return 0
			case 0x201:
				hide()
				pressing = u("GetCursorPos", uintptr(unsafe.Pointer(&initial))) != 0
				moved, outlined = false, false
				dragOrigin, downAt = frame, time.Now()
				if pressing {
					u("SetCapture", bar)
				}
				return 0
			case 0x202:
				if !pressing {
					return 0
				}
				finishDrag()
				if !moved {
					if m := mouse(lp); !handleIconClick(m) {
						pick(m)
					}
				} else {
					iconClick = time.Time{}
				}
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
	makeWindow := func(title string, extra uintptr) uintptr {
		return u("CreateWindowExW", 0x08000088|extra, uintptr(unsafe.Pointer(cls)), uintptr(unsafe.Pointer(utf16(title))), 0x80000000, 0, 0, 1, 1, 0, 0, instance, 0)
	}
	bar = makeWindow("Balance HUD", 0)
	if bar == 0 {
		return fmt.Errorf("cannot create Win32 HUD window")
	}
	defer u("DestroyWindow", bar)
	tip = makeWindow("Balance HUD quota details", 0x80000)
	if tip == 0 {
		return fmt.Errorf("cannot create Win32 quota popup")
	}
	defer u("DestroyWindow", tip)
	mail.badge = makeWindow("Balance HUD messages", 0)
	mail.panel = makeWindow("Balance HUD inbox", 0)
	mail.paper = makeWindow("Balance HUD paper flight", 0x20)
	if mail.badge == 0 || mail.panel == 0 || mail.paper == 0 {
		return fmt.Errorf("cannot create Win32 message surfaces")
	}
	defer u("DestroyWindow", mail.badge)
	defer u("DestroyWindow", mail.panel)
	defer u("DestroyWindow", mail.paper)
	place()
	const settingsStyle = 0x00ca0000 // normal bordered, titled window with close/minimize
	size := rect{Right: prefsW, Bottom: prefsH}
	u("AdjustWindowRectEx", uintptr(unsafe.Pointer(&size)), settingsStyle, 0, 0)
	pw, ph := size.Right-size.Left, size.Bottom-size.Top
	px, py := area.Left+(area.Right-area.Left-pw)/2, area.Top+(area.Bottom-area.Top-ph)/2
	prefs = u("CreateWindowExW", 0, uintptr(unsafe.Pointer(cls)), uintptr(unsafe.Pointer(utf16("Balance HUD Settings"))),
		settingsStyle, uintptr(px), uintptr(py), uintptr(pw), uintptr(ph), 0, 0, instance, 0)
	if prefs == 0 {
		return fmt.Errorf("cannot create Win32 settings window")
	}
	defer u("DestroyWindow", prefs)
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
