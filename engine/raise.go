package engine

import "math"

// Settlements (stage 8-2b, the user's design; VISION "定住場所").
//
// Places to rest are of two kinds: the ones the world lays out, shared by
// all (Map.Shelter as the map gives it), and the ones a body builds by
// skill, its own and its family's. A body may be born with the skill to
// build (BuildBorn). An adult with it and no settlement standing may begin
// one as it falls asleep - how it sleeps, never whether (8-1b): it compares
// sleeping as it is with building here for BuildTicks and then sleeping in
// it - paying BuildEnergy and the burn of the work first, sleeping at the
// warmth it learned (Memory.Warm), and as reserve the nights it expects to
// come back (Memory.Reuse: each sleep says whether it began within a
// place's reach of the last, ReuseNights; pulled toward ReusePrior by
// ReuseWeight) times a night's saving (nightSaving) - and on a tie the
// draw takes either. With SeekBed the comparison is against the best way
// to sleep tonight without building: as it is, or walking to the nearest
// place to rest it remembers. A sleep that may not wait (ForceMax) is not
// put off for it.
//
// Building is a skill's own routine, not decided tick by tick: the body
// works on its site; hungry (a meal fits) with food in sight it goes for
// the nearest unit, eats it and comes back to the site; when sleep may not
// wait it sleeps where it is and works on when it wakes; hungry with no
// food in sight and too little energy for the work left and a night, it
// gives the site up. Done, the land in the BuildSide square round the site
// that is no place to rest yet becomes a settlement: warm at night and open
// to all, like the places the world lays out (lives are some seven days,
// and a builder all but never came back to a place it closed to others -
// the user took ownership out, 2026-10-05). It wears out BuildLife ticks
// after it was done, and its builder, if alive, learns how often it came
// back to sleep in it.

// SkillBuild is the skill of building settlements, SkillTent of making
// tents (tent.go).
const (
	SkillBuild = iota
	SkillTent
	NumSkills
)

// Place is a settlement a body built.
type Place struct {
	ID      int64
	Builder int64
	// Species is its builder's: with ClosedPlaces no body of another
	// species may step onto its tiles or count them as places to rest
	// (stage 9-7b-2b, VISION "他種族は定住場所には入れない").
	Species int `json:",omitempty"`
	// Tiles are its tiles; Until the tick it wears out.
	Tiles []int
	Until int64
	// Returns counts the sleeps its builder began in it after the first.
	Returns int
}

// Work is a settlement a body is building.
type Work struct {
	// Site is the tile it builds round plus one, zero for none; Left the
	// ticks of work left.
	Site int `json:",omitempty"`
	Left int `json:",omitempty"`
	// Food is the tile plus one of the unit it went for, zero when at or
	// on the way back to its site.
	Food int `json:",omitempty"`
}

// drawSkills gives body b the skills it is born with; none to a species
// without skills (NoSkills), and no draw for it.
func (w *World) drawSkills(b *Body) {
	if w.spec(b).NoSkills {
		return
	}
	if w.cfg.Building && w.rng.Float64() < w.cfg.BuildBorn {
		b.Skill[SkillBuild] = 1
	}
	if w.cfg.Tents && w.rng.Float64() < w.cfg.TentBorn {
		b.Skill[SkillTent] = 1
	}
}

// canSettle reports whether body b, falling asleep, could begin a
// settlement here.
func (w *World) canSettle(b *Body) bool {
	if !w.cfg.Building || b.Skill[SkillBuild] <= 0 || b.Mature > w.tick || b.Home != 0 || b.Work.Site != 0 {
		return false
	}
	if b.Pressure >= w.cfg.ForceMax || b.Energy <= w.cfg.BuildEnergy {
		return false
	}
	t := w.tileOf(b.X, b.Y)
	return t >= 0 && w.m.Terrain[t].Ground() && !w.m.ShelterAt(t%w.m.Width, t/w.m.Width)
}

