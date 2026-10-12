package engine

import (
	"bytes"
	"math"
	"testing"
)

// testMap has water and more than one region, and the two layers cut across
// each other, so a save that dropped or mixed up a layer would show.
func testMap() Map {
	m := NewMap(12, 9)
	m.RegionFood = []float64{4, 1, 2, 1}
	for y := 0; y < m.Height; y++ {
		for x := 0; x < m.Width; x++ {
			m.SetRegion(x, y, RegionID(x/6+2*(y/5)))
		}
		m.SetTerrain(7, y, TerrainWater)
	}
	return m
}

// testConfig is the default rules scaled to the small test map, with one
// kind of food in one pool: the tests of kinds and pools (food_test.go)
// set the tables themselves, and every other test places food by hand
// for bodies of either species.
func testConfig(seed int64) Config {
	cfg := DefaultConfig()
	cfg.Seed = seed
	cfg.FoodCap = 30
	cfg.Bodies = 20
	cfg.FoodKinds, cfg.Foods = nil, nil
	cfg.Coins = false   // no coins until 11-2a; coin_test.go turns them on
	cfg.Ornaments = nil // no ornaments until 11-4; ornament_test.go turns them on
	return cfg
}

func newTestWorld(t testing.TB, seed int64) *World {
	t.Helper()
	w, err := NewWorld(testConfig(seed), testMap())
	if err != nil {
		t.Fatal(err)
	}
	return w
}

func run(w *World, ticks int) {
	for i := 0; i < ticks; i++ {
		w.Step()
	}
}

// The same seed and the same settings must give the same run.
func TestDeterminism(t *testing.T) {
	a, b := newTestWorld(t, 3), newTestWorld(t, 3)
	run(a, 500)
	run(b, 500)
	if a.Fingerprint() != b.Fingerprint() || a.Draws() != b.Draws() {
		t.Fatalf("same seed diverged: %x/%d vs %x/%d", a.Fingerprint(), a.Draws(), b.Fingerprint(), b.Draws())
	}
}

