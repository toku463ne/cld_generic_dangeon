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

// A body whose species does not list water may wade into it (Wade) and
// burns WaterBurn times as fast there; its survival tables still read its
// own land. A species of the water (fish) starts in it, never leaves it,
// and eats a pool that comes back in it, which the land species do not
// see (stage 9-7b-2c).
func TestWadeAndFish(t *testing.T) {
	w := newTestWorld(t, 8)
	// A human beside the water (column 7 of testMap) may step in.
	b := Body{X: 6.9, Y: 2.5, Energy: 60, Heading: -1, Goal: -1, Decided: -1}
	into := false
	for _, a := range w.possibleActions(nil, &b) {
		if a.Kind == ActMove && w.m.Terrain[w.tileOf(b.X+float64(moveDirs[a.Dir][0])*w.speedOf(&b), b.Y)] == TerrainWater {
			into = true
		}
	}
	if !into {
		t.Fatal("a wading species is not offered a step into water")
	}
	dry := w.burnOf(&b)
	b.X = 7.5
	if got := w.burnOf(&b); math.Abs(got-dry*w.cfg.WaterBurn) > 1e-12 {
		t.Fatalf("burn in water %v, want %v (WaterBurn x %v)", got, dry*w.cfg.WaterBurn, dry)
	}
	if w.areaOf(0, 1) != len(w.regionLand(1)) {
		t.Fatal("a wading species' area counts the water")
	}
	// Fish.
	cfg := testConfig(9)
	cfg.FoodKinds = []FoodKind{{Name: "land food", Tags: []string{"human", "beast"}}, {Name: "fish food", Tags: []string{"fish"}}}
	cfg.Foods = []FoodSpec{{Name: "land food", Share: 0.5}, {Name: "fish food", Kind: 1, Share: 0.5, Terrain: TerrainWater}}
	cfg.Species = append(cfg.Species, SpeciesSpec{Name: "fish", Share: 1, Terrains: []Terrain{TerrainWater}, Swims: true})
	w, err := NewWorld(cfg, testMap())
	if err != nil {
		t.Fatal(err)
	}
	fishes := 0
	for _, f := range w.Foods() {
		if (f.ID == 1) != (w.m.Terrain[w.m.index(f.X, f.Y)] == TerrainWater) {
			t.Fatalf("a unit of pool %d at (%d,%d)", f.ID, f.X, f.Y)
		}
	}
	for _, b := range w.Bodies() {
		if b.Species == 2 {
			fishes++
			if w.m.Terrain[w.tileOf(b.X, b.Y)] != TerrainWater {
				t.Fatal("a fish starts on land")
			}
		}
	}
	if fishes == 0 {
		t.Fatal("no fish drawn")
	}
	if !w.Home(2, 1) || w.Home(2, 0) || !w.Home(0, 0) {
		t.Fatalf("homes: fish %v %v, human %v", w.Home(2, 1), w.Home(2, 0), w.Home(0, 0))
	}
	if w.areaOf(2, 1) != len(w.food.regionTiles[TerrainWater][1]) {
		t.Fatal("a fish's area is not the water")
	}
	for i := 0; i < 1500; i++ {
		w.Step()
		checkFood(t, w)
		for _, b := range w.bodies {
			if tr := w.m.Terrain[w.tileOf(b.X, b.Y)]; b.Species == 2 && tr != TerrainWater {
				t.Fatalf("tick %d: a fish on land", w.Tick())
			}
			for _, p := range b.HeldPool {
				if (p == 1) != (b.Species == 2) {
					t.Fatalf("tick %d: species %d holds a unit of pool %d", w.Tick(), b.Species, p)
				}
			}
		}
	}
	if w.FoodLedger().Pools[1].Eaten == 0 {
		t.Fatal("the fish never ate")
	}
	// A fish in water pays no WaterBurn.
	f := Body{X: 7.5, Y: 2.5, Energy: 60, Species: 2}
	if w.wading(&f) {
		t.Fatal("a fish wades")
	}
	// Bad rows are refused.
	for _, bad := range []SpeciesSpec{
		{Name: "a", Share: 1, Swims: true},
		{Name: "a", Share: 1, Terrains: []Terrain{9}},
	} {
		cfg := testConfig(1)
		cfg.Species = []SpeciesSpec{bad}
		if _, err := NewWorld(cfg, testMap()); err == nil {
			t.Fatalf("row %+v was accepted", bad)
		}
	}
	cfg = testConfig(1)
	cfg.FoodKinds = []FoodKind{{Name: "a", Tags: []string{"human"}}}
	cfg.Foods = []FoodSpec{{Name: "a", Share: 1, Terrain: 9}}
	if _, err := NewWorld(cfg, testMap()); err == nil {
		t.Fatal("a pool on unknown terrain was accepted")
	}
}

