package main

import (
	"math"
	"strings"
	"time"

	"github.com/ExileOlder/agent-hud-open/internal/codexnotice"
)

// Native port of Agent HUD's dock tail (end-geometry.ts: trailing circle, 6px
// gap, fallback to the head side when out of room) and its paper silhouette
// (flight-plan.ts). The inbox stores only a routing ID and a fixed status.
const (
	mailSize        = 84
	mailGap         = 9
	mailWidth       = 608
	mailHeader      = 72
	mailRow         = 108
	mailVisible     = 6
	mailLimit       = 32
	mailFlight      = 620 * time.Millisecond
	mailOpenBase    = 300
	mailDismissBase = 320
)

type mailRect struct{ x, y, w, h int }
type mailPoint struct{ x, y float64 }
type mailItem struct {
	agent      int
	event      codexnotice.Event
	permission bool
	profile    uint32
	account    string
	key        string
}

func (item mailItem) tone() int {
	if item.permission {
		return 2
	}
	if item.event.Question {
		return 1
	}
	return 0
}

func (item mailItem) summary() string {
	if item.permission {
		return "等你确认一下"
	}
	return noticeCopy(item.event.Question)
}

type mailInbox struct {
	items, queue []mailItem
	statePath    string
	path         flightPath
	started      time.Time
	open         bool
	revision     int
	offset       int
	pageSize     int
	scrollGrab   int // 1 + pointer distance from thumb top; zero when released
	cues         [5]markCue
	profiles     map[string]uint32
	agyCues      map[uint32]markCue
}

func (m *mailInbox) codexHookNotice(thread codexHookThread, approval bool, now time.Time, motion bool) {
	m.receive(mailItem{agent: 1, permission: approval, event: codexnotice.Event{Thread: thread.route(), Question: approval}}, now, motion)
}

func (m *mailInbox) receive(item mailItem, now time.Time, motion bool) {
	if key := item.noticeKey(); key != "" {
		for _, group := range [][]mailItem{m.items, m.queue} {
			for _, old := range group {
				if old.agent == item.agent && old.tone() == item.tone() && old.noticeKey() == key {
					return
				}
			}
		}
	}
	defer m.saveNotices()
	if motion {
		look := cueDone
		if item.permission {
			look = cuePermission
		} else if item.event.Question {
			look = cueQuestion
		}
		cue := markCue{look, now}
		if item.agent == 2 {
			if item.profile != 0 {
				for _, p := range m.profiles {
					if p == item.profile {
						if m.agyCues == nil {
							m.agyCues = make(map[uint32]markCue)
						}
						m.agyCues[p] = cue
						break
					}
				}
			}
		} else if slot := completionSlot(item.agent); slot >= 0 {
			m.cues[slot] = cue
		}
	}
	if !motion {
		m.advance(now, false)
		m.land(item)
		return
	}
	if len(m.queue) == mailLimit {
		// Keep arrival order even when many requests arrive during one flight.
		m.land(m.queue[0])
		m.queue = m.queue[1:]
		m.started = now
	}
	m.queue = append(m.queue, item)
	if m.started.IsZero() {
		m.started = now
	}
}

type cueLook uint8

const (
	cueQuestion cueLook = iota
	cuePermission
	cueDone
)

type markCue struct {
	look cueLook
	at   time.Time
}

func (m *mailInbox) cueFrames(now time.Time, p placement, rows []row, frames markFrames, active bool) (markFrames, bool) {
	if p.ReduceMotion {
		return frames, active
	}
	for i := 0; i < p.slotCount(); i++ {
		cue := m.cues[i]
		var profile uint32
		if i == 1 || i == 2 {
			profile = m.profiles[strings.ToLower(rows[i].Identity)]
			cue = m.agyCues[profile]
		}
		if cue.at.IsZero() {
			continue
		}
		fps, length := 12, 24
		if cue.look == cueDone {
			fps, length = 16, cueFrames
		}
		frame := int(max(0, now.Sub(cue.at)) * time.Duration(fps) / time.Second)
		if frame >= length {
			if profile != 0 {
				delete(m.agyCues, profile)
			} else {
				m.cues[i] = markCue{}
			}
			continue
		}
		if frames[i] >= pendingBase && frames[i] < wakeBase || (cue.look == cueDone && frames[i] >= workBase) {
			continue
		}
		frames[i] = cueBase + int(cue.look)*cueFrames + frame
		active = true
	}
	return frames, active
}