// A world saved and read back must run on exactly as if it had not been.
func TestSaveLoadContinues(t *testing.T) {
	a := newTestWorld(t, 11)
	run(a, 300)
	var buf bytes.Buffer
	if err := a.Save(&buf); err != nil {
		t.Fatal(err)
	}
	b, err := Load(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if b.Fingerprint() != a.Fingerprint() {
		t.Fatalf("loaded world differs before running: %x vs %x", b.Fingerprint(), a.Fingerprint())
	}
	run(a, 300)
	run(b, 300)
	if a.Fingerprint() != b.Fingerprint() || a.Draws() != b.Draws() {
		t.Fatalf("loaded world diverged: %x/%d vs %x/%d", b.Fingerprint(), b.Draws(), a.Fingerprint(), a.Draws())
	}
}

func TestLoadRejectsOtherVersion(t *testing.T) {
	if _, err := Load(bytes.NewBufferString(`{"version":999}`)); err == nil {
		t.Fatal("Load accepted an unknown snapshot version")
	}
}

// TestFingerprint pins the behaviour of the world for a fixed seed and run
// length. Speeding something up must not change it; a rule changed on purpose
// updates the value in the same commit.
func TestFingerprint(t *testing.T) {
	// Re-pinned at stage 9-7b-2a (a first body's species is drawn before
	// its tile, since the tile is drawn among its species' home regions)
	// and 9-7b-2b (settlements closed to other species), 9-7b-2c (bodies
	// may wade into water), 9-7b-3 (no tent pitched in water), 9-7b-4
	// (only humans have skills; water has depth), 10-1 (attack and
	// defense drawn), 10-2a (beasts hunt humans), 10-2b (prey read the
	// hunters they see), 10-2c (a body struck strikes back), 10-2d (a
	// hunted body sleeps in a settlement of its own), 10-2e (its kind
	// within reach strikes with it), 10-2f (a blow wakes a sleeper), 10-2g
	// (a hunter hunts only when hungry), 10-2h (a beast burns more on
	// land, which it does not like), 10-3 (the aggressive level drawn; a
	// hunter reads the odds) and 10-4 (a body struck calls its kind).
	const want = uint64(0x8da6d32b2723b433)
	w, err := NewWorld(testConfig(1), tempMap())
	if err != nil {
		t.Fatal(err)
	}
	run(w, 1000)
	if got := w.Fingerprint(); got != want {
		t.Fatalf("fingerprint = %#x, want %#x", got, want)
	}
}

// With a survival table for every exact most (FullStep 0), the world is
// stage 9-5b as measured: its fingerprint is what TestFingerprint pinned
// before the tables' cap was put on a grid (2026-10-07).
func TestExactIsStage95b(t *testing.T) {
	const want = uint64(0xcec5d53d0cd22b31)
	cfg := testConfig(1)
	cfg.AllotVenture = false
	cfg.Species = cfg.Species[:1]
	cfg.Wade = false
	cfg.AllotCombat = false
	cfg.AllotFierce = false
	cfg.FullStep = 0
	w, err := NewWorld(cfg, tempMap())
	if err != nil {
		t.Fatal(err)
	}
	run(w, 1000)
	if got := w.Fingerprint(); got != want {
		t.Fatalf("fingerprint = %#x, want %#x", got, want)
	}
}

// With one species, the world is stage 9-6a: its fingerprint is what
// TestFingerprint pinned before stage 9-7a.
// With settlements open to every species, the world is stage 9-7b-2a:
// its fingerprint is the one pinned then.
func TestOpenIsStage97b2a(t *testing.T) {
	const want = uint64(0x17f3ecdfc40c6510)
	cfg := testConfig(1)
	cfg.ClosedPlaces = false
	cfg.Wade = false
	cfg.AllotCombat = false
	cfg.AllotFierce = false
	cfg.DeepWater = false
	cfg.Hunt = false                      // no hunting until 10-2a
	cfg.TerrainBurn, cfg.AvoidBurn = 1, 1 // no terrain a species prefers until 10-2h
	cfg.Species = append([]SpeciesSpec(nil), cfg.Species...)
	cfg.Species[1].NoSkills = false // beasts had skills until 9-7b-4
	w, err := NewWorld(cfg, tempMap())
	if err != nil {
		t.Fatal(err)
	}
	run(w, 1000)
	if got := w.Fingerprint(); got != want {
		t.Fatalf("fingerprint = %#x, want %#x", got, want)
	}
}

func TestSingleIsStage96a(t *testing.T) {
	const want = uint64(0xeca10aba5b721dd9)
	cfg := testConfig(1)
	cfg.Species = cfg.Species[:1]
	cfg.Wade = false
	cfg.AllotCombat = false
	cfg.AllotFierce = false
	w, err := NewWorld(cfg, tempMap())
	if err != nil {
		t.Fatal(err)
	}
	run(w, 1000)
	if got := w.Fingerprint(); got != want {
		t.Fatalf("fingerprint = %#x, want %#x", got, want)
	}
}

// With the exploratory trait out of the genome, the world is stage 9-5b:
// its fingerprint is what TestFingerprint pinned before stage 9-6a.
func TestUnventuredIsStage95b(t *testing.T) {
	const want = uint64(0x3d3a3142c7fe1f)
	cfg := testConfig(1)
	cfg.AllotVenture = false
	cfg.Species = cfg.Species[:1]
	cfg.Wade = false
	cfg.AllotCombat = false
	cfg.AllotFierce = false
	w, err := NewWorld(cfg, tempMap())
	if err != nil {
		t.Fatal(err)
	}
	run(w, 1000)
	if got := w.Fingerprint(); got != want {
		t.Fatalf("fingerprint = %#x, want %#x", got, want)
	}
}

// With the size of the budget out of the genome, the world is stage 9-5a:
// its fingerprint is what TestFingerprint pinned before stage 9-5b.
func TestUnsizedIsStage95a(t *testing.T) {
	const want = uint64(0xb7f7a7323c817ef3)
	cfg := testConfig(1)
	cfg.AllotBudget = false
	cfg.AllotVenture = false
	cfg.Species = cfg.Species[:1]
	cfg.Wade = false
	cfg.AllotCombat = false
	cfg.AllotFierce = false
	cfg.FullStep = 0 // exact tables, as before 2026-10-07
	w, err := NewWorld(cfg, tempMap())
	if err != nil {
		t.Fatal(err)
	}
	run(w, 1000)
	if got := w.Fingerprint(); got != want {
		t.Fatalf("fingerprint = %#x, want %#x", got, want)
	}
}

// With every slot from the one drawn parent, the world is stage 9-4: its
// fingerprint is the one pinned then.
func TestUnmixedIsStage94(t *testing.T) {
	const want = uint64(0xacf99f0e85055c7)
	cfg := testConfig(1)
	cfg.AllotBudget = false
	cfg.AllotVenture = false
	cfg.Species = cfg.Species[:1]
	cfg.Wade = false
	cfg.AllotCombat = false
	cfg.AllotFierce = false
	cfg.FullStep = 0 // exact tables, as before 2026-10-07
	cfg.AllotMix = false
	w, err := NewWorld(cfg, tempMap())
	if err != nil {
		t.Fatal(err)
	}
	run(w, 1000)
	if got := w.Fingerprint(); got != want {
		t.Fatalf("fingerprint = %#x, want %#x", got, want)
	}
}

// Without fertility in the budget, the world is stage 9-3b: every mating
// agreed on makes a child, and its fingerprint is the one pinned then.
func TestUnfertileIsStage93b(t *testing.T) {
	const want = uint64(0x8ec1735068644be4)
	cfg := testConfig(1)
	cfg.AllotBudget = false
	cfg.AllotVenture = false
	cfg.Species = cfg.Species[:1]
	cfg.Wade = false
	cfg.AllotCombat = false
	cfg.AllotFierce = false
	cfg.FullStep = 0 // exact tables, as before 2026-10-07
	cfg.AllotMix, cfg.AllotFertility = false, false
	w, err := NewWorld(cfg, tempMap())
	if err != nil {
		t.Fatal(err)
	}
	run(w, 1000)
	if got := w.Fingerprint(); got != want {
		t.Fatalf("fingerprint = %#x, want %#x", got, want)
	}
}

// Without sleep, the world is stage 7-3a: its fingerprint is the one
// pinned then.
func TestSleeplessIsStage73a(t *testing.T) {
	const want = uint64(0x1d4e57530eb1be2)
	cfg := testConfig(1)
	cfg.AllotSight = false
	cfg.AllotFertility = false
	cfg.AllotBudget = false
	cfg.AllotVenture = false
	cfg.Species = cfg.Species[:1]
	cfg.Wade = false
	cfg.AllotCombat = false
	cfg.AllotFierce = false
	cfg.FullStep = 0 // exact tables, as before 2026-10-07
	cfg.ReuseNights, cfg.SeekBed = false, false
	cfg.Building, cfg.Tents = false, false
	cfg.ForceAt = 0
	cfg.DuskRead = false
	cfg.Sleep = false
	w, err := NewWorld(cfg, testMap())
	if err != nil {
		t.Fatal(err)
	}
	run(w, 1000)
	if got := w.Fingerprint(); got != want {
		t.Fatalf("fingerprint = %#x, want %#x", got, want)
	}
}

// With the valuation blind to sight, the world is stage 7-1b: its
// fingerprint is the one pinned then.
func TestSightblindIsStage71b(t *testing.T) {
	const want = uint64(0x409a3a08ad1e8f4d)
	cfg := testConfig(1)
	cfg.AllotSight = false
	cfg.AllotFertility = false
	cfg.AllotBudget = false
	cfg.AllotVenture = false
	cfg.Species = cfg.Species[:1]
	cfg.Wade = false
	cfg.AllotCombat = false
	cfg.AllotFierce = false
	cfg.FullStep = 0 // exact tables, as before 2026-10-07
	cfg.ReuseNights, cfg.SeekBed = false, false
	cfg.Building, cfg.Tents = false, false
	cfg.ForceAt = 0
	cfg.DuskRead = false
	cfg.SightRead, cfg.BurnRead, cfg.Sleep = false, false, false
	w, err := NewWorld(cfg, testMap())
	if err != nil {
		t.Fatal(err)
	}
	run(w, 1000)
	if got := w.Fingerprint(); got != want {
		t.Fatalf("fingerprint = %#x, want %#x", got, want)
	}
}

// A body feels its burn rise, smoothed over BurnSpan; one whose burn is not
// rising values at planBurn exactly; one whose burn rises values at the
// window's average burn, rounded up to a step of BurnStep; and a rising
// burn reads keeping on the move as riskier.
func TestBurnRead(t *testing.T) {
	w, b := soloWorld(t, 30)
	w.cfg.BurnRead, w.cfg.BurnSpan, w.cfg.BurnStep = true, 10, 1.05
	w.cfg.SleepAhead = false // a coming sleep would read every way as death (7-3e)
	base := w.planBurn(b)
	if got := w.readBurn(b); got != base {
		t.Fatalf("not rising: read %v, want %v", got, base)
	}
	w.feelBurn(b)
	b.Build.EnergyBurn = base * 1.5 // the burn jumps
	w.feelBurn(b)
	if want := base * 0.5 / 10; math.Abs(b.BurnRise-want) > 1e-12 {
		t.Fatalf("felt rise %v, want %v", b.BurnRise, want)
	}
	b.Build.EnergyBurn = 0
	b.BurnRise = 0.0001 // per tick: the window's average is +0.05 over 0.1
	ahead := base + 0.0001*float64(w.cfg.Window)/2
	got := w.readBurn(b)
	if got < ahead || got > ahead*1.05+1e-12 {
		t.Fatalf("rising: read %v, want a step of 1.05 at or above %v", got, ahead)
	}
	k := math.Log(got/base) / math.Log(1.05)
	if math.Abs(k-math.Round(k)) > 1e-9 {
		t.Fatalf("read %v is not a whole step above %v", got, base)
	}
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
	rising := risk()
	b.BurnRise = 0
	if steady := risk(); steady >= rising {
		t.Fatalf("waiting risk steady %v, rising %v: a rising burn should read riskier", steady, rising)
	}
	w.cfg.BurnRead = false
	b.BurnRise = 0.0001
	if got := w.readBurn(b); got != base {
		t.Fatalf("BurnRead off: read %v, want %v", got, base)
	}
}

// Keeping on the move reads 1 + 2*Sight*light tiles per tile crossed
// (NightSight 0), the tile stepped on alone without SightRead, and the
// estimate of a region is multiplied by it.
func TestSightGain(t *testing.T) {
	w, b := soloWorld(t, 50)
	w.cfg.Learn, w.cfg.SightRead, w.cfg.Sight, w.cfg.NightSight = true, true, 1, 0
	for light, want := range map[float64]float64{1: 3, 0.5: 2, 0: 1} {
		w.light, w.lightFor = light, w.tick+1
		if got := w.sightGainFor(w.cfg.Sight, w.Brightness()); math.Abs(got-want) > 1e-12 {
			t.Fatalf("light %v: gain %v, want %v", light, got, want)
		}
	}
	w.light = 1
	var rs rates
	w.ratesOf(b, &rs)
	lit := rs.region[0]
	w.cfg.SightRead = false
	if got := w.sightGainFor(w.cfg.Sight, w.Brightness()); got != 1 {
		t.Fatalf("gain %v without SightRead", got)
	}
	w.ratesOf(b, &rs)
	if math.Abs(lit-3*rs.region[0]) > 1e-12 && lit < 1 {
		t.Fatalf("region read %v in daylight, %v blind", lit, rs.region[0])
	}
}

// With no night, the world is stage 4-4: its fingerprint is the one pinned
// then.
func TestDaylongIsStage44(t *testing.T) {
	const want = uint64(0x34007a7f0ff94cb7)
	cfg := testConfig(1)
	cfg.AllotSight = false
	cfg.AllotFertility = false
	cfg.AllotBudget = false
	cfg.AllotVenture = false
	cfg.Species = cfg.Species[:1]
	cfg.Wade = false
	cfg.AllotCombat = false
	cfg.AllotFierce = false
	cfg.FullStep = 0 // exact tables, as before 2026-10-07
	cfg.ReuseNights, cfg.SeekBed = false, false
	cfg.Building, cfg.Tents = false, false
	cfg.ForceAt = 0
	cfg.DuskRead = false
	cfg.DayTicks, cfg.SightRead, cfg.BurnRead, cfg.Sleep = 0, false, false, false
	w, err := NewWorld(cfg, testMap())
	if err != nil {
		t.Fatal(err)
	}
	run(w, 1000)
	if got := w.Fingerprint(); got != want {
		t.Fatalf("fingerprint = %#x, want %#x", got, want)
	}
}

// With no food remembered, the world is stage 5-1: its fingerprint is the
// one pinned then.
func TestUnrecalledIsStage51(t *testing.T) {
	const want = uint64(0xa916798dcb31162b)
	cfg := testConfig(1)
	cfg.AllotSight = false
	cfg.AllotFertility = false
	cfg.AllotBudget = false
	cfg.AllotVenture = false
	cfg.Species = cfg.Species[:1]
	cfg.Wade = false
	cfg.AllotCombat = false
	cfg.AllotFierce = false
	cfg.FullStep = 0 // exact tables, as before 2026-10-07
	cfg.ReuseNights, cfg.SeekBed = false, false
	cfg.Building, cfg.Tents = false, false
	cfg.ForceAt = 0
	cfg.DuskRead = false
	cfg.Sleep = false     // before stage 7-3b
	cfg.BurnRead = false  // before stage 7-3a
	cfg.SightRead = false // before stage 7-2a
	cfg.DayTicks = 0      // before stage 7-1
	cfg.FoodLife = 0      // before stage 4-4
	cfg.SpotRead = 0
	w, err := NewWorld(cfg, testMap())
	if err != nil {
		t.Fatal(err)
	}
	run(w, 1000)
	if got := w.Fingerprint(); got != want {
		t.Fatalf("fingerprint = %#x, want %#x", got, want)
	}
}

// With evidence ageing by a half-life, the world is stage 4-1: its
// fingerprint is the one pinned then.
func TestHalflifeIsStage41(t *testing.T) {
	const want = uint64(0xec3dfcb8c3052cd6)
	cfg := testConfig(1)
	cfg.AllotSight = false
	cfg.AllotFertility = false
	cfg.AllotBudget = false
	cfg.AllotVenture = false
	cfg.Species = cfg.Species[:1]
	cfg.Wade = false
	cfg.AllotCombat = false
	cfg.AllotFierce = false
	cfg.FullStep = 0 // exact tables, as before 2026-10-07
	cfg.ReuseNights, cfg.SeekBed = false, false
	cfg.Building, cfg.Tents = false, false
	cfg.ForceAt = 0
	cfg.DuskRead = false
	cfg.Sleep = false     // before stage 7-3b
	cfg.BurnRead = false  // before stage 7-3a
	cfg.SightRead = false // before stage 7-2a
	cfg.DayTicks = 0      // before stage 7-1
	cfg.FoodLife = 0      // before stage 4-4
	cfg.SpotRead = 0      // before stage 5-2
	cfg.AgeBand = 0
	w, err := NewWorld(cfg, testMap())
	if err != nil {
		t.Fatal(err)
	}
	run(w, 1000)
	if got := w.Fingerprint(); got != want {
		t.Fatalf("fingerprint = %#x, want %#x", got, want)
	}
}

// With no body holding food, the world is stage 4-0: its fingerprint is
// the one pinned then.
func TestUnheldIsStage40(t *testing.T) {
	const want = uint64(0x6ef3c0d651de9b21)
	cfg := testConfig(1)
	cfg.AllotSight = false
	cfg.AllotFertility = false
	cfg.AllotBudget = false
	cfg.AllotVenture = false
	cfg.Species = cfg.Species[:1]
	cfg.Wade = false
	cfg.AllotCombat = false
	cfg.AllotFierce = false
	cfg.FullStep = 0 // exact tables, as before 2026-10-07
	cfg.ReuseNights, cfg.SeekBed = false, false
	cfg.Building, cfg.Tents = false, false
	cfg.ForceAt = 0
	cfg.DuskRead = false
	cfg.Sleep = false     // before stage 7-3b
	cfg.BurnRead = false  // before stage 7-3a
	cfg.SightRead = false // before stage 7-2a
	cfg.DayTicks = 0      // before stage 7-1
	cfg.FoodLife = 0      // before stage 4-4
	cfg.SpotRead = 0      // before stage 5-2
	cfg.AgeBand = 0       // before stage 5-1
	cfg.Carry = 0
	w, err := NewWorld(cfg, testMap())
	if err != nil {
		t.Fatal(err)
	}
	run(w, 1000)
	if got := w.Fingerprint(); got != want {
		t.Fatalf("fingerprint = %#x, want %#x", got, want)
	}
}

// With resting mothers reading their whole window at their resting burn,
// the world is stage 3-8: its fingerprint is the one pinned then.
func TestRestblindIsStage38(t *testing.T) {
	const want = uint64(0x73bc7c20c5e30753)
	cfg := testConfig(1)
	cfg.AllotSight = false
	cfg.AllotFertility = false
	cfg.AllotBudget = false
	cfg.AllotVenture = false
	cfg.Species = cfg.Species[:1]
	cfg.Wade = false
	cfg.AllotCombat = false
	cfg.AllotFierce = false
	cfg.FullStep = 0 // exact tables, as before 2026-10-07
	cfg.ReuseNights, cfg.SeekBed = false, false
	cfg.Building, cfg.Tents = false, false
	cfg.ForceAt = 0
	cfg.DuskRead = false
	cfg.Sleep = false     // before stage 7-3b
	cfg.BurnRead = false  // before stage 7-3a
	cfg.SightRead = false // before stage 7-2a
	cfg.DayTicks = 0      // before stage 7-1
	cfg.FoodLife = 0      // before stage 4-4
	cfg.SpotRead = 0      // before stage 5-2
	cfg.AgeBand = 0       // before stage 5-1
	cfg.Carry = 0         // before stage 4-1
	cfg.RestAhead = false
	w, err := NewWorld(cfg, testMap())
	if err != nil {
		t.Fatal(err)
	}
	run(w, 1000)
	if got := w.Fingerprint(); got != want {
		t.Fatalf("fingerprint = %#x, want %#x", got, want)
	}
}

// Without requests to mate, the world is stage 3-7: its fingerprint is the
// one pinned then.
func TestUnaskedIsStage37(t *testing.T) {
	const want = uint64(0x8b58dcb766926bae)
	cfg := testConfig(1)
	cfg.AllotSight = false
	cfg.AllotFertility = false
	cfg.AllotBudget = false
	cfg.AllotVenture = false
	cfg.Species = cfg.Species[:1]
	cfg.Wade = false
	cfg.AllotCombat = false
	cfg.AllotFierce = false
	cfg.FullStep = 0 // exact tables, as before 2026-10-07
	cfg.ReuseNights, cfg.SeekBed = false, false
	cfg.Building, cfg.Tents = false, false
	cfg.ForceAt = 0
	cfg.DuskRead = false
	cfg.Sleep = false     // before stage 7-3b
	cfg.BurnRead = false  // before stage 7-3a
	cfg.SightRead = false // before stage 7-2a
	cfg.DayTicks = 0      // before stage 7-1
	cfg.FoodLife = 0      // before stage 4-4
	cfg.SpotRead = 0      // before stage 5-2
	cfg.AgeBand = 0       // before stage 5-1
	cfg.Carry = 0         // before stage 4-1
	cfg.RestAhead = false // before stage 4-0
	cfg.Requests = false
	w, err := NewWorld(cfg, testMap())
	if err != nil {
		t.Fatal(err)
	}
	run(w, 1000)
	if got := w.Fingerprint(); got != want {
		t.Fatalf("fingerprint = %#x, want %#x", got, want)
	}
}

// With abilities the same at every age, the world is stage 3-6: its
// fingerprint is the one pinned then.
func TestUnagedIsStage36(t *testing.T) {
	const want = uint64(0x65bf2b7d036f7a4a)
	cfg := testConfig(1)
	cfg.AllotSight = false
	cfg.AllotFertility = false
	cfg.AllotBudget = false
	cfg.AllotVenture = false
	cfg.Species = cfg.Species[:1]
	cfg.Wade = false
	cfg.AllotCombat = false
	cfg.AllotFierce = false
	cfg.FullStep = 0 // exact tables, as before 2026-10-07
	cfg.ReuseNights, cfg.SeekBed = false, false
	cfg.Building, cfg.Tents = false, false
	cfg.ForceAt = 0
	cfg.DuskRead = false
	cfg.Sleep = false     // before stage 7-3b
	cfg.BurnRead = false  // before stage 7-3a
	cfg.SightRead = false // before stage 7-2a
	cfg.DayTicks = 0      // before stage 7-1
	cfg.FoodLife = 0      // before stage 4-4
	cfg.SpotRead = 0      // before stage 5-2
	cfg.AgeBand = 0       // before stage 5-1
	cfg.Carry = 0         // before stage 4-1
	cfg.RestAhead = false // before stage 4-0
	cfg.Requests = false  // before stage M-3
	cfg.ChildAbility = 1
	w, err := NewWorld(cfg, testMap())
	if err != nil {
		t.Fatal(err)
	}
	run(w, 1000)
	if got := w.Fingerprint(); got != want {
		t.Fatalf("fingerprint = %#x, want %#x", got, want)
	}
}

// With children coming of age at 1000 ticks, the world is stage 3-5b (and
// 3-5: the test world runs short of either lifespan): its fingerprint is
// the one pinned then.
func TestSlowgrowIsStage35b(t *testing.T) {
	const want = uint64(0x96b92ba04979efda)
	cfg := testConfig(1)
	cfg.AllotSight = false
	cfg.AllotFertility = false
	cfg.AllotBudget = false
	cfg.AllotVenture = false
	cfg.Species = cfg.Species[:1]
	cfg.Wade = false
	cfg.AllotCombat = false
	cfg.AllotFierce = false
	cfg.FullStep = 0 // exact tables, as before 2026-10-07
	cfg.ReuseNights, cfg.SeekBed = false, false
	cfg.Building, cfg.Tents = false, false
	cfg.ForceAt = 0
	cfg.DuskRead = false
	cfg.Sleep = false     // before stage 7-3b
	cfg.BurnRead = false  // before stage 7-3a
	cfg.SightRead = false // before stage 7-2a
	cfg.DayTicks = 0      // before stage 7-1
	cfg.FoodLife = 0      // before stage 4-4
	cfg.SpotRead = 0      // before stage 5-2
	cfg.AgeBand = 0       // before stage 5-1
	cfg.Carry = 0         // before stage 4-1
	cfg.RestAhead = false // before stage 4-0
	cfg.Requests = false  // before stage M-3
	cfg.ChildAbility = 1  // before stage 3-7
	cfg.MatureAge = 1000
	w, err := NewWorld(cfg, testMap())
	if err != nil {
		t.Fatal(err)
	}
	run(w, 1000)
	if got := w.Fingerprint(); got != want {
		t.Fatalf("fingerprint = %#x, want %#x", got, want)
	}
}

// Without a lifespan, the world is stage 3-4: its fingerprint is the one
// pinned then.
func TestAgelessIsStage34(t *testing.T) {
	const want = uint64(0x2e11cd59bbea95ba)
	cfg := testConfig(1)
	cfg.AllotSight = false
	cfg.AllotFertility = false
	cfg.AllotBudget = false
	cfg.AllotVenture = false
	cfg.Species = cfg.Species[:1]
	cfg.Wade = false
	cfg.AllotCombat = false
	cfg.AllotFierce = false
	cfg.FullStep = 0 // exact tables, as before 2026-10-07
	cfg.ReuseNights, cfg.SeekBed = false, false
	cfg.Building, cfg.Tents = false, false
	cfg.ForceAt = 0
	cfg.DuskRead = false
	cfg.Sleep = false     // before stage 7-3b
	cfg.BurnRead = false  // before stage 7-3a
	cfg.SightRead = false // before stage 7-2a
	cfg.DayTicks = 0      // before stage 7-1
	cfg.FoodLife = 0      // before stage 4-4
	cfg.SpotRead = 0      // before stage 5-2
	cfg.AgeBand = 0       // before stage 5-1
	cfg.Carry = 0         // before stage 4-1
	cfg.RestAhead = false // before stage 4-0
	cfg.Requests = false  // before stage M-3
	cfg.ChildAbility = 1  // before stage 3-7
	cfg.MatureAge = 1000  // before stage 3-6
	cfg.Lifespan = 0
	w, err := NewWorld(cfg, testMap())
	if err != nil {
		t.Fatal(err)
	}
	run(w, 1000)
	if got := w.Fingerprint(); got != want {
		t.Fatalf("fingerprint = %#x, want %#x", got, want)
	}
}

// With resting mothers burning as any body does, the world is stage 3-3d:
// its fingerprint is the one pinned then.
func TestBusyIsStage33d(t *testing.T) {
	const want = uint64(0xf5894766a5495f52)
	cfg := testConfig(1)
	cfg.AllotSight = false
	cfg.AllotFertility = false
	cfg.AllotBudget = false
	cfg.AllotVenture = false
	cfg.Species = cfg.Species[:1]
	cfg.Wade = false
	cfg.AllotCombat = false
	cfg.AllotFierce = false
	cfg.FullStep = 0 // exact tables, as before 2026-10-07
	cfg.ReuseNights, cfg.SeekBed = false, false
	cfg.Building, cfg.Tents = false, false
	cfg.ForceAt = 0
	cfg.DuskRead = false
	cfg.Sleep = false     // before stage 7-3b
	cfg.BurnRead = false  // before stage 7-3a
	cfg.SightRead = false // before stage 7-2a
	cfg.DayTicks = 0      // before stage 7-1
	cfg.FoodLife = 0      // before stage 4-4
	cfg.SpotRead = 0      // before stage 5-2
	cfg.AgeBand = 0       // before stage 5-1
	cfg.Carry = 0         // before stage 4-1
	cfg.Requests = false  // before stage M-3
	cfg.ChildAbility = 1  // before stage 3-7
	cfg.MatureAge = 1000  // before stage 3-6
	cfg.Lifespan = 0      // before stage 3-5
	cfg.RestBurn = 1
	w, err := NewWorld(cfg, testMap())
	if err != nil {
		t.Fatal(err)
	}
	run(w, 1000)
	if got := w.Fingerprint(); got != want {
		t.Fatalf("fingerprint = %#x, want %#x", got, want)
	}
}

// Asking for the birth's cost and the rest's burn, the world is stage 3-3c:
// its fingerprint is the one pinned then.
func TestDoubledIsStage33c(t *testing.T) {
	const want = uint64(0xd4656f63a57a704b)
	cfg := testConfig(1)
	cfg.AllotSight = false
	cfg.AllotFertility = false
	cfg.AllotBudget = false
	cfg.AllotVenture = false
	cfg.Species = cfg.Species[:1]
	cfg.Wade = false
	cfg.AllotCombat = false
	cfg.AllotFierce = false
	cfg.FullStep = 0 // exact tables, as before 2026-10-07
	cfg.ReuseNights, cfg.SeekBed = false, false
	cfg.Building, cfg.Tents = false, false
	cfg.ForceAt = 0
	cfg.DuskRead = false
	cfg.Sleep = false     // before stage 7-3b
	cfg.BurnRead = false  // before stage 7-3a
	cfg.SightRead = false // before stage 7-2a
	cfg.DayTicks = 0      // before stage 7-1
	cfg.FoodLife = 0      // before stage 4-4
	cfg.SpotRead = 0      // before stage 5-2
	cfg.AgeBand = 0       // before stage 5-1
	cfg.Carry = 0         // before stage 4-1
	cfg.Requests = false  // before stage M-3
	cfg.ChildAbility = 1  // before stage 3-7
	cfg.MatureAge = 1000  // before stage 3-6
	cfg.Lifespan = 0      // before stage 3-5
	cfg.RestBurn = 1      // before stage 3-4
	cfg.ProvisionEachWith, cfg.ProvisionEachAlone = 0.0075, 0.00375
	w, err := NewWorld(cfg, testMap())
	if err != nil {
		t.Fatal(err)
	}
	run(w, 1000)
	if got := w.Fingerprint(); got != want {
		t.Fatalf("fingerprint = %#x, want %#x", got, want)
	}
}

// Asking for the birth's cost only, the world is stage 3-3b: its
// fingerprint is the one pinned then.
func TestLeanIsStage33b(t *testing.T) {
	const want = uint64(0xfb7c99516ff15951)
	cfg := testConfig(1)
	cfg.AllotSight = false
	cfg.AllotFertility = false
	cfg.AllotBudget = false
	cfg.AllotVenture = false
	cfg.Species = cfg.Species[:1]
	cfg.Wade = false
	cfg.AllotCombat = false
	cfg.AllotFierce = false
	cfg.FullStep = 0 // exact tables, as before 2026-10-07
	cfg.ReuseNights, cfg.SeekBed = false, false
	cfg.Building, cfg.Tents = false, false
	cfg.ForceAt = 0
	cfg.DuskRead = false
	cfg.Sleep = false     // before stage 7-3b
	cfg.BurnRead = false  // before stage 7-3a
	cfg.SightRead = false // before stage 7-2a
	cfg.DayTicks = 0      // before stage 7-1
	cfg.FoodLife = 0      // before stage 4-4
	cfg.SpotRead = 0      // before stage 5-2
	cfg.AgeBand = 0       // before stage 5-1
	cfg.Carry = 0         // before stage 4-1
	cfg.Requests = false  // before stage M-3
	cfg.ChildAbility = 1  // before stage 3-7
	cfg.MatureAge = 1000  // before stage 3-6
	cfg.Lifespan = 0      // before stage 3-5
	cfg.RestBurn = 1      // before stage 3-4
	cfg.ProvisionEachWith, cfg.ProvisionEachAlone = 0.00375, 0.001875
	w, err := NewWorld(cfg, testMap())
	if err != nil {
		t.Fatal(err)
	}
	run(w, 1000)
	if got := w.Fingerprint(); got != want {
		t.Fatalf("fingerprint = %#x, want %#x", got, want)
	}
}

// Provisioning by the unit, the world is stage 3-3: its fingerprint is the
// one pinned then.
func TestPerunitIsStage33(t *testing.T) {
	const want = uint64(0x160f88aa4a56bb14)
	cfg := testConfig(1)
	cfg.AllotSight = false
	cfg.AllotFertility = false
	cfg.AllotBudget = false
	cfg.AllotVenture = false
	cfg.Species = cfg.Species[:1]
	cfg.Wade = false
	cfg.AllotCombat = false
	cfg.AllotFierce = false
	cfg.FullStep = 0 // exact tables, as before 2026-10-07
	cfg.ReuseNights, cfg.SeekBed = false, false
	cfg.Building, cfg.Tents = false, false
	cfg.ForceAt = 0
	cfg.DuskRead = false
	cfg.Sleep = false         // before stage 7-3b
	cfg.BurnRead = false      // before stage 7-3a
	cfg.SightRead = false     // before stage 7-2a
	cfg.DayTicks = 0          // before stage 7-1
	cfg.FoodLife = 0          // before stage 4-4
	cfg.SpotRead = 0          // before stage 5-2
	cfg.AgeBand = 0           // before stage 5-1
	cfg.Carry = 0             // before stage 4-1
	cfg.Requests = false      // before stage M-3
	cfg.ChildAbility = 1      // before stage 3-7
	cfg.MatureAge = 1000      // before stage 3-6
	cfg.Lifespan = 0          // before stage 3-5
	cfg.RestBurn = 1          // before stage 3-4
	cfg.ProvisionEach = false // ProvisionEachWith and Alone unread
	w, err := NewWorld(cfg, testMap())
	if err != nil {
		t.Fatal(err)
	}
	run(w, 1000)
	if got := w.Fingerprint(); got != want {
		t.Fatalf("fingerprint = %#x, want %#x", got, want)
	}
}

// Without provisioning, the world is stage 3-2: its fingerprint is the one
// pinned then.
func TestUnfedIsStage32(t *testing.T) {
	const want = uint64(0x2cdcb2147b3e2cfc)
	cfg := testConfig(1)
	cfg.AllotSight = false
	cfg.AllotFertility = false
	cfg.AllotBudget = false
	cfg.AllotVenture = false
	cfg.Species = cfg.Species[:1]
	cfg.Wade = false
	cfg.AllotCombat = false
	cfg.AllotFierce = false
	cfg.FullStep = 0 // exact tables, as before 2026-10-07
	cfg.ReuseNights, cfg.SeekBed = false, false
	cfg.Building, cfg.Tents = false, false
	cfg.ForceAt = 0
	cfg.DuskRead = false
	cfg.Sleep = false     // before stage 7-3b
	cfg.BurnRead = false  // before stage 7-3a
	cfg.SightRead = false // before stage 7-2a
	cfg.DayTicks = 0      // before stage 7-1
	cfg.FoodLife = 0      // before stage 4-4
	cfg.SpotRead = 0      // before stage 5-2
	cfg.AgeBand = 0       // before stage 5-1
	cfg.Carry = 0         // before stage 4-1
	cfg.Requests = false  // before stage M-3
	cfg.ChildAbility = 1  // before stage 3-7
	cfg.MatureAge = 1000  // before stage 3-6
	cfg.Lifespan = 0      // before stage 3-5
	cfg.RestBurn = 1      // before stage 3-4
	cfg.Provision = false
	w, err := NewWorld(cfg, testMap())
	if err != nil {
		t.Fatal(err)
	}
	run(w, 1000)
	if got := w.Fingerprint(); got != want {
		t.Fatalf("fingerprint = %#x, want %#x", got, want)
	}
}

// With each parent paying half, the world is stage 3-1: its fingerprint is
// the one pinned then.
func TestEvenIsStage31(t *testing.T) {
	const want = uint64(0x5af7133341342c88)
	cfg := testConfig(1)
	cfg.AllotSight = false
	cfg.AllotFertility = false
	cfg.AllotBudget = false
	cfg.AllotVenture = false
	cfg.Species = cfg.Species[:1]
	cfg.Wade = false
	cfg.AllotCombat = false
	cfg.AllotFierce = false
	cfg.FullStep = 0 // exact tables, as before 2026-10-07
	cfg.ReuseNights, cfg.SeekBed = false, false
	cfg.Building, cfg.Tents = false, false
	cfg.ForceAt = 0
	cfg.DuskRead = false
	cfg.Sleep = false     // before stage 7-3b
	cfg.BurnRead = false  // before stage 7-3a
	cfg.SightRead = false // before stage 7-2a
	cfg.DayTicks = 0      // before stage 7-1
	cfg.FoodLife = 0      // before stage 4-4
	cfg.SpotRead = 0      // before stage 5-2
	cfg.AgeBand = 0       // before stage 5-1
	cfg.Carry = 0         // before stage 4-1
	cfg.Requests = false  // before stage M-3
	cfg.ChildAbility = 1  // before stage 3-7
	cfg.MatureAge = 1000  // before stage 3-6
	cfg.Lifespan = 0      // before stage 3-5
	cfg.RestBurn = 1      // before stage 3-4
	cfg.FemaleBears = false
	w, err := NewWorld(cfg, testMap())
	if err != nil {
		t.Fatal(err)
	}
	run(w, 1000)
	if got := w.Fingerprint(); got != want {
		t.Fatalf("fingerprint = %#x, want %#x", got, want)
	}
}

// Without sexes, the world is stage 2-4: its fingerprint is the one pinned
// then.
func TestSexlessIsStage24(t *testing.T) {
	const want = uint64(0xe11e99c952b0c044)
	cfg := testConfig(1)
	cfg.AllotSight = false
	cfg.AllotFertility = false
	cfg.AllotBudget = false
	cfg.AllotVenture = false
	cfg.Species = cfg.Species[:1]
	cfg.Wade = false
	cfg.AllotCombat = false
	cfg.AllotFierce = false
	cfg.FullStep = 0 // exact tables, as before 2026-10-07
	cfg.ReuseNights, cfg.SeekBed = false, false
	cfg.Building, cfg.Tents = false, false
	cfg.ForceAt = 0
	cfg.DuskRead = false
	cfg.Sleep = false     // before stage 7-3b
	cfg.BurnRead = false  // before stage 7-3a
	cfg.SightRead = false // before stage 7-2a
	cfg.DayTicks = 0      // before stage 7-1
	cfg.FoodLife = 0      // before stage 4-4
	cfg.SpotRead = 0      // before stage 5-2
	cfg.AgeBand = 0       // before stage 5-1
	cfg.Carry = 0         // before stage 4-1
	cfg.Requests = false  // before stage M-3
	cfg.ChildAbility = 1  // before stage 3-7
	cfg.MatureAge = 1000  // before stage 3-6
	cfg.Lifespan = 0      // before stage 3-5
	cfg.Sexes = false
	w, err := NewWorld(cfg, testMap())
	if err != nil {
		t.Fatal(err)
	}
	run(w, 1000)
	if got := w.Fingerprint(); got != want {
		t.Fatalf("fingerprint = %#x, want %#x", got, want)
	}
}

// Passing the path row, the world is stage 2-3: its fingerprint is the one
// pinned then.
func TestAgedIsStage23(t *testing.T) {
	const want = uint64(0x183d565350053ffe)
	cfg := testConfig(1)
	cfg.AllotSight = false
	cfg.AllotFertility = false
	cfg.AllotBudget = false
	cfg.AllotVenture = false
	cfg.Species = cfg.Species[:1]
	cfg.Wade = false
	cfg.AllotCombat = false
	cfg.AllotFierce = false
	cfg.FullStep = 0 // exact tables, as before 2026-10-07
	cfg.ReuseNights, cfg.SeekBed = false, false
	cfg.Building, cfg.Tents = false, false
	cfg.ForceAt = 0
	cfg.DuskRead = false
	cfg.Sleep = false     // before stage 7-3b
	cfg.BurnRead = false  // before stage 7-3a
	cfg.SightRead = false // before stage 7-2a
	cfg.DayTicks = 0      // before stage 7-1
	cfg.FoodLife = 0      // before stage 4-4
	cfg.SpotRead = 0      // before stage 5-2
	cfg.AgeBand = 0       // before stage 5-1
	cfg.Carry = 0         // before stage 4-1
	cfg.Requests = false  // before stage M-3
	cfg.ChildAbility = 1  // before stage 3-7
	cfg.MatureAge = 1000  // before stage 3-6
	cfg.Lifespan = 0      // before stage 3-5
	cfg.Sexes = false     // before stage 3-1
	cfg.PassPath = true
	w, err := NewWorld(cfg, testMap())
	if err != nil {
		t.Fatal(err)
	}
	run(w, 1000)
	if got := w.Fingerprint(); got != want {
		t.Fatalf("fingerprint = %#x, want %#x", got, want)
	}
}

// With the path row never ageing, the world is stage 2-2: its fingerprint
// is the one pinned then.
func TestKinIsStage22(t *testing.T) {
	const want = uint64(0xb5fb25e4363e7f6c)
	cfg := testConfig(1)
	cfg.AllotSight = false
	cfg.AllotFertility = false
	cfg.AllotBudget = false
	cfg.AllotVenture = false
	cfg.Species = cfg.Species[:1]
	cfg.Wade = false
	cfg.AllotCombat = false
	cfg.AllotFierce = false
	cfg.FullStep = 0 // exact tables, as before 2026-10-07
	cfg.ReuseNights, cfg.SeekBed = false, false
	cfg.Building, cfg.Tents = false, false
	cfg.ForceAt = 0
	cfg.DuskRead = false
	cfg.Sleep = false     // before stage 7-3b
	cfg.BurnRead = false  // before stage 7-3a
	cfg.SightRead = false // before stage 7-2a
	cfg.DayTicks = 0      // before stage 7-1
	cfg.FoodLife = 0      // before stage 4-4
	cfg.SpotRead = 0      // before stage 5-2
	cfg.AgeBand = 0       // before stage 5-1
	cfg.Carry = 0         // before stage 4-1
	cfg.Requests = false  // before stage M-3
	cfg.ChildAbility = 1  // before stage 3-7
	cfg.MatureAge = 1000  // before stage 3-6
	cfg.Lifespan = 0      // before stage 3-5
	cfg.Sexes = false     // before stage 3-1
	cfg.AgePath, cfg.PassPath = false, true
	w, err := NewWorld(cfg, testMap())
	if err != nil {
		t.Fatal(err)
	}
	run(w, 1000)
	if got := w.Fingerprint(); got != want {
		t.Fatalf("fingerprint = %#x, want %#x", got, want)
	}
}

// Passing to everyone met, the world is the third run of stage 2-1: its
// fingerprint is the one pinned then.
func TestNearIsThird21(t *testing.T) {
	const want = uint64(0xd32d3dc3d4a4c1c3)
	cfg := testConfig(1)
	cfg.AllotSight = false
	cfg.AllotFertility = false
	cfg.AllotBudget = false
	cfg.AllotVenture = false
	cfg.Species = cfg.Species[:1]
	cfg.Wade = false
	cfg.AllotCombat = false
	cfg.AllotFierce = false
	cfg.FullStep = 0 // exact tables, as before 2026-10-07
	cfg.ReuseNights, cfg.SeekBed = false, false
	cfg.Building, cfg.Tents = false, false
	cfg.ForceAt = 0
	cfg.DuskRead = false
	cfg.Sleep = false     // before stage 7-3b
	cfg.BurnRead = false  // before stage 7-3a
	cfg.SightRead = false // before stage 7-2a
	cfg.DayTicks = 0      // before stage 7-1
	cfg.FoodLife = 0      // before stage 4-4
	cfg.SpotRead = 0      // before stage 5-2
	cfg.AgeBand = 0       // before stage 5-1
	cfg.Carry = 0         // before stage 4-1
	cfg.Requests = false  // before stage M-3
	cfg.ChildAbility = 1  // before stage 3-7
	cfg.MatureAge = 1000  // before stage 3-6
	cfg.Lifespan = 0      // before stage 3-5
	cfg.Sexes = false     // before stage 3-1
	cfg.Kin, cfg.AgePath, cfg.PassPath = false, false, true
	w, err := NewWorld(cfg, testMap())
	if err != nil {
		t.Fatal(err)
	}
	run(w, 1000)
	if got := w.Fingerprint(); got != want {
		t.Fatalf("fingerprint = %#x, want %#x", got, want)
	}
}

// Passing and ageing every row alike, the world is the second run of stage
// 2-1: its fingerprint is the one pinned then.
func TestMixedIsSecond21(t *testing.T) {
	const want = uint64(0xe603a1fde7c5a398)
	cfg := testConfig(1)
	cfg.AllotSight = false
	cfg.AllotFertility = false
	cfg.AllotBudget = false
	cfg.AllotVenture = false
	cfg.Species = cfg.Species[:1]
	cfg.Wade = false
	cfg.AllotCombat = false
	cfg.AllotFierce = false
	cfg.FullStep = 0 // exact tables, as before 2026-10-07
	cfg.ReuseNights, cfg.SeekBed = false, false
	cfg.Building, cfg.Tents = false, false
	cfg.ForceAt = 0
	cfg.DuskRead = false
	cfg.Sleep = false     // before stage 7-3b
	cfg.BurnRead = false  // before stage 7-3a
	cfg.SightRead = false // before stage 7-2a
	cfg.DayTicks = 0      // before stage 7-1
	cfg.FoodLife = 0      // before stage 4-4
	cfg.SpotRead = 0      // before stage 5-2
	cfg.AgeBand = 0       // before stage 5-1
	cfg.Carry = 0         // before stage 4-1
	cfg.Requests = false  // before stage M-3
	cfg.ChildAbility = 1  // before stage 3-7
	cfg.MatureAge = 1000  // before stage 3-6
	cfg.Lifespan = 0      // before stage 3-5
	cfg.Sexes = false     // before stage 3-1
	cfg.StableRows, cfg.Kin, cfg.PassPath = false, false, true
	w, err := NewWorld(cfg, testMap())
	if err != nil {
		t.Fatal(err)
	}
	run(w, 1000)
	if got := w.Fingerprint(); got != want {
		t.Fatalf("fingerprint = %#x, want %#x", got, want)
	}
}

// With evidence at full weight however old, the world is the first run of
// stage 2-1: its fingerprint is the one pinned then.
func TestUndecayedIsFirst21(t *testing.T) {
	const want = uint64(0x4fc09ac1b5ef5f0a)
	cfg := testConfig(1)
	cfg.AllotSight = false
	cfg.AllotFertility = false
	cfg.AllotBudget = false
	cfg.AllotVenture = false
	cfg.Species = cfg.Species[:1]
	cfg.Wade = false
	cfg.AllotCombat = false
	cfg.AllotFierce = false
	cfg.FullStep = 0 // exact tables, as before 2026-10-07
	cfg.ReuseNights, cfg.SeekBed = false, false
	cfg.Building, cfg.Tents = false, false
	cfg.ForceAt = 0
	cfg.DuskRead = false
	cfg.Sleep = false     // before stage 7-3b
	cfg.BurnRead = false  // before stage 7-3a
	cfg.SightRead = false // before stage 7-2a
	cfg.DayTicks = 0      // before stage 7-1
	cfg.FoodLife = 0      // before stage 4-4
	cfg.SpotRead = 0      // before stage 5-2
	cfg.AgeBand = 0       // before stage 5-1
	cfg.Carry = 0         // before stage 4-1
	cfg.Requests = false  // before stage M-3
	cfg.ChildAbility = 1  // before stage 3-7
	cfg.MatureAge = 1000  // before stage 3-6
	cfg.Lifespan = 0      // before stage 3-5
	cfg.Sexes = false     // before stage 3-1
	cfg.EvidenceHalfLife, cfg.StableRows, cfg.Kin, cfg.PassPath = 0, false, false, true
	w, err := NewWorld(cfg, testMap())
	if err != nil {
		t.Fatal(err)
	}
	run(w, 1000)
	if got := w.Fingerprint(); got != want {
		t.Fatalf("fingerprint = %#x, want %#x", got, want)
	}
}

// Without passing evidence, the world is stage 2-0: its fingerprint is the
// one pinned then.
func TestUntoldIsStage20(t *testing.T) {
	const want = uint64(0x75682f5e7fe91d62)
	cfg := testConfig(1)
	cfg.AllotSight = false
	cfg.AllotFertility = false
	cfg.AllotBudget = false
	cfg.AllotVenture = false
	cfg.Species = cfg.Species[:1]
	cfg.Wade = false
	cfg.AllotCombat = false
	cfg.AllotFierce = false
	cfg.FullStep = 0 // exact tables, as before 2026-10-07
	cfg.ReuseNights, cfg.SeekBed = false, false
	cfg.Building, cfg.Tents = false, false
	cfg.ForceAt = 0
	cfg.DuskRead = false
	cfg.Sleep = false     // before stage 7-3b
	cfg.BurnRead = false  // before stage 7-3a
	cfg.SightRead = false // before stage 7-2a
	cfg.DayTicks = 0      // before stage 7-1
	cfg.FoodLife = 0      // before stage 4-4
	cfg.SpotRead = 0      // before stage 5-2
	cfg.AgeBand = 0       // before stage 5-1
	cfg.Carry = 0         // before stage 4-1
	cfg.Requests = false  // before stage M-3
	cfg.ChildAbility = 1  // before stage 3-7
	cfg.MatureAge = 1000  // before stage 3-6
	cfg.Lifespan = 0      // before stage 3-5
	cfg.Sexes = false     // before stage 3-1
	cfg.Tell, cfg.EvidenceHalfLife, cfg.StableRows = false, 0, false
	w, err := NewWorld(cfg, testMap())
	if err != nil {
		t.Fatal(err)
	}
	run(w, 1000)
	if got := w.Fingerprint(); got != want {
		t.Fatalf("fingerprint = %#x, want %#x", got, want)
	}
}

// Reading the truth table, the world is the one before stage 1-6: its
// fingerprint is the one pinned then.
func TestTruthIsBeforeStage16(t *testing.T) {
	const want = uint64(0xb8ed150c7cbf824b)
	cfg := testConfig(1)
	cfg.AllotSight = false
	cfg.AllotFertility = false
	cfg.AllotBudget = false
	cfg.AllotVenture = false
	cfg.Species = cfg.Species[:1]
	cfg.Wade = false
	cfg.AllotCombat = false
	cfg.AllotFierce = false
	cfg.FullStep = 0 // exact tables, as before 2026-10-07
	cfg.ReuseNights, cfg.SeekBed = false, false
	cfg.Building, cfg.Tents = false, false
	cfg.Sleep = false     // before stage 7-3b
	cfg.BurnRead = false  // before stage 7-3a
	cfg.SightRead = false // before stage 7-2a
	cfg.DayTicks = 0      // before stage 7-1
	cfg.FoodLife = 0      // before stage 4-4
	cfg.SpotRead = 0      // before stage 5-2
	cfg.AgeBand = 0       // before stage 5-1
	cfg.Carry = 0         // before stage 4-1
	cfg.Requests = false  // before stage M-3
	cfg.ChildAbility = 1  // before stage 3-7
	cfg.MatureAge = 1000  // before stage 3-6
	cfg.Lifespan = 0      // before stage 3-5
	cfg.Sexes = false     // before stage 3-1
	cfg.Learn = false
	w, err := NewWorld(cfg, testMap())
	if err != nil {
		t.Fatal(err)
	}
	run(w, 1000)
	if got := w.Fingerprint(); got != want {
		t.Fatalf("fingerprint = %#x, want %#x", got, want)
	}
}

// Bouncing off walls, the world is stage 1-5: its fingerprint is the one
// that stage pinned.
func TestBounceIsStage15(t *testing.T) {
	const want = uint64(0xf7c67a3be62ca6be)
	cfg := testConfig(1)
	cfg.AllotSight = false
	cfg.AllotFertility = false
	cfg.AllotBudget = false
	cfg.AllotVenture = false
	cfg.Species = cfg.Species[:1]
	cfg.Wade = false
	cfg.AllotCombat = false
	cfg.AllotFierce = false
	cfg.FullStep = 0 // exact tables, as before 2026-10-07
	cfg.ReuseNights, cfg.SeekBed = false, false
	cfg.Building, cfg.Tents = false, false
	cfg.ForceAt = 0
	cfg.DuskRead = false
	cfg.Sleep = false     // before stage 7-3b
	cfg.BurnRead = false  // before stage 7-3a
	cfg.SightRead = false // before stage 7-2a
	cfg.DayTicks = 0      // before stage 7-1
	cfg.FoodLife = 0      // before stage 4-4
	cfg.SpotRead = 0      // before stage 5-2
	cfg.AgeBand = 0       // before stage 5-1
	cfg.Carry = 0         // before stage 4-1
	cfg.Requests = false  // before stage M-3
	cfg.ChildAbility = 1  // before stage 3-7
	cfg.MatureAge = 1000  // before stage 3-6
	cfg.Lifespan = 0      // before stage 3-5
	cfg.Sexes = false     // before stage 3-1
	cfg.Bounce, cfg.Learn = true, false
	w, err := NewWorld(cfg, testMap())
	if err != nil {
		t.Fatal(err)
	}
	run(w, 1000)
	if got := w.Fingerprint(); got != want {
		t.Fatalf("fingerprint = %#x, want %#x", got, want)
	}
}

// Drawing every share afresh, the world is the one collisions were
// measured on: its fingerprint is the one pinned then.
func TestDrawnIsCollisions(t *testing.T) {
	const want = uint64(0xe4ce1d34b72b02f4)
	cfg := testConfig(1)
	cfg.AllotSight = false
	cfg.AllotFertility = false
	cfg.AllotBudget = false
	cfg.AllotVenture = false
	cfg.Species = cfg.Species[:1]
	cfg.Wade = false
	cfg.AllotCombat = false
	cfg.AllotFierce = false
	cfg.FullStep = 0 // exact tables, as before 2026-10-07
	cfg.ReuseNights, cfg.SeekBed = false, false
	cfg.Building, cfg.Tents = false, false
	cfg.Sleep = false     // before stage 7-3b
	cfg.BurnRead = false  // before stage 7-3a
	cfg.SightRead = false // before stage 7-2a
	cfg.DayTicks = 0      // before stage 7-1
	cfg.FoodLife = 0      // before stage 4-4
	cfg.SpotRead = 0      // before stage 5-2
	cfg.AgeBand = 0       // before stage 5-1
	cfg.Carry = 0         // before stage 4-1
	cfg.Requests = false  // before stage M-3
	cfg.ChildAbility = 1  // before stage 3-7
	cfg.MatureAge = 1000  // before stage 3-6
	cfg.Lifespan = 0      // before stage 3-5
	cfg.Sexes = false     // before stage 3-1
	cfg.AllotInherit = false
	cfg.Bounce, cfg.Learn = true, false
	w, err := NewWorld(cfg, testMap())
	if err != nil {
		t.Fatal(err)
	}
	run(w, 1000)
	if got := w.Fingerprint(); got != want {
		t.Fatalf("fingerprint = %#x, want %#x", got, want)
	}
}

// Without collisions the world is stage 1-4: its fingerprint is the one
// that stage pinned.
func TestOverlapIsStage14(t *testing.T) {
	const want = uint64(0x6dda229acfd53e93)
	cfg := testConfig(1)
	cfg.AllotSight = false
	cfg.AllotFertility = false
	cfg.AllotBudget = false
	cfg.AllotVenture = false
	cfg.Species = cfg.Species[:1]
	cfg.Wade = false
	cfg.AllotCombat = false
	cfg.AllotFierce = false
	cfg.FullStep = 0 // exact tables, as before 2026-10-07
	cfg.ReuseNights, cfg.SeekBed = false, false
	cfg.Building, cfg.Tents = false, false
	cfg.ForceAt = 0
	cfg.DuskRead = false
	cfg.Sleep = false     // before stage 7-3b
	cfg.BurnRead = false  // before stage 7-3a
	cfg.SightRead = false // before stage 7-2a
	cfg.DayTicks = 0      // before stage 7-1
	cfg.FoodLife = 0      // before stage 4-4
	cfg.SpotRead = 0      // before stage 5-2
	cfg.AgeBand = 0       // before stage 5-1
	cfg.Carry = 0         // before stage 4-1
	cfg.Requests = false  // before stage M-3
	cfg.ChildAbility = 1  // before stage 3-7
	cfg.MatureAge = 1000  // before stage 3-6
	cfg.Lifespan = 0      // before stage 3-5
	cfg.Sexes = false     // before stage 3-1
	cfg.Collide, cfg.AllotInherit = false, false
	cfg.Bounce, cfg.Learn = true, false
	w, err := NewWorld(cfg, testMap())
	if err != nil {
		t.Fatal(err)
	}
	run(w, 1000)
	if got := w.Fingerprint(); got != want {
		t.Fatalf("fingerprint = %#x, want %#x", got, want)
	}
}

// Without the budget the world is stage 1-3: its fingerprint is the one
// that stage pinned.
func TestFixedIsStage13(t *testing.T) {
	const want = uint64(0x31db5a227bba52b8)
	cfg := testConfig(1)
	cfg.AllotSight = false
	cfg.AllotFertility = false
	cfg.AllotBudget = false
	cfg.AllotVenture = false
	cfg.Species = cfg.Species[:1]
	cfg.Wade = false
	cfg.AllotCombat = false
	cfg.AllotFierce = false
	cfg.FullStep = 0 // exact tables, as before 2026-10-07
	cfg.ReuseNights, cfg.SeekBed = false, false
	cfg.Building, cfg.Tents = false, false
	cfg.ForceAt = 0
	cfg.DuskRead = false
	cfg.Sleep = false     // before stage 7-3b
	cfg.BurnRead = false  // before stage 7-3a
	cfg.SightRead = false // before stage 7-2a
	cfg.DayTicks = 0      // before stage 7-1
	cfg.FoodLife = 0      // before stage 4-4
	cfg.SpotRead = 0      // before stage 5-2
	cfg.AgeBand = 0       // before stage 5-1
	cfg.Carry = 0         // before stage 4-1
	cfg.Requests = false  // before stage M-3
	cfg.ChildAbility = 1  // before stage 3-7
	cfg.MatureAge = 1000  // before stage 3-6
	cfg.Lifespan = 0      // before stage 3-5
	cfg.Sexes = false     // before stage 3-1
	cfg.Allot, cfg.Collide = false, false
	cfg.Bounce, cfg.Learn = true, false
	w, err := NewWorld(cfg, testMap())
	if err != nil {
		t.Fatal(err)
	}
	run(w, 1000)
	if got := w.Fingerprint(); got != want {
		t.Fatalf("fingerprint = %#x, want %#x", got, want)
	}
}

// Without breeding the world is stage 1-2e: its fingerprint is the one that
// stage pinned.
func TestNoBreedIsStage12e(t *testing.T) {
	const want = uint64(0xc6343895ef84fe4d)
	cfg := testConfig(1)
	cfg.AllotSight = false
	cfg.AllotFertility = false
	cfg.AllotBudget = false
	cfg.AllotVenture = false
	cfg.Species = cfg.Species[:1]
	cfg.Wade = false
	cfg.AllotCombat = false
	cfg.AllotFierce = false
	cfg.FullStep = 0 // exact tables, as before 2026-10-07
	cfg.ReuseNights, cfg.SeekBed = false, false
	cfg.Building, cfg.Tents = false, false
	cfg.ForceAt = 0
	cfg.DuskRead = false
	cfg.Sleep = false     // before stage 7-3b
	cfg.BurnRead = false  // before stage 7-3a
	cfg.SightRead = false // before stage 7-2a
	cfg.DayTicks = 0      // before stage 7-1
	cfg.FoodLife = 0      // before stage 4-4
	cfg.SpotRead = 0      // before stage 5-2
	cfg.AgeBand = 0       // before stage 5-1
	cfg.Carry = 0         // before stage 4-1
	cfg.Requests = false  // before stage M-3
	cfg.ChildAbility = 1  // before stage 3-7
	cfg.MatureAge = 1000  // before stage 3-6
	cfg.Lifespan = 0      // before stage 3-5
	cfg.Sexes = false     // before stage 3-1
	cfg.Breed, cfg.Allot, cfg.Collide = false, false, false
	cfg.Bounce, cfg.Learn = true, false
	w, err := NewWorld(cfg, testMap())
	if err != nil {
		t.Fatal(err)
	}
	run(w, 1000)
	if got := w.Fingerprint(); got != want {
		t.Fatalf("fingerprint = %#x, want %#x", got, want)
	}
}

// Deciding every tick, the world is stage 1-2r: its fingerprint is the one
// that stage pinned.
func TestEverytickIsStage12r(t *testing.T) {
	const want = uint64(0x72d49e5113ca37a8)
	cfg := testConfig(1)
	cfg.AllotSight = false
	cfg.AllotFertility = false
	cfg.AllotBudget = false
	cfg.AllotVenture = false
	cfg.Species = cfg.Species[:1]
	cfg.Wade = false
	cfg.AllotCombat = false
	cfg.AllotFierce = false
	cfg.FullStep = 0 // exact tables, as before 2026-10-07
	cfg.ReuseNights, cfg.SeekBed = false, false
	cfg.Building, cfg.Tents = false, false
	cfg.ForceAt = 0
	cfg.DuskRead = false
	cfg.Sleep = false     // before stage 7-3b
	cfg.BurnRead = false  // before stage 7-3a
	cfg.SightRead = false // before stage 7-2a
	cfg.DayTicks = 0      // before stage 7-1
	cfg.FoodLife = 0      // before stage 4-4
	cfg.SpotRead = 0      // before stage 5-2
	cfg.AgeBand = 0       // before stage 5-1
	cfg.Carry = 0         // before stage 4-1
	cfg.Requests = false  // before stage M-3
	cfg.ChildAbility = 1  // before stage 3-7
	cfg.MatureAge = 1000  // before stage 3-6
	cfg.Lifespan = 0      // before stage 3-5
	cfg.Sexes = false     // before stage 3-1
	cfg.Recheck, cfg.Breed, cfg.Allot, cfg.Collide = 0, false, false, false
	cfg.Bounce, cfg.Learn = true, false
	w, err := NewWorld(cfg, testMap())
	if err != nil {
		t.Fatal(err)
	}
	run(w, 1000)
	if got := w.Fingerprint(); got != want {
		t.Fatalf("fingerprint = %#x, want %#x", got, want)
	}
}

// Without turning off the reverse the world is stage 1-2q: its fingerprint
// is the one that stage pinned.
func TestStraightbackIsStage12q(t *testing.T) {
	const want = uint64(0x598db45e1aaf44c7)
	cfg := testConfig(1)
	cfg.AllotSight = false
	cfg.AllotFertility = false
	cfg.AllotBudget = false
	cfg.AllotVenture = false
	cfg.Species = cfg.Species[:1]
	cfg.Wade = false
	cfg.AllotCombat = false
	cfg.AllotFierce = false
	cfg.FullStep = 0 // exact tables, as before 2026-10-07
	cfg.ReuseNights, cfg.SeekBed = false, false
	cfg.Building, cfg.Tents = false, false
	cfg.ForceAt = 0
	cfg.DuskRead = false
	cfg.Sleep = false     // before stage 7-3b
	cfg.BurnRead = false  // before stage 7-3a
	cfg.SightRead = false // before stage 7-2a
	cfg.DayTicks = 0      // before stage 7-1
	cfg.FoodLife = 0      // before stage 4-4
	cfg.SpotRead = 0      // before stage 5-2
	cfg.AgeBand = 0       // before stage 5-1
	cfg.Carry = 0         // before stage 4-1
	cfg.Requests = false  // before stage M-3
	cfg.ChildAbility = 1  // before stage 3-7
	cfg.MatureAge = 1000  // before stage 3-6
	cfg.Lifespan = 0      // before stage 3-5
	cfg.Sexes = false     // before stage 3-1
	cfg.TurnOffReverse, cfg.Recheck, cfg.Breed, cfg.Allot, cfg.Collide = false, 0, false, false, false
	cfg.Bounce, cfg.Learn = true, false
	w, err := NewWorld(cfg, testMap())
	if err != nil {
		t.Fatal(err)
	}
	run(w, 1000)
	if got := w.Fingerprint(); got != want {
		t.Fatalf("fingerprint = %#x, want %#x", got, want)
	}
}

// With sight and no heading kept the world is stage 1-2p: its fingerprint is
// the one that stage pinned.
func TestNoHeadingIsStage12p(t *testing.T) {
	const want = uint64(0x46ffb00b8b64a80)
	cfg := testConfig(1)
	cfg.AllotSight = false
	cfg.AllotFertility = false
	cfg.AllotBudget = false
	cfg.AllotVenture = false
	cfg.Species = cfg.Species[:1]
	cfg.Wade = false
	cfg.AllotCombat = false
	cfg.AllotFierce = false
	cfg.FullStep = 0 // exact tables, as before 2026-10-07
	cfg.ReuseNights, cfg.SeekBed = false, false
	cfg.Building, cfg.Tents = false, false
	cfg.ForceAt = 0
	cfg.DuskRead = false
	cfg.Sleep = false     // before stage 7-3b
	cfg.BurnRead = false  // before stage 7-3a
	cfg.SightRead = false // before stage 7-2a
	cfg.DayTicks = 0      // before stage 7-1
	cfg.FoodLife = 0      // before stage 4-4
	cfg.SpotRead = 0      // before stage 5-2
	cfg.AgeBand = 0       // before stage 5-1
	cfg.Carry = 0         // before stage 4-1
	cfg.Requests = false  // before stage M-3
	cfg.ChildAbility = 1  // before stage 3-7
	cfg.MatureAge = 1000  // before stage 3-6
	cfg.Lifespan = 0      // before stage 3-5
	cfg.Sexes = false     // before stage 3-1
	cfg.KeepHeading, cfg.Recheck, cfg.Breed, cfg.Allot, cfg.Collide = false, 0, false, false, false
	cfg.Bounce, cfg.Learn = true, false
	w, err := NewWorld(cfg, testMap())
	if err != nil {
		t.Fatal(err)
	}
	run(w, 1000)
	if got := w.Fingerprint(); got != want {
		t.Fatalf("fingerprint = %#x, want %#x", got, want)
	}
}

// With no sight and no heading the world is the first run of stage 1-2,
// valuation over the region rows alone: its fingerprint is the one that run pinned.
func TestNoSightIsFirstStage12(t *testing.T) {
	const want = uint64(0x51de0e4034431e90)
	cfg := testConfig(1)
	cfg.AllotSight = false
	cfg.AllotFertility = false
	cfg.AllotBudget = false
	cfg.AllotVenture = false
	cfg.Species = cfg.Species[:1]
	cfg.Wade = false
	cfg.AllotCombat = false
	cfg.AllotFierce = false
	cfg.FullStep = 0 // exact tables, as before 2026-10-07
	cfg.ReuseNights, cfg.SeekBed = false, false
	cfg.Building, cfg.Tents = false, false
	cfg.ForceAt = 0
	cfg.DuskRead = false
	cfg.Sleep = false     // before stage 7-3b
	cfg.BurnRead = false  // before stage 7-3a
	cfg.SightRead = false // before stage 7-2a
	cfg.DayTicks = 0      // before stage 7-1
	cfg.FoodLife = 0      // before stage 4-4
	cfg.SpotRead = 0      // before stage 5-2
	cfg.AgeBand = 0       // before stage 5-1
	cfg.Carry = 0         // before stage 4-1
	cfg.Requests = false  // before stage M-3
	cfg.ChildAbility = 1  // before stage 3-7
	cfg.MatureAge = 1000  // before stage 3-6
	cfg.Lifespan = 0      // before stage 3-5
	cfg.Sexes = false     // before stage 3-1
	cfg.Sight, cfg.KeepHeading, cfg.Recheck, cfg.Breed, cfg.Allot, cfg.Collide = -1, false, 0, false, false, false
	cfg.Bounce, cfg.Learn = true, false
	w, err := NewWorld(cfg, testMap())
	if err != nil {
		t.Fatal(err)
	}
	run(w, 1000)
	if got := w.Fingerprint(); got != want {
		t.Fatalf("fingerprint = %#x, want %#x", got, want)
	}
}

// With no window and no heading the world is the stage 1-1 control, draw
// for draw: its fingerprint is the one 1-1 pinned before valuation existed.
func TestNoWindowIsStage11(t *testing.T) {
	const want = uint64(0xa43263b50e366b1e)
	cfg := testConfig(1)
	cfg.AllotSight = false
	cfg.AllotFertility = false
	cfg.AllotBudget = false
	cfg.AllotVenture = false
	cfg.Species = cfg.Species[:1]
	cfg.Wade = false
	cfg.AllotCombat = false
	cfg.AllotFierce = false
	cfg.FullStep = 0 // exact tables, as before 2026-10-07
	cfg.ReuseNights, cfg.SeekBed = false, false
	cfg.Building, cfg.Tents = false, false
	cfg.ForceAt = 0
	cfg.DuskRead = false
	cfg.Sleep = false     // before stage 7-3b
	cfg.BurnRead = false  // before stage 7-3a
	cfg.SightRead = false // before stage 7-2a
	cfg.DayTicks = 0      // before stage 7-1
	cfg.FoodLife = 0      // before stage 4-4
	cfg.SpotRead = 0      // before stage 5-2
	cfg.AgeBand = 0       // before stage 5-1
	cfg.Carry = 0         // before stage 4-1
	cfg.Requests = false  // before stage M-3
	cfg.ChildAbility = 1  // before stage 3-7
	cfg.MatureAge = 1000  // before stage 3-6
	cfg.Lifespan = 0      // before stage 3-5
	cfg.Sexes = false     // before stage 3-1
	cfg.Window, cfg.KeepHeading, cfg.Recheck, cfg.Breed, cfg.Allot, cfg.Collide = 0, false, 0, false, false, false
	cfg.Bounce, cfg.Learn = true, false
	w, err := NewWorld(cfg, testMap())
	if err != nil {
		t.Fatal(err)
	}
	run(w, 1000)
	if got := w.Fingerprint(); got != want {
		t.Fatalf("fingerprint = %#x, want %#x", got, want)
	}
}

// BenchmarkWorldStep rebuilds the world every benchTicks ticks with the timer
// stopped, so that it measures a fixed amount of work rather than one world
// running on forever.
func BenchmarkWorldStep(b *testing.B) {
	const benchTicks = 1000
	var w *World
	for i := 0; i < b.N; i++ {
		if i%benchTicks == 0 {
			b.StopTimer()
			w = newTestWorld(b, 1)
			b.StartTimer()
		}
		w.Step()
	}
}

// A loaded world runs on the same whether its tables are built ahead
// (Warm) or as they are read, and warming builds the tables the saved
// world was using.
func TestWarmChangesNothing(t *testing.T) {
	a := newTestWorld(t, 12)
	run(a, 3000)
	var buf bytes.Buffer
	if err := a.Save(&buf); err != nil {
		t.Fatal(err)
	}
	saved := buf.Bytes()
	cold, err := Load(bytes.NewReader(saved))
	if err != nil {
		t.Fatal(err)
	}
	warm, err := Load(bytes.NewReader(saved))
	if err != nil {
		t.Fatal(err)
	}
	if left := warm.Warm(5); left != len(a.Tables())-5 {
		t.Fatalf("after warming 5, %d left of %d", left, len(a.Tables()))
	}
	if left := warm.Warm(0); left != 0 {
		t.Fatalf("%d tables left after warming them all", left)
	}
	if got, want := warm.Tables(), a.Tables(); len(got) != len(want) {
		t.Fatalf("warmed %d tables, the saved world had %d", len(got), len(want))
	}
	run(a, 1500)
	run(cold, 1500)
	run(warm, 1500)
	if cold.Fingerprint() != a.Fingerprint() || warm.Fingerprint() != a.Fingerprint() {
		t.Fatal("a loaded world ran differently from the saved one")
	}
}

// Tables not read for a while are dropped, so the cache holds what the
// world reads lately rather than every food count it ever had.
func TestTablesAreForgotten(t *testing.T) {
	w := newTestWorld(t, 3)
	run(w, 5000)
	for r := range w.pred.cache {
		for k, tb := range w.pred.cache[r] {
			if w.tick-tb.used > forgetAfter+forgetEvery {
				t.Fatalf("region %d table %+v last read at %d, now %d", r, k, tb.used, w.tick)
			}
		}
	}
}

// The light follows the trapezoid: full from tick 0, half at each change,
// dark through the night, back by the next day; a day's light averages a
// half here. Within NightSight a body always sees; out to Sight it sees a
// share of the tiles that follows the light, the same tiles at the same
// light, and nothing past Sight. Settings whose twilight does not fit are
// refused, and a world without days is always light.
func TestNight(t *testing.T) {
	w, b := soloWorld(t, 50)
	w.cfg.DayTicks, w.cfg.NightTicks, w.cfg.TwilightTicks, w.cfg.NightSight = 1000, 500, 100, 0
	for tick, want := range map[int64]float64{0: 1, 100: 1, 450: 0.495, 500: 0, 700: 0, 949: 0.495, 1000: 1} {
		if got := w.brightnessAt(tick); math.Abs(got-want) > 1e-9 {
			t.Fatalf("tick %d: light %v, want %v", tick, got, want)
		}
	}
	sum := 0.0
	for tick := int64(1); tick <= 1000; tick++ {
		sum += w.brightnessAt(tick)
	}
	if math.Abs(sum/1000-0.5) > 0.01 {
		t.Fatalf("light over a day %v, want a half", sum/1000)
	}
	tx, ty := w.tileOf(b.X, b.Y)%w.m.Width, w.tileOf(b.X, b.Y)/w.m.Width
	w.light, w.lightFor = 0.3, w.tick+1
	seen, all := 0, 0
	for id := int64(0); id < 2000; id++ {
		o := Body{ID: id}
		for dy := -1; dy <= 1; dy++ {
			for dx := -1; dx <= 1; dx++ {
				if dx == 0 && dy == 0 {
					if !w.seesFrom(&o, tx, ty, tx, ty) {
						t.Fatal("own tile unseen in the dark")
					}
					continue
				}
				all++
				s := w.seesFrom(&o, tx, ty, tx+dx, ty+dy)
				if s != w.seesFrom(&o, tx, ty, tx+dx, ty+dy) {
					t.Fatal("the same tile at the same light seen and unseen")
				}
				if s {
					seen++
				}
			}
		}
	}
	if share := float64(seen) / float64(all); math.Abs(share-0.3) > 0.02 {
		t.Fatalf("share seen at light 0.3: %v", share)
	}
	if w.seesFrom(b, tx, ty, tx+2, ty) {
		t.Fatal("seen past Sight")
	}
	w.light = 0
	if w.seesFrom(b, tx, ty, tx+1, ty) {
		t.Fatal("seen past NightSight in the dark")
	}
	w.cfg.DayTicks = 0
	w.lightFor = 0
	if w.Brightness() != 1 {
		t.Fatal("dark without days")
	}
	cfg := testConfig(1)
	cfg.DayTicks, cfg.NightTicks, cfg.TwilightTicks = 1000, 500, 600
	if _, err := NewWorld(cfg, testMap()); err == nil {
		t.Fatal("a twilight longer than the night was accepted")
	}
}
