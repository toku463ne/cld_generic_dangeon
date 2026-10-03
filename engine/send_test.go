package engine

import "testing"

// sendWorld is a father on a unit of food and a mother resting after their
// child, who knows a place to rest; the father is fed, so the unit does not
// matter to his life.
func sendWorld(t *testing.T, energy float64) (*World, *Body, *Body) {
	t.Helper()
	w := pairWorld(t, Body{ID: 0, X: 2.5, Y: 2.5, Energy: 60}, Body{ID: 1, X: 9.5, Y: 2.5, Energy: energy})
	w.cfg.FemaleBears, w.cfg.FatherSends, w.cfg.SendGate, w.cfg.Provision = true, true, 0.01, false
	w.cfg.ForceAt = 0
	mother, father := &w.bodies[0], &w.bodies[1]
	mother.Rested, mother.Partner, mother.Asks = w.tick+300, father.ID, 2
	mother.Memory.Shelters = map[int]bool{w.m.index(1, 1): true}
	tile := w.tileOf(father.X, father.Y)
	if w.foodOn(tile) < 0 {
		w.food.foods = append(w.food.foods, Food{X: tile % w.m.Width, Y: tile / w.m.Width})
		w.food.born = append(w.food.born, w.tick)
		w.food.foodAt[tile] = int32(len(w.food.foods))
		w.food.onGround[w.m.Region[tile]]++
	}
	w.listMates()
	return w, mother, father
}

// A fed father on food sends it, while his resting mate has an ask open,
// to the place she knows, and her ask is answered: it
// leaves the ground, is on its way a tick for each Speed of it, keeps its
// age, and lands there; the account closes throughout. A father whose
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
	for w.tick < s.Arrive {
		w.tick++
		w.deliverFood()
	}
	var at int
	for tl := range w.food.delivered {
		at = tl
	}
	if len(w.food.delivered) != 1 || w.foodOn(at) < 0 || w.Stats().Delivered != 1 || !w.FoodLedger().Balanced() {
		t.Fatal("the unit did not come to the place")
	}
	if x, y := at%w.m.Width, at/w.m.Width; max(abs(x-1), abs(y-1)) > 2 {
		t.Fatalf("delivered at (%d,%d), more than two from the place", x, y)
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
	mother.Memory.Shelters = nil
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
