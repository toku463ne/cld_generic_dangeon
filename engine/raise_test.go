package engine

import "testing"

// Without tents and building, the world is stage 6-5a: its fingerprint is
// the one pinned then.
func TestUnbuiltIsStage65a(t *testing.T) {
	const want = uint64(0x29581a36e5ebfa84)
	cfg := testConfig(1)
	cfg.AllotSight = false
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
		w.food.onGround[w.m.Region[ft]]++
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
