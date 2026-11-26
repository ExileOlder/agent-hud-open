package main

import (
	"math"
	"strconv"
	"time"
	"unsafe"
)

type winMail struct {
	mailInbox
	badge, panel, paper            uintptr
	lastTail, lastPanel, lastPaper mailRect
	lastCount, lastRevision        int
	panelVisible, paperVisible     bool
	poly                           [6]point
	leaveAt, clickAt               time.Time
	lastRow, hot                   int
}

func rectToMail(r rect) mailRect {
	return mailRect{int(r.Left), int(r.Top), int(r.Right - r.Left), int(r.Bottom - r.Top)}
}
func showMailWindow(w uintptr, r mailRect) {
	u("SetWindowPos", w, ^uintptr(0), uintptr(r.x), uintptr(r.y), uintptr(r.w), uintptr(r.h), 0x10|0x40)
}
func mailRGB(rgb uintptr) uintptr {
	return (rgb&0xff)<<16 | rgb&0xff00 | rgb>>16
}
func fillMailCard(dc uintptr, x, y, w, h, r int32, rgb uintptr) {
	brush := g("CreateSolidBrush", mailRGB(rgb))
	oldBrush := g("SelectObject", dc, brush)
	oldPen := g("SelectObject", dc, g("GetStockObject", 8))
	g("RoundRect", dc, uintptr(x), uintptr(y), uintptr(x+w), uintptr(y+h), uintptr(2*r), uintptr(2*r))
	g("SelectObject", dc, oldBrush)
	g("SelectObject", dc, oldPen)
	g("DeleteObject", brush)
}
func drawMailStroke(dc, rgb, width uintptr, lines ...[4]int32) {
	pen := g("CreatePen", 0, width, mailRGB(rgb))
	old := g("SelectObject", dc, pen)
	for _, l := range lines {
		g("MoveToEx", dc, uintptr(l[0]), uintptr(l[1]), 0)
		g("LineTo", dc, uintptr(l[2]), uintptr(l[3]))
	}
	g("SelectObject", dc, old)
	g("DeleteObject", pen)
}
func (m *winMail) renderWindow(hwnd, dc uintptr, write func(uintptr, int32, int32, string, uintptr), labelFont, mailSmall, mailTitle, bgBrush uintptr, extent func(uintptr, string) point) {
	measure := func(dc uintptr, text string) int32 { return extent(dc, text).X }
	switch hwnd {
	case m.badge:
		g("SelectObject", dc, labelFont)
		stroke := func(color, width uintptr, fn func()) {
			pen := g("CreatePen", 0, width, color)
			old := g("SelectObject", dc, pen)
			fn()
			g("SelectObject", dc, old)
			g("DeleteObject", pen)
		}
		nullBrush := g("GetStockObject", 5)
		oldBrush := g("SelectObject", dc, nullBrush)
		stroke(0x7b7874, 1, func() { g("Ellipse", dc, 1, 1, mailSize-1, mailSize-1) })
		path := [6]point{{26, 57}, {26, 31}, {56, 31}, {56, 51}, {34, 51}, {26, 57}}
		stroke(0xffaa7e, 2, func() {
			g("Polyline", dc, uintptr(unsafe.Pointer(&path[0])), 6)
			for y := 36; y <= 45; y += 4 {
				g("MoveToEx", dc, 33, uintptr(y), 0)
				g("LineTo", dc, 49, uintptr(y))
			}
		})
		g("SelectObject", dc, bgBrush)
		g("Ellipse", dc, 52, 4, 82, 34)
		g("SelectObject", dc, oldBrush)
		number := strconv.Itoa(len(m.items))
		write(dc, int32(68-4*len(number)), 10, number, 0xf6f4f2)
	case m.panel:
		width, height := int32(m.lastPanel.w), int32(m.lastPanel.h)
		const base, rowBG, hotBG, button, rule = 0x101418, 0x191f25, 0x222c36, 0x2c3947, 0x26313b
		fillMailCard(dc, 0, 0, width, height, 22, base)
		g("SelectObject", dc, mailTitle)
		write(dc, 27, 16, "消息", 0xf6f4f2)
		g("SelectObject", dc, mailSmall)
		total := strconv.Itoa(len(m.items))
		fillMailCard(dc, 121, 17, 55, 40, 11, button)
		write(dc, 148-measure(dc, total)/2, 22, total, 0xf6f4f2)
		clearBG := uintptr(rowBG)
		if m.hot == 92 {
			clearBG = hotBG
		}
		fillMailCard(dc, width-198, 14, 176, 45, 11, clearBG)
		write(dc, width-110-measure(dc, "全部清空")/2, 22, "全部清空", 0xf6f4f2)
		drawMailStroke(dc, rule, 1, [4]int32{22, 66, width - 23, 66})
		tiles := [3]uintptr{0x15362e, 0x1b314a, 0x3b2c24}
		accents := [3]uintptr{0x72dcb1, 0x92baff, 0xf0be85}
		for i, item := range m.visible() {
			y := int32(mailHeader + i*mailRow)
			color := uintptr(rowBG)
			if m.hot == mailOpenBase+i || m.hot == mailDismissBase+i {
				color = hotBG
			}
			fillMailCard(dc, 16, y, width-32, mailRow-12, 15, color)
			y -= 6
			tone := item.tone()
			fillMailCard(dc, 30, y+28, 52, 52, 13, tiles[tone])
			switch tone {
			case 0:
				drawMailStroke(dc, accents[tone], 3, [4]int32{46, y + 55, 53, y + 62}, [4]int32{53, y + 62, 67, y + 45})
			case 1:
				write(dc, 56-measure(dc, "?")/2, y+40, "?", mailRGB(accents[tone]))
			case 2:
				drawMailStroke(dc, accents[tone], 3, [4]int32{56, y + 42, 56, y + 56}, [4]int32{56, y + 61, 56, y + 64})
			}
			const lineAdvance = 46
			status := item.summary()
			textY := y + 6 + (mailRow-12-lineAdvance-extent(dc, status).Y)/2
			g("SelectObject", dc, mailTitle)
			write(dc, 106, textY, agentNames[item.agent], 0xf6f4f2)
			g("SelectObject", dc, mailSmall)
			write(dc, 106, textY+lineAdvance, status, mailRGB(accents[tone]))
			x, cy := width-58, y+55
			closeInk := uintptr(0x74787b)
			if m.hot == mailDismissBase+i {
				fillMailCard(dc, width-88, y+26, 59, 58, 12, button)
				closeInk = 0xe7eef5
			}
			drawMailStroke(dc, closeInk, 3, [4]int32{x - 7, cy - 7, x + 8, cy + 8}, [4]int32{x - 7, cy + 7, x + 8, cy - 8})
		}
	case m.paper:
		pen := g("CreatePen", 0, 1, 0xffaa7e)
		old := g("SelectObject", dc, pen)
		oldBrush := g("SelectObject", dc, g("GetStockObject", 0))
		g("Polygon", dc, uintptr(unsafe.Pointer(&m.poly[0])), 6)
		for _, i := range []int{2, 4} {
			g("MoveToEx", dc, uintptr(m.poly[0].X), uintptr(m.poly[0].Y), 0)
			g("LineTo", dc, uintptr(m.poly[i].X), uintptr(m.poly[i].Y))
		}
		g("SelectObject", dc, oldBrush)
		g("SelectObject", dc, old)
		g("DeleteObject", pen)
	}
}
func (m *winMail) draw(now time.Time, frame, area rect, p placement, rows []row, drag bool, shape func(uintptr, int32, int32, int32), invalidate func(uintptr)) {
	if m.badge == 0 || m.panel == 0 || m.paper == 0 {
		return
	}
	m.advance(now, !p.ReduceMotion)
	bar, screen := rectToMail(frame), rectToMail(area)
	tail := inboxTail(bar, screen, p.Edge)
	count := len(m.items)
	if drag {
		count = 0
	}
	if count == 0 {
		if m.lastCount != 0 {
			u("ShowWindow", m.badge, 0)
		}
	} else if tail != m.lastTail || m.lastCount == 0 {
		shape(m.badge, mailSize, mailSize, mailSize)
		showMailWindow(m.badge, tail)
	}
	if count != m.lastCount {
		invalidate(m.badge)
	}
	m.lastTail, m.lastCount = tail, count
	if !m.leaveAt.IsZero() && !now.Before(m.leaveAt) {
		m.leaveAt = time.Time{}
		m.open = false
	}
	show := count > 0 && m.open
	if show {
		m.fitRows(screen.h)
		box := inboxPanel(tail, bar, screen, p.Edge, len(m.visible()))
		if box != m.lastPanel || !m.panelVisible {
			shape(m.panel, int32(box.w), int32(box.h), 44)
			showMailWindow(m.panel, box)
			m.lastPanel = box
		}
		if !m.panelVisible || m.lastRevision != m.revision {
			invalidate(m.panel)
		}
	} else if m.panelVisible {
		u("ShowWindow", m.panel, 0)
	}
	m.panelVisible, m.lastRevision = show, m.revision
	if len(m.queue) == 0 || drag {
		if m.paperVisible {
			u("ShowWindow", m.paper, 0)
			m.paperVisible = false
		}
		return
	}
	source := m.sourceSlot(m.queue[0], rows)
	if source < 0 || source >= p.slotCount() {
		source = p.slotCount() / 2
	}
	_, _, step := barGeometry(p)
	at, angle := m.flightAt(now, flightGeometry{bar, tail, screen, p.Edge, source, step})
	var outline [6]point
	for i, q := range paperPoints(angle) {
		outline[i] = point{int32(math.Round(q.x)), int32(math.Round(q.y))}
	}
	first, changed := !m.paperVisible, m.poly != outline
	if first || changed {
		m.poly = outline
		r := g("CreatePolygonRgn", uintptr(unsafe.Pointer(&m.poly[0])), 6, 2)
		if r != 0 && u("SetWindowRgn", m.paper, r, 1) == 0 {
			g("DeleteObject", r)
		}
	}
	bounds := mailRect{int(at.x) - paperSize/2, int(at.y) - paperSize/2, paperSize, paperSize}
	if first || bounds != m.lastPaper {
		showMailWindow(m.paper, bounds)
		m.lastPaper = bounds
	}
	m.paperVisible = true
	if first || changed {
		invalidate(m.paper)
	}
}
func (m *winMail) windowAction(hwnd uintptr, msg uint32, wp, lp uintptr) (int, bool) {
	if hwnd != m.badge && hwnd != m.panel {
		return 0, false
	}
	switch msg {
	case 0x200: // WM_MOUSEMOVE
		m.leaveAt = time.Time{}
		e := track{Flags: 2, Window: hwnd}
		e.Size = uint32(unsafe.Sizeof(e))
		u("TrackMouseEvent", uintptr(unsafe.Pointer(&e)))
		if hwnd == m.badge && !m.open {
			return 90, true
		}
		if hwnd == m.panel {
			q := mouse(lp)
			if next := mailTarget(int(q.X), int(q.Y), m.lastPanel.w, len(m.visible())); next != m.hot {
				m.hot = next
				u("InvalidateRect", m.panel, 0, 0)
			}
		}
		return 0, true
	case 0x2a3: // WM_MOUSELEAVE
		m.leaveAt = time.Now().Add(180 * time.Millisecond)
		if hwnd == m.panel && m.hot != 0 {
			m.hot = 0
			u("InvalidateRect", m.panel, 0, 0)
		}
		return 0, true
	case 0x20a: // WM_MOUSEWHEEL
		if hwnd == m.panel {
			if int16(wp>>16) > 0 {
				return 93, true
			}
			return 94, true
		}
		return 0, true
	case 0x205:
		return 91, true
	case 0x202:
		if hwnd == m.badge {
			return 90, true
		}
		q := mouse(lp)
		hit := mailTarget(int(q.X), int(q.Y), m.lastPanel.w, len(m.visible()))
		if hit == 92 || hit >= mailDismissBase {
			m.clickAt = time.Time{}
			return hit, true
		}
		if hit >= mailOpenBase {
			now := time.Now()
			if m.lastRow == hit && now.Sub(m.clickAt) <= 450*time.Millisecond {
				m.clickAt = time.Time{}
				return hit, true
			}
			m.lastRow, m.clickAt = hit, now
		}
		return 0, true
	}
	return 0, false
}
