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

// With AllotVenture the exploratory trait is a slot of the genome outside
// the budget (stage 9-6a): first bodies draw a level, a child takes a
// parent's moved at most one, the level scales what the body expects of a
// region it has little evidence of by VentureStep per level off the
// neutral, and nothing else about the body - budget, speed, most energy,
// sight, fertility - changes with it. The zero field reads as the neutral
// level, and without the slot every body reads the config's Venture.
func TestAllotVenture(t *testing.T) {
	w := newTestWorld(t, 5)
	if !w.cfg.AllotVenture || w.cfg.VentureMost != 2 || w.cfg.VentureStep != 1.5 {
		t.Fatalf("defaults: AllotVenture %v VentureMost %d VentureStep %v, want on, 2, 1.5", w.cfg.AllotVenture, w.cfg.VentureMost, w.cfg.VentureStep)
	}
	seen := map[int]bool{}
	for _, b := range w.Bodies() {
		if b.Bold < 1 || b.Bold > w.cfg.VentureMost+1 {
			t.Fatalf("a first body drew boldness %d", b.Bold)
		}
		seen[b.Bold-1] = true
	}
	if len(seen) != w.cfg.VentureMost+1 {
		t.Fatalf("first bodies drew only levels %v", seen)
	}
	run(w, 1500)
	if !w.BudgetLedger().Balanced() {
		t.Fatalf("budget ledger does not close: %+v", w.BudgetLedger())
	}

	// Outside the budget: the same body at every level.
	body := func(bold int) Body {
		var b Body
		b.Energy = math.Inf(1)
		w.endow(&b, Genome{Level: w.cfg.AllotLevels / 2, Sight: w.cfg.Sight, Fert: w.cfg.FertMost / 2, Size: w.sizeNeutral(), Bold: bold})
		return b
	}
	neutral := w.cfg.VentureMost / 2
	plain, bold, timid := body(neutral), body(w.cfg.VentureMost), body(0)
	for _, b := range []Body{bold, timid} {
		if b.Budget != plain.Budget || b.Build != plain.Build || b.Sight != plain.Sight || b.Fert != plain.Fert || b.Size != plain.Size {
			t.Fatalf("boldness %d changes the body: %+v vs %+v", b.Bold-1, b, plain)
		}
	}
	// The prior: a newborn reads an unknown region at PriorFood times its
	// venture.
	step := math.Pow(w.cfg.VentureStep, float64(neutral))
	for _, c := range []struct {
		b    Body
		want float64
	}{{plain, 1}, {bold, step}, {timid, 1 / step}} {
		if got := w.ventureOf(&c.b); math.Abs(got-c.want) > 1e-12 {
			t.Fatalf("boldness %d ventures %v, want %v", c.b.Bold-1, got, c.want)
		}
		if got, want := w.regionRate(&c.b, 1), math.Min(w.cfg.PriorFood*c.want, 1); math.Abs(got-want) > 1e-12 {
			t.Fatalf("boldness %d reads an unknown region at %v, want %v", c.b.Bold-1, got, want)
		}
	}
	// A child takes a parent's level (mutation off), moved at most one
	// with mutation, inward at the ends.
	w.cfg.AllotMutation = 0
	var child Body
	w.allot(&child, &bold, &bold)
	if child.Bold != bold.Bold {
		t.Fatalf("child boldness %d, parent %d", child.Bold, bold.Bold)
	}
	w.cfg.AllotMutation = 1
	for i := 0; i < 50; i++ {
		var c Body
		w.allot(&c, &bold, &bold)
		if c.Bold != bold.Bold-1 {
			t.Fatalf("mutation from the top level gave %d, want %d", c.Bold-1, bold.Bold-2)
		}
	}
	// The zero value stands for the neutral level.
	none := Body{ID: 3}
	if w.boldOf(&none) != neutral || w.GenomeOf(none).Bold != neutral || w.ventureOf(&none) != w.cfg.Venture {
		t.Fatalf("zero boldness reads as level %d venture %v, want %d and %v", w.boldOf(&none), w.ventureOf(&none), neutral, w.cfg.Venture)
	}
	// Steps are counted by level.
	st := w.Stats()
	if len(st.BoldSteps) != w.cfg.VentureMost+1 || st.BoldSteps[0] == 0 || st.BoldSteps[w.cfg.VentureMost] == 0 {
		t.Fatalf("steps by boldness %v", st.BoldSteps)
	}
	var sum int64
	for _, n := range st.BoldSteps {
		sum += n
	}
	if sum != st.Steps {
		t.Fatalf("steps by level sum to %d, all steps %d", sum, st.Steps)
	}
	// Without the slot every body reads the config's Venture, whatever
	// the field says.
	w.cfg.AllotVenture = false
	w.cfg.Venture = 1.5
	if w.ventureOf(&bold) != 1.5 || w.ventureOf(&timid) != 1.5 {
		t.Fatal("without AllotVenture boldness still reads")
	}
}

