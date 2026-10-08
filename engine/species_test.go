package engine

import (
	"math"
	"testing"
)

// With a species table the first bodies are drawn among its rows by share,
// a body mates within its row only, a child is of its parents' species, and
// a row's Size shifts the scale of its budget by levels of BudgetStep
// (stage 9-7a). One row is one species, as before the stage.
func TestSpecies(t *testing.T) {
	w := newTestWorld(t, 5)
	if len(w.cfg.Species) != 2 || w.cfg.Species[1].Size != 1 {
		t.Fatalf("defaults: %+v, want human and beast at +1", w.cfg.Species)
	}
	var count [2]int
	for _, b := range w.Bodies() {
		count[b.Species]++
	}
	if count[0] == 0 || count[1] == 0 {
		t.Fatalf("first bodies drew species %v, want both", count)
	}
	run(w, 1500)
	// Children are of their parents' species, and mates were of one species.
	for _, b := range w.Bodies() {
		if b.Parents[0] < 0 {
			continue
		}
		for _, o := range w.Bodies() {
			if o.ID == b.Parents[0] || o.ID == b.Parents[1] {
				if o.Species != b.Species {
					t.Fatalf("child %d of species %d has parent %d of species %d", b.ID, b.Species, o.ID, o.Species)
				}
			}
		}
	}
	// No body sees a mate of another species.
	for i := range w.bodies {
		b := &w.bodies[i]
		w.mates(b, func(o *Body) {
			if o.Species != b.Species {
				t.Fatalf("body %d (species %d) counts %d (species %d) as a mate", b.ID, b.Species, o.ID, o.Species)
			}
		})
	}
	// The scale: the same genome buys a beast BudgetStep times the budget,
	// so BudgetStep^AllotCurve the speed; its level one below the neutral
	// buys the config's budget.
	body := func(species, size int) Body {
		var b Body
		b.Energy = math.Inf(1)
		w.endow(&b, Genome{Level: w.cfg.AllotLevels / 2, Sight: w.cfg.Sight, Fert: w.cfg.FertMost / 2, Size: size, Bold: w.cfg.VentureMost / 2, Species: species})
		return b
	}
	n := w.sizeNeutral()
	human, beast, small := body(0, n), body(1, n), body(1, n-1)
	if beast.Species != 1 || human.Species != 0 {
		t.Fatal("endow did not keep the species")
	}
	if math.Abs(beast.Budget-human.Budget*w.cfg.BudgetStep) > 1e-12 || math.Abs(small.Budget-human.Budget) > 1e-12 {
		t.Fatalf("beast budgets %v / %v (one level down), human %v: want x%v and equal", beast.Budget, small.Budget, human.Budget, w.cfg.BudgetStep)
	}
	if f := beast.Build.Speed / human.Build.Speed; math.Abs(f-math.Pow(w.cfg.BudgetStep, w.cfg.AllotCurve)) > 1e-12 {
		t.Fatalf("a beast at the neutral level is x%v as fast, want BudgetStep^AllotCurve", f)
	}
	if w.ownBurn(&beast, false) <= w.ownBurn(&human, false) {
		t.Fatal("a beast does not pay the price of its budget")
	}
	// Genomes carry the species through NewWorldFrom and inherit.
	g := w.inherit(w.genomeOf(&beast), w.genomeOf(&beast))
	if g.Species != 1 {
		t.Fatalf("inherited species %d, want 1", g.Species)
	}
	w2, err := NewWorldFrom(testConfig(2), testMap(), []Genome{w.genomeOf(&beast), w.genomeOf(&human)})
	if err != nil {
		t.Fatal(err)
	}
	if bs := w2.Bodies(); bs[0].Species != 1 || bs[1].Species != 0 {
		t.Fatalf("NewWorldFrom gave species %d, %d, want 1, 0", bs[0].Species, bs[1].Species)
	}
	// Bad tables are refused.
	for _, bad := range [][]SpeciesSpec{
		{{Name: "a", Share: 1}, {Name: "a", Share: 1}},
		{{Name: "a", Share: 0}, {Name: "b", Share: 0}},
		{{Name: "", Share: 1}},
	} {
		cfg := testConfig(1)
		cfg.Species = bad
		if _, err := NewWorld(cfg, testMap()); err == nil {
			t.Fatalf("table %+v was accepted", bad)
		}
	}
}