// Only a species with skills is born with any (NoSkills, stage 9-7b-4):
// beasts build nothing and pitch no tent; the zero row (no table) is
// skilled.
func TestSkillsByRow(t *testing.T) {
	w := newTestWorld(t, 12)
	skilled := 0
	for i := 0; i < 3000; i++ {
		for _, b := range w.Bodies() {
			has := b.Skill[SkillBuild] > 0 || b.Skill[SkillTent] > 0
			if b.Species == 1 && has {
				t.Fatalf("tick %d: beast %d has a skill", w.Tick(), b.ID)
			}
			if b.Species == 0 && has {
				skilled++
			}
		}
		w.Step()
	}
	if skilled == 0 {
		t.Fatal("no human ever had a skill")
	}
	for _, p := range w.Places() {
		if p.Species != 0 {
			t.Fatal("a beast built a settlement")
		}
	}
	if (SpeciesSpec{}).NoSkills {
		t.Fatal("the zero row has no skills")
	}
}

// In water bodies pass each other (DeepWater, stage 9-7b-4): a step
// onto a water tile another body stands in is offered, as it is not on
// land.
func TestDeepWater(t *testing.T) {
	for _, deep := range []bool{true, false} {
		w := pairWorld(t, Body{ID: 0, X: 7.5, Y: 2.5, Energy: 60}, Body{ID: 1, X: 7.5, Y: 3.1, Energy: 60})
		w.cfg.DeepWater = deep
		b := &w.bodies[1]
		if w.m.Terrain[w.tileOf(7.5, 2.5)] != TerrainWater {
			t.Fatal("column 7 is not water")
		}
		north := false
		for _, a := range w.possibleActions(nil, b) {
			if a.Kind == ActMove && w.tileOf(b.X, b.Y+float64(moveDirs[a.Dir][1])*w.speedOf(b)) == w.tileOf(7.5, 2.5) {
				north = true
			}
		}
		if north != deep {
			t.Fatalf("deep %v: a step into the occupied water tile offered %v", deep, north)
		}
	}
}

