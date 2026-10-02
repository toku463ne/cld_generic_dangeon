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

// Food that comes back goes to a resting mother instead of the ground with
// ProvisionWith if her partner is in her region and ProvisionAlone if not;
// it appears and is eaten at once, so the ledger still closes; a mother not
// resting, or a region without one, gets nothing.
func TestProvision(t *testing.T) {
	for _, c := range []struct {
		name            string
		partnerX        float64
		resting         bool
		with, alone     float64
		wantProvisioned bool
	}{
		{"partner near", 3.5, true, 1, 0, true},
		{"partner away", 3.5, true, 1, 0, false},
		{"partner gone", -1, true, 0, 1, true},
		{"not resting", 3.5, false, 1, 1, false},
	} {
		m := NewMap(10, 10)
		m.RegionFood = []float64{1, 1}
		for y := 0; y < m.Height; y++ {
			for x := 5; x < m.Width; x++ {
				m.SetRegion(x, y, 1)
			}
		}
		cfg := DefaultConfig()
		cfg.Bodies, cfg.FoodCap, cfg.FoodReturn = 0, 20, 1
		cfg.ProvisionWith, cfg.ProvisionAlone = c.with, c.alone
		cfg.ProvisionEach = false // each unit by chance (3-3)
		w, err := NewWorld(cfg, m)
		if err != nil {
			t.Fatal(err)
		}
		mother := Body{ID: 0, X: 2.5, Y: 2.5, Energy: 10, Sex: Female, Partner: 1, Heading: -1, Goal: -1, Decided: -1, Parents: [2]int64{-1, -1}}
		if c.resting {
			mother.Rested = w.tick + 100
		}
		w.bodies = append(w.bodies, mother)
		if c.partnerX >= 0 {
			// Near: the same region (x < 5); away: the other region.
			x := c.partnerX
			if c.name == "partner away" {
				x = 7.5
			}
			w.bodies = append(w.bodies, Body{ID: 1, X: x, Y: 2.5, Energy: 100, Sex: Male, Partner: -1, Heading: -1, Goal: -1, Decided: -1, Parents: [2]int64{-1, -1}})
		}
		w.nextID = 2
		w.buildGrid()
		for i := 0; i < 20; i++ {
			w.eatFood(0)
		}
		w.returnFood()
		checkFood(t, w)
		l := w.FoodLedger()
		got := l.Provisioned > 0
		if got != c.wantProvisioned {
			t.Fatalf("%s: provisioned %d", c.name, l.Provisioned)
		}
		if got && w.bodies[0].Energy <= 10 {
			t.Fatalf("%s: provisioned, but the mother's energy is %v", c.name, w.bodies[0].Energy)
		}
		if got && l.Provisioned+int64(l.OnGround) != 20 {
			t.Fatalf("%s: %d provisioned and %d on the ground of 20 returned", c.name, l.Provisioned, l.OnGround)
		}
	}
}

