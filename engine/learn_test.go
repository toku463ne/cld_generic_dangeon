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

// Stepping to a new tile, the tiles that come into view are evidence of
// their region, food or not; those the body left lately are evidence of
// its path too; and the estimates move from the priors toward what was
// seen.
func TestSteppingTeaches(t *testing.T) {
	cfg := testConfig(1)
	cfg.Bodies, cfg.FoodCap = 0, 0
	cfg.EvidenceHalfLife = 0 // counts, not ages
	w, err := NewWorld(cfg, testMap())
	if err != nil {
		t.Fatal(err)
	}
	// Food at (4,2), which comes into view stepping east from (2,2) to (3,2).
	w.food.foods = append(w.food.foods, Food{X: 4, Y: 2})
	w.food.foodAt[w.m.index(4, 2)] = 1
	b := &Body{Goal: -1}
	a, c := w.m.index(2, 2), w.m.index(3, 2)
	w.stepped(b, a, c)
	r := b.Memory.Regions[0]
	if r.N != 3 || r.K != 1 || b.Memory.Path.N != 0 {
		t.Fatalf("stepping east: region %+v path %+v, want 3 tiles with 1 food and no path", r, b.Memory.Path)
	}
	// Back west: the column x=1 comes into view; none of it was walked.
	w.tick++
	w.stepped(b, c, a)
	if r := b.Memory.Regions[0]; r.N != 6 || b.Memory.Path.N != 0 {
		t.Fatalf("stepping back: region %+v path %+v", r, b.Memory.Path)
	}
	// East again: (4,*) comes into view again, and (3,2), left a tick ago,
	// is not new; but (4,2) was never walked. Walk it, then come back.
	w.tick++
	w.stepped(b, a, c)
	w.tick++
	w.stepped(b, c, w.m.index(4, 2))
	w.tick++
	w.stepped(b, w.m.index(4, 2), c)
	w.tick++
	w.stepped(b, c, a)
	w.tick++
	before := b.Memory.Path.N
	w.stepped(b, a, c) // (4,*) comes into view; (4,2) was left 2 ticks ago
	if b.Memory.Path.N != before+1 || b.Memory.Path.K != 1 {
		t.Fatalf("path %+v after seeing a tile it walked, with food, again", b.Memory.Path)
	}
	// Far more evidence than the weights pulls the estimate to the rate seen.
	b.Memory.Regions[0] = Tally{N: 1e6, K: 3e5}
	if got := w.regionRate(b, 0); math.Abs(got-0.3) > 1e-3 {
		t.Fatalf("region estimate %v after 1e6 tiles at 0.3", got)
	}
	// A tile left beyond PathRecall ticks no longer reads as walked.
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

// Valued with its own estimates, a body's options read as its own decision
// reads them.
func TestValueWithOwnEstimates(t *testing.T) {
	w := newTestWorld(t, 7)
	run(w, 800)
	var got Valuation
	var want Valuation
	b := w.bodies[0]
	w.SetTrace(func(tb Body, v Valuation, _ Action) {
		if tb.ID != b.ID || len(want.Options) > 0 {
			return
		}
		want = v
		want.Risk = [][]float64{append([]float64(nil), v.Risk[0]...)}
		want.Options = append([]Action(nil), v.Options...)
		var rs rates
		w.ratesOf(&tb, &rs)
		got = w.ValueWith(tb, rs.region, rs.path)
	})
	for i := 0; i < 400 && len(want.Options) == 0; i++ {
		w.Step()
	}
	if len(want.Options) == 0 {
		t.Skip("the body did not decide")
	}
	for j := range want.Options {
		if got.Options[j] != want.Options[j] || got.Risk[0][j] != want.Risk[0][j] {
			t.Fatalf("option %d: %v %v, the decision read %v %v", j, got.Options[j], got.Risk[0][j], want.Options[j], want.Risk[0][j])
		}
	}
}

// Evidence halves in weight every EvidenceHalfLife ticks, own and heard
// alike, and an estimate reads it as it weighs now.
func TestEvidenceAges(t *testing.T) {
	w := newTestWorld(t, 1)
	hl := int64(w.cfg.EvidenceHalfLife)
	tl := Tally{N: 100, K: 10, Heard: []Heard{{ID: 7, N: 40, K: 4}}, HN: 40, HK: 4, T: w.tick}
	w.tick += hl
	if f := w.Fresh(tl); math.Abs(f.N-50) > 1e-9 || math.Abs(f.K-5) > 1e-9 {
		t.Fatalf("after a half-life: %+v", f)
	}
	w.age(&tl)
	if math.Abs(tl.N-50) > 1e-9 || math.Abs(tl.scale()*tl.heard(7).N-20) > 1e-9 || math.Abs(tl.HN-20) > 1e-9 || tl.T != w.tick {
		t.Fatalf("aged: %+v", tl)
	}
	tl.settle()
	if math.Abs(tl.heard(7).N-20) > 1e-9 || tl.S != 0 || math.Abs(tl.N-50) > 1e-9 {
		t.Fatalf("settled: %+v", tl)
	}
	// Long enough, and an entry is let go when settled.
	w.tick += 20 * hl
	w.age(&tl)
	tl.settle()
	if len(tl.Heard) != 0 || tl.HN != 0 {
		t.Fatalf("an entry worth nothing kept: %+v", tl)
	}
}
