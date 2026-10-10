package engine

import "testing"

// Without tents and building, the world is stage 6-5a: its fingerprint is
// the one pinned then.
func TestUnbuiltIsStage65a(t *testing.T) {
	const want = uint64(0x29581a36e5ebfa84)
	cfg := testConfig(1)
	cfg.AllotSight = false
	cfg.AllotFertility = false
	cfg.AllotBudget = false
	cfg.AllotVenture = false
	cfg.Species = cfg.Species[:1]
	cfg.Wade = false
	cfg.AllotCombat = false
	cfg.AllotFierce = false
	cfg.FullStep = 0 // exact tables, as before 2026-10-07
	cfg.ReuseNights, cfg.SeekBed = false, false
	cfg.ShelterFoodLife = 0
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

// Without settlements, the world is stage 8-2a3: its fingerprint is the
// one pinned then.
func TestUnsettledIsStage82a3(t *testing.T) {
	const want = uint64(0x9548c615c7b26c29)
	cfg := testConfig(1)
	cfg.AllotSight = false
	cfg.AllotFertility = false
	cfg.AllotBudget = false
	cfg.AllotVenture = false
	cfg.Species = cfg.Species[:1]
	cfg.Wade = false
	cfg.AllotCombat = false
	cfg.AllotFierce = false
	cfg.FullStep = 0 // exact tables, as before 2026-10-07
	cfg.ReuseNights, cfg.SeekBed = false, false
	cfg.ShelterFoodLife = 0
	cfg.Building = false
	w, err := NewWorld(cfg, tempMap())
	if err != nil {
		t.Fatal(err)
	}
	run(w, 1000)
	if got := w.Fingerprint(); got != want {
		t.Fatalf("fingerprint = %#x, want %#x", got, want)
	}
}

// settleWorld is a pair with a builder at (5,5), sleepy and fed, and
// another body far off.
func settleWorld(t *testing.T) (*World, *Body, *Body) {
	t.Helper()
	w := pairWorld(t, Body{ID: 0, X: 5.5, Y: 5.5, Energy: 99, Pressure: 0.5}, Body{ID: 1, X: 20.5, Y: 20.5, Energy: 60, Pressure: 0.5})
	w.cfg.Building, w.cfg.Tents = true, false
	b := &w.bodies[0]
	b.Skill[SkillBuild] = 1
	return w, b, &w.bodies[1]
}

// A builder falling asleep may begin a settlement only with the skill,
// no home, and a sleep that can wait; begun, it pays and works BuildTicks
// at its site, and done, the square round it is its own place to rest,
// it falls asleep there and has a home.
func TestSettle(t *testing.T) {
	w, b, o := settleWorld(t)
	if w.canSettle(o) {
		t.Fatal("a body without the skill can settle")
	}
	b.Pressure = w.cfg.ForceMax
	if w.canSettle(b) {
		t.Fatal("a sleep that may not wait is put off to build")
	}
	b.Pressure = 0.5
	if !w.canSettle(b) {
		t.Fatal("a fed builder cannot settle")
	}
	b.Energy = w.maxOf(b)
	b.Work = Work{Site: w.tileOf(b.X, b.Y) + 1, Left: w.cfg.BuildTicks}
	for i := 0; i < w.cfg.BuildTicks-1; i++ {
		if a := w.work(b); a.Kind != ActBuild {
			t.Fatalf("tick %d of work: %v", i, a)
		}
		w.act(0, b, Action{Kind: ActBuild})
	}
	if len(w.places) != 0 || b.Asleep {
		t.Fatal("done before its time")
	}
	w.act(0, b, Action{Kind: ActBuild})
	if len(w.places) != 1 || !b.Asleep || b.Home != w.places[0].ID || b.Work.Site != 0 || len(w.places[0].Tiles) != 9 {
		t.Fatalf("not settled: places %v home %d work %+v asleep %v", w.places, b.Home, b.Work, b.Asleep)
	}
	if !b.Memory.Own[w.tileOf(b.X, b.Y)] || !w.m.ShelterAt(5, 5) {
		t.Fatal("the builder does not know its own place, or it is no place to rest")
	}
	if w.canSettle(b) {
		t.Fatal("a body with a home can settle again")
	}
}

// Hungry with food in sight, a builder goes for it, eats, and comes back
// to its site; hungry with none and too little for the work and a night,
// it gives the site up; when sleep may not wait it sleeps.
func TestSettleWork(t *testing.T) {
	w, b, _ := settleWorld(t)
	site := w.tileOf(b.X, b.Y)
	b.Work = Work{Site: site + 1, Left: 50}
	b.Energy = 40
	for _, f := range w.Foods() {
		w.eatFood(w.foodOn(w.m.index(f.X, f.Y)))
	}
	ft := w.m.index(6, 5) // in sight (Sight 1)
	if w.foodOn(ft) < 0 {
		w.food.foods = append(w.food.foods, Food{X: 6, Y: 5})
		w.food.born = append(w.food.born, w.tick)
		w.food.owner = append(w.food.owner, noOwner)
		w.food.foodAt[ft] = int32(len(w.food.foods))
		w.foodMoved(w.m.Region[ft], 0, +1)
	}
	for i := 0; i < 20 && w.tileOf(b.X, b.Y) != ft; i++ {
		a := w.work(b)
		if a.Kind != ActMove {
			t.Fatalf("step %d to food: %v", i, a)
		}
		w.act(0, b, a)
	}
	if a := w.work(b); a.Kind != ActEat {
		t.Fatalf("at the food it does %v", a)
	}
	w.act(0, b, Action{Kind: ActEat})
	for i := 0; i < 20 && w.tileOf(b.X, b.Y) != site; i++ {
		w.act(0, b, w.work(b))
	}
	if w.tileOf(b.X, b.Y) != site || w.stats.Fetched != 1 {
		t.Fatal("it did not come back to its site")
	}
	b.Pressure = w.cfg.ForceMax
	if a := w.work(b); a.Kind != ActSleep {
		t.Fatalf("a sleep that may not wait: %v", a)
	}
	b.Pressure = 0.5
	b.Energy = 1
	for _, f := range w.Foods() {
		w.eatFood(w.foodOn(w.m.index(f.X, f.Y)))
	}
	if a := w.work(b); a.Kind != ActWait || b.Work.Site != 0 || w.stats.Abandoned != 1 {
		t.Fatalf("a starving builder with no food in sight kept on: %v %+v", a, b.Work)
	}
}

// A settlement wears out BuildLife ticks after it was done: its tiles are
// no place to rest again, and its builder learns its returns and has no
// home.
func TestPlaceWears(t *testing.T) {
	w, b, _ := settleWorld(t)
	w.cfg.ReuseNights = false // the worn-out learning is the old rule's
	b.Work = Work{Site: w.tileOf(b.X, b.Y) + 1, Left: 1}
	w.act(0, b, Action{Kind: ActBuild})
	p := &w.places[0]
	b.Asleep = false
	w.act(0, b, Action{Kind: ActSleep})
	w.tick = p.Until
	w.wearPlaces()
	if b.Home != 0 || b.Memory.Reuse != [2]float64{1, 1} || w.m.ShelterAt(5, 5) {
		t.Fatalf("worn out: home %d reuse %v", b.Home, b.Memory.Reuse)
	}
}

// With ReuseNights, each sleep after the first teaches whether it began
// within a built place's reach of the last (stage 9-3b), and the credit
// read before building falls with the evidence.
func TestReuseNights(t *testing.T) {
	w, b, _ := settleWorld(t)
	if !w.cfg.ReuseNights {
		t.Fatal("ReuseNights is off by default")
	}
	b.Memory.Warm, b.Memory.Cold = [2]float64{0.5, 1}, [2]float64{1.3, 1}
	rich := w.reuseCredit(b)
	if rich <= 0 {
		t.Fatalf("no evidence and a warm night learned read credit %d", rich)
	}
	w.noteReturn(b, w.tileOf(b.X, b.Y))
	if b.Memory.Reuse != [2]float64{0, 0} || b.LastSlept != w.tileOf(b.X, b.Y)+1 {
		t.Fatalf("first sleep: reuse %v, last %d", b.Memory.Reuse, b.LastSlept)
	}
	w.noteReturn(b, w.tileOf(b.X, b.Y)+1) // next door: within a place's reach
	if b.Memory.Reuse != [2]float64{1, 1} {
		t.Fatalf("a sleep within reach: %v", b.Memory.Reuse)
	}
	w.noteReturn(b, w.m.index(10, 8))
	if b.Memory.Reuse != [2]float64{1, 2} {
		t.Fatalf("a sleep far off: %v", b.Memory.Reuse)
	}
	b.Memory.Reuse = [2]float64{0, 40}
	if poor := w.reuseCredit(b); poor >= rich {
		t.Fatalf("forty nights of never returning left the credit %d, was %d", poor, rich)
	}
}

// With SeekBed, a builder weighs building against the best way to sleep
// tonight without building - walking to a known warm bed beats raising a
// new place next to it (stage 9-3b).
func TestSeekBed(t *testing.T) {
	w, b, _ := settleWorld(t)
	if !w.cfg.SeekBed {
		t.Fatal("SeekBed is off by default")
	}
	if r := w.bedRisk(b, nil, 100, 0); r != 1 {
		t.Fatalf("no bed known reads %v, want 1", r)
	}
	// A warm bed next door, its warmth learned; never one to come back.
	bt := w.m.index(6, 5)
	w.m.SetShelter(6, 5)
	b.Memory.Shelters = map[int]bool{bt: true}
	b.Memory.Warm = [2]float64{0.5, 1}
	b.Memory.Cold = [2]float64{1.3, 1}
	b.Memory.Reuse = [2]float64{0, 40}
	b.Pressure, b.Energy = 0.8, 40
	var rs rates
	w.ratesOf(b, &rs)
	win := w.pred.windows[0]
	bed := w.bedRisk(b, &rs, win, 0)
	plain := w.sleepRiskAt(b, &rs, win, 0, w.planBurn(b), b.Energy)
	if bed >= plain {
		t.Fatalf("a warm bed next door reads no better than the cold ground: %v >= %v", bed, plain)
	}
	begun := w.stats.Begun
	for i := 0; i < 50; i++ {
		b.Work, b.Home = Work{}, 0
		if a := w.settleInstead(b, Action{Kind: ActSleep}); a.Kind == ActBuild {
			b.Energy += w.cfg.BuildEnergy // undo the payment for the next trial
		}
	}
	if w.stats.Begun != begun {
		t.Fatalf("built %d times beside a warm bed", w.stats.Begun-begun)
	}
}

// With the nights not teaching and known beds not weighed, the world is
// stage 9-3: its fingerprint is the one pinned then (the pinned 1000
// ticks hold no second sleep, so it equals the base pin, as a pin it
// stays once they part).
func TestHopefulIsStage93(t *testing.T) {
	const want = uint64(0x8ec1735068644be4)
	cfg := testConfig(1)
	cfg.AllotBudget = false
	cfg.AllotVenture = false
	cfg.Species = cfg.Species[:1]
	cfg.Wade = false
	cfg.AllotCombat = false
	cfg.AllotFierce = false
	cfg.FullStep = 0 // exact tables, as before 2026-10-07
	cfg.AllotMix, cfg.AllotFertility = false, false
	cfg.ReuseNights, cfg.SeekBed = false, false
	w, err := NewWorld(cfg, tempMap())
	if err != nil {
		t.Fatal(err)
	}
	run(w, 1000)
	if got := w.Fingerprint(); got != want {
		t.Fatalf("fingerprint = %#x, want %#x", got, want)
	}
}
