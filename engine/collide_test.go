package engine

import (
	"math"
	"testing"
)

// A step into a tile another body stands on is not an option; a step
// within the body's own tile is, whoever shares that tile.
func TestOccupiedTileIsNoOption(t *testing.T) {
	// Body 0 at the east edge of tile (2,2), body 1 on tile (3,2).
	w := pairWorld(t, Body{ID: 0, X: 2.9, Y: 2.5, Energy: 100}, Body{ID: 1, X: 3.5, Y: 2.5, Energy: 100})
	opts := w.possibleActions(nil, &w.bodies[0])
	for _, o := range opts {
		if o == (Action{Kind: ActMove, Dir: 0}) {
			t.Fatal("offered a step east into the occupied tile")
		}
	}
	// Back at the tile's centre, the step east stays in its own tile.
	w.bodies[0].X = 2.5
	w.buildGrid()
	east := false
	for _, o := range w.possibleActions(nil, &w.bodies[0]) {
		east = east || o == (Action{Kind: ActMove, Dir: 0})
	}
	if !east {
		t.Fatal("a step within the body's own tile was not offered")
	}
	w.cfg.Collide = false
	east = false
	w.bodies[0].X = 2.9
	for _, o := range w.possibleActions(nil, &w.bodies[0]) {
		east = east || o == (Action{Kind: ActMove, Dir: 0})
	}
	if !east {
		t.Fatal("without Collide the step east was not offered")
	}
}

// A body whose intent steps into an occupied tile decides again.
func TestBlockedIntentDecides(t *testing.T) {
	w := pairWorld(t, Body{ID: 0, X: 2.9, Y: 2.5, Energy: 100}, Body{ID: 1, X: 3.5, Y: 2.5, Energy: 100})
	w.tick = 10
	b := &w.bodies[0]
	b.Decided, b.Intent = w.tick, Action{Kind: ActMove, Dir: 0}
	b.Under, b.Saw = w.perceive(b)
	b.Mates = w.sawMates(b)
	if got := w.trigger(b, b.Under, b.Saw, b.Mates); got != TriggerBlocked {
		t.Fatalf("trigger %s, want blocked", TriggerNames[got])
	}
}

// Over a run, bodies share a tile far less often with collisions than
// without: only a child born beside its parent, or bodies placed together,
// start out sharing.
func TestCollisionsSpreadBodies(t *testing.T) {
	shared := func(collide bool) float64 {
		cfg := testConfig(7)
		cfg.Collide = collide
		w, err := NewWorld(cfg, testMap())
		if err != nil {
			t.Fatal(err)
		}
		n, k := 0.0, 0.0
		for i := 0; i < 2000; i++ {
			w.Step()
			seen := map[int]int{}
			for _, b := range w.bodies {
				seen[w.tileOf(b.X, b.Y)]++
			}
			for _, c := range seen {
				if c > 1 {
					k += float64(c)
				}
			}
			n += float64(len(w.bodies))
		}
		return k / math.Max(n, 1)
	}
	with, without := shared(true), shared(false)
	if with >= without/2 {
		t.Fatalf("share of bodies on a shared tile: %.3f with collisions, %.3f without", with, without)
	}
}
