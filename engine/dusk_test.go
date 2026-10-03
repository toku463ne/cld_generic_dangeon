package engine

import (
	"math"
	"testing"
)

// Without learning when dark and light come, the world is stage 7-4: its
// fingerprint is the one pinned then.
func TestDuskblindIsStage74(t *testing.T) {
	const want = uint64(0x1bfef7f1f18d1e4)
	cfg := testConfig(1)
	cfg.DuskRead = false
	w, err := NewWorld(cfg, tempMap())
	if err != nil {
		t.Fatal(err)
	}
	run(w, 1000)
	if got := w.Fingerprint(); got != want {
		t.Fatalf("fingerprint = %#x, want %#x", got, want)
	}
}

// nextCross is the first tick after tick t whose light crosses half the
// way given (down: falls below half).
func nextCross(w *World, t int64, down bool) int64 {
	prev := w.brightnessAt(t)
	for u := t + 1; ; u++ {
		l := w.brightnessAt(u)
		if down && prev >= 0.5 && l < 0.5 || !down && prev < 0.5 && l >= 0.5 {
			return u
		}
		prev = l
	}
}

// A body that watches the light for some days learns, in full light, how
// long until dark and until light within about a band of LightBand; asleep
// it sees nothing and loses what was waiting.
func TestDuskLearn(t *testing.T) {
	w, b := soloWorld(t, 50)
	w.cfg.DuskRead = true
	day := int64(w.cfg.DayTicks)
	look := func(to int64) {
		for w.tick < to {
			w.tick++
			w.seeLight(b)
		}
	}
	look(3 * day)
	for _, at := range []int64{3*day + 60, 3*day + 210, 3*day + 330, 3*day + 700} {
		look(at)
		toDark, toLight, ok := w.duskAhead(b)
		if !ok {
			t.Fatalf("tick %d: nothing learned", at)
		}
		wantDark, wantLight := float64(nextCross(w, at, true)-at), float64(nextCross(w, at, false)-at)
		tol := float64(w.cfg.LightBand)
		if math.Abs(toDark-wantDark) > tol || math.Abs(toLight-wantLight) > tol {
			t.Fatalf("tick %d: learned %.0f to dark and %.0f to light, want about %.0f and %.0f", at, toDark, toLight, wantDark, wantLight)
		}
	}
	b.Asleep = true
	w.tick++
	w.seeLight(b)
	if b.LightKnown {
		t.Fatal("asleep, it still sees the light")
	}
	for _, r := range b.Dusk {
		if r.WaitDark[0] != 0 || r.WaitLit[0] != 0 {
			t.Fatalf("asleep, row %d still waits", r.Key)
		}
	}
	if _, _, ok := w.duskAhead(b); ok {
		t.Fatal("asleep, it reads when dark comes")
	}
}

// A scheduled table that never switches is the plain table of its light,
// and dark at the start of the window is worse than dark at its end for a
// body low on energy.
func TestDuskSchedule(t *testing.T) {
	tt := TruthTable{Meal: 10, Full: 40, Reach: 3}
	const L = 200
	win := []int{L}
	plain := tt.NewSurvival(0.06, win)
	same := tt.newScheduled(0.02, 0.06, L, L, true, win)
	for n := 1; n <= 40; n++ {
		if plain.Dead(0, n) != same.Dead(0, n) {
			t.Fatalf("n %d: never switching reads %v, the lit table %v", n, same.Dead(0, n), plain.Dead(0, n))
		}
	}
	darkFirst := tt.newScheduled(0.02, 0.06, L/2, L, false, win)
	darkLast := tt.newScheduled(0.02, 0.06, L/2, L, true, win)
	if !(darkFirst.Dead(0, 15) > darkLast.Dead(0, 15)) {
		t.Fatalf("dark first %v, dark last %v: dark first should be worse", darkFirst.Dead(0, 15), darkLast.Dead(0, 15))
	}
}

// In full light, a body that has learned dark comes soon reads keeping on
// the move as riskier than one reading the light as staying.
func TestDuskRead(t *testing.T) {
	w, b := soloWorld(t, 30)
	w.cfg.DuskRead, w.cfg.SightRead = true, true
	w.light, w.lightFor = 1, w.tick+1
	b.LightKnown, b.LightSeen, b.LightSince = true, 1, -1
	b.LightKey = w.lightKey(1, 0, -1)
	b.Dusk = []DuskRow{{Key: b.LightKey, ToDark: [2]float64{100, 1}, ToLight: [2]float64{600, 1}}}
	risk := func() float64 {
		w.decide(b)
		v := w.valuation
		for j, o := range v.Options {
			if o.Kind == ActWait {
				return v.Risk[0][j]
			}
		}
		t.Fatal("no wait")
		return 0
	}
	ahead := risk()
	w.cfg.DuskRead = false
	if staying := risk(); !(ahead > staying) {
		t.Fatalf("waiting risk with dark ahead %v, light staying %v", ahead, staying)
	}
}
