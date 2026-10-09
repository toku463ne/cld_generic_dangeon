package engine

import "fmt"

// The map is two layers over the same grid of tiles. Terrain says what moving
// onto a tile demands; region says what the world gives there. They are
// separate slices on purpose: painting one can never change the other.
//
// The engine does not read map files. Whoever builds a world (a generator, or
// later a Tiled loader on the client side) hands it a Map.

// Terrain is what moving onto a tile demands.
type Terrain uint8

const (
	TerrainLand Terrain = iota
	TerrainWater
)

// RegionID names the region a tile belongs to. What a region gives (how fast
// food appears, and so on) is attached to the ID by later stages.
type RegionID uint16

// Map is the terrain layer and the region layer of a world, and what each
// region gives.
type Map struct {
	Width, Height int
	Terrain       []Terrain
	Region        []RegionID

	// RegionFood is each region's share of the food that appears, indexed by
	// RegionID. It is a share of one map-wide total, not an amount of its
	// own: a richer region takes more of the same food, it does not make more.
	// Only the ratios matter.
	RegionFood []float64

	// SeasonFood, if not empty, replaces RegionFood season by season: the
	// world is in season (tick / SeasonTicks) mod len(SeasonFood), and food
	// that comes back goes to the regions by that season's shares. Food on
	// the ground stays where it is, so it follows the seasons late. Each
	// season lists a share per region, as RegionFood does.
	SeasonFood  [][]float64 `json:",omitempty"`
	SeasonTicks int         `json:",omitempty"`

	// PoolFood, if set, gives a pool of food (Config.Foods, by name) its
	// own region shares in place of RegionFood and the seasons (stage
	// 9-7b-2): where that pool comes back. A pool not named here follows
	// RegionFood. This is how a map keeps species apart: each species'
	// food returns in its own regions.
	PoolFood map[string][]float64 `json:",omitempty"`

	// RegionTemp is each region's range of temperature, indexed by
	// RegionID (stage 7-4): a tile's temperature is its region's Low in the
	// dark and High in full light, and follows the light between. Empty
	// for a map without temperature.
	RegionTemp []TempRange `json:",omitempty"`

	// Shelter marks the tiles of the places to rest (stage 6-1a): on them
	// the temperature stays normal through the night. Empty for a map
	// without any.
	Shelter []bool `json:",omitempty"`
}

// TempRange is the range a region's temperature moves in over a day.
type TempRange struct{ Low, High float64 }

// Temp returns the temperature in region r at light l (0 to 1), and ok
// false on a map without temperature.
func (m *Map) Temp(r RegionID, l float64) (t float64, ok bool) {
	if len(m.RegionTemp) == 0 {
		return 0, false
	}
	tr := m.RegionTemp[r]
	return tr.Low + (tr.High-tr.Low)*l, true
}

// Season returns the index of the season at tick t, and 0 for a map
// without seasons.
func (m *Map) Season(t int64) int {
	if len(m.SeasonFood) == 0 || m.SeasonTicks <= 0 {
		return 0
	}
	return int((t / int64(m.SeasonTicks)) % int64(len(m.SeasonFood)))
}

// FoodShares returns the regions' shares of the food that comes back at
// tick t: the season's, or RegionFood.
func (m *Map) FoodShares(t int64) []float64 {
	if len(m.SeasonFood) == 0 || m.SeasonTicks <= 0 {
		return m.RegionFood
	}
	return m.SeasonFood[m.Season(t)]
}

// PoolShares returns the regions' shares of the food of the named pool
// that comes back at tick t: the pool's own (PoolFood), or FoodShares.
func (m *Map) PoolShares(name string, t int64) []float64 {
	if s, ok := m.PoolFood[name]; ok {
		return s
	}
	return m.FoodShares(t)
}

// NewMap returns a map that is all land and all region 0, which takes all of
// the food.
func NewMap(width, height int) Map {
	n := width * height
	return Map{
		Width:      width,
		Height:     height,
		Terrain:    make([]Terrain, n),
		Region:     make([]RegionID, n),
		RegionFood: []float64{1},
	}
}

