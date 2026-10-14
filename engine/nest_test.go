package engine

import (
	"bytes"
	"testing"
)

// nestMap is the test map with a nest of each species.
func nestMap() Map {
	m := testMap()
	m.Nests = []Nest{{Species: "human", X: 2, Y: 2}, {Species: "beast", X: 10, Y: 7}}
	return m
}

func nestConfig(seed int64) Config {
	cfg := testConfig(seed)
	cfg.Nests = true
	cfg.NestGap, cfg.NestLow, cfg.NestRadius = 10, 3, 2
	return cfg
}

// A world on a map with nests begins with one body out of each nest, on
// its tile, and none of Config.Bodies (stage 13-3).
func TestNestsSendTheFirstBodies(t *testing.T) {
	w, err := NewWorld(nestConfig(1), nestMap())
	if err != nil {
		t.Fatal(err)
	}
	bs := w.Bodies()
	if len(bs) != 2 {
		t.Fatalf("the world begins with %d bodies, want one a nest", len(bs))
	}
	for i, n := range w.Map().Nests {
		b := bs[i]
		if w.SpeciesName(b.Species) != n.Species || int(b.X) != n.X || int(b.Y) != n.Y {
			t.Fatalf("body %d is a %s at (%v,%v), want a %s on nest (%d,%d)", i, w.SpeciesName(b.Species), b.X, b.Y, n.Species, n.X, n.Y)
		}
		if b.Parents != [2]int64{-1, -1} || b.Energy != w.maxOf(&b) || w.tick < b.Mature {
			t.Fatalf("body %d is not a first body: parents %v, energy %v, mature at %d", i, b.Parents, b.Energy, b.Mature)
		}
	}
	if s := w.Stats().NestSent; len(s) != 2 || s[0] != 1 || s[1] != 1 {
		t.Fatalf("nests sent %v, want one each", s)
	}
}

// A nest refills: it sends a body only on the ticks it looks, and only
// while fewer than NestLow of its species stand near it. With the whole
// map near and no births or deaths, each species comes to NestLow and
// stays there.
func TestNestsRefillOnlyWhenScarce(t *testing.T) {
	cfg := nestConfig(2)
	cfg.NestRadius = 20
	cfg.Breed = false
	cfg.Hunt = false
	w, err := NewWorld(cfg, nestMap())
	if err != nil {
		t.Fatal(err)
	}
	for tick := 1; tick <= 200; tick++ {
		before := len(w.Bodies())
		w.Step()
		if len(w.Bodies()) != before && tick%cfg.NestGap != 0 {
			t.Fatalf("tick %d: bodies %d -> %d off a tick the nests look", tick, before, len(w.Bodies()))
		}
	}
	if w.Stats().Deaths != ([NumCauses]int64{}) {
		t.Fatalf("bodies died (%v): the test reads nothing", w.Stats().Deaths)
	}
	n := make([]int, w.SpeciesCount())
	for _, b := range w.Bodies() {
		n[b.Species]++
	}
	for s, k := range n {
		if k != cfg.NestLow || w.Stats().NestSent[s] != int64(cfg.NestLow) {
			t.Fatalf("species %d: %d living, %d sent, want %d", s, k, w.Stats().NestSent[s], cfg.NestLow)
		}
	}
	// Out of reach of the nest, it sends again.
	cfg.NestRadius = 0
	v, _ := NewWorld(cfg, nestMap())
	run(v, 200)
	if v.Stats().NestSent[0] <= int64(cfg.NestLow) {
		t.Fatalf("a nest that sees only its own tile sent %d in 200 ticks", v.Stats().NestSent[0])
	}
}

// A nest sends at most NestQuota bodies in the world's life, its first
// body among them, and then sends no more however few stand near it; with
// no quota it goes on (stage 13-3, the user's choice 2026-10-14).
func TestNestsStopAtTheQuota(t *testing.T) {
	cfg := nestConfig(3)
	cfg.NestRadius = 0 // the nest sees only its own tile: it keeps sending
	cfg.NestQuota = 5
	w, err := NewWorld(cfg, nestMap())
	if err != nil {
		t.Fatal(err)
	}
	run(w, 300)
	out := w.Stats().NestOut
	if len(out) != len(w.Map().Nests) {
		t.Fatalf("NestOut %v, want one count a nest", out)
	}
	sent := w.Stats().NestSent
	for i, k := range out {
		if k != int64(cfg.NestQuota) {
			t.Fatalf("nest %d sent %d in 300 ticks, want its quota %d", i, k, cfg.NestQuota)
		}
		if sent[w.nestSpecies[i]] != k { // one nest a species on this map
			t.Fatalf("nest %d sent %d, its species' count says %d", i, k, sent[w.nestSpecies[i]])
		}
	}
	cfg.NestQuota = 0
	v, _ := NewWorld(cfg, nestMap())
	run(v, 300)
	if v.Stats().NestOut[0] <= 5 {
		t.Fatalf("a nest with no quota sent %d in 300 ticks", v.Stats().NestOut[0])
	}
	cfg.NestQuota = -1
	if _, err := NewWorld(cfg, nestMap()); err == nil {
		t.Fatal("a negative quota was taken")
	}
}

