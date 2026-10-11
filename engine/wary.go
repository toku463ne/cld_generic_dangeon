package engine

// Wary (stage 10-6; the user's wish of 2026-10-11: where the threats
// were, passed on, and many memories summed up). A hunted body keeps a
// row per region of the blows its kind took there against the ticks it
// spent there - the same "K of N" tally as the region rows of food, so
// that many events in one region sum into one rate, and the row ages
// (EvidenceHalfLife) and passes between bodies (tell.go) as heard
// evidence. Evidence comes from a blow landing on the body itself and
// from a call heard that one of its kind is struck (the alarm of stage
// 10-4, folded in here). A hunter merely seen is no evidence: it is read
// live while in sight (Fear), and one that did not strike cost nothing.
//
// The valuation reads the row as the blows the region would land on the
// body over the window - rate times Window times a blow at parity - and
// takes them off the energy every option that leaves the body there is
// read from, as the hunters in sight are (predict.go). The pull out of a
// region of blows thus comes from the same currency as everything else.

// threatRow returns mem's row of region r, grown to hold it.
func (w *World) threatRow(mem *Memory, r RegionID) *Tally {
	for int(r) >= len(mem.Threats) {
		mem.Threats = append(mem.Threats, Tally{T: w.tick})
	}
	return &mem.Threats[r]
}

// wary reports whether body b keeps and reads the rows of blows: Wary,
// and a species something hunts.
func (w *World) wary(b *Body) bool {
	return w.cfg.Wary && w.hunted(b.Species)
}

// noteThreats has body b count the tick in the row of the region it
// stands in: a tick spent there, no blow.
func (w *World) noteThreats(b *Body) {
	if !w.wary(b) {
		return
	}
	t := w.tileOf(b.X, b.Y)
	if t < 0 {
		return
	}
	row := w.threatRow(&b.Memory, w.m.Region[t])
	w.ageBy(row)
	row.N++
}

// noteBlow has body b count a blow in the row of region r, where it fell:
// on itself, or on one of its kind whose call it heard (one tick there,
// one blow). Something new to pass on.
func (w *World) noteBlow(b *Body, r RegionID) {
	if !w.wary(b) {
		return
	}
	row := w.threatRow(&b.Memory, r)
	w.ageBy(row)
	row.N++
	row.K++
	b.Memory.Ver++
}

// threatRate is body b's estimate of the blows per tick its kind takes in
// region r: its row, as it weighs now, pulled toward none by WaryWeight
// ticks.
func (w *World) threatRate(b *Body, r RegionID) float64 {
	if int(r) >= len(b.Memory.Threats) {
		return 0
	}
	t := b.Memory.Threats[r]
	f := w.decay(t.T)
	return Tally{N: f * t.N, K: f * t.K}.estimate(0, w.cfg.WaryWeight)
}

// regionLoss is the energy body b reads the region of (x, y) taking from
// it over the window: the blows at its rate there, each a blow at parity.
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
	return rate * float64(w.cfg.Window) * w.cfg.HitEnergy / 2
}

// Threat is body b's estimate of the blows per tick its kind takes in
// region r, read off its memory: zero where it knows of none.
func (w *World) Threat(b Body, r RegionID) float64 { return w.threatRate(&b, r) }
