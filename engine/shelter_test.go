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
	there, _ := w.shelterRisk(b, &rs, f, b.X, b.Y, n-1, win, full, warm, w.wokenBurn(b), w.speedOf(b), w.readBurn(b))
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
	cfg.ShelterFoodLife = 0
	cfg.ForceAt = 0
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

// A body remembers the last tile it stood on of the ground its species
// likes (Homing, stage 10-5), and with sleep near, off such ground, reads
// it as a place to rest at its own burn: sleeping there is less risky
// than sleeping where it stands at TerrainBurn times its burn, so the
// valuation offers the walk back. A species without preferences keeps no
// such tile, and without Homing none is kept or read.
func TestHoming(t *testing.T) {
	cfg := testConfig(24)
	cfg.FoodKinds, cfg.Foods = DefaultConfig().FoodKinds, append([]FoodSpec(nil), DefaultConfig().Foods...)
	cfg.Foods[1].Terrain = TerrainLand
	cfg.Bodies, cfg.Hunt = 0, false
	m := testMap()
	for y := 1; y <= 3; y++ {
		m.SetTerrain(5, y, TerrainForest)
	}
	w, err := NewWorld(cfg, m)
	if err != nil {
		t.Fatal(err)
	}
	w.bodies = append(w.bodies, Body{ID: 0, X: 5.5, Y: 2.5, Energy: 60, Species: 1, Heading: -1, Goal: -1, Decided: -1, Parents: [2]int64{-1, -1}, Sex: Female, Sight: 3})
	w.nextID = 1
	w.buildGrid()
	b := &w.bodies[0]
	home := w.m.index(5, 2)
	w.noteHaunt(b)
	if b.Memory.Haunt != home+1 {
		t.Fatalf("on forest a beast remembers %d, want tile %d", b.Memory.Haunt-1, home)
	}
	b.X = 2.5 // off onto land
	w.buildGrid()
	w.noteHaunt(b)
	if b.Memory.Haunt != home+1 {
		t.Fatal("on land it forgot the forest")
	}
	if _, _, ok := w.hauntAhead(b); ok {
		t.Fatal("a body that may not sleep reads its haunt")
	}
	b.Pressure = 0.5
	f, burn, ok := w.hauntAhead(b)
	if !ok || f.X != 5 || f.Y != 2 || math.Abs(burn-w.ownBurn(b, true)*w.tempBurn(b)) > 1e-12 || !(burn < w.planBurn(b)) {
		t.Fatalf("haunt ahead %v at burn %v (%v), burn here %v", f, burn, ok, w.planBurn(b))
	}
	w.decide(b)
	v := w.valuation
	if len(v.Shelter) != 1 || v.Shelter[0] != f || v.ShelterBurn[0] != burn {
		t.Fatalf("valuation offers %v at %v, want %v at %v", v.Shelter, v.ShelterBurn, f, burn)
	}
	var rs rates
	w.ratesOf(b, &rs)
	win, full := w.windowGrains(), w.units(w.maxOf(b))
	n := w.units(b.Energy)
	there, _ := w.shelterRisk(b, &rs, f, b.X, b.Y, n-1, win, full, burn, w.wokenBurn(b), w.speedOf(b), w.readBurn(b))
	if here := w.sleepRisk(b, &rs, win, 0); !(there < here) {
		t.Fatalf("sleeping at the haunt %v, where it stands %v", there, here)
	}
	// Back on the forest it reads nothing to go to.
	b.X = 5.5
	w.buildGrid()
	if _, _, ok := w.hauntAhead(b); ok {
		t.Fatal("standing on its haunt it reads it")
	}
	// A human, liking land, standing on land keeps the land tile and reads
	// nothing: it is home.
	b.X = 2.5
	w.buildGrid()
	h := Body{ID: 1, X: 2.5, Y: 2.5, Energy: 60, Species: 0, Pressure: 0.5}
	w.noteHaunt(&h)
	if h.Memory.Haunt != w.m.index(2, 2)+1 {
		t.Fatal("a human on land remembers no haunt")
	}
	if _, _, ok := w.hauntAhead(&h); ok {
		t.Fatal("at home it reads a walk home")
	}
	// Without Homing nothing is kept or read.
	w.cfg.Homing = false
	if _, _, ok := w.hauntAhead(b); ok {
		t.Fatal("read without Homing")
	}
	c := Body{ID: 2, X: 5.5, Y: 2.5, Species: 1}
	w.noteHaunt(&c)
	if c.Memory.Haunt != 0 {
		t.Fatal("kept without Homing")
	}
	// A row without preferences keeps none.
	w.cfg.Homing = true
	w.cfg.Species = append([]SpeciesSpec(nil), w.cfg.Species...)
	w.cfg.Species[1].Likes = nil
	w.noteHaunt(&c)
	if c.Memory.Haunt != 0 {
		t.Fatal("a species at home everywhere keeps a haunt")
	}
}
