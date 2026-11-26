package main

import (
	"math"
	"strconv"
	"time"
	"unsafe"
)

type winMail struct {
	mailInbox
	badge, panel, paper        uintptr
	lastTail, lastPanel        mailRect
	lastCount, lastRevision    int
	panelVisible, paperVisible bool
	poly                       [6]point
	leaveAt, clickAt           time.Time
	lastRow                    int
}

func rectToMail(r rect) mailRect {
	return mailRect{int(r.Left), int(r.Top), int(r.Right - r.Left), int(r.Bottom - r.Top)}
}
func showMailWindow(w uintptr, r mailRect) {
	u("SetWindowPos", w, ^uintptr(0), uintptr(r.x), uintptr(r.y), uintptr(r.w), uintptr(r.h), 0x10|0x40)
}
func fillMailRect(dc uintptr, x, y, w, h int32, brush uintptr) {
	rc := rect{x, y, x + w, y + h}
	u("FillRect", dc, uintptr(unsafe.Pointer(&rc)), brush)
}
func (m *winMail) renderWindow(hwnd, dc uintptr, write func(uintptr, int32, int32, string, uintptr), labelFont, headFont, bgBrush uintptr, measure func(uintptr, string) int32) {
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
		g("SelectObject", dc, headFont)
		write(dc, 22, 12, "消息", 0xf6f4f2)
		g("SelectObject", dc, labelFont)
		write(dc, int32(m.lastPanel.w-103), 17, "全部清空", 0xa9a6a5)
		for i, item := range m.visible() {
			y := int32(mailHeader + i*mailRow)
			fillMailRect(dc, 15, y, int32(m.lastPanel.w-30), mailRow-4, bgBrush)
			write(dc, 27, y+14, agentNames[item.agent], 0xf6f4f2)
			write(dc, max(168, 27+measure(dc, agentNames[item.agent])+18), y+14, noticeCopy(item.event.Question), 0xd4d0cc)
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
func (m *winMail) draw(now time.Time, frame, area rect, p placement, drag bool, shape func(uintptr, int32, int32, int32), invalidate func(uintptr)) {
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
		box := inboxPanel(tail, bar, screen, p.Edge, count)
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
	source := completionSlot(m.queue[0].agent)
	if source < 0 || source >= p.slotCount() {
		source = p.slotCount() / 2
	}
	_, _, step := barGeometry(p)
	at, angle := inboxFlight(bar, tail, screen, p.Edge, source, step, float64(now.Sub(m.started))/float64(mailFlight))
	for i, q := range paperPoints(angle) {
		m.poly[i] = point{int32(math.Round(q.x)), int32(math.Round(q.y))}
	}
	r := g("CreatePolygonRgn", uintptr(unsafe.Pointer(&m.poly[0])), 6, 2)
	if r != 0 && u("SetWindowRgn", m.paper, r, 1) == 0 {
		g("DeleteObject", r)
	}
	showMailWindow(m.paper, mailRect{int(at.x) - paperSize/2, int(at.y) - paperSize/2, paperSize, paperSize})
	m.paperVisible = true
	invalidate(m.paper)
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
		return 0, true
	case 0x2a3: // WM_MOUSELEAVE
		m.leaveAt = time.Now().Add(180 * time.Millisecond)
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
		if q.Y < mailHeader && int(q.X) > m.lastPanel.w-115 {
			return 92, true
		}
		if q.Y >= mailHeader {
			row := int(q.Y-mailHeader) / mailRow
			now := time.Now()
			if row < min(len(m.items), mailVisible) && m.lastRow == row && now.Sub(m.clickAt) <= 450*time.Millisecond {
				m.clickAt = time.Time{}
				return 300 + row, true
			}
			m.lastRow, m.clickAt = row, now
		}
		return 0, true
	}
	return 0, false
}
