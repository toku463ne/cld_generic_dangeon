package engine

import (
	"math"
	"testing"
)

// checkFood verifies the food account and the tile index against the food
// itself.
func checkFood(t *testing.T, w *World) {
	t.Helper()
	l := w.FoodLedger()
	if !l.Balanced() {
		t.Fatalf("tick %d: food ledger does not close: %+v", w.Tick(), l)
	}
	seen := map[int]bool{}
	for i, f := range w.food.foods {
		tile := w.m.index(f.X, f.Y)
		if w.m.Terrain[tile] != TerrainLand {
			t.Fatalf("tick %d: food on water at (%d,%d)", w.Tick(), f.X, f.Y)
		}
		if seen[tile] {
			t.Fatalf("tick %d: two units on tile (%d,%d)", w.Tick(), f.X, f.Y)
		}
		seen[tile] = true
		if w.foodOn(tile) != i {
			t.Fatalf("tick %d: tile index says %d, food is %d", w.Tick(), w.foodOn(tile), i)
		}
	}
	for tile := range w.food.foodAt {
		if w.foodOn(tile) >= 0 && !seen[tile] {
			t.Fatalf("tick %d: tile %d indexed with no food on it", w.Tick(), tile)
		}
	}
}

func TestFoodLedgerClosesEveryTick(t *testing.T) {
	w := newTestWorld(t, 4)
	checkFood(t, w)
	for i := 0; i < 3000; i++ {
		w.Step()
		checkFood(t, w)
	}
	if w.FoodLedger().Eaten == 0 {
		t.Fatal("nothing was eaten, so the test checked nothing")
	}
}

// A vacancy comes back, and only up to the cap.
func TestVacanciesReturnUpToCap(t *testing.T) {
	cfg := testConfig(2)
	cfg.Bodies = 0
	cfg.FoodReturn = 1
	w, err := NewWorld(cfg, testMap())
	if err != nil {
		t.Fatal(err)
	}
	if got := len(w.food.foods); got != cfg.FoodCap {
		t.Fatalf("world starts with %d food, want the cap %d", got, cfg.FoodCap)
	}
	for i := 0; i < 5; i++ {
		w.eatFood(0)
	}
	for i := 0; i < 20; i++ {
		w.Step()
		checkFood(t, w)
	}
	if got := len(w.food.foods); got != cfg.FoodCap {
		t.Fatalf("after returning, %d food, want the cap %d", got, cfg.FoodCap)
	}
}

// Where food appears follows the regions' shares, not their areas.
func TestFoodFollowsRegionShares(t *testing.T) {
	m := NewMap(40, 40)
	m.RegionFood = []float64{3, 1}
	for y := 0; y < m.Height; y++ {
		for x := 0; x < m.Width; x++ {
			if x >= 10 { // region 1 has three times the area of region 0
				m.SetRegion(x, y, 1)
			}
		}
	}
	cfg := DefaultConfig()
	cfg.Bodies = 0
	cfg.FoodCap = 400
	counts := [2]int{}
	for seed := int64(1); seed <= 20; seed++ {
		cfg.Seed = seed
		w, err := NewWorld(cfg, m)
		if err != nil {
			t.Fatal(err)
		}
		for _, f := range w.food.foods {
			counts[m.RegionAt(f.X, f.Y)]++
		}
	}
	share := float64(counts[0]) / float64(counts[0]+counts[1])
	if math.Abs(share-0.75) > 0.03 {
		t.Fatalf("region 0 holds %.3f of the food, want its share 0.75", share)
	}
}

func TestMapWithoutFoodShareIsRejected(t *testing.T) {
	m := NewMap(4, 4)
	m.SetRegion(0, 0, 1) // region 1 has no share
	if _, err := NewWorld(DefaultConfig(), m); err == nil {
		t.Fatal("NewWorld accepted a region with no food share")
	}
	m = NewMap(4, 4)
	m.RegionFood = []float64{0}
	if _, err := NewWorld(DefaultConfig(), m); err == nil {
		t.Fatal("NewWorld accepted a map where no region has food")
	}
}

// On a map with seasons, food that comes back goes by the season's shares,
// the food on the ground stays where it is, and the ledger still closes.
func TestFoodFollowsSeasons(t *testing.T) {
	m := NewMap(20, 20)
	m.RegionFood = []float64{1, 1}
	m.SeasonFood = [][]float64{{1, 0}, {0, 1}}
	m.SeasonTicks = 100
	for y := 0; y < m.Height; y++ {
		for x := 10; x < m.Width; x++ {
			m.SetRegion(x, y, 1)
		}
	}
	cfg := DefaultConfig()
	cfg.Bodies, cfg.FoodCap, cfg.FoodReturn = 0, 100, 1
	w, err := NewWorld(cfg, m)
	if err != nil {
		t.Fatal(err)
	}
	if got := w.food.onGround[1]; got != 0 {
		t.Fatalf("season 0 started with %d food in region 1, which has no share then", got)
	}
	// In season 1 food eaten comes back to region 1 only.
	w.tick = 100
	for i := 0; i < 30; i++ {
		w.eatFood(0)
	}
	for i := 0; i < 5; i++ {
		w.Step()
		checkFood(t, w)
	}
	if w.food.onGround[1] != 30 || w.food.onGround[0] != 70 {
		t.Fatalf("season 1: region 0 %d, region 1 %d, want 70 and 30", w.food.onGround[0], w.food.onGround[1])
	}
	if s := m.Season(399); s != 1 {
		t.Fatalf("season at tick 399 is %d, want 1", s)
	}
}

func TestBadSeasonsAreRejected(t *testing.T) {
	for _, m := range []Map{
		func() Map { m := NewMap(4, 4); m.SeasonFood = [][]float64{{1}}; return m }(),                         // no length
		func() Map { m := NewMap(4, 4); m.SeasonFood, m.SeasonTicks = [][]float64{{1, 1}}, 10; return m }(),   // shares per region
		func() Map { m := NewMap(4, 4); m.SeasonFood, m.SeasonTicks = [][]float64{{1}, {0}}, 10; return m }(), // a season with no food
	} {
		if _, err := NewWorld(DefaultConfig(), m); err == nil {
			t.Fatalf("NewWorld accepted seasons %v of %d ticks", m.SeasonFood, m.SeasonTicks)
		}
	}
}
