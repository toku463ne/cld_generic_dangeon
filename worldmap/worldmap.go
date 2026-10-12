// Package worldmap builds the maps that stages 1 and 2 measure on. It lives
// outside the engine because the engine does not know where maps come from.
//
// The maps are laid out by rule, not drawn at random: the world owns the only
// random source, and a map generator with its own would be a second one.
package worldmap

import (
	"fmt"

	"github.com/toku463ne/cld_generic_dangeon/engine"
)

// Names lists the maps Build knows.
var Names = []string{"flat", "constrained", "seasons", "patches", "waters"}

// Build returns the named map at the given size.
func Build(name string, width, height int) (engine.Map, error) {
	switch name {
	case "flat":
		return Flat(width, height), nil
	case "constrained":
		return Constrained(width, height), nil
	case "seasons":
		return Seasons(width, height), nil
	case "waters":
		return Waters(width, height), nil
	case "patches":
		return Patches(width, height), nil
	}
	return engine.Map{}, fmt.Errorf("unknown map %q (known: %v)", name, Names)
}

// DayNightTemp is every region's range of temperature on the maps here
// (stage 7-4): the normal temperature in full light, cold enough in the
// dark to raise the burn by 30%. One range for every region, so that the
// cold changes when bodies burn more and not where. Provisional;
// PARAMETERS.md records it.
var DayNightTemp = engine.TempRange{Low: 11, High: 20}

// temps is DayNightTemp for each of n regions.
func temps(n int) []engine.TempRange {
	ts := make([]engine.TempRange, n)
	for i := range ts {
		ts[i] = DayNightTemp
	}
	return ts
}

// ShelterSize is the side of each place to rest (stage 6-1a): one square
// of ShelterSize tiles at the centre of each quadrant of the maps here.
// Provisional; PARAMETERS.md records it.
const ShelterSize = 3

// shelters puts a place to rest at the centre of each quadrant, on its land.
func shelters(m *engine.Map) {
	for _, c := range [][2]int{{m.Width / 4, m.Height / 4}, {3 * m.Width / 4, m.Height / 4}, {m.Width / 4, 3 * m.Height / 4}, {3 * m.Width / 4, 3 * m.Height / 4}} {
		for y := c[1] - ShelterSize/2; y < c[1]-ShelterSize/2+ShelterSize; y++ {
			for x := c[0] - ShelterSize/2; x < c[0]-ShelterSize/2+ShelterSize; x++ {
				if m.InBounds(x, y) && m.TerrainAt(x, y).Ground() {
					m.SetShelter(x, y)
				}
			}
		}
	}
}

// The shop a map sets (stages 11-2b and 11-2c, the user's design:
// prices, number and goods are the map's; for now one shop at fixed
// prices). It sells human food at ShopPrice coins a unit and pays
// ShopOffer for one sent, starts with ShopStock units and holds ShopCap
// at most; it sells tents at TentPrice and pays TentOffer, holding up to
// TentCap, none at first; and it takes up to ShopFund coins. The offers
// are under the prices (the shop's margin), which is why food costs 2:
// coins are whole. Provisional; PARAMETERS.md records them.
const (
	ShopPrice = 2
	ShopOffer = 1
	ShopStock = 50
	ShopCap   = 100
	TentPrice = 4
	TentOffer = 2
	TentCap   = 20
	// Each kind of ornament (stage 11-4): sold at OrnamentPrice, bought
	// at OrnamentOffer, OrnamentCap at most, none at first.
	OrnamentPrice = 3
	OrnamentOffer = 2
	OrnamentCap   = 10
	ShopFund      = 500
)

// addShop sets the map's shop centred on (x, y), where its building and
// doors fit (a small map may have no room between its places to rest).
func addShop(m *engine.Map, x, y int) {
	m.Shops = []engine.Shop{shop(x, y)}
	if m.Validate() != nil {
		m.Shops = nil
	}
}