// A beast hunts humans (Hunt, stage 10-2a): it sees a prey, walks up to
// it and strikes once a tick, four blows at even strength killing a body
// at EnergyMax; the prey dies of CausePreyed and leaves MeatUnits units
// of raw meat owned by the hunter, which no human sees and the hunter
// eats. Humans attack nothing; with Hunt off nothing is attacked.
func TestHunt(t *testing.T) {
	cfg := testConfig(14)
	cfg.FoodKinds, cfg.Foods = DefaultConfig().FoodKinds, DefaultConfig().Foods
	cfg.Bodies = 0
	cfg.AllotCombat = false // even strength
	w, err := NewWorld(cfg, testMap())
	if err != nil {
		t.Fatal(err)
	}
	// A beast two tiles west of a human on land.
	beast := Body{ID: 0, X: 2.5, Y: 2.5, Energy: 90, Species: 1, Heading: -1, Goal: -1, Decided: -1, Parents: [2]int64{-1, -1}, Sex: Female, Sight: 4}
	human := Body{ID: 1, X: 4.5, Y: 2.5, Energy: 100, Species: 0, Heading: -1, Goal: -1, Decided: -1, Parents: [2]int64{-1, -1}, Sex: Male, Sight: 2}
	w.bodies = append(w.bodies, beast, human)
	w.nextID = 2
	w.buildGrid()
	b, h := &w.bodies[0], &w.bodies[1]
	attack := false
	for _, a := range w.possibleActions(nil, b) {
		attack = attack || a == Action{Kind: ActAttack, Mate: 1}
	}
	if !attack {
		t.Fatal("the beast is not offered the attack")
	}
	if k := w.huntTicks(1); k != 4+int(math.Ceil(1/w.speedOf(b))) {
		t.Fatalf("hunt ticks %d, want the walk of a tile plus four blows", k)
	}
	for _, a := range w.possibleActions(nil, h) {
		if a.Kind == ActAttack {
			t.Fatal("a human is offered an attack")
		}
	}
	// Walk up, then four blows.
	for i := 0; i < 20 && !adjacent(b, h); i++ {
		w.act(0, b, Action{Kind: ActAttack, Mate: 1})
	}
	if !adjacent(b, h) {
		t.Fatal("the beast never reached the human")
	}
	for i := 0; i < 3; i++ {
		w.act(0, b, Action{Kind: ActAttack, Mate: 1})
	}
	if h.Slain || h.Energy <= 0 || math.Abs(h.Energy-25) > 1e-9 {
		t.Fatalf("after three blows the human has %v energy, slain %v", h.Energy, h.Slain)
	}
	w.act(0, b, Action{Kind: ActAttack, Mate: 1})
	if !h.Slain || h.Slayer != 0 {
		t.Fatalf("after four blows: slain %v by %d", h.Slain, h.Slayer)
	}
	ht := w.tileOf(h.X, h.Y)
	w.removeDead()
	if len(w.bodies) != 1 || w.stats.Deaths[CausePreyed] != 1 || w.stats.Kills != 1 || w.stats.Blows != 4 {
		t.Fatalf("bodies %d, preyed %d, kills %d, blows %d", len(w.bodies), w.stats.Deaths[CausePreyed], w.stats.Kills, w.stats.Blows)
	}
	meat := 0
	for i, f := range w.food.foods {
		if f.ID == 2 {
			meat++
			if w.food.owner[i] != 0 {
				t.Fatalf("meat owned by %d, want the hunter 0", w.food.owner[i])
			}
			if abs(w.m.index(f.X, f.Y)%w.m.Width-ht%w.m.Width) > 1 || abs(w.m.index(f.X, f.Y)/w.m.Width-ht/w.m.Width) > 1 {
				t.Fatal("meat lies away from the kill")
			}
		}
	}
	if meat != cfg.MeatUnits {
		t.Fatalf("%d units of meat, want %d", meat, cfg.MeatUnits)
	}
	checkFood(t, w)
	if !w.FoodLedger().Balanced() {
		t.Fatalf("ledger: %+v", w.FoodLedger())
	}
	// The hunter eats it; a human would not see it.
	b = &w.bodies[0]
	other := Body{ID: 5, X: b.X, Y: b.Y, Species: 0}
	if w.foodFor(&other, ht) >= 0 {
		t.Fatal("a human sees the beast's meat")
	}
	if w.foodOn(ht) >= 0 && w.foodFor(b, ht) < 0 {
		t.Fatal("the hunter does not see its meat")
	}
	// Hunt off: no attack offered.
	w.cfg.Hunt = false
	w.bodies = append(w.bodies, human)
	w.buildGrid()
	for _, a := range w.possibleActions(nil, &w.bodies[0]) {
		if a.Kind == ActAttack {
			t.Fatal("with Hunt off the beast is offered an attack")
		}
	}
}
