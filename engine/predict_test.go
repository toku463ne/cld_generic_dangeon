package engine

import (
	"math"
	"testing"
)

func testTable() TruthTable { return TruthTable{Meal: 30, Full: 100} }

// With no food to meet, a body is alive at the end of a window exactly when
// its energy lasts longer than the window: the deadline.
func TestSurvivalWithoutFoodIsTheDeadline(t *testing.T) {
	s := testTable().NewSurvival(0, []int{1, 10, 50})
	for i, w := range s.Windows {
		for n := 0; n <= 100; n++ {
			want := 0.0
			if n > w-1 {
				want = 1
			}
			if got := s.Alive(i, n); got != want {
				t.Fatalf("window %d, energy %d: %v, want %v", w, n, got, want)
			}
		}
	}
}

// Meeting food can only help, more energy can only help, and a body whose
// energy outlasts the window is sure to be alive whatever it meets.
func TestSurvivalIsMonotone(t *testing.T) {
	windows := []int{40, 80}
	low := testTable().NewSurvival(0.01, windows)
	high := testTable().NewSurvival(0.05, windows)
	for i, w := range windows {
		for n := 1; n <= 100; n++ {
			if high.Alive(i, n) < low.Alive(i, n)-1e-12 {
				t.Fatalf("window %d, energy %d: more food gave less", w, n)
			}
			if n > 1 && low.Alive(i, n) < low.Alive(i, n-1)-1e-12 {
				t.Fatalf("window %d, energy %d: more energy gave less", w, n)
			}
			if n >= w && math.Abs(low.Alive(i, n)-1) > 1e-12 {
				t.Fatalf("window %d, energy %d: %v, want 1", w, n, low.Alive(i, n))
			}
		}
	}
}

// One step by hand: from 2 ticks of energy, alive after 2 more ticks only if
// food is met in the first of them (then 2+30-1 remain) or in the second.
func TestSurvivalByHand(t *testing.T) {
	const q = 0.2
	s := testTable().NewSurvival(q, []int{3})
	// Two steps from n=2: step 1 meets (q) -> 31, sure to last; misses ->
	// 1, then step 2 must meet (q).
	want := q + (1-q)*q
	if got := s.Alive(0, 2); math.Abs(got-want) > 1e-12 {
		t.Fatalf("got %v, want %v", got, want)
	}
}

// The table reads the food on the ground: a region's chance of meeting food
// is its food over its land, times the tiles entered per tick.
func TestTruthTableReadsTheGround(t *testing.T) {
	w := newTestWorld(t, 5)
	tab := w.TruthTable()
	if tab.Full != 1000 || tab.Meal != 300 {
		t.Fatalf("Full %d Meal %d, want 1000 and 300", tab.Full, tab.Meal)
	}
	count := make([]float64, len(tab.Meet))
	for _, f := range w.Foods() {
		count[w.m.RegionAt(f.X, f.Y)]++
	}
	for r, q := range tab.Meet {
		want := count[r] / float64(len(w.food.regionLand[r])) * w.cfg.Speed
		if math.Abs(q-want) > 1e-12 {
			t.Fatalf("region %d: meet %v, want %v", r, q, want)
		}
	}
}

// Valuing options draws nothing and changes nothing: the world runs on as if
// it had not been asked.
func TestValueChangesNothing(t *testing.T) {
	a, b := newTestWorld(t, 8), newTestWorld(t, 8)
	for i := 0; i < 200; i++ {
		tab := a.TruthTable()
		surv := make([]Survival, len(tab.Meet))
		for r, q := range tab.Meet {
			surv[r] = tab.NewSurvival(q, []int{50, 300})
		}
		for _, body := range a.Bodies() {
			a.Value(tab, surv, body)
		}
		a.Step()
		b.Step()
	}
	if a.Fingerprint() != b.Fingerprint() {
		t.Fatal("valuing options changed the world")
	}
}