func shop(x, y int) engine.Shop {
	s := engine.Shop{X: x, Y: y, FundCap: ShopFund, Goods: []engine.Good{
		{Item: "human food", Price: ShopPrice, Offer: ShopOffer, Stock: ShopStock, StockCap: ShopCap},
		{Item: engine.TentGood, Price: TentPrice, Offer: TentOffer, StockCap: TentCap},
	}}
	// Every kind a world may have (engine.SixOrnaments); a kind the world
	// lacks is a good the shop keeps shut.
	for _, o := range engine.SixOrnaments() {
		s.Goods = append(s.Goods, engine.Good{Item: o.Name, Price: OrnamentPrice, Offer: OrnamentOffer, StockCap: OrnamentCap})
	}
	return s
}

// Flat is the calibration map: all land, one region.
func Flat(width, height int) engine.Map {
	m := engine.NewMap(width, height)
	m.RegionTemp = temps(1)
	shelters(&m)
	return m
}

// ConstrainedFood is each region's share of the food on the constrained
// map, in region ID order: the humans' ground (engine.HumanShare of it)
// five to one between left and right, the beasts' strip three to one.
// Provisional; PARAMETERS.md records it.
var ConstrainedFood = []float64{engine.HumanShare * 5 / 6, engine.HumanShare / 6, (1 - engine.HumanShare) * 3 / 4, (1 - engine.HumanShare) / 4}

// beastRows is how many rows at the foot of a map of the given height
// are the beasts' ground: one part in nine (engine.HumanShare), at
// least one.
func beastRows(height int) int {
	return max(int(float64(height)*(1-engine.HumanShare)+0.5), 1)
}

// Constrained is the map where food is meant to bind the population. It
// has four regions (IDs 0 to 3 in reading order): the humans' ground,
// eight parts in nine of the height, cut in two at half the width, and
// the beasts' strip at the foot, cut the same way, which takes unequal
// shares of the food (ConstrainedFood); and a two-tile strip of water
// running the full height that cuts the land in two. (Until stage 10-6
// the beasts had the bottom half; the user chose a human-centred world
// to measure in, 2026-10-11.)
//
// The water sits at five eighths of the width, not on the region boundary at
// one half, so that terrain and region cut across each other: the two are
// separate maps, and a layout where they coincide could not show a rule
// confusing one for the other.
func Constrained(width, height int) engine.Map {
	m := engine.NewMap(width, height)
	m.RegionFood = append([]float64(nil), ConstrainedFood...)
	m.RegionTemp = temps(len(ConstrainedFood))
	split := height - beastRows(height)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			r := 0
			if x >= width/2 {
				r++
			}
			if y >= split {
				r += 2
			}
			m.SetRegion(x, y, engine.RegionID(r))
		}
	}
	// The beasts' strip is forest (stage 10-2h): their food returns in
	// it, and each species burns more off its own ground.
	for y := split; y < height; y++ {
		for x := 0; x < width; x++ {
			m.SetTerrain(x, y, engine.TerrainForest)
		}
	}
	water := width * 5 / 8
	for y := 0; y < height; y++ {
		for x := water; x < water+2 && x < width; x++ {
			m.SetTerrain(x, y, engine.TerrainWater)
		}
	}
	// The species keep apart (stage 9-7b-2): human food comes back on
	// the humans' ground (regions 0 and 1), beast food in the strip (2
	// and 3), each at the map's ratio between its two regions.
	m.PoolFood = map[string][]float64{
		"human food": {ConstrainedFood[0], ConstrainedFood[1], 0, 0},
		"beast food": {0, 0, ConstrainedFood[2], ConstrainedFood[3]},
	}
	// Coins are issued on the humans' ground, as their food is (stage
	// 11-2a): beasts have nothing to buy.
	m.RegionCoin = append([]float64(nil), m.PoolFood["human food"]...)
	shelters(&m)
	// One shop (stage 11-2b), in the middle of the richer human region,
	// between its two places to rest.
	addShop(&m, width/4, (height-beastRows(height))/2)
	return m
}

// WaterWidth is the width of the water on the waters map (stage 9-7b-2c):
// wide enough for a species of the water to live in, where the
// constrained map's two tiles are a wall to cross.
const WaterWidth = 8