func (m *mailInbox) clearWaitCue(slot int) {
	if m.cues[slot].look != cueDone {
		m.cues[slot] = markCue{}
	}
}
func (m *mailInbox) sourceSlot(item mailItem, rows []row) int {
	if item.agent == 2 && item.account != "" {
		for slot := 1; slot <= 2; slot++ {
			if strings.EqualFold(rows[slot].Identity, item.account) {
				return slot
			}
		}
	}
	if item.agent == 2 && item.profile != 0 {
		for _, i := range []int{1, 2} {
			if m.profiles[strings.ToLower(rows[i].Identity)] == item.profile {
				return i
			}
		}
	}
	if item.agent == 2 && item.event.Question {
		return -1 // No account attribution for a shared or unknown service.
	}
	return completionSlot(item.agent)
}

func (m *mailInbox) land(item mailItem) {
	m.revision++
	if m.offset > 0 {
		m.offset++ // Keep the same message near the top during a new arrival.
	}
	m.items = append([]mailItem{item}, m.items...)
	if len(m.items) > mailLimit {
		m.items = m.items[:mailLimit]
	}
	m.offset = min(m.offset, max(0, len(m.items)-m.pageRows()))
}
func (m *mailInbox) advance(now time.Time, motion bool) {
	if !motion {
		for _, item := range m.queue {
			m.land(item)
		}
		m.queue = nil
		m.started = time.Time{}
		return
	}
	if len(m.queue) > 0 && now.Sub(m.started) >= mailFlight {
		m.land(m.queue[0])
		m.queue = m.queue[1:]
		m.started = time.Time{}
		if len(m.queue) > 0 {
			m.started = now
		}
	}
}
func (m *mailInbox) action(action int) (mailItem, bool) {
	switch {
	case action == 90:
		m.open = len(m.items) > 0
	case action == 91:
		m.open = false
		m.scrollGrab = 0
	case action == 92:
		m.items, m.queue = nil, nil
		m.started = time.Time{}
		m.revision++
		m.open = false
		m.offset = 0
		m.scrollGrab = 0
		m.saveNotices()
	case action == 93 || action == 94:
		step := 1
		if action == 93 {
			step = -1
		}
		m.scrollTo(m.offset + step)
	case action >= mailDismissBase && action < mailDismissBase+len(m.visible()):
		index := m.offset + action - mailDismissBase
		copy(m.items[index:], m.items[index+1:])
		m.items[len(m.items)-1] = mailItem{}
		m.items = m.items[:len(m.items)-1]
		m.offset = max(0, min(m.offset, len(m.items)-m.pageRows()))
		m.revision++
		if len(m.items) == 0 {
			m.open = false
		}
		m.saveNotices()
	case action >= mailOpenBase && action < mailOpenBase+len(m.visible()):
		return m.visible()[action-mailOpenBase], true
	}
	return mailItem{}, false
}

// Cards stay on row boundaries; the thumb travels proportionately to the
// whole inbox, even when a shorter screen shows fewer rows.
const mailScrollBottom = 24

func (m *mailInbox) thumb(height int) (top, size int) {
	track := height - mailHeader - mailScrollBottom
	if track <= 0 || len(m.items) <= m.pageRows() {
		return 0, 0
	}
	size = min(track, max(24, track*m.pageRows()/len(m.items)))
	top = mailHeader + (track-size)*m.offset/(len(m.items)-m.pageRows())
	return
}

func (m *mailInbox) scrollTo(offset int) {
	offset = max(0, min(offset, len(m.items)-m.pageRows()))
	if m.offset != offset {
		m.offset = offset
		m.revision++
	}
}

