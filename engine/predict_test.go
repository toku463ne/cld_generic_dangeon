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
		w.act(-1, &b, a)
	}
	// Column 7 is water: east is no longer an option. Straight back west
	// would walk the same row again, so the body turns north-west or
	// south-west, drawing once between them.
	b.X = 6.9
	draws := w.Draws()
	if a := w.decide(&b); (a != Action{Kind: ActMove, Dir: 3} && a != Action{Kind: ActMove, Dir: 5}) || w.Draws() != draws+1 {
		t.Fatalf("at the water: took %v with %d draws, want south-west or north-west with one", a, w.Draws()-draws)
	}
	// Without TurnOffReverse (stage 1-2q) it bounces straight back west
	// without a draw.
	w.cfg.TurnOffReverse = false
	draws = w.Draws()
	if a := w.decide(&b); a != (Action{Kind: ActMove, Dir: 4}) || w.Draws() != draws {
		t.Fatalf("at the water, turning off the reverse off: took %v with %d draws, want west with none", a, w.Draws()-draws)
	}
}

// A bounce that would send the body straight back turns 45 degrees off the
// reverse to whichever side is among the best, and keeps the reverse only
// where neither side is.
func TestTurnOff(t *testing.T) {
	w, err := NewWorld(testConfig(1), testMap())
	if err != nil {
		t.Fatal(err)
	}
	only := func(in ...int) func(int) bool {
		return func(d int) bool {
			for _, o := range in {
				if d == o {
					return true
				}
			}
			return false
		}
	}
	for _, c := range []struct {
		heading, back int
		in            []int
		want          []int
	}{
		{0, 4, []int{3, 4, 5}, []int{3, 5}}, // east into a wall: south-west or north-west
		{6, 2, []int{2, 3}, []int{3}},       // north, only south-west open beside back
		{6, 2, []int{1, 2}, []int{1}},       // north, only south-east
		{7, 3, []int{2, 3, 4}, []int{2, 4}}, // north-east into a corner: south or west
		{0, 4, []int{4}, []int{4}},          // neither side: straight back
	} {
		got := w.turnOff(c.heading, c.back, only(c.in...))
		ok := false
		for _, x := range c.want {
			ok = ok || got == x
		}
		if !ok {
			t.Fatalf("heading %d back %d with %v best: turned to %d, want one of %v", c.heading, c.back, c.in, got, c.want)
		}
	}
}

// A heading bounces off the axis that makes it worse: a diagonal meeting a
// wall keeps its run along the wall and turns back from it, one meeting a
// corner turns back on both, and a straight one turns back.
func TestBounce(t *testing.T) {
	// best allows every direction except those in out.
	best := func(out ...int) func(int) bool {
		return func(d int) bool {
			for _, o := range out {
				if d == o {
					return false
				}
			}
			return true
		}
	}
	for _, c := range []struct {
		heading int
		out     []int
		want    int
	}{
		{1, nil, 1},                  // nothing in the way: unchanged
		{-1, []int{0}, -1},           // no heading yet
		{7, []int{5, 6, 7}, 1},       // north-east into a wall above: south-east
		{3, []int{3, 4, 5}, 1},       // south-west into a wall on the left: south-east
		{5, []int{3, 4, 5, 6, 7}, 1}, // north-west into the corner: south-east
		{0, []int{7, 0, 1}, 4},       // east into a wall: west
		{7, []int{7, 0, 1}, 5},       // north-east into a wall on the right: north-west
		{7, []int{7}, 3},             // only the diagonal is out: back
	} {
		if got := bounce(c.heading, best(c.out...)); got != c.want {
			t.Fatalf("heading %d with %v out: bounced to %d, want %d", c.heading, c.out, got, c.want)
		}
	}
}

// A body in rich ground meeting the edge of a poorer region bounces off it
// rather than running along it: moves into the poorer region carry more
// risk, so they are not among the best.
func TestBouncesOffPoorerRegion(t *testing.T) {
	cfg := testConfig(1)
	cfg.Bodies, cfg.FoodCap = 0, 0
	m := NewMap(12, 9)
	m.RegionFood = []float64{1, 1}
	for y := 0; y < m.Height; y++ {
		for x := 6; x < m.Width; x++ {
			m.SetRegion(x, y, 1)
		}
	}
	w, err := NewWorld(cfg, m)
	if err != nil {
		t.Fatal(err)
	}
	// Food on the left only, away from the body's sight.
	for _, p := range [][2]int{{0, 0}, {0, 8}, {2, 8}} {
		w.food.foods = append(w.food.foods, Food{X: p[0], Y: p[1]})
		w.food.foodAt[m.index(p[0], p[1])] = int32(len(w.food.foods))
		w.food.onGround[0]++
	}
	w.refreshRegion(0)
	b := Body{X: 5.9, Y: 4.5, Energy: 50, Heading: 7} // north-east, at the edge of region 1
	if a := w.decide(&b); a != (Action{Kind: ActMove, Dir: 5}) {
		t.Fatalf("took %v, want north-west", a)
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
	// Value reads the config's build, so every body here has it.
	cfg := testConfig(4)
	cfg.Allot = false
	w, err := NewWorld(cfg, testMap())
	if err != nil {
		t.Fatal(err)
	}
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
		// The choice is made on the score: the risk less ChildWorth for
		// each child.
		best := math.Inf(1)
		for j := range v.Options {
			if v.Options[j] != want.Options[j] || v.Risk[0][j] != want.Risk[0][j] || v.Child[j] != want.Child[j] {
				t.Fatalf("tick %d body %d option %d: traced %v %v %v, Value gives %v %v %v",
					w.Tick(), b.ID, j, v.Options[j], v.Risk[0][j], v.Child[j], want.Options[j], want.Risk[0][j], want.Child[j])
			}
			if s := v.Risk[0][j] - w.cfg.ChildWorth*v.Child[j]; v.Score[j] != s {
				t.Fatalf("tick %d body %d option %d: score %v, risk and child give %v", w.Tick(), b.ID, j, v.Score[j], s)
			}
			best = math.Min(best, v.Score[j])
		}
		taken := false
		for j := range v.Options {
			taken = taken || (v.Options[j] == a && v.Score[j] == best)
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
