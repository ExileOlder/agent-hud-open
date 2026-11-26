package main

import (
	"math"
	"testing"
	"time"
)

func TestPaperUsesDockEllipseAndLandsAtTheBadge(t *testing.T) {
	screen := mailRect{x: -1920, y: -40, w: 1920, h: 1080}
	for _, edge := range []int{0, 1, 2, 3} {
		bar := mailRect{x: -1350, y: -37, w: 2*barPadding + 4*barItem, h: barItem + 2*barPadding}
		switch edge {
		case 1:
			bar = mailRect{x: -barItem - 3, y: 260, w: barItem, h: 2*barPadding + 4*barItem}
		case 2:
			bar.y = screen.y + screen.h - bar.h - 3
		case 3:
			bar = mailRect{x: screen.x + 3, y: 260, w: barItem, h: 2*barPadding + 4*barItem}
		}
		tail := inboxTail(bar, screen, edge)
		normal := [...]mailPoint{{0, 1}, {-1, 0}, {0, -1}, {1, 0}}[edge]
		for _, slot := range []int{0, 3} {
			var inbox mailInbox
			inbox.started = time.Now()
			g := flightGeometry{bar, tail, screen, edge, slot, barItem}
			from, angle := inbox.flightAt(inbox.started, g)
			got, _ := inbox.flightAt(inbox.started.Add(mailFlight), g)
			want := mailPoint{float64(tail.x + mailSize/2), float64(tail.y + mailSize/2)}
			if got != want || from == got {
				t.Fatalf("edge %d mark %d: route did not pin its endpoints: %v → %v, want %v", edge, slot, from, got, want)
			}
			bow := 0.0
			for tick := range 21 {
				last, _ := inbox.flightAt(inbox.started.Add(time.Duration(tick)*mailFlight/20), g)
				if bounded := paperCentre(last, screen); bounded != last {
					t.Fatalf("edge %d mark %d: plane clipped off its screen: %v", edge, slot, last)
				}
				bow = max(bow, (last.x-from.x)*normal.x+(last.y-from.y)*normal.y)
			}
			if bow < 90 {
				t.Fatalf("edge %d mark %d: route did not arc into the display", edge, slot)
			}
			if slot == 3 && math.Cos(angle)*(want.x-from.x)+math.Sin(angle)*(want.y-from.y) >= 0 {
				t.Fatalf("edge %d: the close mark should begin its loop away from the badge", edge)
			}
		}
	}
}

func TestNotificationFlightUsesOriginalEasing(t *testing.T) {
	if endFlightEase(0) != 0 || endFlightEase(1) != 1 {
		t.Fatal("curve missed an endpoint")
	}
	if progress := endFlightEase(.5); math.Abs(progress-.76226) > .001 {
		t.Fatalf("wrong halfway progress for the original notification timing: %f", progress)
	}
}
