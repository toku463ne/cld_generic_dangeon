package engine

import "testing"

// Painting one layer must never change the other.
func TestLayersAreIndependent(t *testing.T) {
	m := NewMap(8, 5)
	for y := 0; y < m.Height; y++ {
		for x := 0; x < m.Width; x++ {
			m.SetRegion(x, y, RegionID(x/4+2*(y/3)))
		}
	}
	regions := append([]RegionID(nil), m.Region...)
	for y := 0; y < m.Height; y++ {
		m.SetTerrain(3, y, TerrainWater)
	}
	for i, r := range m.Region {
		if r != regions[i] {
			t.Fatalf("painting terrain changed region of tile %d: %d, want %d", i, r, regions[i])
		}
	}

	terrain := append([]Terrain(nil), m.Terrain...)
	for x := 0; x < m.Width; x++ {
		m.SetRegion(x, 0, 9)
	}
	for i, tr := range m.Terrain {
		if tr != terrain[i] {
			t.Fatalf("painting regions changed terrain of tile %d: %d, want %d", i, tr, terrain[i])
		}
	}
}

func TestMapValidate(t *testing.T) {
	if err := NewMap(3, 2).Validate(); err != nil {
		t.Fatalf("fresh map: %v", err)
	}
	bad := []Map{
		{Width: 0, Height: 2},
		{Width: 2, Height: 2, Terrain: make([]Terrain, 3), Region: make([]RegionID, 4)},
		{Width: 2, Height: 2, Terrain: make([]Terrain, 4), Region: make([]RegionID, 1)},
		{Width: 1, Height: 1, Terrain: []Terrain{7}, Region: make([]RegionID, 1)},
	}
	for i, m := range bad {
		if m.Validate() == nil {
			t.Errorf("map %d: Validate accepted a malformed map", i)
		}
	}
}

func TestCloneSharesNothing(t *testing.T) {
	m := NewMap(2, 2)
	c := m.Clone()
	c.SetTerrain(0, 0, TerrainWater)
	c.SetRegion(1, 1, 5)
	if m.TerrainAt(0, 0) != TerrainLand || m.RegionAt(1, 1) != 0 {
		t.Fatal("painting a clone changed the original")
	}
}