// Validate reports whether the layers match the declared size.
func (m Map) Validate() error {
	if m.Width <= 0 || m.Height <= 0 {
		return fmt.Errorf("map size %dx%d is not positive", m.Width, m.Height)
	}
	n := m.Width * m.Height
	if len(m.Terrain) != n {
		return fmt.Errorf("terrain layer has %d tiles, want %d", len(m.Terrain), n)
	}
	if len(m.Region) != n {
		return fmt.Errorf("region layer has %d tiles, want %d", len(m.Region), n)
	}
	for i, t := range m.Terrain {
		if t > TerrainWater {
			return fmt.Errorf("tile %d has unknown terrain %d", i, t)
		}
	}
	for i, r := range m.Region {
		if int(r) >= len(m.RegionFood) {
			return fmt.Errorf("tile %d is in region %d, which has no food share", i, r)
		}
	}
	sum := 0.0
	for r, f := range m.RegionFood {
		if !(f >= 0) {
			return fmt.Errorf("region %d has food share %v", r, f)
		}
		sum += f
	}
	if !(sum > 0) {
		return fmt.Errorf("no region has a food share")
	}
	if len(m.Shelter) > 0 && len(m.Shelter) != n {
		return fmt.Errorf("shelter layer has %d tiles, want %d", len(m.Shelter), n)
	}
	if len(m.RegionTemp) > 0 && len(m.RegionTemp) != len(m.RegionFood) {
		return fmt.Errorf("%d regions have a temperature, want %d", len(m.RegionTemp), len(m.RegionFood))
	}
	for r, tr := range m.RegionTemp {
		if !(tr.Low <= tr.High) {
			return fmt.Errorf("region %d has temperature %v to %v", r, tr.Low, tr.High)
		}
	}
	if len(m.SeasonFood) > 0 && m.SeasonTicks <= 0 {
		return fmt.Errorf("seasons of %d ticks", m.SeasonTicks)
	}
	for name, shares := range m.PoolFood {
		if len(shares) != len(m.RegionFood) {
			return fmt.Errorf("pool %s has %d shares, want %d", name, len(shares), len(m.RegionFood))
		}
		sum := 0.0
		for r, f := range shares {
			if !(f >= 0) {
				return fmt.Errorf("pool %s: region %d has food share %v", name, r, f)
			}
			sum += f
		}
		if !(sum > 0) {
			return fmt.Errorf("pool %s: no region has a food share", name)
		}
	}
	for s, shares := range m.SeasonFood {
		if len(shares) != len(m.RegionFood) {
			return fmt.Errorf("season %d has %d shares, want %d", s, len(shares), len(m.RegionFood))
		}
		sum := 0.0
		for r, f := range shares {
			if !(f >= 0) {
				return fmt.Errorf("season %d: region %d has food share %v", s, r, f)
			}
			sum += f
		}
		if !(sum > 0) {
			return fmt.Errorf("season %d: no region has a food share", s)
		}
	}
	return nil
}

// InBounds reports whether (x, y) is a tile of the map.
func (m *Map) InBounds(x, y int) bool {
	return x >= 0 && y >= 0 && x < m.Width && y < m.Height
}

func (m *Map) index(x, y int) int { return y*m.Width + x }

// TerrainAt returns the terrain of tile (x, y), which must be in bounds.
func (m *Map) TerrainAt(x, y int) Terrain { return m.Terrain[m.index(x, y)] }

// RegionAt returns the region of tile (x, y), which must be in bounds.
func (m *Map) RegionAt(x, y int) RegionID { return m.Region[m.index(x, y)] }

// SetTerrain paints the terrain layer only.
func (m Map) SetTerrain(x, y int, t Terrain) { m.Terrain[m.index(x, y)] = t }

// SetShelter marks tile (x, y) as a place to rest, making the layer if the
// map has none.
func (m *Map) SetShelter(x, y int) {
	if len(m.Shelter) == 0 {
		m.Shelter = make([]bool, m.Width*m.Height)
	}
	m.Shelter[m.index(x, y)] = true
}

// ShelterAt reports whether tile (x, y) is a place to rest.
func (m *Map) ShelterAt(x, y int) bool { return len(m.Shelter) > 0 && m.Shelter[m.index(x, y)] }

// SetRegion paints the region layer only.
func (m Map) SetRegion(x, y int, r RegionID) { m.Region[m.index(x, y)] = r }

// Clone returns a copy that shares no storage with m.
func (m Map) Clone() Map {
	c := m
	c.Terrain = append([]Terrain(nil), m.Terrain...)
	c.Region = append([]RegionID(nil), m.Region...)
	c.RegionFood = append([]float64(nil), m.RegionFood...)
	c.RegionTemp = append([]TempRange(nil), m.RegionTemp...)
	c.Shelter = append([]bool(nil), m.Shelter...)
	c.SeasonFood = nil
	for _, s := range m.SeasonFood {
		c.SeasonFood = append(c.SeasonFood, append([]float64(nil), s...))
	}
	c.PoolFood = nil
	if m.PoolFood != nil {
		c.PoolFood = map[string][]float64{}
		for name, s := range m.PoolFood {
			c.PoolFood[name] = append([]float64(nil), s...)
		}
	}
	return c
}
