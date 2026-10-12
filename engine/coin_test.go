package engine

import (
	"bytes"
	"testing"
)

// coinConfig is testConfig with coins issued fast enough to read in a
// short run: 0.5 a tick, halved every 200 ticks, 150 in all.
func coinConfig(seed int64) Config {
	cfg := testConfig(seed)
	cfg.Coins = true
	cfg.CoinRate = 0.5
	cfg.CoinHalving = 200
	cfg.CoinCap = 150
	return cfg
}

// Coins are issued as bitcoin is (stage 11-2a): CoinRate a tick, halved
// every CoinHalving ticks, until CoinCap; once issued none is lost, so
// the coins issued are always those on the ground plus those held,
// through deaths.
func TestCoinsIssueAndKeep(t *testing.T) {
	w, err := NewWorld(coinConfig(2), tempMap())
	if err != nil {
		t.Fatal(err)
	}
	// 0.5 a tick for 200 ticks, 0.25 for the next 200: 100 then 150.
	for _, step := range []struct {
		ticks int
		want  int64
	}{{200, 100}, {200, 150}, {600, 150}} {
		for i := 0; i < step.ticks; i++ {
			w.Step()
			if l := w.CoinLedger(); !l.Balanced() {
				t.Fatalf("tick %d: coins %+v do not balance", w.Tick(), l)
			}
		}
		if got := w.CoinLedger().Issued; got != step.want {
			t.Fatalf("tick %d: %d coins issued, want %d", w.Tick(), got, step.want)
		}
	}
	st := w.Stats()
	if st.CoinsPicked == 0 || st.CoinsDropped == 0 {
		t.Fatalf("picked %d, dropped %d: want coins taken up and left by the dead", st.CoinsPicked, st.CoinsDropped)
	}
}

// Coins are issued where the map says (RegionCoin), on ground only.
func TestCoinsWhereTheMapSays(t *testing.T) {
	m := tempMap()
	m.RegionCoin = []float64{0, 0, 1, 0}
	cfg := coinConfig(3)
	cfg.Bodies = 0
	w, err := NewWorld(cfg, m)
	if err != nil {
		t.Fatal(err)
	}
	run(w, 100)
	if w.CoinLedger().Ground != 50 {
		t.Fatalf("%d coins on the ground, want 50", w.CoinLedger().Ground)
	}
	for _, g := range w.GroundCoins() {
		if r := m.Region[g.Tile]; r != 2 || m.Terrain[g.Tile] == TerrainWater {
			t.Fatalf("coins on tile %d, region %d terrain %d", g.Tile, r, m.Terrain[g.Tile])
		}
	}
	m.RegionCoin = []float64{0, 1}
	if m.Validate() == nil {
		t.Fatal("a coin share for 2 of 4 regions validated")
	}
}

// A body takes up the coins it stands on, all of them, unless its
// species leaves them (NoCoins); a dead body leaves its coins on its
// tile.
func TestCoinsPickedAndDropped(t *testing.T) {
	cfg := coinConfig(4)
	cfg.Bodies = 0
	w, err := NewWorld(cfg, tempMap())
	if err != nil {
		t.Fatal(err)
	}
	w.bodies = append(w.bodies,
		Body{ID: 1, X: 0.5, Y: 0.5, Energy: 50, Heading: -1, Goal: -1, Decided: -1, Parents: [2]int64{-1, -1}},
		Body{ID: 2, X: 2.5, Y: 0.5, Energy: 50, Heading: -1, Goal: -1, Decided: -1, Parents: [2]int64{-1, -1}, Species: 1}) // a beast: NoCoins in the default table
	w.coins.at[0], w.coins.at[2] = 3, 4
	w.coins.issued = 7
	w.pickCoins(&w.bodies[0])
	w.pickCoins(&w.bodies[1])
	if w.bodies[0].Coins != 3 || w.coins.at[0] != 0 {
		t.Fatalf("human holds %d, tile keeps %d; want 3 and 0", w.bodies[0].Coins, w.coins.at[0])
	}
	if w.bodies[1].Coins != 0 || w.coins.at[2] != 4 {
		t.Fatalf("beast holds %d, tile keeps %d; want 0 and 4", w.bodies[1].Coins, w.coins.at[2])
	}
	w.bodies[0].X = 5.5
	w.bodies[0].Energy = 0
	w.removeDead()
	if w.coins.at[5] != 3 || !w.CoinLedger().Balanced() {
		t.Fatalf("tile 5 holds %d after the death, ledger %+v", w.coins.at[5], w.CoinLedger())
	}
}

// A world with coins saved and read back runs on as if it had not been.
func TestCoinsSaveLoad(t *testing.T) {
	a, err := NewWorld(coinConfig(11), tempMap())
	if err != nil {
		t.Fatal(err)
	}
	run(a, 301)
	var buf bytes.Buffer
	if err := a.Save(&buf); err != nil {
		t.Fatal(err)
	}
	b, err := Load(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if a.CoinLedger() != b.CoinLedger() || a.Fingerprint() != b.Fingerprint() {
		t.Fatalf("loaded coins %+v, saved %+v", b.CoinLedger(), a.CoinLedger())
	}
	run(a, 200)
	run(b, 200)
	if a.Fingerprint() != b.Fingerprint() {
		t.Fatal("loaded world with coins diverged")
	}
}

// With coins issued, the world is stage 11-2a: its fingerprint is
// pinned here (testConfig leaves coins off, so the older pins hold).
func TestCoinsFingerprint(t *testing.T) {
	const want = uint64(0xea699bf5c665fea4)
	w, err := NewWorld(coinConfig(1), tempMap())
	if err != nil {
		t.Fatal(err)
	}
	run(w, 1000)
	if got := w.Fingerprint(); got != want {
		t.Fatalf("fingerprint = %#x, want %#x", got, want)
	}
}