// Attack and defense are slots of the budget (AllotCombat, stage 10-1):
// drawn within 0..CombatMost, inherited and mutated like every slot,
// each level off the neutral multiplying the strength by CombatStep, and
// each level above the neutral costing CombatTrade of the build's budget
// (free at zero). Without AllotCombat every body is at the neutral level.
func TestCombatSlots(t *testing.T) {
	w := newTestWorld(t, 13)
	seen := map[[2]int]bool{}
	for _, b := range w.Bodies() {
		a, d := w.BodyAttack(b), w.BodyDefense(b)
		if a < 0 || a > w.cfg.CombatMost || d < 0 || d > w.cfg.CombatMost {
			t.Fatalf("body %d has attack %d defense %d", b.ID, a, d)
		}
		seen[[2]int{a, d}] = true
	}
	if len(seen) < 4 {
		t.Fatalf("only %d attack/defense pairs drawn among %d bodies", len(seen), len(w.Bodies()))
	}
	// Powers: CombatStep per level off the neutral, one at it.
	n := w.cfg.CombatMost / 2
	for lvl := 0; lvl <= w.cfg.CombatMost; lvl++ {
		b := Body{Attack: lvl + 1, Defense: lvl + 1}
		want := math.Pow(w.cfg.CombatStep, float64(lvl-n))
		if math.Abs(w.attackPower(&b)-want) > 1e-12 || math.Abs(w.defensePower(&b)-want) > 1e-12 {
			t.Fatalf("level %d: powers %v / %v, want %v", lvl, w.attackPower(&b), w.defensePower(&b), want)
		}
	}
	// Inheritance carries the slots; the genome round-trips.
	g := Genome{Level: 0, Sight: w.cfg.Sight, Fert: w.cfg.FertMost / 2, Size: w.sizeNeutral(), Bold: w.cfg.VentureMost / 2, Attack: w.cfg.CombatMost, Defense: 0}
	w.cfg.AllotMutation = 0
	if h := w.inherit(g, g); h.Attack != w.cfg.CombatMost || h.Defense != 0 {
		t.Fatalf("inherited attack %d defense %d, want %d and 0", h.Attack, h.Defense, w.cfg.CombatMost)
	}
	var b Body
	b.Energy = math.Inf(1)
	w.endow(&b, g)
	if got := w.genomeOf(&b); got.Attack != g.Attack || got.Defense != g.Defense {
		t.Fatalf("endowed genome %+v, want attack %d defense %d", got, g.Attack, g.Defense)
	}
	// Free at CombatTrade zero; a price shrinks the build.
	free := b.Build
	w.cfg.CombatTrade = 0.2
	var c Body
	c.Energy = math.Inf(1)
	w.endow(&c, Genome{Level: 0, Sight: w.cfg.Sight, Fert: w.cfg.FertMost / 2, Size: w.sizeNeutral(), Bold: w.cfg.VentureMost / 2, Attack: w.cfg.CombatMost, Defense: w.cfg.CombatMost})
	if c.Build.EnergyMax >= free.EnergyMax {
		t.Fatalf("a priced combat level did not shrink the build: %v vs %v", c.Build.EnergyMax, free.EnergyMax)
	}
	// Without the slot every body is neutral.
	cfg := testConfig(2)
	cfg.AllotCombat = false
	cfg.AllotFierce = false
	w2, err := NewWorld(cfg, testMap())
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range w2.Bodies() {
		if w2.BodyAttack(b) != n || w2.BodyDefense(b) != n || b.Attack != 0 {
			t.Fatalf("without AllotCombat body %d is at %d/%d", b.ID, w2.BodyAttack(b), w2.BodyDefense(b))
		}
	}
	cfg.AllotCombat, cfg.CombatMost = true, 0
	if _, err := NewWorld(cfg, testMap()); err == nil {
		t.Fatal("CombatMost 0 was accepted")
	}
}
