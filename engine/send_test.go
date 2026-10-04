package engine

import "testing"

// sendWorld is a father on a unit of food and a mother resting after their
// child, who knows a place to rest; the father is fed, so the unit does not
// matter to his life.
func sendWorld(t *testing.T, energy float64) (*World, *Body, *Body) {
	t.Helper()
	w := pairWorld(t, Body{ID: 0, X: 2.5, Y: 2.5, Energy: 60}, Body{ID: 1, X: 9.5, Y: 2.5, Energy: energy})
	w.cfg.FemaleBears, w.cfg.FatherSends, w.cfg.SendGate, w.cfg.Provision = true, true, 0.01, false
	w.cfg.HomeRead = true // delivered onto the tile sent to (6-4b)
	w.cfg.ForceAt = 0
	mother, father := &w.bodies[0], &w.bodies[1]
	mother.Rested, mother.Partner, mother.Asks = w.tick+300, father.ID, 2
	mother.Memory.Shelters = map[int]bool{w.m.index(1, 1): true}
	mother.Home = w.m.index(1, 1) + 1 // her home for this rest (6-4d)
	tile := w.tileOf(father.X, father.Y)
	if w.foodOn(tile) < 0 {
		w.food.foods = append(w.food.foods, Food{X: tile % w.m.Width, Y: tile / w.m.Width})
		w.food.born = append(w.food.born, w.tick)
		w.food.owner = append(w.food.owner, noOwner)
		w.food.foodAt[tile] = int32(len(w.food.foods))
		w.food.onGround[w.m.Region[tile]]++
	}
	w.listMates()
	return w, mother, father
}

// A fed father on food sends it, while his resting mate has an ask open,
// to the place she knows, and her ask is answered: it
// leaves the ground, is on its way a tick for each Speed of it, keeps its
// age, waits while the tile it is sent to has food on it, and lands there
// once it is free; the account closes throughout. A father whose
// life needs the unit keeps to his choice, and so does one whose mate has
// not asked or knows no place to rest.
func TestFatherSends(t *testing.T) {
	w, mother, father := sendWorld(t, 99)
	w.decide(father)
	a := w.sendInstead(father, Action{Kind: ActWait})
	if a.Kind != ActSend {
		t.Fatalf("a fed father did %v, want send", a)
	}
	tile := w.tileOf(father.X, father.Y)
	before := w.FoodLedger()
	w.act(1, father, a)
	if w.foodOn(tile) >= 0 || w.Stats().Sent != 1 || len(w.food.sending) != 1 || mother.Asks != 1 {
		t.Fatal("the unit did not leave, or the ask stayed open")
	}
	if l := w.FoodLedger(); !l.Balanced() || l.OnGround+l.Held != before.OnGround+before.Held {
		t.Fatalf("account after sending: %+v", l)
	}
	s := w.food.sending[0]
	if s.Tile != w.m.index(1, 1) || s.Arrive != w.tick+int64(8/w.cfg.Speed+0.5) || s.Mother != mother.ID {
		t.Fatalf("sent %+v", s)
	}
	home := w.m.index(1, 1)
	if w.foodOn(home) < 0 { // the tile taken: the unit waits on its way
		w.food.foods = append(w.food.foods, Food{X: 1, Y: 1})
		w.food.born = append(w.food.born, w.tick)
		w.food.owner = append(w.food.owner, noOwner)
		w.food.foodAt[home] = int32(len(w.food.foods))
		w.food.onGround[w.m.Region[home]]++
	}
	for w.tick < s.Arrive {
		w.tick++
		w.deliverFood()
	}
	if len(w.food.sending) != 1 || w.Stats().Delivered != 0 {
		t.Fatal("delivered onto a tile with food on it")
	}
	w.eatFood(w.foodOn(home))
	w.tick++
	w.deliverFood()
	var at int
	for tl := range w.food.delivered {
		at = tl
	}
	if len(w.food.delivered) != 1 || w.foodOn(at) < 0 || w.Stats().Delivered != 1 || !w.FoodLedger().Balanced() {
		t.Fatal("the unit did not come to the place")
	}
	if at != w.m.index(1, 1) {
		t.Fatalf("delivered at (%d,%d), not on the tile sent to", at%w.m.Width, at/w.m.Width)
	}
	w.noteDelivered(mother, at)
	if w.Stats().MotherAte != 1 {
		t.Fatal("the mother taking it is not counted")
	}

	w, mother, father = sendWorld(t, 20) // a meal would save him
	w.decide(father)
	if a := w.sendInstead(father, Action{Kind: ActEat}); a.Kind != ActEat {
		t.Fatalf("a hungry father did %v, want to eat", a)
	}
	w, mother, father = sendWorld(t, 99)
	mother.Asks = 0
	w.decide(father)
	if a := w.sendInstead(father, Action{Kind: ActWait}); a.Kind != ActWait {
		t.Fatal("sent to a mother who has not asked")
	}
	w, mother, father = sendWorld(t, 99)
	mother.Memory.Shelters, mother.Home = nil, 0
	w.decide(father)
	if a := w.sendInstead(father, Action{Kind: ActWait}); a.Kind != ActWait {
		t.Fatal("sent to a mother who knows no place to rest")
	}
}

