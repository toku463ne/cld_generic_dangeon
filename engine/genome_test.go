package engine

import (
	"math"
	"testing"
)

// A body's genome, handed to a fresh body, buys the same body: the same
// share, sight, fertility level and build. So genomes carry everything
// that is inherited, and nothing a body learns.
func TestGenomeEndowsTheSameBody(t *testing.T) {
	w := newTestWorld(t, 5)
	run(w, 2000)
	if len(w.Bodies()) == 0 {
		t.Fatal("no bodies to read genomes off")
	}
	for _, b := range w.Bodies() {
		g := w.GenomeOf(b)
		var c Body
		c.Energy = math.Inf(1)
		w.endow(&c, g)
		if c.Level != b.Level || c.Share != b.Share || c.Sight != b.Sight || c.Fert != b.Fert || c.Build != b.Build {
			t.Fatalf("genome %+v of body %d bought %+v, want level %d share %v sight %d fert %d build %+v",
				g, b.ID, c, b.Level, b.Share, b.Sight, b.Fert, b.Build)
		}
		if c.Energy != w.maxOf(&c) {
			t.Fatalf("energy %v not held to the most %v", c.Energy, w.maxOf(&c))
		}
		if h := w.GenomeOf(c); h != g {
			t.Fatalf("genome %+v read back as %+v", g, h)
		}
	}
}

// A world begun from genomes has one first body per genome, in order,
// and otherwise runs like any world; with no genomes it is NewWorld.
func TestNewWorldFrom(t *testing.T) {
	cfg := testConfig(9)
	genomes := []Genome{
		{Level: 0, Sight: 0, Fert: 0},
		{Level: cfg.AllotLevels - 1, Sight: cfg.SightMost, Fert: cfg.FertMost},
		{Level: cfg.AllotLevels / 2, Sight: cfg.Sight, Fert: cfg.FertMost / 2},
	}
	w, err := NewWorldFrom(cfg, testMap(), genomes)
	if err != nil {
		t.Fatal(err)
	}
	bodies := w.Bodies()
	if len(bodies) != len(genomes) {
		t.Fatalf("%d first bodies, want one per genome (%d), not Config.Bodies (%d)", len(bodies), len(genomes), cfg.Bodies)
	}
	for i, b := range bodies {
		if g := w.GenomeOf(b); g != genomes[i] {
			t.Fatalf("body %d has genome %+v, want %+v", i, g, genomes[i])
		}
		if b.Energy != w.maxOf(&b) {
			t.Fatalf("body %d begins with energy %v, want its most %v", i, b.Energy, w.maxOf(&b))
		}
	}
	if !w.BudgetLedger().Balanced() {
		t.Fatal("budget ledger does not close for handed genomes")
	}
	run(w, 200)
	if !w.BudgetLedger().Balanced() {
		t.Fatal("budget ledger does not close after running")
	}

	// No genomes, no first bodies: the list is the first bodies.
	if e, err := NewWorldFrom(cfg, testMap(), nil); err != nil || len(e.Bodies()) != 0 {
		t.Fatalf("no genomes gave %d bodies, err %v", len(e.Bodies()), err)
	}

	// Genomes outside the config's levels are refused.
	for _, bad := range []Genome{
		{Level: cfg.AllotLevels, Sight: 0, Fert: 0},
		{Level: 0, Sight: cfg.SightMost + 1, Fert: 0},
		{Level: 0, Sight: 0, Fert: -1},
	} {
		if _, err := NewWorldFrom(cfg, testMap(), []Genome{bad}); err == nil {
			t.Fatalf("genome %+v was accepted", bad)
		}
	}
}

// A child bred through genomes has each slot one parent's or one level
// off it, within the config's levels.
func TestInheritStaysWithinLevels(t *testing.T) {
	w := newTestWorld(t, 7)
	a := &Body{ID: 1, Level: 1, Sight: 0 + 1, Fert: 0 + 1}
	b := &Body{ID: 2, Level: w.cfg.AllotLevels - 2, Sight: w.cfg.SightMost + 1, Fert: w.cfg.FertMost + 1}
	near := func(k, p, q int) bool { return k == p || k == q || k == p-1 || k == p+1 || k == q-1 || k == q+1 }
	for i := 0; i < 100; i++ {
		var c Body
		w.allot(&c, a, b)
		g := w.GenomeOf(c)
		if !near(g.Level, a.Level, b.Level) || !near(g.Sight, a.Sight-1, b.Sight-1) || !near(g.Fert, a.Fert-1, b.Fert-1) {
			t.Fatalf("child genome %+v is not within one level of a parent's", g)
		}
		if err := w.validGenome(g); err != nil {
			t.Fatal(err)
		}
	}
}
