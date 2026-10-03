package engine

import "math"

// When dark and light come (stage 7-2b, VISION.md "昼と夜の明るさを認知し、
// 暗く/明るくなるまでの大体の時間を把握する").
//
// A body knows no clock. It perceives the light as it is, whether it rose
// or fell since the tick before, and how long it has been in its present
// step - the light read in LightStages steps, that time in bands of
// LightBand ticks. That is the key of a row. From its own days it learns,
// for each key, the mean ticks until the light next falls below half
// (dark comes) and next rises to half (light comes). A body asleep sees no
// light: what it was waiting to learn is lost, and it starts again when it
// wakes, not knowing how long the light has been as it is.

// DuskRow is one key's row: the ticks until dark and until light learned
// (sum and count of the samples), and the ticks seen in the key that are
// still waiting for dark or light to come (count and sum of their ticks).
type DuskRow struct {
	Key               int32
	ToDark, ToLight   [2]float64
	WaitDark, WaitLit [2]float64
}

// lightKey is the key of a body that perceives light l, moving dir (-1,
// 0, +1) and in its step since tick since (-1: not known).
func (w *World) lightKey(l float64, dir int, since int64) int32 {
	stage := int32(math.Round(l * float64(w.cfg.LightStages)))
	band := int32(w.cfg.LightBands) // not known
	if since >= 0 {
		band = int32(min((w.tick-since)/int64(w.cfg.LightBand), int64(w.cfg.LightBands-1)))
	}
	return (stage*3+int32(dir+1))*int32(w.cfg.LightBands+1) + band
}

// lightStage is the step light l reads as.
func (w *World) lightStage(l float64) int {
	return int(math.Round(l * float64(w.cfg.LightStages)))
}

// duskRow returns body b's row for key k, adding it if it is new.
func duskRow(b *Body, k int32) *DuskRow {
	for i := range b.Dusk {
		if b.Dusk[i].Key == k {
			return &b.Dusk[i]
		}
	}
	b.Dusk = append(b.Dusk, DuskRow{Key: k})
	return &b.Dusk[len(b.Dusk)-1]
}

// seeLight has body b perceive the light after a tick (DuskRead): dark or
// light coming settles what its rows were waiting for, and the tick joins
// the row of its key. Asleep, it sees nothing and loses what was waiting.
func (w *World) seeLight(b *Body) {
	if !w.cfg.DuskRead {
		return
	}
	if b.Asleep {
		if b.LightKnown {
			b.LightKnown = false
			for i := range b.Dusk {
				b.Dusk[i].WaitDark, b.Dusk[i].WaitLit = [2]float64{}, [2]float64{}
			}
		}
		return
	}
	l := w.Brightness()
	if !b.LightKnown {
		b.LightKnown, b.LightSeen, b.LightSince = true, l, -1
		return
	}
	prev := b.LightSeen
	if w.lightStage(l) != w.lightStage(prev) {
		b.LightSince = w.tick
	}
	t := float64(w.tick)
	settle := func(wait *[2]float64, to *[2]float64) {
		if wait[0] > 0 {
			to[0] += wait[0]*t - wait[1]
			to[1] += wait[0]
			*wait = [2]float64{}
		}
	}
	if prev >= 0.5 && l < 0.5 {
		for i := range b.Dusk {
			settle(&b.Dusk[i].WaitDark, &b.Dusk[i].ToDark)
		}
	}
	if prev < 0.5 && l >= 0.5 {
		for i := range b.Dusk {
			settle(&b.Dusk[i].WaitLit, &b.Dusk[i].ToLight)
		}
	}
	b.LightKey = w.lightKey(l, lightDir(prev, l), b.LightSince)
	r := duskRow(b, b.LightKey)
	r.WaitDark[0]++
	r.WaitDark[1] += t
	r.WaitLit[0]++
	r.WaitLit[1] += t
	b.LightSeen = l
}

// lightDir is whether the light rose (+1), fell (-1) or stayed (0).
func lightDir(prev, l float64) int {
	switch {
	case l > prev:
		return 1
	case l < prev:
		return -1
	}
	return 0
}

// duskAhead returns the ticks body b expects until dark and until light
// come, from the row of what it perceived last tick, and ok false where it
// has not learned both (or did not perceive the light).
func (w *World) duskAhead(b *Body) (toDark, toLight float64, ok bool) {
	if !w.cfg.DuskRead || !b.LightKnown {
		return 0, 0, false
	}
	for i := range b.Dusk {
		if r := &b.Dusk[i]; r.Key == b.LightKey {
			if r.ToDark[1] == 0 || r.ToLight[1] == 0 {
				return 0, 0, false
			}
			return r.ToDark[0] / r.ToDark[1], r.ToLight[0] / r.ToLight[1], true
		}
	}
	return 0, 0, false
}

// duskSched is how the valuation reads the light over the window (stage
// 7-2b): lit (full light's sight) or dark from the start, switching at
// grain s1 and back at s2. Off, the light now is read as staying.
type duskSched struct {
	on     bool
	lit    bool
	s1, s2 int
	// dark converts a chance per grain read at the light now to the dark's.
	dark float64
}

// duskOf is body b's schedule for a window read at the given burn.
func (w *World) duskOf(b *Body, burn float64) duskSched {
	if !w.cfg.SightRead {
		return duskSched{}
	}
	toDark, toLight, ok := w.duskAhead(b)
	if !ok {
		return duskSched{}
	}
	lit := b.LightSeen >= 0.5
	first, second := toDark, toLight
	if !lit {
		first, second = toLight, toDark
	}
	win := w.windowGrains()
	step := func(ticks float64) int {
		g := w.grains(int(math.Round(ticks)), burn)
		g = int(math.Round(float64(g)/float64(w.cfg.DuskStep))) * w.cfg.DuskStep
		return min(max(g, 0), win)
	}
	s1, s2 := step(first), step(second)
	s2 = max(s2, s1)
	if s1 == 0 {
		// The first switch is now: the window starts in the other light.
		lit, s1, s2 = !lit, s2, win
	}
	if s1 == 0 {
		s1 = win
	}
	return duskSched{on: true, lit: lit, s1: s1, s2: s2, dark: w.sightGainAt(0) / w.sightGain()}
}

// sightGainAt is sightGain at light l.
func (w *World) sightGainAt(l float64) float64 {
	if !w.cfg.SightRead || w.cfg.Sight < 0 {
		return 1
	}
	return float64(2*w.cfg.NightSight+1) + 2*float64(w.cfg.Sight-w.cfg.NightSight)*l
}

// DuskAhead is duskAhead for reading: the ticks body b expects until dark
// and until light, and ok false where it has not learned them.
func (w *World) DuskAhead(b Body) (toDark, toLight float64, ok bool) { return w.duskAhead(&b) }