// With ProvisionEach a resting mother asks for a unit by her own chance -
// ProvisionEachWith with her partner in her region, ProvisionEachAlone
// without - holding at most two asks, and food coming back goes to mothers
// who asked before the ground; a mother who has not asked, or is not
// resting, gets none.
func TestProvisionEach(t *testing.T) {
	m := NewMap(10, 10)
	m.RegionFood = []float64{1, 1}
	for y := 0; y < m.Height; y++ {
		for x := 5; x < m.Width; x++ {
			m.SetRegion(x, y, 1)
		}
	}
	cfg := DefaultConfig()
	cfg.Bodies, cfg.FoodCap, cfg.FoodReturn = 0, 20, 1
	cfg.ProvisionEachWith, cfg.ProvisionEachAlone = 1, 0
	w, err := NewWorld(cfg, m)
	if err != nil {
		t.Fatal(err)
	}
	body := func(id int64, x float64, sex Sex, partner int64, rested int64) Body {
		return Body{ID: id, X: x, Y: 2.5, Energy: 10, Sex: sex, Partner: partner, Rested: rested, Heading: -1, Goal: -1, Decided: -1, Parents: [2]int64{-1, -1}}
	}
	w.bodies = append(w.bodies,
		body(0, 1.5, Female, 2, 100), // resting, partner near: asks
		body(1, 3.5, Female, 3, 100), // resting, partner away: never asks
		body(2, 2.5, Male, -1, 0),
		body(3, 7.5, Male, -1, 0),
		body(4, 4.5, Female, 2, 0), // not resting
	)
	w.nextID = 5
	w.buildGrid()
	w.askProvision()
	w.askProvision()
	w.askProvision()
	if got := []int{w.bodies[0].Asks, w.bodies[1].Asks, w.bodies[4].Asks}; got[0] != 2 || got[1] != 0 || got[2] != 0 {
		t.Fatalf("asks %v, want [2 0 0]", got)
	}
	for i := 0; i < 20; i++ {
		w.eatFood(0)
	}
	w.bodies[0].Asks = 1 // one ask left before the food comes back
	w.returnFood()       // asks again (to 2), then is served both
	checkFood(t, w)
	if l := w.FoodLedger(); l.Provisioned != 2 || w.bodies[0].Asks != 0 {
		t.Fatalf("provisioned %d, mother's asks %d, want 2 and 0", l.Provisioned, w.bodies[0].Asks)
	}
	if w.bodies[0].Energy != 10+2*cfg.FoodEnergy || w.bodies[1].Energy != 10 || w.bodies[4].Energy != 10 {
		t.Fatalf("energies %v %v %v", w.bodies[0].Energy, w.bodies[1].Energy, w.bodies[4].Energy)
	}
	// Her rest over, her asks go.
	w.bodies[0].Asks, w.tick = 2, 100
	w.askProvision()
	if w.bodies[0].Asks != 0 {
		t.Fatalf("rested mother keeps %d asks", w.bodies[0].Asks)
	}
}

// A unit decays FoodLife ticks after it appears, on the ground or held,
// and not before; it becomes a vacancy, and the ledger still closes.
func TestFoodDecays(t *testing.T) {
	w, b := soloWorld(t, 90)
	w.cfg.FoodLife = 10
	w.cfg.FoodReturn = 0 // nothing comes back, so the ground holds only the test's units
	w.cfg.Carry = 1
	w.act(0, b, Action{Kind: ActPick})
	if b.Held != 1 || len(b.HeldBorn) != 1 {
		t.Fatalf("held %d, ages %v", b.Held, b.HeldBorn)
	}
	tile := w.tileOf(b.X, b.Y)
	w.food.foods = append(w.food.foods, Food{X: tile % w.m.Width, Y: tile / w.m.Width})
	w.food.born = append(w.food.born, w.tick+5) // appears five ticks later
	w.food.foodAt[tile] = int32(len(w.food.foods))
	w.food.appeared++
	w.foodMoved(w.m.Region[tile], +1)
	w.tick = 9
	w.decayFood()
	if l := w.FoodLedger(); l.Decayed != 0 || l.OnGround != 1 || l.Held != 1 {
		t.Fatalf("decayed early: %+v", l)
	}
	w.tick = 10
	w.decayFood()
	l := w.FoodLedger()
	if l.Decayed != 1 || l.Held != 0 || b.Held != 0 || len(b.HeldBorn) != 0 || l.OnGround != 1 || !l.Balanced() {
		t.Fatalf("held unit at its life: %+v, body held %d", l, b.Held)
	}
	w.tick = 15
	w.decayFood()
	l = w.FoodLedger()
	if l.Decayed != 2 || l.OnGround != 0 || w.foodOn(tile) >= 0 || !l.Balanced() {
		t.Fatalf("ground unit at its life: %+v", l)
	}
	w.cfg.FoodLife = 0
	w.food.foods = append(w.food.foods, Food{X: tile % w.m.Width, Y: tile / w.m.Width})
	w.food.born = append(w.food.born, 0)
	w.food.foodAt[tile] = int32(len(w.food.foods))
	w.tick = 1000
	w.decayFood()
	if len(w.food.foods) != 1 {
		t.Fatal("decayed with FoodLife 0")
	}
}

// Over a long run with decay, the ledger closes every tick and some units
// decay.
func TestFoodLedgerClosesWithDecay(t *testing.T) {
	cfg := testConfig(3)
	cfg.FoodLife = 50
	w, err := NewWorld(cfg, testMap())
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 400; i++ {
		w.Step()
		if l := w.FoodLedger(); !l.Balanced() {
			t.Fatalf("tick %d: %+v", w.Tick(), l)
		}
	}
	if w.FoodLedger().Decayed == 0 {
		t.Fatal("nothing decayed")
	}
}
