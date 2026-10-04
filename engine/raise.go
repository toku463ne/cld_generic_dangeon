package engine

import "math"

// Building places to rest (stage 8-1, the user's design).
//
// A body may be born with the skill to build (BuildBorn). An adult with it,
// awake, off any place to rest, and sleepy enough that sleep is among its
// options, has one more: build here, then sleep. It pays BuildEnergy, the
// land in the BuildSide square round it that is not a place to rest yet
// becomes one (warm at night, as any place to rest is), and it falls asleep
// there. The place wears out BuildLife ticks later and is gone.
//
// The valuation reads building as the sleep it ends in, paid for first:
// asleep at the burn the body learned on a place to rest (Memory.Warm), or
// at its burn now if it has learned none (nothing yet known in its favour).
// A place lasts beyond the window, so what it is worth beyond this night
// comes in as reserve, as a resting mother's provision does: what one
// night's sleep there saves (the burn now less the warm burn, over
// SleepTicks) times the nights the body expects to come back, learned from
// its own places (Memory.Reuse: how often it slept again on a place it
// built before the place wore out). Nothing else draws it to build: with
// nothing learned, building ties sleeping where both read as no risk, and
// the draw among ties sometimes takes it.

// SkillBuild is the skill of building places to rest.
const (
	SkillBuild = iota
	NumSkills
)

// Place is a place to rest a body built.
type Place struct {
	ID      int64
	Builder int64
	// Tiles are the tiles it made places to rest; Until the tick it wears
	// out.
	Tiles []int
	Until int64
	// Returns counts the sleeps its builder began on it after the first.
	Returns int
}

// drawSkills gives body b the skills it is born with.
func (w *World) drawSkills(b *Body) {
	if w.cfg.Building && w.rng.Float64() < w.cfg.BuildBorn {
		b.Skill[SkillBuild] = 1
	}
}

// canBuild reports whether body b may build here now.
func (w *World) canBuild(b *Body) bool {
	if !w.cfg.Building || b.Skill[SkillBuild] <= 0 || b.Asleep || b.Mature > w.tick || b.Energy <= w.cfg.BuildEnergy {
		return false
	}
	if !w.cfg.Sleep || b.Pressure <= w.cfg.WakeAt {
		return false
	}
	t := w.tileOf(b.X, b.Y)
	return t >= 0 && !w.m.ShelterAt(t%w.m.Width, t/w.m.Width)
}

// build has body b build a place to rest round it and fall asleep there.
func (w *World) build(b *Body) {
	b.Energy -= w.cfg.BuildEnergy
	if w.placeAt == nil {
		w.placeAt = make([]int64, len(w.m.Terrain))
	}
	w.nextPlace++
	p := Place{ID: w.nextPlace, Builder: b.ID, Until: w.tick + int64(w.cfg.BuildLife)}
	bx, by := int(math.Floor(b.X)), int(math.Floor(b.Y))
	lo := w.cfg.BuildSide / 2
	for y := by - lo; y < by-lo+w.cfg.BuildSide; y++ {
		for x := bx - lo; x < bx-lo+w.cfg.BuildSide; x++ {
			if !w.m.InBounds(x, y) || w.m.TerrainAt(x, y) != TerrainLand || w.m.ShelterAt(x, y) {
				continue
			}
			w.m.SetShelter(x, y)
			t := w.m.index(x, y)
			if w.cfg.OwnPlaces {
				if b.Memory.Own == nil {
					b.Memory.Own = map[int]bool{}
				}
				b.Memory.Own[t] = true
				if w.cfg.Learn && w.cfg.ShelterRead {
					w.rememberShelter(b, t) // it knows what it built
				}
			}
			w.placeAt[t] = p.ID
			p.Tiles = append(p.Tiles, t)
		}
	}
	w.places = append(w.places, p)
	w.stats.Built++
	w.fallAsleep(b)
}

// noteReturn counts body b's sleep, begun on tile t, as a return to a
// place it built.
func (w *World) noteReturn(b *Body, t int) {
	if t < 0 || w.placeAt == nil || w.placeAt[t] == 0 {
		return
	}
	w.stats.BuiltSleeps++
	for i := range w.places {
		if p := &w.places[i]; p.ID == w.placeAt[t] && p.Builder == b.ID {
			p.Returns++
			w.stats.Returns++
		}
	}
}

// wearPlaces removes the places worn out by now; a builder still alive
// learns how often it came back to it.
func (w *World) wearPlaces() {
	kept := w.places[:0]
	for _, p := range w.places {
		if p.Until > w.tick {
			kept = append(kept, p)
			continue
		}
		for _, t := range p.Tiles {
			if w.placeAt[t] == p.ID {
				w.placeAt[t] = 0
				w.m.Shelter[t] = false
			}
		}
		if !w.cfg.Learn {
			continue
		}
		for i := range w.bodies {
			if b := &w.bodies[i]; b.ID == p.Builder {
				b.Memory.Reuse[0] += float64(p.Returns)
				b.Memory.Reuse[1]++
			}
		}
	}
	w.places = kept
}

// buildRisk is the risk in a window of win grains of body b building here
// now and sleeping on it (valueInto): the sleep at the warm burn it learned
// (its burn now if none), BuildEnergy paid first, and the nights it expects
// to come back, each saving what this one does, as reserve.
func (w *World) buildRisk(b *Body, rs *rates, win, reserve int) float64 {
	now := w.planBurn(b)
	warm := now
	if rs != nil && b.Memory.Warm[1] > 0 {
		warm = math.Min(now, w.ownBurn(b, true)*b.Memory.Warm[0]/b.Memory.Warm[1])
	}
	if rs != nil {
		reserve += w.reuseCredit(b, now, warm)
	}
	return w.sleepRiskAt(b, rs, win, reserve, warm, b.Energy-w.cfg.BuildEnergy)
}

// reuseCredit is the worth beyond the window, in grains, of a place body
// b built: the returns it expects (what it learned, pulled toward
// ReusePrior by ReuseWeight) times one night's saving there, sleeping at
// warm instead of now.
func (w *World) reuseCredit(b *Body, now, warm float64) int {
	n := b.Memory.Reuse[1] + w.cfg.ReuseWeight
	if n <= 0 || now <= warm {
		return 0
	}
	back := (b.Memory.Reuse[0] + w.cfg.ReusePrior*w.cfg.ReuseWeight) / n
	return w.units(back * (now - warm) * float64(w.cfg.SleepTicks))
}

// forgetPlace has body b, seeing tile t is no place to rest, forget it was
// one (a built place worn out).
func (w *World) forgetPlace(b *Body, t int) {
	if b.Memory.Shelters[t] {
		delete(b.Memory.Shelters, t)
	}
	delete(b.Memory.Own, t)
}

// loadPlaces rebuilds the index of built places' tiles after a load.
func (w *World) loadPlaces() {
	if len(w.places) == 0 {
		return
	}
	w.placeAt = make([]int64, len(w.m.Terrain))
	for _, p := range w.places {
		for _, t := range p.Tiles {
			w.placeAt[t] = p.ID
		}
	}
}

// Places returns the built places standing now.
func (w *World) Places() []Place { return append([]Place(nil), w.places...) }
