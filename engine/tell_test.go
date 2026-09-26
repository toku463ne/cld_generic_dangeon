package engine

import (
	"math"
	"testing"
)

func tallyOf(n, k float64) Tally { return Tally{N: n, K: k} }

// Bodies that come within sight pass their evidence once, both ways, under
// its first observer; nothing returns to its own observer; and the same
// observer's evidence arriving twice is kept once, the larger copy.
func TestTellPassesOnce(t *testing.T) {
	w := pairWorld(t, Body{ID: 0, X: 2.5, Y: 2.5, Energy: 100}, Body{ID: 1, X: 3.5, Y: 2.5, Energy: 100})
	w.cfg.StableRows = false // regions pass too
	a, b := &w.bodies[0], &w.bodies[1]
	a.Memory.Regions = []Tally{tallyOf(100, 5)}
	a.Memory.Path = tallyOf(10, 0)
	b.Memory.Regions = []Tally{tallyOf(40, 1)}
	w.tell(a)
	if got := b.Memory.Regions[0]; got.N != 140 || got.K != 6 || got.heard(0) != (Heard{0, 100, 5}) {
		t.Fatalf("b's region after meeting a: %+v", got)
	}
	if got := a.Memory.Regions[0]; got.N != 140 || got.K != 6 || got.heard(1) != (Heard{1, 40, 1}) || len(got.Heard) != 1 {
		t.Fatalf("a's region after meeting b: %+v", got)
	}
	if got := b.Memory.Path; got.N != 10 || got.heard(0) != (Heard{0, 10, 0}) {
		t.Fatalf("b's path: %+v", got)
	}
	if st := w.Stats(); st.RegionRows[0].Passed != 2 || st.PathRow.Passed != 1 {
		t.Fatalf("passes counted: region %+v, path %+v", st.RegionRows[0], st.PathRow)
	}
	// Still in sight: nothing passes again.
	a.Memory.Regions[0].N += 50
	w.tell(a)
	w.tell(b)
	if got := b.Memory.Regions[0]; got.N != 140 {
		t.Fatalf("met again: b's region %+v", got)
	}
	// A third body hears a's evidence through b, then from a itself, larger:
	// one entry for a, the larger.
	c := &Body{ID: 2}
	w.pass(b, c)
	if got := c.Memory.Regions[0].heard(0); got != (Heard{0, 100, 5}) {
		t.Fatalf("c heard of a through b: %+v", got)
	}
	w.pass(a, c)
	if got := c.Memory.Regions[0]; got.heard(0) != (Heard{0, 150, 5}) || got.N != 150+40 {
		t.Fatalf("c after hearing a itself: %+v", got)
	}
}

// A body keeps at most HeardLimit observers per row, the ones with most
// evidence, and its estimate reads what it heard.
func TestHeardLimitAndEstimate(t *testing.T) {
	w := newTestWorld(t, 1)
	w.cfg.StableRows = false // regions pass too
	w.cfg.HeardLimit = 3
	dst := &Body{ID: 99, Goal: -1}
	for i := int64(0); i < 6; i++ {
		src := &Body{ID: i, Memory: Memory{Regions: []Tally{tallyOf(float64(10*(i+1)), float64(i))}}}
		w.pass(src, dst)
	}
	tl := dst.Memory.Regions[0]
	if len(tl.Heard) != 3 || tl.heard(5).N != 60 || tl.heard(4).N != 50 || tl.heard(3).N != 40 {
		t.Fatalf("kept %+v", tl.Heard)
	}
	if got, want := dst.Memory.Regions[0].N, 150.0; got != want {
		t.Fatalf("N %v, want %v", got, want)
	}
	before := w.regionRate(&Body{}, 0)
	if after := w.regionRate(dst, 0); after == before {
		t.Fatal("heard evidence did not move the estimate")
	}
}

// In a world where bodies pass evidence, evidence outlives its observers.
func TestEvidenceOutlivesObservers(t *testing.T) {
	w := newTestWorld(t, 4)
	orphans := 0.0
	for i := 0; i < 8; i++ {
		run(w, 500)
		for _, rp := range w.Provenance() {
			orphans += rp.Orphan
		}
	}
	if orphans == 0 {
		t.Fatal("no orphan evidence in a world that passes evidence")
	}
}

// With StableRows only the path row passes, it does not age, and it is read
// as how much less food walked tiles hold than their region: its estimate
// follows the region's.
func TestStableRows(t *testing.T) {
	w := pairWorld(t, Body{ID: 0, X: 2.5, Y: 2.5, Energy: 100}, Body{ID: 1, X: 3.5, Y: 2.5, Energy: 100})
	a, b := &w.bodies[0], &w.bodies[1]
	a.Memory.Regions = []Tally{{N: 100, K: 5, T: w.tick}}
	a.Memory.Path = Tally{N: 2, K: 1} // food twice as often as the region led it to expect
	w.tell(a)
	if len(b.Memory.Regions) != 0 {
		t.Fatalf("a region row passed: %+v", b.Memory.Regions)
	}
	if got := b.Memory.Path; got.N != 2 || got.K != 1 {
		t.Fatalf("path row not passed: %+v", got)
	}
	// Not aged: ten half-lives on, the same.
	w.tick += 10 * int64(w.cfg.EvidenceHalfLife)
	if got := w.pathTally(b); got.N != 2 || got.K != 1 {
		t.Fatalf("path row aged: %+v", got)
	}
	// Read against the region: the ratio (1 + weight p)/(2 + weight p) of
	// whatever the region is believed to hold.
	for _, p := range []float64{0.005, 0.02} {
		weight := w.cfg.PathWeight * p
		want := (1 + weight) / (2 + weight) * p
		if got := w.pathFrom(b.Memory.Path, p); math.Abs(got-want) > 1e-12 {
			t.Fatalf("region at %v: path %v, want %v", p, got, want)
		}
	}
	// No evidence: a walked tile reads as any tile of its region.
	if got := w.pathFrom(Tally{}, 0.01); got != 0.01 {
		t.Fatalf("no evidence: %v", got)
	}
}
