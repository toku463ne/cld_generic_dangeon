package engine

import (
	"math"
	"testing"
)

// A body that has seen nothing reads its inborn expectations.
func TestNewbornReadsPriors(t *testing.T) {
	w := newTestWorld(t, 1)
	b := &Body{X: 2.5, Y: 2.5}
	if got := w.regionRate(b, 0); got != w.cfg.PriorFood {
		t.Fatalf("region %v, want the prior %v", got, w.cfg.PriorFood)
	}
	if got := w.pathRate(b, 0); got != w.cfg.PriorFood {
		t.Fatalf("path %v, want the prior %v", got, w.cfg.PriorFood)
	}
	if got := w.childRate(b); got != w.cfg.PriorChild {
		t.Fatalf("child %v, want the prior %v", got, w.cfg.PriorChild)
	}
}

// Stepping onto a tile is one tile of its region's evidence, food or not;
// stepping back onto a tile left lately is one of the path's too; and the
// estimates move from the priors toward what was seen.
func TestSteppingTeaches(t *testing.T) {
	cfg := testConfig(1)
	cfg.Bodies, cfg.FoodCap = 0, 0
	w, err := NewWorld(cfg, testMap())
	if err != nil {
		t.Fatal(err)
	}
	w.food.foods = append(w.food.foods, Food{X: 3, Y: 2})
	w.food.foodAt[w.m.index(3, 2)] = 1
	b := &Body{Goal: -1}
	a, c := w.m.index(2, 2), w.m.index(3, 2)
	w.stepped(b, a, c) // onto food
	w.tick++
	w.stepped(b, c, a) // back onto the tile it left: no food, and its path
	r := b.Memory.Regions[0]
	if r.N != 2 || r.K != 1 || b.Memory.Path.N != 1 || b.Memory.Path.K != 0 {
		t.Fatalf("region %+v path %+v, want 2/1 and 1/0", r, b.Memory.Path)
	}
	// A step of a walk to food in sight is no evidence.
	b.Goal = c
	w.tick++
	w.stepped(b, a, c)
	if r := b.Memory.Regions[0]; r.N != 2 {
		t.Fatalf("a step of a walk to food counted: %+v", r)
	}
	b.Goal = -1
	// Far more evidence than the weights pulls the estimate to the rate seen.
	b.Memory.Regions[0] = Tally{N: 1e6, K: 3e5}
	if got := w.regionRate(b, 0); math.Abs(got-0.3) > 1e-3 {
		t.Fatalf("region estimate %v after 1e6 tiles at 0.3", got)
	}
	// Leaving a tile beyond PathRecall ticks does not make it a path tile.
	w.tick += int64(cfg.PathRecall) + 1
	if w.walked(b, a) {
		t.Fatal("a tile left too long ago still reads as walked")
	}
}

// With nothing of its path ahead of any move, a learning body values its
// options exactly as it would with no path row.
func TestPathReadingNeutralWithoutWalkedTiles(t *testing.T) {
	w := newTestWorld(t, 5)
	b := w.bodies[0]
	b.Memory = Memory{}
	burn, speed := w.burnOf(&b), w.speedOf(&b)
	meal, full := energyTicks(w.cfg.FoodEnergy, burn), energyTicks(w.maxOf(&b), burn)
	alive := func(r RegionID) Survival { return w.learnedTable(w.regionRate(&b, r), meal, full, speed) }
	var rs rates
	w.ratesOf(&b, &rs)
	var with, without Valuation
	w.valueInto(&with, meal, full, w.pred.windows, alive, w.childRate(&b), &rs, &b)
	w.valueInto(&without, meal, full, w.pred.windows, alive, w.childRate(&b), nil, &b)
	for j := range with.Options {
		if with.Risk[0][j] != without.Risk[0][j] {
			t.Fatalf("option %v: %v with the path row, %v without", with.Options[j], with.Risk[0][j], without.Risk[0][j])
		}
	}
}

// A learning body does not read the food on the ground beyond what it
// sees: two worlds that differ only in food out of its sight value its
// options the same.
func TestLearningReadsNoUnseenFood(t *testing.T) {
	cfg := testConfig(1)
	cfg.Bodies, cfg.FoodCap = 0, 0
	value := func(extra bool) Valuation {
		w, err := NewWorld(cfg, testMap())
		if err != nil {
			t.Fatal(err)
		}
		if extra {
			for x := 0; x < 5; x++ {
				w.food.foods = append(w.food.foods, Food{X: x, Y: 8})
				w.food.foodAt[w.m.index(x, 8)] = int32(len(w.food.foods))
				w.food.onGround[w.m.RegionAt(x, 8)]++
			}
		}
		b := Body{X: 2.5, Y: 2.5, Energy: 30, Heading: -1, Goal: -1, Decided: -1}
		w.decide(&b)
		v := w.valuation
		v.Risk = [][]float64{append([]float64(nil), v.Risk[0]...)}
		return v
	}
	a, b := value(false), value(true)
	for j := range a.Options {
		if a.Risk[0][j] != b.Risk[0][j] {
			t.Fatalf("option %v: %v, and %v with food out of sight", a.Options[j], a.Risk[0][j], b.Risk[0][j])
		}
	}
}

// Every mate a body carries out is an ask, every child it has is counted,
// and the trace shows the belief the choice was made on.
func TestMateRowCounts(t *testing.T) {
	w := newTestWorld(t, 3)
	asks := map[int64]float64{}
	traced := false
	w.SetTrace(func(b Body, v Valuation, a Action) {
		if a.Kind == ActMate {
			asks[b.ID]++
		}
		if v.Belief.Land > 0 {
			traced = true
		}
	})
	run(w, 2000)
	for _, b := range w.Bodies() {
		if b.Memory.Asks < asks[b.ID] {
			t.Fatalf("body %d asked %v times by the trace, remembers %v", b.ID, asks[b.ID], b.Memory.Asks)
		}
		if b.Memory.Kids > b.Memory.Asks+1 {
			t.Fatalf("body %d: %v children from %v asks", b.ID, b.Memory.Kids, b.Memory.Asks)
		}
	}
	if !traced {
		t.Fatal("no belief on the trace")
	}
}
