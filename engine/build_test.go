package engine

import (
	"math"
	"testing"
)

// A build that spells out the config's own values is the zero build: the
// world runs the same, draw for draw.
func TestBuildOfTheConfigIsTheZeroBuild(t *testing.T) {
	cfg := testConfig(6)
	cfg.Allot = false
	a, err := NewWorld(cfg, testMap())
	if err != nil {
		t.Fatal(err)
	}
	b, err := NewWorld(cfg, testMap())
	if err != nil {
		t.Fatal(err)
	}
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
	w.food.born = append(w.food.born, w.tick)
	tile := w.tileOf(b.X, b.Y)
	if w.foodOn(tile) < 0 {
		f := Food{X: tile % w.m.Width, Y: tile / w.m.Width}
		w.food.foods = append(w.food.foods[:len(w.food.foods)-1], f)
		w.food.foodAt[tile] = int32(len(w.food.foods))
		w.food.onGround[w.m.Region[tile]]++
	} else {
		w.food.foods = w.food.foods[:len(w.food.foods)-1]
		w.food.born = w.food.born[:len(w.food.born)-1]
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

// A child's level of the share is one of its parents', or one level from
// it; it moves away from both about as often as AllotMutation says; and
// the budget account closes every tick.
func TestChildInheritsTheShare(t *testing.T) {
	births, moved := 0, 0
	mutation := 0.0
	for seed := int64(1); seed <= 4; seed++ {
		w := newTestWorld(t, seed)
		mutation = w.cfg.AllotMutation
		level := map[int64]int{}
		for _, b := range w.Bodies() {
			level[b.ID] = b.Level
		}
		for i := 0; i < 4000; i++ {
			w.Step()
			if l := w.BudgetLedger(); !l.Balanced() {
				t.Fatalf("tick %d: budget ledger does not close: %+v", w.Tick(), l)
			}
			for _, b := range w.Bodies() {
				if _, ok := level[b.ID]; ok {
					continue
				}
				level[b.ID] = b.Level
				p, q := level[b.Parents[0]], level[b.Parents[1]]
				near := func(x int) bool { return b.Level == x || b.Level == x-1 || b.Level == x+1 }
				if !near(p) && !near(q) {
					t.Fatalf("child %d at level %d of parents at %d and %d", b.ID, b.Level, p, q)
				}
				births++
				if b.Level != p && b.Level != q {
					moved++
				}
			}
		}
	}
	if births < 100 {
		t.Fatalf("only %d births", births)
	}
	if f := float64(moved) / float64(births); f < 0.02 || f > 0.2 {
		t.Fatalf("%d of %d children moved away from both parents' levels (%.3f), mutation %v", moved, births, f, mutation)
	}
}

// A body's speed and most energy follow its age: ChildAbility at birth,
// rising to full when it comes of age, full through adulthood, falling
// from OldAge back to ChildAbility at Lifespan, in steps of AgeStep; the
// first bodies, adults from the start, are full from the start.
func TestAgeFactor(t *testing.T) {
	w := newTestWorld(t, 1)
	w.cfg.ChildAbility, w.cfg.OldAge, w.cfg.Lifespan, w.cfg.AgeStep, w.cfg.MatureAge = 0.5, 3000, 4000, 0.05, 400
	w.tick = 10000
	child := Body{Born: 10000, Mature: 10400}
	for _, c := range []struct {
		age  int64
		want float64
	}{{0, 0.5}, {200, 0.75}, {399, 0.95}, {400, 1}, {2999, 1}, {3500, 0.75}, {3999, 0.5}} {
		b := child
		b.Born, b.Mature = w.tick-c.age, w.tick-c.age+400
		if got := w.ageFactor(&b); math.Abs(got-c.want) > 1e-9 {
			t.Errorf("age %d: factor %v, want %v", c.age, got, c.want)
		}
	}
	first := Body{Born: 0, Mature: 0}
	w.tick = 10
	if got := w.ageFactor(&first); got != 1 {
		t.Errorf("a first body at tick 10: factor %v, want 1", got)
	}
	if got, want := w.speedOf(&child), w.cfg.Speed*w.ageFactor(&child); got != want {
		t.Errorf("speed %v, want %v", got, want)
	}
	w.cfg.ChildAbility = 1
	if got := w.ageFactor(&child); got != 1 {
		t.Errorf("without the curve: factor %v", got)
	}
}