// Without sight, eating carries less risk than waiting whenever the window
// binds, and the same once the energy outlasts the window. (With sight,
// waiting on the unit and eating it next tick is a plan too.)
func TestEatingAgainstTheWindow(t *testing.T) {
	cfg := testConfig(2)
	cfg.Sight = -1
	w, err := NewWorld(cfg, testMap())
	if err != nil {
		t.Fatal(err)
	}
	tab := w.TruthTable()
	surv := make([]Survival, len(tab.Meet))
	for r := range tab.Meet {
		surv[r] = tab.NewSurvival(0, []int{200})
	}
	f := w.Foods()[0]
	eat := func(energy float64) (float64, float64) {
		v := w.Value(tab, surv, Body{X: float64(f.X) + 0.5, Y: float64(f.Y) + 0.5, Energy: energy})
		var e, wait float64
		for j, a := range v.Options {
			switch a.Kind {
			case ActEat:
				e = v.Risk[0][j]
			case ActWait:
				wait = v.Risk[0][j]
			}
		}
		return e, wait
	}
	if e, wait := eat(10); e >= wait {
		t.Fatalf("hungry: eat %v, wait %v", e, wait)
	}
	if e, wait := eat(90); e != wait {
		t.Fatalf("full: eat %v, wait %v, want equal", e, wait)
	}
}

// Where food is plentiful, the chance of living on rounds to 1 for eating
// and for waiting alike; the chance of dying keeps them apart.
func TestPlentyKeepsEatingApart(t *testing.T) {
	s := TruthTable{Meal: 300, Full: 1000}.NewSurvival(0.1, []int{1000})
	fed, unfed := 799, 499
	if s.Alive(0, fed) != 1 || s.Alive(0, unfed) != 1 {
		t.Skipf("the chances of living on no longer round to 1 (%v, %v); the case is not shown", s.Alive(0, fed), s.Alive(0, unfed))
	}
	if !(s.Dead(0, fed) < s.Dead(0, unfed)) || s.Dead(0, fed) <= 0 {
		t.Fatalf("dead fed %v, unfed %v: want 0 < fed < unfed", s.Dead(0, fed), s.Dead(0, unfed))
	}
}

// A body that sees a unit a tile away and cannot last the window without
// it walks towards it: the moves that bring it nearer carry the least risk,
// and the plan the trace shows is that unit.
func TestWalksToFoodInSight(t *testing.T) {
	cfg := testConfig(1)
	cfg.Bodies = 0
	cfg.FoodCap = 0
	m := NewMap(9, 9)
	m.RegionFood = []float64{1}
	w, err := NewWorld(cfg, m)
	if err != nil {
		t.Fatal(err)
	}
	w.food.foods = append(w.food.foods, Food{X: 5, Y: 4})
	w.food.foodAt[m.index(5, 4)] = 1
	w.food.onGround[0] = 1
	w.refreshRegion(0)
	b := Body{X: 4.5, Y: 4.5, Energy: 10}
	tab := w.TruthTable()
	surv := []Survival{tab.NewSurvival(tab.Meet[0], []int{cfg.Window})}
	v := w.Value(tab, surv, b)
	if len(v.Seen) != 1 || v.Seen[0] != (Food{X: 5, Y: 4}) {
		t.Fatalf("seen %v, want the unit at (5,4)", v.Seen)
	}
	best := math.Inf(1)
	for _, r := range v.Risk[0] {
		best = math.Min(best, r)
	}
	for j, a := range v.Options {
		east := a.Kind == ActMove && a.Dir == 0
		if east != (v.Risk[0][j] == best) {
			t.Fatalf("option %v: risk %v, best %v; want the move east alone to be best", a, v.Risk[0][j], best)
		}
		if a.Kind == ActMove && v.Plan[j] != 0 {
			t.Fatalf("option %v plans %d, want the unit in sight", a, v.Plan[j])
		}
	}
	for i := 0; i < 10; i++ {
		if a := w.decide(&b); a.Kind != ActMove || a.Dir != 0 {
			t.Fatalf("draw %d: took %v, want the move east", i, a)
		}
	}
}

