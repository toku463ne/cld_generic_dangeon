package engine

import (
	"bytes"
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

// testConfig is the default rules scaled to the small test map.
func testConfig(seed int64) Config {
	cfg := DefaultConfig()
	cfg.Seed = seed
	cfg.FoodCap = 30
	cfg.Bodies = 20
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
	const want = uint64(0xe85e05167296fc73)
	w := newTestWorld(t, 1)
	run(w, 1000)
	if got := w.Fingerprint(); got != want {
		t.Fatalf("fingerprint = %#x, want %#x", got, want)
	}
}

// Reading the truth table, the world is the one before stage 1-6: its
// fingerprint is the one pinned then.
func TestTruthIsBeforeStage16(t *testing.T) {
	const want = uint64(0x6031cdab7f6ff3fb)
	cfg := testConfig(1)
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
	const want = uint64(0x8f80778b1d21631e)
	cfg := testConfig(1)
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
