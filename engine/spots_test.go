package engine

import "testing"

// spotWorld is a learning world with one adult body and no food at all.
func spotWorld(t *testing.T, energy float64) (*World, *Body) {
	t.Helper()
	w, b := soloWorld(t, energy)
	for len(w.food.foods) > 0 {
		w.eatFood(0)
	}
	return w, b
}

// place puts a unit of food on tile (x, y).
func place(w *World, x, y int) {
	t := w.m.index(x, y)
	w.food.foods = append(w.food.foods, Food{X: x, Y: y})
	w.food.foodAt[t] = int32(len(w.food.foods))
	w.food.appeared++
	w.foodMoved(w.m.Region[t], +1)
}

// A body remembers the food it sees and forgets a tile it sees empty; the
// spot row learns only from a walk to a remembered tile.
func TestNoteSpots(t *testing.T) {
	w, b := spotWorld(t, 50)
	place(w, 3, 3)
	w.noteSpots(b)
	tile := w.m.index(3, 3)
	if _, ok := b.Memory.Spots[tile]; !ok {
		t.Fatal("food in sight not remembered")
	}
	w.eatFood(w.foodOn(tile))
	w.noteSpots(b)
	if _, ok := b.Memory.Spots[tile]; ok {
		t.Fatal("a tile seen empty is still remembered")
	}
	if b.Memory.Spot.N != 0 {
		t.Fatalf("learned from a tile it did not walk to: %+v", b.Memory.Spot)
	}
	place(w, 3, 3)
	w.noteSpots(b)
	b.Goal = tile
	w.noteSpots(b)
	if b.Memory.Spot.N != 1 || b.Memory.Spot.K != 1 {
		t.Fatalf("a walk that found it: %+v", b.Memory.Spot)
	}
}

// The units read are the SpotRead nearest out of sight, nearest first.
func TestRecalledNearest(t *testing.T) {
	w, b := spotWorld(t, 50)
	w.cfg.SpotRead = 2
	b.Memory.Spots = map[int]int64{
		w.m.index(3, 3): 0, // in sight
		w.m.index(5, 2): 0,
		w.m.index(6, 2): 0,
		w.m.index(2, 6): 0,
		w.m.index(9, 7): 0,
	}
	got := w.recalled(nil, b)
	want := []Food{{X: 5, Y: 2}, {X: 6, Y: 2}} // (6,2) ties (2,6) and has the lower tile
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("recalled %v, want %v", got, want)
	}
}

// A hungry body that sees no food reads a step toward a remembered unit as
// better than a step away; believing remembered food never stays, it does
// not.
func TestRememberedFoodDraws(t *testing.T) {
	for _, stays := range []bool{true, false} {
		w, b := spotWorld(t, 15)
		b.Memory.Spots = map[int]int64{w.m.index(6, 2): 0}
		if !stays {
			w.cfg.PriorSpot = 0
		}
		w.decide(b)
		v := w.valuation
		east, west := -1, -1
		for j, o := range v.Options {
			if o == (Action{Kind: ActMove, Dir: 0}) {
				east = j
			}
			if o == (Action{Kind: ActMove, Dir: 4}) {
				west = j
			}
		}
		re, rw := v.Risk[0][east], v.Risk[0][west]
		if stays && !(re < rw) {
			t.Errorf("toward the remembered unit %v, away %v", re, rw)
		}
		if !stays && re < rw-1e-12 {
			t.Errorf("never there, toward %v still beats away %v", re, rw)
		}
	}
}

// A window the table does not keep reads as the shortest kept above it.
func TestDeadAtLeast(t *testing.T) {
	s := TruthTable{Meal: 30, Full: 100, Reach: 5}.NewSurvival(0.05, []int{150})
	if got, want := s.deadAtLeast(60, 50), s.deadIn(144, 50); got != want {
		t.Fatalf("read %v, want the kept window's %v", got, want)
	}
}