// Where the options of least risk include the way the body last moved, it
// keeps moving that way and draws nothing; where they do not, it draws.
func TestKeepsHeading(t *testing.T) {
	cfg := testConfig(1)
	cfg.Bodies, cfg.FoodCap = 0, 0
	w, err := NewWorld(cfg, testMap())
	if err != nil {
		t.Fatal(err)
	}
	b := Body{X: 1.5, Y: 2.5, Energy: 50, Heading: 0}
	for i := 0; i < 16; i++ {
		draws := w.Draws()
		a := w.decide(&b)
		if a != (Action{Kind: ActMove, Dir: 0}) || w.Draws() != draws {
			t.Fatalf("step %d at (%v,%v): took %v with %d draws, want east with none", i, b.X, b.Y, a, w.Draws()-draws)
		}
		w.act(&b, a)
	}
	// Column 7 is water: east is no longer an option, and the heading
	// bounces back west without a draw.
	b.X = 6.9
	draws := w.Draws()
	if a := w.decide(&b); a != (Action{Kind: ActMove, Dir: 4}) || w.Draws() != draws {
		t.Fatalf("at the water: took %v with %d draws, want west with none", a, w.Draws()-draws)
	}
}

// A heading bounces off the axis that stops it: a diagonal meeting a wall
// keeps its run along the wall and turns back from it, and one meeting a
// corner turns back on both.
func TestBounce(t *testing.T) {
	cfg := testConfig(1)
	cfg.Bodies, cfg.FoodCap = 0, 0
	w, err := NewWorld(cfg, testMap())
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		x, y    float64
		heading int
		want    int
	}{
		{3.5, 4.5, 1, 1}, // open ground: unchanged
		{3.5, 0.1, 7, 1}, // north-east into the top edge: south-east
		{0.1, 4.5, 3, 1}, // south-west into the left edge: south-east
		{0.1, 0.1, 5, 1}, // north-west into the corner: south-east
		{6.9, 4.5, 0, 4}, // east into the water: west
		{6.9, 4.5, 7, 5}, // north-east into the water: north-west
		{3.5, 8.9, 2, 6}, // south into the bottom edge: north
	} {
		b := Body{X: c.x, Y: c.y, Heading: c.heading}
		if got := w.bounce(&b); got != c.want {
			t.Fatalf("heading %d at (%v,%v): bounced to %d, want %d", c.heading, c.x, c.y, got, c.want)
		}
	}
}

// The walk is the octile distance: straight and diagonal moves both cover
// Speed of it per tick.
func TestWalkTicks(t *testing.T) {
	for _, c := range []struct {
		dx, dy, speed float64
		want          int
	}{
		{0, 0, 0.25, 0},
		{0.5, 0, 0.25, 2},
		{0.3, 0, 0.25, 2},
		{1, 1, 0.25, 6}, // sqrt2 / 0.25 = 5.66
		{1, 0.5, 0.25, 5},
	} {
		if got := walkTicks(c.dx, c.dy, c.speed); got != c.want {
			t.Fatalf("walkTicks(%v,%v,%v) = %d, want %d", c.dx, c.dy, c.speed, got, c.want)
		}
	}
}

