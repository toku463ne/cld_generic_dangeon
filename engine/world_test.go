package engine

import (
	"bytes"
	"testing"
)

// testMap has water and more than one region, and the two layers cut across
// each other, so a save that dropped or mixed up a layer would show.
func testMap() Map {
	m := NewMap(12, 9)
	for y := 0; y < m.Height; y++ {
		for x := 0; x < m.Width; x++ {
			m.SetRegion(x, y, RegionID(x/6+2*(y/5)))
		}
		m.SetTerrain(7, y, TerrainWater)
	}
	return m
}

func newTestWorld(t testing.TB, seed int64) *World {
	t.Helper()
	w, err := NewWorld(Config{Seed: seed}, testMap())
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
	const want = uint64(0x29a89240a7037963)
	w := newTestWorld(t, 1)
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
