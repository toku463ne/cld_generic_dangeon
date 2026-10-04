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
			if !w.m.InBounds(x, y) || !w.seesFrom(b, bx, by, x, y) {
				continue
			}
			if !w.m.ShelterAt(x, y) {
				w.forgetPlace(b, w.m.index(x, y))
				continue
			}
			w.rememberShelter(b, w.m.index(x, y))
		}
	}
}

// rememberShelter has body b remember tile t as a place to rest; a tile
// new to it is something it can pass on (PassShelters).
func (w *World) rememberShelter(b *Body, t int) {
	if b.Memory.Shelters[t] {
		return
	}
	if b.Memory.Shelters == nil {
		b.Memory.Shelters = map[int]bool{}
	}
	b.Memory.Shelters[t] = true
	if w.cfg.PassShelters {
		b.Memory.Ver++
	}
}

// feelWarm has body b, after a tick, learn the burn it felt on a place to
// rest in the dark, over the burn it knows of its own body.
func (w *World) feelWarm(b *Body) {
	if !w.cfg.Learn || !w.cfg.ShelterRead || w.Brightness() >= 0.5 {
		return
	}
	x, y := int(math.Floor(b.X)), int(math.Floor(b.Y))
	switch {
	case w.m.ShelterAt(x, y):
		w.rememberShelter(b, w.m.index(x, y)) // it stands on it
	case !b.Tent.Pitched: // a tent is as warm (stage 8-2a)
		return
	}
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
	f, ok = w.nearestOf(b, b.Memory.Shelters)
	return f, burn, ok
}

// ownAhead returns the nearest place body b built and remembers, off the
// tile it stands on (OwnPlaces, stage 8-1b); it is read only where
// shelterAhead found a place to walk to.
func (w *World) ownAhead(b *Body) (Food, bool) {
	if !w.cfg.OwnPlaces || len(b.Memory.Own) == 0 {
		return Food{}, false
	}
	return w.nearestOf(b, b.Memory.Own)
}

// nearestOf returns the tile of tiles nearest body b by its walk, off the
// tile it stands on.
func (w *World) nearestOf(b *Body, tiles map[int]bool) (Food, bool) {
	here, speed := w.tileOf(b.X, b.Y), w.speedOf(b)
	best, bt := math.Inf(1), -1
	for t := range tiles {
		if t == here {
			continue
		}
		x, y := t%w.m.Width, t/w.m.Width
		d := float64(walkTicks(gapTo(b.X, x), gapTo(b.Y, y), speed))
		if d < best || d == best && t < bt {
			best, bt = d, t
		}
	}
	if bt < 0 {
		return Food{}, false
	}
	return Food{X: bt % w.m.Width, Y: bt / w.m.Width}, true
}

// ShelterAt reports whether tile (x, y) is a place to rest.
func (w *World) ShelterAt(x, y int) bool { return w.m.ShelterAt(x, y) }

// shelterRisk is the risk in a window of win grains of body b, left at
// (x, y) with after grains by an option, going on by walking to place and
// sleeping there (valueInto): the walk at its burn now, SleepTicks asleep
// at warm, then keeping on the move through place's region at woken. It
// returns the ticks of the walk too. speed and burn are b's (speedOf,
// readBurn), read once by the caller; credit is the place's worth beyond
// the window, in grains, added to what the body can last (a place it
// built, OwnPlaces).
func (w *World) shelterRisk(b *Body, rs *rates, place Food, x, y float64, after, win, full int, warm, woken, speed, burn float64, credit int) (float64, int) {
	after += credit
	k := walkTicks(gapTo(x, place.X), gapTo(y, place.Y), speed)
	ku, ts := w.grains(k, burn), w.grains(w.cfg.SleepTicks, warm)
	switch {
	case after-ku-ts <= 0:
		return 1, k
	case ku+ts >= win:
		return 0, k
	}
	reg := w.m.RegionAt(place.X, place.Y)
	s := w.learnedTable(rs.region[reg]*rs.slept*w.factor(speed, woken), full)
	return s.deadAtLeast(win-ku-ts, after-ku-ts), k
}
