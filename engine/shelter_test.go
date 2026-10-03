package engine

import (
	"math"
	"testing"
)

// shelterWorld is soloWorld on tempMap's places to rest, in the dark, with
// the rules of temperature and shelter on: the body stands at (5.5, 2.5),
// two tiles off the place at (1..3, 1..3), in region 0.
func shelterWorld(t *testing.T, energy float64) (*World, *Body) {
	t.Helper()
	w, b := soloWorld(t, energy)
	w.m = tempMap()
	w.cfg.Temperature, w.cfg.Shelter, w.cfg.ShelterRead, w.cfg.Sleep = true, true, true, true
	w.light, w.lightFor = 0, w.tick+1
	b.X, b.Y = 5.5, 2.5
	w.buildGrid()
	return w, b
}

// A body remembers the places to rest it sees, and on one in the dark
// learns the burn it feels over the burn it knows of its own body: 1 where
// the place keeps the cold off. Off one, or in the light, it learns
// nothing.
func TestShelterLearn(t *testing.T) {
	w, b := shelterWorld(t, 50)
	w.feelWarm(b)
	if b.Memory.Warm[1] != 0 {
		t.Fatal("learned warmth off the place")
	}
	b.X, b.Y = 4.5, 2.5 // sees (3, 1..3) in the light
	w.light = 1
	w.noteShelters(b)
	w.light = 0
	if !b.Memory.Shelters[w.m.index(3, 2)] || b.Memory.Shelters[w.m.index(2, 2)] {
		t.Fatalf("remembers %v, want the tiles in sight only", b.Memory.Shelters)
	}
	b.X = 2.5
	w.feelWarm(b)
	if b.Memory.Warm[1] != 1 || math.Abs(b.Memory.Warm[0]-1) > 1e-12 {
		t.Fatalf("on the place in the dark: learned %v", b.Memory.Warm)
	}
	w.light = 1
	w.feelWarm(b)
	if b.Memory.Warm[1] != 1 {
		t.Fatal("learned in the light")
	}
}

// With a place remembered and its warmth learned, a sleepy body in the
// cold reads walking there to sleep, and its valuation offers it: sleeping
// there reads safer than sleeping where it stands. Unlearned, not sleepy,
// or without ShelterRead, there is no such way.
func TestShelterWalk(t *testing.T) {
	w, b := shelterWorld(t, 80)
	b.Memory.Shelters = map[int]bool{w.m.index(3, 2): true}
	b.Pressure = 0.9
	if _, _, ok := w.shelterAhead(b); ok {
		t.Fatal("a place unlearned reads as warm")
	}
	b.Memory.Warm = [2]float64{1, 1}
	f, warm, ok := w.shelterAhead(b)
	if !ok || f.X != 3 || f.Y != 2 || math.Abs(warm*w.tempBurn(b)-w.planBurn(b)) > 1e-12 {
		t.Fatalf("shelter ahead %v at burn %v (%v)", f, warm, ok)
	}
	w.decide(b)
	if v := w.valuation; len(v.Shelter) != 1 || v.Shelter[0] != f {
		t.Fatalf("valuation offers %v, want %v", v.Shelter, f)
	}
	var rs rates
	w.ratesOf(b, &rs)
	win, full := w.windowGrains(), w.units(w.maxOf(b))
	n := w.units(b.Energy)
	there, _ := w.shelterRisk(b, &rs, f, b.X, b.Y, n-1, win, full, warm, w.wokenBurn(b))
	if here := w.sleepRisk(b, &rs, win, 0); !(there < here) {
		t.Fatalf("sleeping at the place %v, where it stands %v", there, here)
	}
	w.cfg.ShelterRead = false
	if _, _, ok := w.shelterAhead(b); ok {
		t.Fatal("read without ShelterRead")
	}
	w.cfg.ShelterRead = true
	b.Pressure = w.cfg.WakeAt / 2
	if _, _, ok := w.shelterAhead(b); ok {
		t.Fatal("a body that may not sleep reads the place")
	}
}

// Without knowing places to rest, the world is stage 6-1a: its
// fingerprint is the one pinned then.
func TestShelterblindIsStage61a(t *testing.T) {
	const want = uint64(0x4837b6704b743d89)
	cfg := testConfig(1)
	cfg.ShelterRead = false
	w, err := NewWorld(cfg, tempMap())
	if err != nil {
		t.Fatal(err)
	}
	run(w, 1000)
	if got := w.Fingerprint(); got != want {
		t.Fatalf("fingerprint = %#x, want %#x", got, want)
	}
}
