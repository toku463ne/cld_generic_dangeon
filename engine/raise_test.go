package engine

import "testing"

// Without building, the world is stage 6-5a: its fingerprint is the one
// pinned then.
func TestUnbuiltIsStage65a(t *testing.T) {
	const want = uint64(0x29581a36e5ebfa84)
	cfg := testConfig(1)
	cfg.Building, cfg.Tents = false, false
	w, err := NewWorld(cfg, tempMap())
	if err != nil {
		t.Fatal(err)
	}
	run(w, 1000)
	if got := w.Fingerprint(); got != want {
		t.Fatalf("fingerprint = %#x, want %#x", got, want)
	}
}

// A sleepy adult born with the skill may build where no place to rest is,
// and only then: it pays, the land round it becomes a place to rest, and
// it falls asleep there. Its later sleeps there count as returns, others'
// do not; when the place wears out its tiles are gone, the builder learns
// how often it came back, and a body that sees the tiles forgets them.
func TestBuild(t *testing.T) {
	w := pairWorld(t, Body{ID: 0, X: 5.5, Y: 5.5, Energy: 60, Pressure: 0.5}, Body{ID: 1, X: 9.5, Y: 9.5, Energy: 60, Pressure: 0.5})
	w.cfg.Building, w.cfg.ForceAt = true, 0
	b, o := &w.bodies[0], &w.bodies[1]
	b.Skill[SkillBuild] = 1
	if hasKind(w.possibleActions(nil, o), ActBuild) {
		t.Fatal("a body without the skill can build")
	}
	if !hasKind(w.possibleActions(nil, b), ActBuild) {
		t.Fatal("a sleepy adult with the skill cannot build")
	}
	b.Pressure = w.cfg.WakeAt
	if hasKind(w.possibleActions(nil, b), ActBuild) {
		t.Fatal("a body that may not sleep can build")
	}
	b.Pressure = 0.5
	b.Energy = w.cfg.BuildEnergy
	if hasKind(w.possibleActions(nil, b), ActBuild) {
		t.Fatal("a body that cannot pay can build")
	}
	b.Energy = 60
	w.act(0, b, Action{Kind: ActBuild})
	if b.Energy != 60-w.cfg.BuildEnergy || !b.Asleep || len(w.places) != 1 {
		t.Fatal("building did not pay, sleep or make a place")
	}
	p := w.places[0]
	if len(p.Tiles) != w.cfg.BuildSide*w.cfg.BuildSide || p.Until != w.tick+int64(w.cfg.BuildLife) {
		t.Fatalf("place = %+v", p)
	}
	for _, tl := range p.Tiles {
		if !w.m.ShelterAt(tl%w.m.Width, tl/w.m.Width) {
			t.Fatal("a built tile is not a place to rest")
		}
	}
	if hasKind(w.possibleActions(nil, &Body{X: 5.5, Y: 5.5, Pressure: 0.5, Energy: 60, Skill: b.Skill}), ActBuild) {
		t.Fatal("a body can build on a place to rest")
	}
	b.Asleep = false
	w.act(0, b, Action{Kind: ActSleep})
	o.X, o.Y = 4.5, 4.5
	w.act(1, o, Action{Kind: ActSleep})
	if w.places[0].Returns != 1 || w.stats.Returns != 1 || w.stats.BuiltSleeps != 2 {
		t.Fatalf("returns %d, stats %+v", w.places[0].Returns, w.stats)
	}
	o.Asleep, o.Pressure = false, 0
	w.noteShelters(o)
	if !o.Memory.Shelters[w.m.index(4, 4)] {
		t.Fatal("a body does not remember a built place it sees")
	}
	w.tick = p.Until
	w.wearPlaces()
	if len(w.places) != 0 || w.m.ShelterAt(5, 5) || b.Memory.Reuse != [2]float64{1, 1} {
		t.Fatalf("worn out: places %d, reuse %v", len(w.places), b.Memory.Reuse)
	}
	w.noteShelters(o)
	if o.Memory.Shelters[w.m.index(4, 4)] {
		t.Fatal("a body still remembers a worn-out place it sees")
	}
}

// Building reads as the sleep it ends in, paid for first: with nothing
// learned, at the burn now; with warmth learned, at the warm burn; and the
// nights the body expects to come back add to its reserve.
func TestBuildRisk(t *testing.T) {
	w := pairWorld(t, Body{ID: 0, X: 5.5, Y: 5.5, Energy: 30, Pressure: 0.5}, Body{ID: 1, X: 9.5, Y: 9.5, Energy: 60})
	w.cfg.Building = true
	b := &w.bodies[0]
	b.Skill[SkillBuild] = 1
	var rs rates
	w.ratesOf(b, &rs)
	win := w.pred.windows[0]
	sleep, plain := w.sleepRisk(b, &rs, win, 0), w.buildRisk(b, &rs, win, 0)
	if plain < sleep {
		t.Fatalf("an unlearned build reads safer than sleeping: %v < %v", plain, sleep)
	}
	b.Memory.Warm = [2]float64{0.5, 1}
	b.BurnFelt = 0
	warm := w.buildRisk(b, &rs, win, 0)
	b.Memory.Reuse = [2]float64{5, 1}
	back := w.buildRisk(b, &rs, win, 0)
	if !(warm <= plain && back <= warm) {
		t.Fatalf("plain %v, warm %v, with returns %v", plain, warm, back)
	}
}

