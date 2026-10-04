package engine

import "testing"

// A unit with an owner is there for its owner alone: others do not see it,
// cannot eat or pick it up, and do not take it as food in their evidence;
// when the owner dies, anyone may take it.
func TestOwnership(t *testing.T) {
	w := pairWorld(t, Body{ID: 0, X: 2.5, Y: 2.5, Energy: 60}, Body{ID: 1, X: 9.5, Y: 2.5, Energy: 99})
	w.cfg.Ownership = true
	owner, other := &w.bodies[0], &w.bodies[1]
	tile := w.tileOf(other.X, other.Y)
	if w.foodOn(tile) < 0 {
		w.food.foods = append(w.food.foods, Food{X: tile % w.m.Width, Y: tile / w.m.Width})
		w.food.born = append(w.food.born, w.tick)
		w.food.owner = append(w.food.owner, noOwner)
		w.food.foodAt[tile] = int32(len(w.food.foods))
		w.food.onGround[w.m.Region[tile]]++
	}
	i := w.foodOn(tile)
	w.food.owner[i] = owner.ID
	if w.foodFor(other, tile) >= 0 || w.foodFor(owner, tile) != i {
		t.Fatal("an owned unit is there for others, or not for its owner")
	}
	for _, a := range w.possibleActions(nil, other) {
		if a.Kind == ActEat || a.Kind == ActPick {
			t.Fatalf("another body can %v an owned unit", a)
		}
	}
	w.decide(other)
	for _, f := range w.valuation.Seen {
		if w.m.index(f.X, f.Y) == tile {
			t.Fatal("another body sees an owned unit")
		}
	}
	w.release(owner.ID)
	if w.foodFor(other, tile) != i {
		t.Fatal("the unit of a dead owner is not anyone's")
	}
}
