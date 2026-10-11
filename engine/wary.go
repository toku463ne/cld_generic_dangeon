package engine

// Wary (stages 10-6 and 10-6b; the user's wish of 2026-10-11: where the
// threats were, passed on, and many memories summed up). A hunted body
// keeps a row per region of the ticks it saw a hunter standing there
// against the ticks it spent awake in a region - the same "K of N" tally
// as the region rows of food, so that many sightings in one region sum
// into one rate - and one row of the blows that came of its sightings:
// the ticks it had a hunter in sight against the blows its kind took,
// on itself or heard of (the call of stage 10-4, folded in here). The
// rows age (EvidenceHalfLife) and pass between bodies (tell.go) as heard
// evidence, so that what one body saw across the border reaches the
// others.
//
// Stage 10-6 counted blows alone as a region's evidence, and the forest
// stayed unwritten: those struck there were alone and died untold.
// Sightings are plentiful - but counting each as a blow would read the
// home region, where a hunter crosses now and then, as death too (a
// tenth of a blow a tick, a thousand ticks). So a sighting counts as a
// blow at the rate the body has learned sightings turn into blows, and a
// region's loss over the window is sightings per tick there, times blows
// per sighting, times the window, times a blow at parity, taken off the
// energy every option that leaves the body there is read from, as the
// hunters in sight are (predict.go).

// threatRow returns mem's row of region r, grown to hold it.
func (w *World) threatRow(mem *Memory, r RegionID) *Tally {
	for int(r) >= len(mem.Threats) {
		mem.Threats = append(mem.Threats, Tally{T: w.tick})
	}
	return &mem.Threats[r]
}

// wary reports whether body b keeps and reads the rows: Wary, and a
// species something hunts.
func (w *World) wary(b *Body) bool {
	return w.cfg.Wary && w.hunted(b.Species)
}

// noteSightings has body b, awake and deciding, count the tick in the
// row of the region it stands in, and a sighting in the row of every
// region a hunter in its sight stands in (one a region a tick, where the
// hunter is, not where b is: the forest is seen from its edge). A tick
// with any hunter in sight is one its sightings row counts.
func (w *World) noteSightings(b *Body) {
	if !w.wary(b) {
		return
	}
	t := w.tileOf(b.X, b.Y)
	if t < 0 {
		return
	}
	here := w.threatRow(&b.Memory, w.m.Region[t])
	w.ageBy(here)
	here.N++
	var counted [8]RegionID
	seen := 0
	w.hunters(b, func(o *Body) {
		ot := w.tileOf(o.X, o.Y)
		if ot < 0 {
			return
		}
		r := w.m.Region[ot]
		for _, c := range counted[:min(seen, len(counted))] {
			if c == r {
				return // one a region a tick
			}
		}
		if seen < len(counted) {
			counted[seen] = r
		}
		seen++
		row := w.threatRow(&b.Memory, r)
		w.ageBy(row)
		row.K++
	})
	if seen > 0 {
		w.ageBy(&b.Memory.Blows)
		b.Memory.Blows.N++
	}
}

// noteBlow has body b count a blow its kind took in region r: on itself
// (heard false; its sighting of the hunter is counted by noteSightings)
// or one it heard of (heard true: a tick a hunter stood there, and a
// sighting that became a blow). Something new to pass on.
func (w *World) noteBlow(b *Body, r RegionID, heard bool) {
	if !w.wary(b) {
		return
	}
	if heard {
		row := w.threatRow(&b.Memory, r)
		w.ageBy(row)
		row.N++
		row.K++
		w.ageBy(&b.Memory.Blows)
		b.Memory.Blows.N++
	} else {
		w.ageBy(&b.Memory.Blows)
	}
	b.Memory.Blows.K++
	b.Memory.Ver++
}

// fresh is tally t as it weighs now, aged from when it was last brought
// up to date.
func (w *World) fresh(t Tally) Tally {
	f := w.decay(t.T)
	return Tally{N: f * t.N, K: f * t.K}
}

// threatRate is body b's estimate of the sightings per tick in region
// r: its row, as it weighs now, pulled toward none by WaryWeight ticks,
// held to one (a region seen into from outside can hold more sightings
// than ticks spent).
func (w *World) threatRate(b *Body, r RegionID) float64 {
	if int(r) >= len(b.Memory.Threats) {
		return 0
	}
	return min(w.fresh(b.Memory.Threats[r]).estimate(0, w.cfg.WaryWeight), 1)
}

// blowRate is body b's estimate of the blows per sighting: its row, as
// it weighs now, pulled toward none by WaryWeight sightings.
func (w *World) blowRate(b *Body) float64 {
	return min(w.fresh(b.Memory.Blows).estimate(0, w.cfg.WaryWeight), 1)
}

// regionLoss is the energy body b reads the region of (x, y) taking from
// it over the window: the sightings at its rate there, each a blow at
// the rate sightings become blows, each a blow at parity.
func (w *World) regionLoss(b *Body, x, y float64) float64 {
	if !w.wary(b) {
		return 0
	}
	t := w.tileOf(x, y)
	if t < 0 {
		return 0
	}
	rate := w.threatRate(b, w.m.Region[t])
	if rate <= 0 {
		return 0
	}
	return rate * w.blowRate(b) * float64(w.cfg.Window) * w.cfg.HitEnergy / 2
}

// Threat is body b's estimate of the sightings per tick in region r,
// read off its memory: zero where it knows of none.
func (w *World) Threat(b Body, r RegionID) float64 { return w.threatRate(&b, r) }

// BlowRate is body b's estimate of the blows per sighting, read off its
// memory.
func (w *World) BlowRate(b Body) float64 { return w.blowRate(&b) }
