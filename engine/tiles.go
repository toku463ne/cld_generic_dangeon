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

// Map is the terrain layer and the region layer of a world.
type Map struct {
	Width, Height int
	Terrain       []Terrain
	Region        []RegionID
}

// NewMap returns a map that is all land and all region 0.
func NewMap(width, height int) Map {
	n := width * height
	return Map{
		Width:   width,
		Height:  height,
		Terrain: make([]Terrain, n),
		Region:  make([]RegionID, n),
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
	return nil
}

// InBounds reports whether (x, y) is a tile of the map.
func (m Map) InBounds(x, y int) bool {
	return x >= 0 && y >= 0 && x < m.Width && y < m.Height
}

func (m Map) index(x, y int) int { return y*m.Width + x }

// TerrainAt returns the terrain of tile (x, y), which must be in bounds.
func (m Map) TerrainAt(x, y int) Terrain { return m.Terrain[m.index(x, y)] }

// RegionAt returns the region of tile (x, y), which must be in bounds.
func (m Map) RegionAt(x, y int) RegionID { return m.Region[m.index(x, y)] }

// SetTerrain paints the terrain layer only.
func (m Map) SetTerrain(x, y int, t Terrain) { m.Terrain[m.index(x, y)] = t }

// SetRegion paints the region layer only.
func (m Map) SetRegion(x, y int, r RegionID) { m.Region[m.index(x, y)] = r }

// Clone returns a copy that shares no storage with m.
func (m Map) Clone() Map {
	c := m
	c.Terrain = append([]Terrain(nil), m.Terrain...)
	c.Region = append([]RegionID(nil), m.Region...)
	return c
}
