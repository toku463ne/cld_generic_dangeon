package engine

import (
	"math"
	"testing"
)

// A build that spells out the config's own values is the zero build: the
// world runs the same, draw for draw.
func TestBuildOfTheConfigIsTheZeroBuild(t *testing.T) {
	a, b := newTestWorld(t, 6), newTestWorld(t, 6)
	cfg := b.Config()
	for _, x := range b.Bodies() {
		b.SetBuild(x.ID, Build{Speed: cfg.Speed, EnergyMax: cfg.EnergyMax, EnergyBurn: cfg.EnergyBurn})
	}
	run(a, 800)
	run(b, 800)
	as, bs := a.Bodies(), b.Bodies()
	if len(as) != len(bs) || a.Draws() != b.Draws() {
		t.Fatalf("%d bodies/%d draws against %d/%d", len(as), a.Draws(), len(bs), b.Draws())
	}
	for i := range as {
		if as[i].ID != bs[i].ID || as[i].X != bs[i].X || as[i].Y != bs[i].Y || as[i].Energy != bs[i].Energy {
			t.Fatalf("body %d differs: %+v against %+v", as[i].ID, as[i], bs[i])
		}
	}
}

// A body's own build is what moves, burns and caps it.
func TestOwnBuild(t *testing.T) {
	w := newTestWorld(t, 1)
	id := w.bodies[0].ID
	if !w.SetBuild(id, Build{Speed: 0.5, EnergyMax: 60, EnergyBurn: 0.2}) {
		t.Fatal("body not found")
	}
	b := &w.bodies[0]
	if b.Energy != 60 {
		t.Fatalf("energy %v, want held to the new most 60", b.Energy)
	}
	x := b.X
	w.act(0, b, Action{Kind: ActMove, Dir: 0})
	if b.X-x != 0.5 {
		t.Fatalf("moved %v, want 0.5", b.X-x)
	}
	b.Energy = 50
	w.food.foods = append(w.food.foods, Food{})
	tile := w.tileOf(b.X, b.Y)
	if w.foodOn(tile) < 0 {
		f := Food{X: tile % w.m.Width, Y: tile / w.m.Width}
		w.food.foods = append(w.food.foods[:len(w.food.foods)-1], f)
		w.food.foodAt[tile] = int32(len(w.food.foods))
		w.food.onGround[w.m.Region[tile]]++
	} else {
		w.food.foods = w.food.foods[:len(w.food.foods)-1]
	}
	w.act(0, b, Action{Kind: ActEat})
	if b.Energy != 60 {
		t.Fatalf("energy after eating %v, want capped at 60", b.Energy)
	}
	before := b.Energy
	w.Step()
	for _, o := range w.Bodies() {
		if o.ID == id && math.Abs(before-o.Energy-0.2) > 1e-9 {
			t.Fatalf("burned %v in a tick, want 0.2", before-o.Energy)
		}
	}
}
