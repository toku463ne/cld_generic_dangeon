package engine

import (
	"math"
	"testing"
)

// Learning when dark and light come changes nothing a body does yet:
// over days long enough to learn them, the world runs as without (stage
// 7-4).
func TestDuskReadChangesNothing(t *testing.T) {
	fp := func(dusk bool) uint64 {
		cfg := testConfig(1)
		cfg.DuskRead = dusk
		w, err := NewWorld(cfg, tempMap())
		if err != nil {
			t.Fatal(err)
		}
		run(w, 2500)
		return w.Fingerprint()
	}
	if with, without := fp(true), fp(false); with != without {
		t.Fatalf("fingerprint %#x learning, %#x not", with, without)
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