// Waters is the constrained map with its water WaterWidth tiles wide, and
// a pool of fish food that comes back in the water (regions 1 and 3,
// which the water runs through). The land species keep apart as on the
// constrained map.
func Waters(width, height int) engine.Map {
	m := Constrained(width, height)
	water := width * 5 / 8
	for y := 0; y < height; y++ {
		for x := water; x < water+WaterWidth && x < width; x++ {
			m.SetTerrain(x, y, engine.TerrainWater)
		}
	}
	m.PoolFood["fish food"] = []float64{0, 1, 0, 1}
	m.Shelter = nil
	shelters(&m)
	return m
}

// SeasonTicks is the length of a season on the seasons map: four of them,
// 4000 ticks, run over three median lives, so that one life cannot see the
// whole round. Provisional; PARAMETERS.md records it.
const SeasonTicks = 1000

// Seasons is the constrained map with its food shares moving: each season
// the shares move on by one region, 0 to 1 to 3 to 2 and round (clockwise
// through the quadrants), so the richest quadrant goes round the map once
// every four seasons. Which region will be rich next is a law of the map,
// not of where bodies are - but one life sees only part of the round.
func Seasons(width, height int) engine.Map {
	m := Constrained(width, height)
	m.PoolFood = nil           // every pool follows the seasons
	round := []int{0, 1, 3, 2} // the quadrants clockwise
	for s := range round {
		shares := make([]float64, len(ConstrainedFood))
		for i, r := range round {
			// In season s the quadrant i steps on holds what the one s
			// steps back held in season 0.
			shares[r] = ConstrainedFood[round[(i-s+len(round))%len(round)]]
		}
		m.SeasonFood = append(m.SeasonFood, shares)
	}
	m.SeasonTicks = SeasonTicks
	return m
}

// PatchSize is the side of each food patch on the patches map (stage
// 8-1b): four of them hold some 400 land tiles for a food cap of 300.
// Provisional; PARAMETERS.md records it.
const PatchSize = 10

// Patches is the map where food is held in four places (stage 8-1b, the
// user's design): all land, the food coming back only to four square
// patches (regions 1 to 4, a quarter of the food each) set as a diamond
// round the centre - midway along the edges of the square the quadrant
// centres make, so that no patch lies on the places to rest placed there.
// The rest of the land (region 0) gets no food. Against the flat map,
// where food is everywhere, it asks whether a place to stay is worth
// anything when food holds bodies to a place.
func Patches(width, height int) engine.Map {
	m := engine.NewMap(width, height)
	h := engine.HumanShare / 3
	m.RegionFood = []float64{0, h, h, h, 1 - engine.HumanShare}
	m.RegionTemp = temps(len(m.RegionFood))
	centres := [][2]int{{width / 2, height / 4}, {width / 4, height / 2}, {3 * width / 4, height / 2}, {width / 2, 3 * height / 4}}
	for i, c := range centres {
		for y := c[1] - PatchSize/2; y < c[1]-PatchSize/2+PatchSize; y++ {
			for x := c[0] - PatchSize/2; x < c[0]-PatchSize/2+PatchSize; x++ {
				if m.InBounds(x, y) {
					m.SetRegion(x, y, engine.RegionID(i+1))
				}
			}
		}
	}
	// The species keep apart (stage 9-7b-2): human food comes back in
	// the top, left and right patches (regions 1 to 3), beast food in
	// the bottom one (4), which is forest (stage 10-2h; the beasts had
	// the right patch too until the user chose a human-centred world,
	// 2026-10-11).
	for i := range m.Region {
		if m.Region[i] == 4 {
			m.Terrain[i] = engine.TerrainForest
		}
	}
	m.PoolFood = map[string][]float64{
		"human food": {0, 1.0 / 3, 1.0 / 3, 1.0 / 3, 0},
		"beast food": {0, 0, 0, 0, 1},
	}
	m.RegionCoin = append([]float64(nil), m.PoolFood["human food"]...) // as on the constrained map
	shelters(&m)
	addShop(&m, width/2, height/2) // in the middle, between the patches
	return m
}
