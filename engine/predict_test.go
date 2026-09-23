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

// Eating is worth more than waiting whenever the window binds, and worth
// the same once the energy outlasts the window.
func TestEatingAgainstTheWindow(t *testing.T) {
	w := newTestWorld(t, 2)
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
				e = v.Worth[0][j]
			case ActWait:
				wait = v.Worth[0][j]
			}
		}
		return e, wait
	}
	if e, wait := eat(10); e <= wait {
		t.Fatalf("hungry: eat %v, wait %v", e, wait)
	}
	if e, wait := eat(90); e != wait {
		t.Fatalf("full: eat %v, wait %v, want equal", e, wait)
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
