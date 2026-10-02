package engine

import (
	"math"
	"testing"
)

// soloWorld is a world of one adult body with the given energy, and of
// food only the unit on its tile.
func soloWorld(t *testing.T, energy float64) (*World, *Body) {
	t.Helper()
	cfg := testConfig(1)
	cfg.Bodies = 0
	w, err := NewWorld(cfg, testMap())
	if err != nil {
		t.Fatal(err)
	}
	w.bodies = append(w.bodies, Body{ID: 0, X: 2.5, Y: 2.5, Energy: energy, Heading: -1, Goal: -1, Decided: -1, Parents: [2]int64{-1, -1}, Sex: Female})
	w.nextID = 1
	w.buildGrid()
	for len(w.food.foods) > 0 {
		w.eatFood(0)
	}
	b := &w.bodies[0]
	tile := w.tileOf(b.X, b.Y)
	w.food.foods = append(w.food.foods, Food{X: tile % w.m.Width, Y: tile / w.m.Width})
	w.food.born = append(w.food.born, w.tick)
	w.food.foodAt[tile] = int32(len(w.food.foods))
	w.food.appeared++
	w.foodMoved(w.m.Region[tile], +1)
	return w, b
}

// A body picks up the unit underfoot only while it holds fewer than Carry;
// the unit leaves the ground, is neither on the ground nor vacant, and the
// ledger still closes.
func TestPickUp(t *testing.T) {
	w, b := soloWorld(t, 90)
	if !hasKind(w.possibleActions(nil, b), ActPick) {
		t.Fatal("no pick offered with food underfoot and room")
	}
	ground := len(w.food.foods)
	w.act(0, b, Action{Kind: ActPick})
	if b.Held != 1 || len(w.food.foods) != ground-1 || w.foodOn(w.tileOf(b.X, b.Y)) >= 0 {
		t.Fatalf("held %d, ground %d -> %d", b.Held, ground, len(w.food.foods))
	}
	l := w.FoodLedger()
	if !l.Balanced() || l.Held != 1 || l.Picked != 1 {
		t.Fatalf("ledger %+v", l)
	}
	w.cfg.Carry = 1
	tile := w.tileOf(b.X, b.Y)
	w.food.foods = append(w.food.foods, Food{X: tile % w.m.Width, Y: tile / w.m.Width})
	w.food.born = append(w.food.born, w.tick)
	w.food.foodAt[tile] = int32(len(w.food.foods))
	if hasKind(w.possibleActions(nil, b), ActPick) {
		t.Fatal("pick offered with the hold full")
	}
	w.cfg.Carry = 0
	b.Held = 0
	if hasKind(w.possibleActions(nil, b), ActPick) {
		t.Fatal("pick offered without Carry")
	}
}

// A body eats what it holds, spending no turn, as soon as a whole meal fits
// and not before.
func TestEatHeld(t *testing.T) {
	w, b := soloWorld(t, 90)
	w.act(0, b, Action{Kind: ActPick})
	w.eatHeld(b)
	if b.Held != 1 || b.Energy != 90 {
		t.Fatalf("ate with a meal not fitting: held %d energy %v", b.Held, b.Energy)
	}
	b.Energy = w.maxOf(b) - w.cfg.FoodEnergy
	w.eatHeld(b)
	if b.Held != 0 || b.Energy != w.maxOf(b) {
		t.Fatalf("did not eat with a meal fitting: held %d energy %v", b.Held, b.Energy)
	}
	if l := w.FoodLedger(); !l.Balanced() || l.Held != 0 {
		t.Fatalf("ledger %+v", l)
	}
}

// What a dead body held becomes vacant: lost, and owed back like any
// vacancy.
func TestHeldIsLostAtDeath(t *testing.T) {
	w, b := soloWorld(t, 90)
	w.act(0, b, Action{Kind: ActPick})
	b.Energy = 0
	w.removeDead()
	l := w.FoodLedger()
	if !l.Balanced() || l.Held != 0 || l.Lost != 1 {
		t.Fatalf("ledger %+v", l)
	}
}

// Near full, where a meal does not fit and starving in the window is still
// possible, picking the unit up is valued strictly above eating it: held,
// none of it is lost. Where a meal fits the two read the same.
func TestPickIsValued(t *testing.T) {
	for _, c := range []struct {
		energy float64
		better bool
	}{{90, true}, {40, false}} {
		w, b := soloWorld(t, c.energy)
		w.decide(b)
		v := w.valuation
		eat, pick := -1, -1
		for j, o := range v.Options {
			switch o.Kind {
			case ActEat:
				eat = j
			case ActPick:
				pick = j
			}
		}
		if eat < 0 || pick < 0 {
			t.Fatalf("energy %v: options %+v", c.energy, v.Options)
		}
		re, rp := v.Risk[0][eat], v.Risk[0][pick]
		if c.better && !(rp < re) {
			t.Errorf("energy %v: pick %v not below eat %v", c.energy, rp, re)
		}
		if !c.better && math.Abs(rp-re) > 1e-15 {
			t.Errorf("energy %v: pick %v and eat %v differ with a meal fitting", c.energy, rp, re)
		}
	}
}

// Past Full the survival table passes food on the ground by: more energy
// held is never worse, and the entries below Full are as without it.
func TestSurvivalOver(t *testing.T) {
	plain := TruthTable{Meal: 30, Full: 100}.NewSurvival(0.05, []int{150})
	over := TruthTable{Meal: 30, Full: 100, Over: 30}.NewSurvival(0.05, []int{150})
	for n := 1; n <= 100; n++ {
		if plain.Dead(0, n) != over.Dead(0, n) {
			t.Fatalf("n %d: %v with Over, %v without", n, over.Dead(0, n), plain.Dead(0, n))
		}
	}
	for n := 101; n <= 130; n++ {
		if over.Dead(0, n) > over.Dead(0, n-1) {
			t.Fatalf("n %d: more energy reads worse", n)
		}
	}
	if !(over.Dead(0, 130) < over.Dead(0, 100)) {
		t.Fatalf("held energy reads nothing: %v at 130, %v at 100", over.Dead(0, 130), over.Dead(0, 100))
	}
}

func hasKind(opts []Action, k ActionKind) bool {
	for _, o := range opts {
		if o.Kind == k {
			return true
		}
	}
	return false
}