// Expecting nothing of a place until one wears out, and not knowing its own
// places, the world is stage 8-1: its fingerprint is the one pinned then.
func TestNeutralIsStage81(t *testing.T) {
	const want = uint64(0x7189c62dc5180e9f)
	cfg := testConfig(1)
	cfg.Building, cfg.Tents = true, false
	cfg.OwnPlaces, cfg.ReusePrior, cfg.ReuseWeight = false, 0, 0
	w, err := NewWorld(cfg, tempMap())
	if err != nil {
		t.Fatal(err)
	}
	run(w, 1000)
	if got := w.Fingerprint(); got != want {
		t.Fatalf("fingerprint = %#x, want %#x", got, want)
	}
}

// A builder remembers the tiles it built as its own, forgets them when it
// sees them gone, and does not pass them on. The returns it expects start
// at ReusePrior and move with what it sees; their worth adds to building
// and to walking back to its own place, not to another's.
func TestOwnPlaces(t *testing.T) {
	w := pairWorld(t, Body{ID: 0, X: 5.5, Y: 5.5, Energy: 60, Pressure: 0.5}, Body{ID: 1, X: 20.5, Y: 20.5, Energy: 60, Pressure: 0.5})
	w.cfg.Building, w.cfg.ForceAt = true, 0
	b := &w.bodies[0]
	b.Skill[SkillBuild] = 1
	w.act(0, b, Action{Kind: ActBuild})
	if len(b.Memory.Own) != w.cfg.BuildSide*w.cfg.BuildSide || !b.Memory.Own[w.m.index(5, 5)] {
		t.Fatalf("own tiles %v", b.Memory.Own)
	}
	now, warm := 0.2, 0.1
	w.cfg.ReusePrior, w.cfg.ReuseWeight = 2, 1
	prior := w.reuseCredit(b, now, warm)
	if want := w.units(2 * (now - warm) * float64(w.cfg.SleepTicks)); prior != want {
		t.Fatalf("credit with nothing seen = %d, want %d", prior, want)
	}
	b.Memory.Reuse = [2]float64{0, 3}
	if c := w.reuseCredit(b, now, warm); c >= prior {
		t.Fatalf("seeing no returns did not lower the credit: %d", c)
	}
	w.cfg.ReusePrior, w.cfg.ReuseWeight = 0, 0
	if c := w.reuseCredit(b, now, warm); c != 0 {
		t.Fatalf("8-1 with no returns seen gives credit %d", c)
	}
	w.cfg.ReusePrior, w.cfg.ReuseWeight = 2, 1
	b.Memory.Reuse = [2]float64{}

	// Away from it, after a night there, it reads walking back with credit.
	b.Asleep, b.Pressure, b.X, b.Y = false, 0.5, 15.5, 5.5
	b.Memory.Warm = [2]float64{0.3, 1}
	w.tick = 600 // dark
	w.decide(b)
	v := &w.valuation
	if len(v.Shelter) == 0 || len(v.Credit) != len(v.Shelter) {
		t.Fatalf("no place to walk back to: %v", v.Shelter)
	}
	if !b.Memory.Own[v.Shelter[len(v.Shelter)-1].Y*w.m.Width+v.Shelter[len(v.Shelter)-1].X] || v.Credit[len(v.Credit)-1] <= 0 {
		t.Fatalf("own place %v read with credit %v", v.Shelter, v.Credit)
	}

	// Another body learns the tiles but not as its own.
	o := &w.bodies[1]
	o.X, o.Y, o.Asleep = 6.5, 6.5, false
	w.tick = 0
	w.noteShelters(o)
	if len(o.Memory.Own) != 0 || !o.Memory.Shelters[w.m.index(5, 5)] {
		t.Fatal("another body knows the place as its own, or not at all")
	}
	w.tick = w.places[0].Until
	w.wearPlaces()
	w.tick = (w.tick/int64(w.cfg.DayTicks)+1)*int64(w.cfg.DayTicks) + 100 // in full light
	b.X, b.Y = 5.5, 5.5
	w.noteShelters(b)
	if len(b.Memory.Own) != 0 {
		t.Fatal("a builder still holds a worn-out place as its own")
	}
}
