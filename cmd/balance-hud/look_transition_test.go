package main

import (
	"bytes"
	"testing"
	"time"
)

func TestWorkingTransitionsFollowOriginalCurveWithoutRestartingClips(t *testing.T) {
	var motion lookTransition
	now := time.Now()
	prior := markFrames{workBase + 15, toolBase + 3, -9, cueBase + 7, workBase + 16}
	first, moving := motion.visual(now, prior, false)
	if moving || first != plainVisuals(prior) {
		t.Fatalf("working entrance started an extra transition: %+v", first)
	}
	next := markFrames{toolBase, workBase + 20, -6, cueBase + 8, toolBase + toolFrames}
	at := now.Add(time.Second)
	got, moving := motion.visual(at, next, false)
	for _, slot := range []int{0, 1, 4} {
		if got[slot] != (markVisual{frame: next[slot], previous: prior[slot], oldOpacity: 255}) || !moving {
			t.Fatalf("slot %d did not start from the exact previous frame: %+v", slot, got[slot])
		}
	}
	if got[2].oldOpacity != 0 || got[3].oldOpacity != 0 {
		t.Fatal("idle and reminder clips should keep their original entrances")
	}
	next[0] += 2
	got, moving = motion.visual(at.Add(lookHandoff/2), next, false)
	if got[0].frame != next[0] || got[0].previous != prior[0] || got[0].oldOpacity != 32 || !moving {
		t.Fatalf("expected advancing target and 12.5%% outgoing contribution: %+v", got[0])
	}
	got, moving = motion.visual(at.Add(lookHandoff), next, false)
	if moving || got != plainVisuals(next) {
		t.Fatalf("handoff outlived original duration: %+v", got)
	}
	got, moving = motion.visual(at.Add(time.Second), prior, true)
	if moving || got != plainVisuals(prior) {
		t.Fatal("a handoff still ran with reduced motion")
	}
}

func TestWorkingTransitionCompositesOneMarkWithoutChangingQuota(t *testing.T) {
	var painter barPainter
	rows := view(nil)
	for i := range rows {
		rows[i].Percent = 72
	}
	p := placement{Edge: 3, Numbers: true}
	old, next := markFrames{workBase + 15}, markFrames{toolBase + 9}
	before := bytes.Clone(painter.paint(rows, p, old).Pix)
	target := bytes.Clone(painter.paint(rows, p, next).Pix)
	vis := plainVisuals(next)
	vis[0].previous = old[0]
	vis[0].oldOpacity = 255
	if got := painter.paintVisuals(rows, p, vis); !bytes.Equal(got.Pix, before) {
		t.Fatal("the first painted transition frame differed from the preceding pose")
	}
	vis[0].oldOpacity = 128
	got := painter.paintVisuals(rows, p, vis)
	if bytes.Equal(got.Pix, before) || bytes.Equal(got.Pix, target) {
		t.Fatal("no intermediate pose painted")
	}
	patch := painter.damage[0]
	for i, damage := range painter.damage {
		if i != 0 && !damage.Empty() {
			t.Fatalf("transition damaged neighbor %d", i)
		}
	}
	for y := got.Bounds().Min.Y; y < got.Bounds().Max.Y; y++ {
		for x := got.Bounds().Min.X; x < got.Bounds().Max.X; x++ {
			if x >= patch.Min.X && x < patch.Max.X && y >= patch.Min.Y && y < patch.Max.Y {
				continue
			}
			idx := got.PixOffset(x, y)
			if !bytes.Equal(got.Pix[idx:idx+4], before[idx:idx+4]) {
				t.Fatalf("transition changed non-actor pixel %d,%d", x, y)
			}
		}
	}
}

func TestAllMarksEaseToLogoWhenActivityEnds(t *testing.T) {
	var motion lookTransition
	now := time.Now()
	poses := markFrames{toolBase + 3, workBase + 2, pendingBase + 5, compactBase + 1, cueBase + 2*cueFrames + 12}
	motion.visual(now, poses, false)
	at := now.Add(time.Second)
	for _, test := range []struct {
		offset time.Duration
		alpha  uint8
	}{{0, 255}, {250 * time.Millisecond, 128}, {500 * time.Millisecond, 0}} {
		visuals, active := motion.visual(at.Add(test.offset), markFrames{}, false)
		for i, pose := range poses {
			if visuals[i].oldOpacity != test.alpha || (test.alpha > 0 && visuals[i].previous != pose) {
				t.Errorf("slot %d at %v: %+v", i, test.offset, visuals[i])
			}
		}
		if active != (test.alpha > 0) {
			t.Fatalf("animation kept running after the logo settled: %v", active)
		}
	}
	// A new request takes over immediately rather than fading stale information above it.
	motion.visual(now.Add(2*time.Second), poses, false)
	motion.visual(now.Add(3*time.Second), markFrames{}, false)
	next := markFrames{pendingBase + 1, workBase, 0, 0, 0}
	got, _ := motion.visual(now.Add(3100*time.Millisecond), next, false)
	if got[0] != (markVisual{frame: next[0]}) || got[1] != (markVisual{frame: next[1]}) {
		t.Fatal("rest fade covered a new request or new work:", got)
	}
}
