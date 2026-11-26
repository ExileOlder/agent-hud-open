package main

import (
	"math"
	"time"
)

// The light dock uses Agent HUD's notification duration and easing
// (EndFlights.tsx), with the paper plane's dock arc (flight-route.ts).
// Only one live trajectory is retained and sampled by distance.
const routeSamples = 48

type routeSample struct {
	point, tangent mailPoint
	distance       float64
}

type flightGeometry struct {
	bar, tail, screen  mailRect
	edge, source, step int
}
type flightPath struct {
	geometry flightGeometry
	samples  [routeSamples + 1]routeSample
}

func (m *mailInbox) flightAt(now time.Time, geometry flightGeometry) (mailPoint, float64) {
	if m.path.geometry != geometry {
		m.path.plan(geometry)
	}
	return m.path.at(endFlightEase(float64(now.Sub(m.started)) / float64(mailFlight)))
}

func endFlightEase(t float64) float64 {
	t = clamp01(t)
	if t == 0 || t == 1 {
		return t
	}
	// Solve cubic-bezier(0.45, 0, 0.2, 1).
	lo, hi := 0.0, 1.0
	for range 16 {
		m := (lo + hi) * .5
		a := 1 - m
		x := 3*a*a*m*.45 + 3*a*m*m*.2 + m*m*m
		if x < t {
			lo = m
		} else {
			hi = m
		}
	}
	m := (lo + hi) * .5
	return m * m * (3 - 2*m)
}

func (f *flightPath) plan(g flightGeometry) {
	f.geometry = g
	bar, tail, screen := g.bar, g.tail, g.screen
	start := mailPoint{float64(bar.x + barPadding + g.source*g.step + barItem/2), float64(bar.y + bar.h/2)}
	if g.edge%2 == 1 {
		start = mailPoint{float64(bar.x + barItem/2), float64(bar.y + barPadding + g.source*barItem + barItem/2)}
	}
	end := mailPoint{float64(tail.x + tail.w/2), float64(tail.y + tail.h/2)}
	normal := [...]mailPoint{{0, 1}, {-1, 0}, {0, -1}, {1, 0}}[g.edge]
	start, end = paperCentre(start, screen), paperCentre(end, screen)
	for reach := 140.0; ; reach *= .85 {
		if f.ellipse(start, end, normal, reach, screen) || reach < 48 {
			break
		}
	}
}

// Two widely spaced marks get half an ellipse. Close marks get a rounder
// loop past the mark, just as the original dock route does.
func (f *flightPath) ellipse(start, end, n mailPoint, reach float64, screen mailRect) bool {
	dx, dy := end.x-start.x, end.y-start.y
	d := math.Hypot(dx, dy)
	u := mailPoint{-n.y, n.x}
	if d > 1e-6 {
		u = mailPoint{dx / d, dy / d}
	}
	center := mailPoint{(start.x + end.x) / 2, (start.y + end.y) / 2}
	a, b, side := min(120, .3*d), d/2, math.Pi/2
	if d < 2*reach {
		b = max(d/2+30, .4*reach)
		k := math.Sqrt(1 - (d/(2*b))*(d/(2*b)))
		a = reach / (1 + k)
		center.x += n.x * a * k
		center.y += n.y * a * k
		side = math.Atan2(d/(2*b), -k)
	}
	fits := true
	total := 0.0
	for i := range f.samples {
		angle := -side + 2*side*float64(i)/routeSamples
		s, c := math.Sincos(angle)
		p := mailPoint{center.x + n.x*a*c + u.x*b*s, center.y + n.y*a*c + u.y*b*s}
		switch i {
		case 0:
			p = start
		case routeSamples:
			p = end
		}
		bounded := paperCentre(p, screen)
		fits = fits && p == bounded
		p = bounded
		v := mailPoint{-n.x*a*s + u.x*b*c, -n.y*a*s + u.y*b*c}
		if i > 0 {
			prev := f.samples[i-1].point
			total += math.Hypot(p.x-prev.x, p.y-prev.y)
		}
		f.samples[i] = routeSample{p, v, total}
	}
	return fits
}

func paperCentre(at mailPoint, screen mailRect) mailPoint {
	const margin = paperSize / 2
	left, right := screen.x+margin, screen.x+screen.w-margin
	top, bottom := screen.y+margin, screen.y+screen.h-margin
	at.x = max(float64(left), min(at.x, float64(right)))
	at.y = max(float64(top), min(at.y, float64(bottom)))
	return at
}

func (f *flightPath) at(progress float64) (mailPoint, float64) {
	want := progress * f.samples[routeSamples].distance
	for i := 1; i <= routeSamples; i++ {
		b := f.samples[i]
		if want > b.distance {
			continue
		}
		a := f.samples[i-1]
		t := 0.0
		if b.distance > a.distance {
			t = (want - a.distance) / (b.distance - a.distance)
		}
		mix := func(a, b float64) float64 { return a + (b-a)*t }
		return mailPoint{mix(a.point.x, b.point.x), mix(a.point.y, b.point.y)},
			math.Atan2(mix(a.tangent.y, b.tangent.y), mix(a.tangent.x, b.tangent.x))
	}
	b := f.samples[routeSamples]
	return b.point, math.Atan2(b.tangent.y, b.tangent.x)
}
