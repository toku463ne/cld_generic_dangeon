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
		{Level: 0, Sight: 0, Fert: 0, Size: 0},
		{Level: cfg.AllotLevels - 1, Sight: cfg.SightMost, Fert: cfg.FertMost, Size: cfg.BudgetLevels - 1},
		{Level: cfg.AllotLevels / 2, Sight: cfg.Sight, Fert: cfg.FertMost / 2, Size: (cfg.BudgetLevels - 1) / 2},
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
		{Level: 0, Sight: 0, Fert: 0, Size: cfg.BudgetLevels},
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
	a := &Body{ID: 1, Level: 1, Sight: 0 + 1, Fert: 0 + 1, Size: 0 + 1}
	b := &Body{ID: 2, Level: w.cfg.AllotLevels - 2, Sight: w.cfg.SightMost + 1, Fert: w.cfg.FertMost + 1, Size: w.cfg.BudgetLevels}
	near := func(k, p, q int) bool { return k == p || k == q || k == p-1 || k == p+1 || k == q-1 || k == q+1 }
	for i := 0; i < 100; i++ {
		var c Body
		w.allot(&c, a, b)
		g := w.GenomeOf(c)
		if !near(g.Level, a.Level, b.Level) || !near(g.Sight, a.Sight-1, b.Sight-1) || !near(g.Fert, a.Fert-1, b.Fert-1) || !near(g.Size, a.Size-1, b.Size-1) {
			t.Fatalf("child genome %+v is not within one level of a parent's", g)
		}
		if err := w.validGenome(g); err != nil {
			t.Fatal(err)
		}
	}
}

// With AllotBudget the size of the budget is a slot of the genome (stage
// 9-5b): first bodies draw a level, the neutral level buys the config's
// budget and body, each level off it scales the budget by BudgetStep and
// every ability with it, and with BudgetBurn the burn per tick scales
// linearly with the budget. A child takes a parent's level, moved at
// most one; the ledger closes with budgets of every size; the zero field
// reads as the neutral level.
func TestAllotBudget(t *testing.T) {
	w := newTestWorld(t, 5)
	if !w.cfg.AllotBudget || !w.cfg.BudgetBurn || w.cfg.BudgetLevels != 5 {
		t.Fatalf("defaults: AllotBudget %v BudgetBurn %v BudgetLevels %d, want on, on, 5", w.cfg.AllotBudget, w.cfg.BudgetBurn, w.cfg.BudgetLevels)
	}
	seen := map[int]bool{}
	for _, b := range w.Bodies() {
		if b.Size < 1 || b.Size > w.cfg.BudgetLevels {
			t.Fatalf("a first body drew size %d", b.Size)
		}
		seen[b.Size-1] = true
	}
	if len(seen) < 3 {
		t.Fatalf("first bodies drew only sizes %v", seen)
	}
	if !w.BudgetLedger().Balanced() {
		t.Fatal("budget ledger does not close at the start")
	}
	run(w, 1500)
	if !w.BudgetLedger().Balanced() {
		t.Fatalf("budget ledger does not close after births and deaths: %+v", w.BudgetLedger())
	}

	// What a level buys, with the other slots neutral.
	neutral := (w.cfg.BudgetLevels - 1) / 2
	body := func(size int) Body {
		var b Body
		b.Energy = math.Inf(1)
		w.endow(&b, Genome{Level: w.cfg.AllotLevels / 2, Sight: w.cfg.Sight, Fert: w.cfg.FertMost / 2, Size: size})
		return b
	}
	plain, big, small := body(neutral), body(w.cfg.BudgetLevels-1), body(0)
	if plain.Budget != w.cfg.Budget || math.Abs(plain.Build.Speed-w.cfg.Speed) > 1e-12 || math.Abs(plain.Build.EnergyMax-w.cfg.EnergyMax) > 1e-12 {
		t.Fatalf("neutral size buys budget %v speed %v most %v, want the config's", plain.Budget, plain.Build.Speed, plain.Build.EnergyMax)
	}
	step := math.Pow(w.cfg.BudgetStep, float64(neutral))
	if math.Abs(big.Budget-w.cfg.Budget*step) > 1e-12 || math.Abs(small.Budget-w.cfg.Budget/step) > 1e-12 {
		t.Fatalf("sizes %d/0 buy budgets %v/%v, want %v/%v", w.cfg.BudgetLevels-1, big.Budget, small.Budget, w.cfg.Budget*step, w.cfg.Budget/step)
	}
	if !(big.Build.Speed > plain.Build.Speed && plain.Build.Speed > small.Build.Speed) || !(big.Build.EnergyMax > plain.Build.EnergyMax && plain.Build.EnergyMax > small.Build.EnergyMax) {
		t.Fatal("abilities do not rise with the budget")
	}
	want := math.Pow(step, w.cfg.AllotCurve)
	if f := big.Build.Speed / plain.Build.Speed; math.Abs(f-want) > 1e-12 {
		t.Fatalf("the biggest budget buys speed x%v, want x%v (budget^AllotCurve)", f, want)
	}
	// The price: burn scales linearly with the budget, and not without
	// BudgetBurn.
	if f := w.ownBurn(&big, false) / w.ownBurn(&plain, false); math.Abs(f-step) > 1e-12 {
		t.Fatalf("the biggest budget burns x%v, want x%v", f, step)
	}
	if f := w.ownBurn(&small, false) / w.ownBurn(&plain, false); math.Abs(f-1/step) > 1e-12 {
		t.Fatalf("the smallest budget burns x%v, want x%v", f, 1/step)
	}
	w.cfg.BudgetBurn = false
	if w.ownBurn(&big, false) != w.ownBurn(&plain, false) {
		t.Fatal("without BudgetBurn a big body burns more")
	}
	w.cfg.BudgetBurn = true
	// Starving takes a bigger body less long: its most energy grows by
	// budget^AllotCurve, its burn by the budget.
	if big.Build.EnergyMax/w.ownBurn(&big, false) >= plain.Build.EnergyMax/w.ownBurn(&plain, false) {
		t.Fatal("a bigger body lasts as long on a full stomach as the neutral one; the price does not bite")
	}

	// A child takes a parent's level (mutation off), moved at most one
	// with mutation, inward at the ends.
	w.cfg.AllotMutation = 0
	var child Body
	w.allot(&child, &big, &big)
	if child.Size != big.Size {
		t.Fatalf("child size %d, parent %d", child.Size, big.Size)
	}
	w.cfg.AllotMutation = 1
	for i := 0; i < 50; i++ {
		var c Body
		w.allot(&c, &big, &big)
		if c.Size != big.Size-1 {
			t.Fatalf("mutation from the top level gave %d, want %d", c.Size-1, big.Size-2)
		}
	}
	// The zero value stands for the neutral level and the config's budget.
	none := Body{ID: 3}
	if w.sizeOf(&none) != neutral || w.GenomeOf(none).Size != neutral {
		t.Fatalf("zero size reads as level %d, want %d", w.sizeOf(&none), neutral)
	}
	// Mean budget by generation is kept.
	st := w.Stats()
	if len(st.GenBudget) != len(st.GenBirths) || st.GenBudget[0] <= 0 {
		t.Fatalf("GenBudget %v does not follow GenBirths %v", st.GenBudget, st.GenBirths)
	}
}
