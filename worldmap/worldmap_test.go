package worldmap

import (
	"testing"

	"github.com/toku463ne/cld_generic_dangeon/engine"
)

func TestBuildKnowsEveryName(t *testing.T) {
	for _, name := range Names {
		m, err := Build(name, 16, 12)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if err := m.Validate(); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
	if _, err := Build("nowhere", 4, 4); err == nil {
		t.Fatal("Build accepted an unknown name")
	}
}

func TestFlatIsOneRegionOfLand(t *testing.T) {
	m := Flat(10, 7)
	for i := range m.Terrain {
		if m.Terrain[i] != engine.TerrainLand || m.Region[i] != 0 {
			t.Fatalf("tile %d is terrain %d region %d", i, m.Terrain[i], m.Region[i])
		}
	}
}

// The constrained map must have four regions, water that cuts the land in two
// from edge to edge, and water that does not follow a region boundary.
func TestConstrainedLayout(t *testing.T) {
	m := Constrained(32, 24)
	seen := map[engine.RegionID]bool{}
	for _, r := range m.Region {
		seen[r] = true
	}
	if len(seen) != 4 {
		t.Fatalf("constrained map has %d regions, want 4", len(seen))
	}
	for y := 0; y < m.Height; y++ {
		water := 0
		for x := 0; x < m.Width; x++ {
			if m.TerrainAt(x, y) == engine.TerrainWater {
				water++
			}
		}
		if water == 0 {
			t.Fatalf("row %d has no water, so the land is not cut in two", y)
		}
	}
	// Some water tile and its land neighbour on the left share a region.
	x := m.Width * 5 / 8
	if m.RegionAt(x, 0) != m.RegionAt(x-1, 0) {
		t.Fatal("water lies on a region boundary; the layers should cut across each other")
	}
}

// On the seasons map the richest quadrant goes round clockwise, one step a
// season, over the constrained map's layout.
func TestSeasonsGoRound(t *testing.T) {
	m := Seasons(64, 48)
	if err := m.Validate(); err != nil {
		t.Fatal(err)
	}
	richest := func(shares []float64) int {
		best := 0
		for r, s := range shares {
			if s > shares[best] {
				best = r
			}
		}
		return best
	}
	want := []int{0, 1, 3, 2}
	for s, shares := range m.SeasonFood {
		if got := richest(shares); got != want[s] {
			t.Fatalf("season %d: richest region %d, want %d", s, got, want[s])
		}
	}
	if m.Season(int64(SeasonTicks)*5) != 1 {
		t.Fatalf("season at tick %d is %d, want 1", SeasonTicks*5, m.Season(int64(SeasonTicks)*5))
	}
}