// Without fathers sending, and with provisioning, the world is stage
// 6-5a: its fingerprint is the one pinned then.
func TestFatherlessIsStage65a(t *testing.T) {
	const want = uint64(0x29581a36e5ebfa84)
	cfg := testConfig(1)
	cfg.HomeKeep = false
	cfg.Ownership = false
	cfg.HideShelter, cfg.HomeRead = false, false
	cfg.FatherSends, cfg.Provision = false, true
	w, err := NewWorld(cfg, tempMap())
	if err != nil {
		t.Fatal(err)
	}
	run(w, 1000)
	if got := w.Fingerprint(); got != want {
		t.Fatalf("fingerprint = %#x, want %#x", got, want)
	}
}

// Food on a place to rest shows only from a place to rest. A resting
// mother who asked reads home as a unit she remembers, at the chance her
// visits there found food, until she has been there; there she learns
// whether food was.
func TestHomeRead(t *testing.T) {
	w, mother, _ := sendWorld(t, 99)
	w.cfg.HideShelter, w.cfg.HomeRead, w.cfg.HomeKeep = true, true, false // home as 6-4b reads it
	w.m = tempMap()                                                       // places to rest at (1..3, 1..3)
	w.light, w.lightFor = 1, w.tick+1
	if !w.m.ShelterAt(1, 1) || w.seesFood(mother, 4, 2, 3, 2) || !w.seesFood(mother, 2, 2, 3, 2) || !w.seesFood(mother, 4, 2, 5, 2) {
		t.Fatal("food on a place to rest shows from outside it, or not from inside")
	}
	if _, _, ok := w.homeAhead(mother); ok {
		t.Fatal("reads home before asking")
	}
	w.tick += 10
	mother.AskedAt = w.tick
	f, p, ok := w.homeAhead(mother)
	if !ok || f.X != 1 || f.Y != 1 || p != w.cfg.PriorSpot {
		t.Fatalf("home ahead %v at %v (%v)", f, p, ok)
	}
	w.decide(mother)
	if v := w.valuation; len(v.Home) != 1 || v.Home[0] != f {
		t.Fatalf("valuation reads home %v", v.Home)
	}
	mother.X, mother.Y = 1.5, 1.5
	w.tick++
	w.noteHome(mother)
	if mother.HomeAt != w.tick || mother.Memory.Home.N != 1 {
		t.Fatalf("at home: learned %+v", mother.Memory.Home)
	}
	if _, _, ok := w.homeAhead(mother); ok {
		t.Fatal("reads home again without asking again")
	}
}

// With food on places to rest showing from anywhere and no mother reading
// home, the world is stage 6-4a: its fingerprint is the one pinned then.
func TestHomelessIsStage64a(t *testing.T) {
	const want = uint64(0x910c4709afac44ef)
	cfg := testConfig(1)
	cfg.HomeKeep = false
	cfg.Ownership = false
	cfg.HideShelter, cfg.HomeRead = false, false
	w, err := NewWorld(cfg, tempMap())
	if err != nil {
		t.Fatal(err)
	}
	run(w, 1000)
	if got := w.Fingerprint(); got != want {
		t.Fatalf("fingerprint = %#x, want %#x", got, want)
	}
}

// With HomeKeep a mother's home is fixed at the birth, her food is sent
// there whatever her position, and once she has asked in her rest she
// reads home until the rest ends: a visit that finds nothing does not end
// it, and she learns once at each arrival.
func TestHomeKeep(t *testing.T) {
	w, mother, father := sendWorld(t, 99)
	w.cfg.HomeKeep, w.cfg.HomeRead = true, true
	w.m = tempMap()
	w.light, w.lightFor = 1, w.tick+1
	mother.Home = w.m.index(3, 3) + 1 // fixed at the birth, whatever is near now
	if dest, ok := w.homeOf(mother); !ok || dest != w.m.index(3, 3) {
		t.Fatalf("home %d (%v)", dest, ok)
	}
	w.decide(father)
	w.act(1, father, w.sendInstead(father, Action{Kind: ActWait}))
	if len(w.food.sending) != 1 || w.food.sending[0].Tile != w.m.index(3, 3) {
		t.Fatal("food not sent to her home")
	}
	mother.AskedAt = mother.Rested - int64(w.cfg.RecoverTicks) - 1 // before this rest
	if _, _, ok := w.homeAhead(mother); ok {
		t.Fatal("reads home before asking")
	}
	w.tick += 5
	mother.AskedAt = w.tick
	mother.X, mother.Y = 3.5, 3.5
	if j := w.foodOn(w.m.index(3, 3)); j >= 0 {
		w.eatFood(j)
	}
	w.noteHome(mother)
	w.noteHome(mother) // still there: learned once
	if mother.Memory.Home.N != 1 || !mother.AtHome {
		t.Fatalf("learned %+v at home", mother.Memory.Home)
	}
	if _, _, ok := w.homeAhead(mother); !ok {
		t.Fatal("gave up reading home after a visit that found nothing")
	}
	mother.X, mother.Y = 6.5, 3.5
	w.noteHome(mother)
	if mother.AtHome {
		t.Fatal("still at home after leaving")
	}
	mother.Rested = w.tick
	if _, _, ok := w.homeAhead(mother); ok {
		t.Fatal("reads home after the rest")
	}
}

// With home read as 6-4c reads it, the world is stage 6-4c: its
// fingerprint is the one pinned then.
func TestHomeforgetIsStage64c(t *testing.T) {
	const want = uint64(0xbebc957b64718322)
	cfg := testConfig(1)
	cfg.HomeKeep = false
	w, err := NewWorld(cfg, tempMap())
	if err != nil {
		t.Fatal(err)
	}
	run(w, 1000)
	if got := w.Fingerprint(); got != want {
		t.Fatalf("fingerprint = %#x, want %#x", got, want)
	}
}