// settleInstead is what body b, falling asleep (a is ActSleep), does: it
// begins a settlement where that reads as less risk than sleeping as it
// is (drawn on a tie), and goes on sleeping otherwise.
func (w *World) settleInstead(b *Body, a Action) Action {
	if a.Kind != ActSleep || !w.canSettle(b) || !w.cfg.Learn || w.cfg.Window <= 0 {
		return a
	}
	reserve := w.restReserve(b) + b.Held*w.pred.meal
	win := w.pred.windows[0]
	var rs rates
	w.ratesOf(b, &rs)
	now := w.planBurn(b)
	plain := w.sleepRiskAt(b, &rs, win, reserve, now, b.Energy)
	if w.cfg.SeekBed {
		plain = math.Min(plain, w.bedRisk(b, &rs, win, reserve))
	}
	work := w.cfg.BuildEnergy + float64(w.cfg.BuildTicks)*now
	built := w.sleepRiskAt(b, &rs, win, reserve+w.reuseCredit(b), w.tentWarm(b), b.Energy-work)
	if built < plain || built == plain && w.rng.Intn(2) == 0 {
		b.Energy -= w.cfg.BuildEnergy
		b.Work = Work{Site: w.tileOf(b.X, b.Y) + 1, Left: w.cfg.BuildTicks}
		w.stats.Begun++
		return Action{Kind: ActBuild}
	}
	return a
}

// bedRisk is the risk of walking to the nearest place to rest body b
// remembers and sleeping there (SeekBed, stage 9-3b): the best way to
// sleep tonight without building, besides sleeping as it stands. One
// where it knows no place it would sleep warmer at.
func (w *World) bedRisk(b *Body, rs *rates, win, reserve int) float64 {
	f, warm, ok := w.shelterAhead(b)
	if !ok {
		return 1
	}
	after := w.units(b.Energy) + reserve - 1
	r, _ := w.shelterRisk(b, rs, f, b.X, b.Y, after, win, w.units(w.maxOf(b)), warm, w.wokenBurn(b), w.speedOf(b), w.readBurn(b))
	return r
}

// reuseCredit is the worth beyond tonight, in grains, of a settlement body
// b would build: the nights it expects to come back (what it learned,
// pulled toward ReusePrior by ReuseWeight) times a night's saving.
func (w *World) reuseCredit(b *Body) int {
	n := b.Memory.Reuse[1] + w.cfg.ReuseWeight
	saving := w.nightSaving(b)
	if n <= 0 || saving <= 0 {
		return 0
	}
	back := (b.Memory.Reuse[0] + w.cfg.ReusePrior*w.cfg.ReuseWeight) / n
	return w.units(back * saving)
}

// work is what body b, building, does this tick (see the top of the file).
func (w *World) work(b *Body) Action {
	site := b.Work.Site - 1
	here := w.tileOf(b.X, b.Y)
	if w.sleepDue(b) && b.Pressure >= w.cfg.ForceMax {
		return Action{Kind: ActSleep} // and works on when it wakes
	}
	if b.Work.Food != 0 {
		f := b.Work.Food - 1
		switch {
		case w.foodFor(b, f) < 0:
			b.Work.Food = 0 // gone: back to the site
		case f == here:
			b.Work.Food = 0
			return Action{Kind: ActEat}
		default:
			return w.toward(b, f)
		}
	}
	if here != site {
		return w.toward(b, site)
	}
	if b.Energy <= w.maxOf(b)-w.cfg.FoodEnergy { // hungry: a meal fits
		if seen := w.inSight(nil, b); len(seen) > 0 {
			best, bt := math.Inf(1), -1
			for _, f := range seen {
				t := w.m.index(f.X, f.Y)
				if d := octile(gapTo(b.X, f.X), gapTo(b.Y, f.Y)); d < best || d == best && t < bt {
					best, bt = d, t
				}
			}
			b.Work.Food = bt + 1
			w.stats.Fetched++
			return w.toward(b, bt)
		}
		if b.Energy < float64(b.Work.Left+w.cfg.SleepTicks)*w.planBurn(b) {
			b.Work = Work{}
			w.stats.Abandoned++
			return Action{Kind: ActWait}
		}
	}
	return Action{Kind: ActBuild}
}

// toward is a step of body b towards tile t, or a wait where that step is
// closed to it.
func (w *World) toward(b *Body, t int) Action {
	tx, ty := float64(t%w.m.Width)+0.5, float64(t/w.m.Width)+0.5
	a := math.Atan2(ty-b.Y, tx-b.X)
	d := (int(math.Round(a/(math.Pi/4))) + 8) % 8
	x, y := b.X+moveDirs[d][0]*w.speedOf(b), b.Y+moveDirs[d][1]*w.speedOf(b)
	if to := w.tileOf(x, y); to < 0 || !w.mayEnter(b, to) || w.blocks(b, x, y) {
		return Action{Kind: ActWait}
	}
	return Action{Kind: ActMove, Dir: d}
}

// buildTick is a tick of body b's work; the last one finishes it, and the
// body falls asleep in its settlement.
func (w *World) buildTick(b *Body) {
	if b.Work.Site == 0 {
		return
	}
	b.Work.Left--
	if b.Work.Left > 0 {
		return
	}
	w.settle(b)
	b.Work = Work{}
	w.fallAsleep(b)
}