// Sight sees the square around the body's tile and nothing past it.
func TestInSight(t *testing.T) {
	cfg := testConfig(1)
	cfg.Bodies = 0
	w, err := NewWorld(cfg, testMap())
	if err != nil {
		t.Fatal(err)
	}
	for _, sight := range []int{-1, 0, 1, 2} {
		w.cfg.Sight = sight
		for _, f := range w.Foods() {
			b := Body{X: float64(f.X) + 0.5, Y: float64(f.Y) + 0.5}
			for _, g := range w.Foods() {
				want := sight >= 0 && max(abs(g.X-f.X), abs(g.Y-f.Y)) <= sight
				got := false
				for _, s := range w.inSight(nil, &b) {
					got = got || s == g
				}
				if got != want {
					t.Fatalf("sight %d from %v: sees %v %v, want %v", sight, f, g, got, want)
				}
			}
		}
	}
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// The trace is handed the numbers the choice was made from: they are the
// ones Value gives for the same body at the same moment, and the action taken
// is one of the options of the least risk.
func TestTraceIsTheChoice(t *testing.T) {
	w := newTestWorld(t, 4)
	decisions := 0
	// Tables are built here from TruthTable, not taken from the world's
	// cache; they are kept by chance of meeting only to keep the test fast.
	built := map[float64]Survival{}
	w.SetTrace(func(b Body, v Valuation, a Action) {
		tab := w.TruthTable()
		surv := make([]Survival, len(tab.Meet))
		for r, q := range tab.Meet {
			if _, ok := built[q]; !ok {
				built[q] = tab.NewSurvival(q, []int{w.cfg.Window})
			}
			surv[r] = built[q]
		}
		want := w.Value(tab, surv, b)
		if len(want.Seen) != len(v.Seen) || len(want.Plan) != len(v.Plan) {
			t.Fatalf("tick %d body %d: traced %v/%v, Value gives %v/%v", w.Tick(), b.ID, v.Seen, v.Plan, want.Seen, want.Plan)
		}
		for j := range v.Plan {
			if v.Plan[j] != want.Plan[j] || v.Arrive[j] != want.Arrive[j] {
				t.Fatalf("tick %d body %d option %d: traced plan %d/%d, Value gives %d/%d", w.Tick(), b.ID, j, v.Plan[j], v.Arrive[j], want.Plan[j], want.Arrive[j])
			}
		}
		if len(want.Options) != len(v.Options) {
			t.Fatalf("tick %d body %d: %d options traced, Value gives %d", w.Tick(), b.ID, len(v.Options), len(want.Options))
		}
		best := math.Inf(1)
		for j := range v.Options {
			if v.Options[j] != want.Options[j] || v.Risk[0][j] != want.Risk[0][j] {
				t.Fatalf("tick %d body %d option %d: traced %v %v, Value gives %v %v",
					w.Tick(), b.ID, j, v.Options[j], v.Risk[0][j], want.Options[j], want.Risk[0][j])
			}
			best = math.Min(best, v.Risk[0][j])
		}
		taken := false
		for j := range v.Options {
			taken = taken || (v.Options[j] == a && v.Risk[0][j] == best)
		}
		if !taken {
			t.Fatalf("tick %d body %d: took %v, not an option of the least risk %v", w.Tick(), b.ID, a, best)
		}
		decisions++
	})
	run(w, 300)
	if decisions == 0 {
		t.Fatal("no decision was traced")
	}
}

// Tracing draws nothing and changes nothing.
func TestTraceChangesNothing(t *testing.T) {
	a, b := newTestWorld(t, 9), newTestWorld(t, 9)
	a.SetTrace(func(Body, Valuation, Action) {})
	run(a, 500)
	run(b, 500)
	if a.Fingerprint() != b.Fingerprint() {
		t.Fatal("tracing changed the world")
	}
}

// A body with food underfoot and energy that does not outlast the window
// eats: eating is the one best option.
func TestHungryBodyEats(t *testing.T) {
	cfg := testConfig(1)
	cfg.Bodies = 0
	w, err := NewWorld(cfg, testMap())
	if err != nil {
		t.Fatal(err)
	}
	f := w.Foods()[0]
	b := Body{X: float64(f.X) + 0.5, Y: float64(f.Y) + 0.5, Energy: 20}
	for i := 0; i < 20; i++ {
		if a := w.decide(&b); a.Kind != ActEat {
			t.Fatalf("draw %d: took %v with food underfoot", i, a)
		}
	}
}

// BenchmarkDecide is one decision of a body in a world under way, the
// survival tables already cached.
func BenchmarkDecide(b *testing.B) {
	w := newTestWorld(b, 1)
	run(w, 200)
	body := w.bodies[0]
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w.decide(&body)
	}
}

// BenchmarkSurvival1000 is the table a 1-2 world builds per region per tick
// at the window the split count chose.
func BenchmarkSurvival1000(b *testing.B) {
	tab := TruthTable{Meal: 300, Full: 1000}
	for i := 0; i < b.N; i++ {
		tab.NewSurvival(0.02, []int{1000})
	}
}
