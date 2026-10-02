package engine

import (
	"math"
	"testing"
)

// bandWorld is a learning world with age bands of 100 ticks, chunks of 10,
// and trusts 1, 1/2, 1/4, 1/8.
func bandWorld(t *testing.T) *World {
	t.Helper()
	cfg := testConfig(1)
	cfg.Bodies = 0
	cfg.AgeBand, cfg.AgeEpoch, cfg.AgeTrust = 100, 10, []float64{1, 0.5, 0.25, 0.125}
	w, err := NewWorld(cfg, testMap())
	if err != nil {
		t.Fatal(err)
	}
	return w
}

// Evidence is trusted by its step of age: the same within a step, the
// next step's trust past each edge, the last step's for all older.
func TestTrustSteps(t *testing.T) {
	w := bandWorld(t)
	w.tick = 1000
	for _, c := range []struct {
		age  int64
		want float64
	}{{0, 1}, {99, 1}, {100, 0.5}, {199, 0.5}, {200, 0.25}, {300, 0.125}, {900, 0.125}} {
		if got := w.trust(w.tick - c.age); got != c.want {
			t.Errorf("age %d: trust %v, want %v", c.age, got, c.want)
		}
	}
}

// A new observation joins the chunk of its epoch and leaves the times of
// older chunks as they were; the tally reads each chunk at its own step.
func TestObserveKeepsOldSteps(t *testing.T) {
	w := bandWorld(t)
	var tl Tally
	w.tick = 5
	w.observe(&tl, 4, 2)
	w.tick = 8
	w.observe(&tl, 2, 0)
	w.tick = 150
	w.observe(&tl, 10, 5)
	if len(tl.Chunks) != 2 || tl.Chunks[0] != (Chunk{T: 0, N: 6, K: 2}) || tl.Chunks[1] != (Chunk{T: 150, N: 10, K: 5}) {
		t.Fatalf("chunks %+v", tl.Chunks)
	}
	if tl.N != 16 || tl.K != 7 {
		t.Fatalf("counted %v of %v, want 7 of 16", tl.K, tl.N)
	}
	f := w.Fresh(tl) // the first chunk 150 old (1/2), the second new (1)
	if f.N != 6*0.5+10 || f.K != 2*0.5+5 {
		t.Fatalf("read %v of %v", f.K, f.N)
	}
}

// Chunks old enough to share the last step merge, and the reading stays
// what it was.
func TestOldChunksMerge(t *testing.T) {
	w := bandWorld(t)
	var tl Tally
	for _, at := range []int64{0, 20, 40, 60} {
		w.tick = at
		w.observe(&tl, 1, 1)
	}
	w.tick = 365
	before := w.Fresh(tl)
	w.observe(&tl, 1, 0)
	if len(tl.Chunks) != 2 {
		t.Fatalf("chunks %+v, want the four old ones merged", tl.Chunks)
	}
	after := w.Fresh(tl)
	if math.Abs(after.N-(before.N+1)) > 1e-12 || math.Abs(after.K-before.K) > 1e-12 {
		t.Fatalf("merging changed the reading: %+v then %+v", before, after)
	}
}

// Age lowers trust, not size: a large old experience can outweigh a small
// recent one.
func TestStrongOldOutweighsWeakNew(t *testing.T) {
	w := bandWorld(t)
	var old, recent Tally
	w.observe(&old, 100, 0)
	w.tick = 400
	w.observe(&recent, 5, 5)
	if o, r := w.Fresh(old).N, w.Fresh(recent).N; !(o > r) {
		t.Fatalf("old read %v, recent %v", o, r)
	}
}

// With bands the mate row ages like every row: asks long ago pull the
// estimate toward the inborn expectation again.
func TestMateRowAges(t *testing.T) {
	w := bandWorld(t)
	b := Body{}
	for i := 0; i < 20; i++ {
		w.observe(&b.Memory.Mate, 1, 0)
	}
	fresh := w.childRate(&b)
	w.tick = 1000
	if aged := w.childRate(&b); !(aged > fresh) || aged > w.cfg.PriorChild {
		t.Fatalf("child rate fresh %v, aged %v", fresh, aged)
	}
}

// Bands do not carry evidence passed between bodies.
func TestBandsRefusePassedRows(t *testing.T) {
	cfg := testConfig(1)
	cfg.PassPath = true
	if _, err := NewWorld(cfg, testMap()); err == nil {
		t.Fatal("bands taken with a row that passes")
	}
}
