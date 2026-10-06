package engine

import (
	"math"
	"testing"
)

// tempMap is testMap with a range of temperature in each region: some
// only cold, one cold and hot, so that heat is exercised too.
func tempMap() Map {
	m := testMap()
	m.RegionTemp = []TempRange{{11, 20}, {5, 25}, {15, 30}, {11, 20}}
	for y := 1; y <= 3; y++ {
		for x := 1; x <= 3; x++ {
			m.SetShelter(x, y) // a place to rest in region 0 (stage 6-1a)
		}
	}
	return m
}

// A tile's temperature is its region's Low in the dark and High in full
// light; the burn rises by 1 for each TempBurn degrees off NormalTemp,
// colder or hotter, and not at all without Temperature or on a map with
// no temperature.
func TestTemperature(t *testing.T) {
	w, b := soloWorld(t, 50)
	w.cfg.Temperature, w.cfg.NormalTemp, w.cfg.TempBurn = true, 20, 30
	if got := w.tempBurn(b); got != 1 {
		t.Fatalf("map without temperature: factor %v, want 1", got)
	}
	w.m.RegionTemp = []TempRange{{11, 20}, {5, 25}, {15, 30}, {11, 20}}
	setLight := func(l float64) { w.light, w.lightFor = l, w.tick+1 }
	for _, c := range []struct{ light, temp float64 }{{0, 11}, {1, 20}, {0.5, 15.5}} {
		setLight(c.light)
		got, ok := w.TempAt(2, 2)
		if !ok || math.Abs(got-c.temp) > 1e-12 {
			t.Fatalf("light %v: temperature %v (%v), want %v", c.light, got, ok, c.temp)
		}
		want := 1 + math.Abs(c.temp-20)/30
		if f := w.tempBurn(b); math.Abs(f-want) > 1e-12 {
			t.Fatalf("light %v: factor %v, want %v", c.light, f, want)
		}
	}
	// The factor grows with the distance from NormalTemp either way.
	setLight(0)
	prev := 0.0
	for _, temp := range []float64{20, 15, 10, 0, -10} {
		w.m.RegionTemp[0] = TempRange{temp, temp}
		f := w.tempBurn(b)
		if f <= prev {
			t.Fatalf("cold %v: factor %v not above %v", temp, f, prev)
		}
		prev = f
	}
	prev = 0
	for _, temp := range []float64{20, 25, 30, 40} {
		w.m.RegionTemp[0] = TempRange{temp, temp}
		f := w.tempBurn(b)
		if f <= prev {
			t.Fatalf("heat %v: factor %v not above %v", temp, f, prev)
		}
		prev = f
	}
	// The burn spent and the burn valued both carry it.
	w.m.RegionTemp[0] = TempRange{11, 11}
	burn := w.burnOf(b)
	w.cfg.Temperature = false
	if cool := w.burnOf(b); math.Abs(burn-cool*1.3) > 1e-12 || w.planBurn(b) != cool {
		t.Fatalf("burn %v with cold, %v without (plan %v): want 1.3 times", burn, cool, w.planBurn(b))
	}
}

// A map's ranges must match its regions and run low to high, and the
// rule needs a TempBurn.
func TestTemperatureRejects(t *testing.T) {
	m := tempMap()
	m.RegionTemp = m.RegionTemp[:3]
	if m.Validate() == nil {
		t.Fatal("accepted fewer ranges than regions")
	}
	m = tempMap()
	m.RegionTemp[1] = TempRange{25, 5}
	if m.Validate() == nil {
		t.Fatal("accepted a range from high to low")
	}
	cfg := testConfig(1)
	cfg.TempBurn = 0
	if _, err := NewWorld(cfg, tempMap()); err == nil {
		t.Fatal("accepted Temperature with no TempBurn")
	}
	if c := tempMap().Clone(); &c.RegionTemp[0] == &tempMap().RegionTemp[0] {
		t.Fatal("clone shares ranges")
	}
}

// Without temperature, the world is stage 7-3b: its fingerprint is the
// one pinned then.
func TestTemplessIsStage73b(t *testing.T) {
	const want = uint64(0x646724f710d7c857)
	cfg := testConfig(1)
	cfg.AllotSight = false
	cfg.AllotFertility = false
	cfg.ReuseNights, cfg.SeekBed = false, false
	cfg.Building, cfg.Tents = false, false
	cfg.ShelterFoodLife = 0
	cfg.ForceAt = 0
	cfg.DuskRead = false
	cfg.Temperature = false
	w, err := NewWorld(cfg, tempMap())
	if err != nil {
		t.Fatal(err)
	}
	run(w, 1000)
	if got := w.Fingerprint(); got != want {
		t.Fatalf("fingerprint = %#x, want %#x", got, want)
	}
}

// On a place to rest the temperature is normal in the dark, so the cold
// costs nothing there; off it, or without Shelter, the region's range
// holds. The layer must match the map.
func TestShelter(t *testing.T) {
	w, b := soloWorld(t, 50)
	w.m.RegionTemp = []TempRange{{11, 20}, {5, 25}, {15, 30}, {11, 20}}
	w.m.SetShelter(2, 2)
	w.cfg.Temperature, w.cfg.Shelter = true, true
	w.light, w.lightFor = 0, w.tick+1
	if temp, _ := w.TempAt(2, 2); temp != w.cfg.NormalTemp || w.tempBurn(b) != 1 {
		t.Fatalf("on the shelter at night: %v degrees, factor %v", temp, w.tempBurn(b))
	}
	if temp, _ := w.TempAt(3, 2); temp != 11 {
		t.Fatalf("off the shelter at night: %v degrees, want 11", temp)
	}
	w.cfg.Shelter = false
	if temp, _ := w.TempAt(2, 2); temp != 11 {
		t.Fatalf("without Shelter: %v degrees, want 11", temp)
	}
	m := tempMap()
	m.Shelter = m.Shelter[:5]
	if m.Validate() == nil {
		t.Fatal("accepted a shelter layer of the wrong size")
	}
	if c := tempMap().Clone(); !c.ShelterAt(2, 2) || c.ShelterAt(5, 5) {
		t.Fatal("clone lost the shelter layer")
	}
}

// Without places to rest, the world is stage 7-2b: its fingerprint is the
// one pinned then.
func TestShelterlessIsStage72b(t *testing.T) {
	const want = uint64(0x1bfef7f1f18d1e4)
	cfg := testConfig(1)
	cfg.AllotSight = false
	cfg.AllotFertility = false
	cfg.ReuseNights, cfg.SeekBed = false, false
	cfg.Building, cfg.Tents = false, false
	cfg.ShelterFoodLife = 0
	cfg.ForceAt = 0
	cfg.Shelter = false
	w, err := NewWorld(cfg, tempMap())
	if err != nil {
		t.Fatal(err)
	}
	run(w, 1000)
	if got := w.Fingerprint(); got != want {
		t.Fatalf("fingerprint = %#x, want %#x", got, want)
	}
}