// Given a save, a nest sends out only its stocks of the nest's species,
// each with its genome and its skills; the budgets it brings in are the
// ledger's own row, and the ledger closes.
func TestNestsDrawFromTheSave(t *testing.T) {
	src := newTestWorld(t, 4)
	run(src, 300)
	var stocks []Stock
	for s := 0; s < src.SpeciesCount(); s++ {
		stocks = append(stocks, src.Stocks(s)...)
	}
	cfg := nestConfig(5)
	cfg.Breed = false
	w, err := NewWorldNested(cfg, nestMap(), stocks)
	if err != nil {
		t.Fatal(err)
	}
	run(w, 300)
	nest := 0.0
	for _, b := range w.Bodies() {
		found := false
		for _, st := range stocks {
			if st.Genome == w.GenomeOf(b) && st.Skill == b.Skill {
				found = true
			}
		}
		if !found {
			t.Fatalf("body %d (%s) is none of the save's stocks", b.ID, w.SpeciesName(b.Species))
		}
		nest += b.Budget
	}
	l := w.BudgetLedger()
	if !l.Balanced() || l.Nest != l.In || l.Nest < nest {
		t.Fatalf("ledger %+v, living from the nests %v", l, nest)
	}
	if sent := w.Stats().NestSent; sent[0]+sent[1] <= 2 {
		t.Fatalf("the nests sent %v: the test reads nothing", sent)
	}
}

// A world with nests and a save carries on the same after a save and load.
func TestNestsSaveLoadContinues(t *testing.T) {
	src := newTestWorld(t, 4)
	run(src, 300)
	a, err := NewWorldNested(nestConfig(6), nestMap(), append(src.Stocks(0), src.Stocks(1)...))
	if err != nil {
		t.Fatal(err)
	}
	run(a, 100)
	var buf bytes.Buffer
	if err := a.Save(&buf); err != nil {
		t.Fatal(err)
	}
	b, err := Load(&buf)
	if err != nil {
		t.Fatal(err)
	}
	run(a, 300)
	run(b, 300)
	if a.Fingerprint() != b.Fingerprint() || a.Draws() != b.Draws() {
		t.Fatalf("loaded world diverged: %x/%d vs %x/%d", b.Fingerprint(), b.Draws(), a.Fingerprint(), a.Draws())
	}
}

// Nests off, the map's nests are ignored: the world is the one without
// them. And the ways of building a world that do not fit nests refuse.
func TestNestsOffAndRefusals(t *testing.T) {
	cfg := nestConfig(7)
	cfg.Nests = false
	a, err := NewWorld(cfg, nestMap())
	if err != nil {
		t.Fatal(err)
	}
	b, _ := NewWorld(cfg, testMap())
	run(a, 200)
	run(b, 200)
	if a.Fingerprint() != b.Fingerprint() {
		t.Fatal("a world with Nests off differs from one on a map without nests")
	}
	if _, err := NewWorldNested(nestConfig(7), testMap(), nil); err == nil {
		t.Fatal("NewWorldNested accepted a map without nests")
	}
	if _, err := NewWorldFrom(nestConfig(7), nestMap(), make([]Genome, 3)); err == nil {
		t.Fatal("NewWorldFrom placed first bodies on a map whose nests send them")
	}
	m := nestMap()
	m.Nests[0].X = 7 // the water
	if _, err := NewWorld(nestConfig(7), m); err == nil {
		t.Fatal("a nest of humans stood in the water")
	}
	m = nestMap()
	m.Nests[1].Species = "dragon"
	if _, err := NewWorld(nestConfig(7), m); err == nil {
		t.Fatal("a nest of an unknown species was accepted")
	}
}