// settle makes the land round body b's site its settlement.
func (w *World) settle(b *Body) {
	if w.placeAt == nil {
		w.placeAt = make([]int64, len(w.m.Terrain))
		w.placeSp = make([]int8, len(w.m.Terrain))
	}
	w.nextPlace++
	p := Place{ID: w.nextPlace, Builder: b.ID, Species: b.Species, Until: w.tick + int64(w.cfg.BuildLife)}
	site := b.Work.Site - 1
	bx, by := site%w.m.Width, site/w.m.Width
	lo := w.cfg.BuildSide / 2
	for y := by - lo; y < by-lo+w.cfg.BuildSide; y++ {
		for x := bx - lo; x < bx-lo+w.cfg.BuildSide; x++ {
			if !w.m.InBounds(x, y) || !w.m.TerrainAt(x, y).Ground() || w.m.ShelterAt(x, y) {
				continue
			}
			w.m.SetShelter(x, y)
			t := w.m.index(x, y)
			if b.Memory.Own == nil {
				b.Memory.Own = map[int]bool{}
			}
			b.Memory.Own[t] = true
			if w.cfg.Learn && w.cfg.ShelterRead {
				w.rememberShelter(b, t) // it knows what it built
			}
			w.placeAt[t] = p.ID
			w.placeSp[t] = int8(b.Species + 1)
			p.Tiles = append(p.Tiles, t)
		}
	}
	w.places = append(w.places, p)
	b.Home = p.ID
	w.stats.Built++
}

// noteReturn counts body b's sleep, begun on tile t, as a return to a
// settlement it built, and with ReuseNights has it learn whether the
// sleep began within a built place's reach of its last one (stage 9-3b):
// whether it is one to come back to where it sleeps.
func (w *World) noteReturn(b *Body, t int) {
	if t >= 0 && w.placeAt != nil && w.placeAt[t] != 0 {
		w.stats.BuiltSleeps++
		for i := range w.places {
			if p := &w.places[i]; p.ID == w.placeAt[t] && p.Builder == b.ID {
				p.Returns++
				w.stats.Returns++
			}
		}
	}
	if !w.cfg.ReuseNights || !w.cfg.Learn || t < 0 {
		return
	}
	if b.LastSlept > 0 {
		lo := w.cfg.BuildSide / 2
		px, py := (b.LastSlept-1)%w.m.Width, (b.LastSlept-1)/w.m.Width
		if abs(t%w.m.Width-px) <= lo && abs(t/w.m.Width-py) <= lo {
			b.Memory.Reuse[0]++
		}
		b.Memory.Reuse[1]++
	}
	b.LastSlept = t + 1
}

// wearPlaces removes the settlements worn out by now; a builder still
// alive learns how often it came back to it, and has no home.
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
				w.placeSp[t] = 0
				w.m.Shelter[t] = false
			}
		}
		for i := range w.bodies {
			if b := &w.bodies[i]; b.ID == p.Builder {
				if b.Home == p.ID {
					b.Home = 0
				}
				if w.cfg.Learn && !w.cfg.ReuseNights {
					// Worn-out learning: with ReuseNights the nights
					// themselves teach it (stage 9-3b).
					b.Memory.Reuse[0] += float64(p.Returns)
					b.Memory.Reuse[1]++
				}
			}
		}
	}
	w.places = kept
}

// forgetPlace has body b, seeing tile t is no place to rest, forget it was
// one (a settlement worn out).
func (w *World) forgetPlace(b *Body, t int) {
	if b.Memory.Shelters[t] {
		delete(b.Memory.Shelters, t)
	}
	delete(b.Memory.Own, t)
}

// loadPlaces rebuilds the index of settlements' tiles after a load.
func (w *World) loadPlaces() {
	if len(w.places) == 0 {
		return
	}
	w.placeAt = make([]int64, len(w.m.Terrain))
	w.placeSp = make([]int8, len(w.m.Terrain))
	for _, p := range w.places {
		for _, t := range p.Tiles {
			w.placeAt[t] = p.ID
			w.placeSp[t] = int8(p.Species + 1)
		}
	}
}

// barred reports whether tile t is a settlement of another species than
// body b's, which b may not enter nor count as a place to rest
// (ClosedPlaces).
func (w *World) barred(b *Body, t int) bool {
	return w.cfg.ClosedPlaces && w.placeSp != nil && w.placeSp[t] != 0 && int(w.placeSp[t])-1 != b.Species
}

// Places returns the settlements standing now.
func (w *World) Places() []Place { return append([]Place(nil), w.places...) }
