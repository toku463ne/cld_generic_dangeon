package engine

import "math"

// Knowing places to rest (stage 6-1b).
//
// A place to rest (Map.Shelter) is seen like food: every one of its tiles a
// body sees it remembers (Memory.Shelters). It does not perceive the
// temperature; on a place to rest in the dark it feels its burn, and learns
// how much more than the burn it knows of its own body - build, rest, debt
// - that is (Memory.Warm). With that learned, the valuation can read going
// on by walking to the nearest place remembered and sleeping there, at the
// burn learned, before keeping on the move; unlearned, a place reads like
// anywhere else and is never worth the walk.

// noteShelters has body b remember the places to rest it sees now.
func (w *World) noteShelters(b *Body) {
	if !w.cfg.Learn || !w.cfg.ShelterRead || len(w.m.Shelter) == 0 {
		return
	}
	bx, by := int(math.Floor(b.X)), int(math.Floor(b.Y))
	s := max(w.Sight(), 0)
	for y := by - s; y <= by+s; y++ {
		for x := bx - s; x <= bx+s; x++ {
			if !w.m.InBounds(x, y) || !w.m.ShelterAt(x, y) || !w.seesFrom(b, bx, by, x, y) {
				continue
			}
			if b.Memory.Shelters == nil {
				b.Memory.Shelters = map[int]bool{}
			}
			b.Memory.Shelters[w.m.index(x, y)] = true
		}
	}
}

// feelWarm has body b, after a tick, learn the burn it felt on a place to
// rest in the dark, over the burn it knows of its own body.
func (w *World) feelWarm(b *Body) {
	if !w.cfg.Learn || !w.cfg.ShelterRead || w.Brightness() >= 0.5 {
		return
	}
	x, y := int(math.Floor(b.X)), int(math.Floor(b.Y))
	if !w.m.ShelterAt(x, y) {
		return
	}
	if b.Memory.Shelters == nil {
		b.Memory.Shelters = map[int]bool{}
	}
	b.Memory.Shelters[w.m.index(x, y)] = true // it stands on it
	b.Memory.Warm[0] += w.planBurn(b) / w.ownBurn(b, true)
	b.Memory.Warm[1]++
}

// shelterAhead returns the nearest place to rest body b remembers, off the
// tile it stands on, and the burn it would sleep there at; ok is false
// where it may not sleep now, knows no place, has not learned one's warmth,
// or would burn no less there than it does now.
func (w *World) shelterAhead(b *Body) (f Food, burn float64, ok bool) {
	if !w.cfg.Learn || !w.cfg.ShelterRead || !w.cfg.Sleep || b.Pressure <= w.cfg.WakeAt || b.Memory.Warm[1] == 0 {
		return Food{}, 0, false
	}
	burn = w.ownBurn(b, true) * b.Memory.Warm[0] / b.Memory.Warm[1]
	if burn >= w.planBurn(b)*(1-1e-9) { // rounding is not warmth
		return Food{}, 0, false
	}
	here := w.tileOf(b.X, b.Y)
	best, bt := math.Inf(1), -1
	for t := range b.Memory.Shelters {
		if t == here {
			continue
		}
		x, y := t%w.m.Width, t/w.m.Width
		d := float64(walkTicks(gapTo(b.X, x), gapTo(b.Y, y), w.speedOf(b)))
		if d < best || d == best && t < bt {
			best, bt = d, t
		}
	}
	if bt < 0 {
		return Food{}, 0, false
	}
	return Food{X: bt % w.m.Width, Y: bt / w.m.Width}, burn, true
}

// ShelterAt reports whether tile (x, y) is a place to rest.
func (w *World) ShelterAt(x, y int) bool { return w.m.ShelterAt(x, y) }

// shelterRisk is the risk in a window of win grains of body b, left at
// (x, y) with after grains by an option, going on by walking to place and
// sleeping there (valueInto): the walk at its burn now, SleepTicks asleep
// at warm, then keeping on the move through place's region at woken. It
// returns the ticks of the walk too.
func (w *World) shelterRisk(b *Body, rs *rates, place Food, x, y float64, after, win, full int, warm, woken float64) (float64, int) {
	speed := w.speedOf(b)
	k := walkTicks(gapTo(x, place.X), gapTo(y, place.Y), speed)
	ku, ts := w.grains(k, w.readBurn(b)), w.grains(w.cfg.SleepTicks, warm)
	switch {
	case after-ku-ts <= 0:
		return 1, k
	case ku+ts >= win:
		return 0, k
	}
	reg := w.m.RegionAt(place.X, place.Y)
	s := w.learnedTable(rs.region[reg]*w.factor(speed, woken), full)
	return s.deadAtLeast(win-ku-ts, after-ku-ts), k
}