func (m *mailInbox) beginScroll(y, height int) {
	top, size := m.thumb(height)
	if size == 0 {
		return
	}
	if y >= top && y < top+size {
		m.scrollGrab = 1 + y - top
	} else if y < top {
		m.scrollTo(m.offset - m.pageRows())
	} else {
		m.scrollTo(m.offset + m.pageRows())
	}
}

func (m *mailInbox) dragScroll(y, height int) {
	_, size := m.thumb(height)
	travel := height - mailHeader - mailScrollBottom - size
	if m.scrollGrab == 0 || size == 0 || travel <= 0 {
		return
	}
	pos := min(travel, max(0, y-(m.scrollGrab-1)-mailHeader))
	m.scrollTo((pos*(len(m.items)-m.pageRows()) + travel/2) / travel)
}

func (m *mailInbox) pageRows() int {
	if m.pageSize > 0 {
		return m.pageSize
	}
	return mailVisible
}
func (m *mailInbox) fitRows(height int) {
	n := max(1, min(mailVisible, (height-mailHeader-12)/mailRow))
	if n == m.pageRows() {
		return
	}
	m.pageSize = n
	m.offset = max(0, min(m.offset, len(m.items)-n))
	m.revision++
}
func (m *mailInbox) visible() []mailItem {
	return m.items[m.offset:min(len(m.items), m.offset+m.pageRows())]
}

// Hit testing matches the card geometry. The × is separate from the
// double-click target so dismissing a row can never open its client.
func mailTarget(x, y, width, rows int) int {
	if x >= width-198 && x < width-22 && y >= 14 && y < 59 {
		return 92
	}
	if x < 16 || x >= width-16 || y < mailHeader {
		return 0
	}
	row, inside := (y-mailHeader)/mailRow, (y-mailHeader)%mailRow
	if row >= rows || inside >= mailRow-12 {
		return 0
	}
	if x >= width-88 && x < width-29 && inside >= 20 && inside < 78 {
		return mailDismissBase + row
	}
	return mailOpenBase + row
}

func inboxTail(bar, screen mailRect, edge int) mailRect {
	r := mailRect{w: mailSize, h: mailSize}
	if edge%2 == 0 {
		r.x = bar.x + bar.w + mailGap
		r.y = bar.y + (bar.h-mailSize)/2
		if r.x+mailSize > screen.x+screen.w {
			r.x = bar.x - mailGap - mailSize
		}
	} else {
		r.x = bar.x + (barItem-mailSize)/2
		r.y = bar.y + bar.h + mailGap
		if r.y+mailSize > screen.y+screen.h {
			r.y = bar.y - mailGap - mailSize
		}
	}
	r.x = max(screen.x, min(r.x, screen.x+screen.w-r.w))
	r.y = max(screen.y, min(r.y, screen.y+screen.h-r.h))
	return r
}
func inboxPanel(tail, bar, screen mailRect, edge, count int) mailRect {
	r := mailRect{w: min(mailWidth, screen.w), h: mailHeader + min(count, mailVisible)*mailRow + 12}
	if edge%2 == 0 {
		r.x = tail.x + (tail.w-r.w)/2
		r.y = bar.y + bar.h + mailGap
		if edge == 2 {
			r.y = bar.y - mailGap - r.h
		}
	} else {
		r.y = tail.y + (tail.h-r.h)/2
		r.x = bar.x + bar.w + mailGap
		if edge == 1 {
			r.x = bar.x - mailGap - r.w
		}
	}
	r.x = max(screen.x, min(r.x, screen.x+screen.w-r.w))
	r.y = max(screen.y, min(r.y, screen.y+screen.h-r.h))
	return r
}

const paperSize = 44

var paperOutline = [...]mailPoint{{22.5, 37.8}, {0, 0}, {100, 50}, {23.8, 100}, {23.3, 77.1}, {4, 76.3}}

func paperPoints(angle float64) [6]mailPoint {
	var pts [6]mailPoint
	s, c := math.Sincos(angle)
	for i, p := range paperOutline {
		x, y := (p.x-50)*.31, (p.y-50)*.25
		pts[i] = mailPoint{paperSize/2 + c*x - s*y, paperSize/2 + s*x + c*y}
	}
	return pts
}
