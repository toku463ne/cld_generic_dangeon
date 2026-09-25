package engine

import (
	"math"
	"testing"
)

func TestBodiesStayOnLand(t *testing.T) {
	w := newTestWorld(t, 6)
	for i := 0; i < 3000; i++ {
		w.Step()
		for _, b := range w.bodies {
			tile := w.tileOf(b.X, b.Y)
			if tile < 0 || w.m.Terrain[tile] != TerrainLand {
				t.Fatalf("tick %d: body %d at (%v,%v) is off land", w.Tick(), b.ID, b.X, b.Y)
			}
		}
	}
}

// With no food, every body starves when its energy runs out. Energy is spent
// by repeated subtraction, so rounding may carry a body one tick past the
// exact figure.
func TestNoFoodStarvesEveryBody(t *testing.T) {
	cfg := testConfig(1)
	cfg.FoodCap = 0
	cfg.Breed = false // no one is born to outlive the count
	w, err := NewWorld(cfg, testMap())
	if err != nil {
		t.Fatal(err)
	}
	last := int(math.Ceil(cfg.EnergyMax / cfg.EnergyBurn))
	for i := 0; i < last-1; i++ {
		w.Step()
	}
	if len(w.bodies) != cfg.Bodies {
		t.Fatalf("tick %d: %d bodies, want all %d still alive", w.Tick(), len(w.bodies), cfg.Bodies)
	}
	w.Step()
	if len(w.bodies) != 0 {
		w.Step()
	}
	if len(w.bodies) != 0 || w.stats.Deaths[CauseStarved] != int64(cfg.Bodies) {
		t.Fatalf("tick %d: %d alive, %d starved; want 0 and %d", w.Tick(), len(w.bodies), w.stats.Deaths[CauseStarved], cfg.Bodies)
	}
	if want := float64(cfg.Bodies*int(w.Tick())) * cfg.EnergyBurn; math.Abs(w.stats.EnergyBurned-want) > 1e-6 {
		t.Fatalf("energy burned %v, want %v", w.stats.EnergyBurned, want)
	}
}

// Eating takes the unit off the tile and gives energy up to the maximum.
func TestEatingRestoresEnergyUpToMax(t *testing.T) {
	cfg := testConfig(1)
	cfg.Bodies = 0
	w, err := NewWorld(cfg, testMap())
	if err != nil {
		t.Fatal(err)
	}
	f := w.food.foods[0]
	b := Body{X: float64(f.X) + 0.5, Y: float64(f.Y) + 0.5, Energy: cfg.EnergyMax - cfg.FoodEnergy/2}
	opts := w.possibleActions(nil, &b)
	found := false
	for _, a := range opts {
		found = found || a.Kind == ActEat
	}
	if !found {
		t.Fatal("eating is not offered on a tile with food")
	}
	w.act(-1, &b, Action{Kind: ActEat})
	if b.Energy != cfg.EnergyMax {
		t.Fatalf("energy after eating %v, want capped at %v", b.Energy, cfg.EnergyMax)
	}
	if w.foodOn(w.m.index(f.X, f.Y)) >= 0 {
		t.Fatal("the unit is still on the tile after being eaten")
	}
	for _, a := range w.possibleActions(nil, &b) {
		if a.Kind == ActEat {
			t.Fatal("eating is offered on a tile with no food")
		}
	}
}

// A move onto water or off the map is not offered.
func TestMovesOntoWaterAreNotOffered(t *testing.T) {
	cfg := testConfig(1)
	cfg.Bodies = 0
	cfg.Speed = 1
	w, err := NewWorld(cfg, testMap())
	if err != nil {
		t.Fatal(err)
	}
	// testMap has water in column 7; (6.5, 4.5) is next to it, and (0.5, 0.5)
	// is in a corner.
	for _, p := range [][2]float64{{6.5, 4.5}, {0.5, 0.5}} {
		b := Body{X: p[0], Y: p[1], Energy: 1}
		for _, a := range w.possibleActions(nil, &b) {
			if a.Kind != ActMove {
				continue
			}
			v := moveDirs[a.Dir]
			tile := w.tileOf(b.X+v[0], b.Y+v[1])
			if tile < 0 || w.m.Terrain[tile] != TerrainLand {
				t.Fatalf("from (%v,%v) a move in direction %d leaves the land", p[0], p[1], a.Dir)
			}
		}
	}
}
