package engine

import "testing"

// A unit with an owner is there for its owner alone: others do not see it,
// cannot eat or pick it up, and do not take it as food in their evidence;
// when the owner dies, anyone may take it. Food a father sends is the
// mother's with Ownership and anyone's without.
func TestOwnership(t *testing.T) {
	w, mother, father := sendWorld(t, 99)
	w.cfg.Ownership = true
	tile := w.tileOf(father.X, father.Y)
	i := w.foodOn(tile)
	w.food.owner[i] = mother.ID
	if w.foodFor(father, tile) >= 0 || w.foodFor(mother, tile) != i {
		t.Fatal("an owned unit is there for others, or not for its owner")
	}
	for _, a := range w.possibleActions(nil, father) {
		if a.Kind == ActEat || a.Kind == ActPick {
			t.Fatalf("another body can %v an owned unit", a)
		}
	}
	w.decide(father)
	for _, f := range w.valuation.Seen {
		if w.m.index(f.X, f.Y) == tile {
			t.Fatal("another body sees an owned unit")
		}
	}
	w.release(mother.ID)
	if w.foodFor(father, tile) != i {
		t.Fatal("the unit of a dead owner is not anyone's")
	}

	// Sent food is the mother's.
	w, mother, father = sendWorld(t, 99)
	w.cfg.Ownership = true
	w.decide(father)
	w.act(1, father, w.sendInstead(father, Action{Kind: ActWait}))
	s := w.food.sending[0]
	if j := w.foodOn(s.Tile); j >= 0 {
		w.eatFood(j)
	}
	w.tick = s.Arrive
	w.deliverFood()
	if j := w.foodOn(s.Tile); j < 0 || w.food.owner[j] != mother.ID {
		t.Fatal("sent food did not come as the mother's")
	}
}

// Without owners, the world is stage 6-4b: its fingerprint is the one
// pinned then.
func TestOwnerlessIsStage64b(t *testing.T) {
	const want = uint64(0xf68b7173e11b46c2)
	cfg := testConfig(1)
	cfg.Ownership = false
	w, err := NewWorld(cfg, tempMap())
	if err != nil {
		t.Fatal(err)
	}
	run(w, 1000)
	if got := w.Fingerprint(); got != want {
		t.Fatalf("fingerprint = %#x, want %#x", got, want)
	}
}
