package engine

import (
	"fmt"
	"math"
)

// Species (stage 9-7a).
//
// A species is a row of a table the config holds, and a body carries the
// index of its row: every species runs the same rules on the same engine,
// and what sets them apart is only what the row says - for now how the
// scale of their budget sits, and later which terrains they may enter,
// which kinds of food they may eat, and how they breed (NODE.md "種族").
// No rule asks "is it a beast"; it reads the row. A body mates only within
// its species, and a child is of its parents' species. A world with no
// table, or one row, is a world of one species: the one before this stage.

// SpeciesSpec is one species: its name, its share of the first bodies
// drawn, and the levels of BudgetStep its whole budget scale is shifted
// by (a beast at +1 has the config's budget at one level below its
// neutral, and its neutral is BudgetStep times the config's).
type SpeciesSpec struct {
	Name  string
	Share float64
	Size  int
	// Terrains are the terrains its bodies may enter (stage 9-7b-2c); nil
	// for land alone. A species that does not list water may still wade
	// into it with Config.Wade, at WaterBurn times its burn; Swims says
	// water costs it nothing extra (and it lists water).
	Terrains []Terrain `json:",omitempty"`
	Swims    bool      `json:",omitempty"`
	// NoSkills says its bodies are born with no skill and use none
	// (stage 9-7b-4, VISION: only humans acquire and use skills). The
	// zero row is skilled, so a world without a table is as before.
	NoSkills bool `json:",omitempty"`
	// NoCoins says its bodies leave coins lying (stage 11-2a): they have
	// nothing to buy. The zero row takes them up.
	NoCoins bool `json:",omitempty"`
	// Preys names the species its bodies hunt (stage 10-2a, hunt.go).
	Preys []string `json:",omitempty"`
	// Likes and Dislikes are the terrains its bodies burn least and most
	// on (stage 10-2h, VISION "好む地形・苦手な地形"): on a liked terrain
	// the burn is its own, on any other TerrainBurn times it, on a
	// disliked one AvoidBurn times. Water for a wader is WaterBurn apart.
	Likes    []Terrain `json:",omitempty"`
	Dislikes []Terrain `json:",omitempty"`
}

// terrainBurn is the factor the terrain of tile t puts on a body of
// species s: one on a terrain it likes, AvoidBurn on one it dislikes,
// TerrainBurn on any other ground; water is read apart (waterBurn).
func (w *World) terrainBurn(s int, t Terrain) float64 {
	if !t.Ground() {
		return 1
	}
	spec := w.specOf(s)
	for _, x := range spec.Likes {
		if x == t {
			return 1
		}
	}
	for _, x := range spec.Dislikes {
		if x == t {
			return w.cfg.AvoidBurn
		}
	}
	if len(spec.Likes) == 0 && len(spec.Dislikes) == 0 {
		return 1 // a row without preferences is at home everywhere
	}
	return w.cfg.TerrainBurn
}

// groundBurn is the factor the ground body b stands on puts on its burn.
func (w *World) groundBurn(b *Body) float64 {
	t := w.tileOf(b.X, b.Y)
	if t < 0 {
		return 1
	}
	return w.terrainBurn(b.Species, w.m.Terrain[t])
}

// enters reports whether species s may be on terrain t by its row alone.
func (s SpeciesSpec) enters(t Terrain) bool {
	if len(s.Terrains) == 0 {
		return t.Ground()
	}
	for _, x := range s.Terrains {
		if x == t {
			return true
		}
	}
	return false
}

// mayEnter reports whether body b may step onto tile t: its species' own
// terrains, or water by wading (Wade).
func (w *World) mayEnter(b *Body, t int) bool {
	return w.canStand(b.Species, w.m.Terrain[t]) && !w.walled(t)
}

// canStand reports whether a body of species s may stand on terrain t.
func (w *World) canStand(s int, t Terrain) bool {
	spec := w.specOf(s)
	return spec.enters(t) || t == TerrainWater && w.cfg.Wade
}

// wading reports whether body b stands in water its species does not
// swim in: it burns WaterBurn times as fast there.
func (w *World) wading(b *Body) bool {
	t := w.tileOf(b.X, b.Y)
	return t >= 0 && w.m.Terrain[t] == TerrainWater && !w.spec(b).Swims
}

// waterBurn is the factor the water body b stands in puts on its burn.
func (w *World) waterBurn(b *Body) float64 {
	if w.wading(b) {
		return w.cfg.WaterBurn
	}
	return 1
}

// specOf is the row of species s; the zero row without a table.
func (w *World) specOf(s int) SpeciesSpec {
	if s < len(w.cfg.Species) {
		return w.cfg.Species[s]
	}
	return SpeciesSpec{}
}

// validSpecies reports whether the table can be drawn from.
func validSpecies(table []SpeciesSpec) error {
	sum := 0.0
	seen := map[string]bool{}
	for _, s := range table {
		if s.Name == "" || seen[s.Name] {
			return fmt.Errorf("species names must be set and distinct, got %q", s.Name)
		}
		seen[s.Name] = true
		if !(s.Share >= 0) {
			return fmt.Errorf("species %s has share %v", s.Name, s.Share)
		}
		for _, t := range s.Terrains {
			if t >= NumTerrains {
				return fmt.Errorf("species %s enters unknown terrain %d", s.Name, t)
			}
		}
		for _, t := range append(append([]Terrain(nil), s.Likes...), s.Dislikes...) {
			if t >= NumTerrains {
				return fmt.Errorf("species %s likes or dislikes unknown terrain %d", s.Name, t)
			}
		}
		if s.Swims && !s.enters(TerrainWater) {
			return fmt.Errorf("species %s swims but does not enter water", s.Name)
		}
		sum += s.Share
	}
	if len(table) > 1 && !(sum > 0) {
		return fmt.Errorf("the species' shares sum to %v", sum)
	}
	return nil
}

// spec is the row of body b's species; the zero row without a table.
func (w *World) spec(b *Body) SpeciesSpec {
	if b.Species < len(w.cfg.Species) {
		return w.cfg.Species[b.Species]
	}
	return SpeciesSpec{}
}

// drawSpecies draws a first body's species by the table's shares; zero,
// with no draw, for a world of one species.
func (w *World) drawSpecies() int {
	n := len(w.cfg.Species)
	if n <= 1 {
		return 0
	}
	// The map may set a species' share of the first bodies
	// (Map.SpeciesShare, 2026-10-12).
	share := func(s SpeciesSpec) float64 {
		if v, ok := w.m.SpeciesShare[s.Name]; ok {
			return v
		}
		return s.Share
	}
	sum := 0.0
	for _, s := range w.cfg.Species {
		sum += share(s)
	}
	x := w.rng.Float64() * sum
	for i, s := range w.cfg.Species {
		x -= share(s)
		if x < 0 {
			return i
		}
	}
	return n - 1
}

// speciesScale is what body b's species multiplies the config's budget
// by: BudgetStep to the row's Size. Exactly one at zero.
func (w *World) speciesScale(b *Body) float64 {
	if s := w.spec(b).Size; s != 0 {
		return math.Pow(w.cfg.BudgetStep, float64(s))
	}
	return 1
}

// sameSpecies reports whether a and b may mate as far as species go.
func sameSpecies(a, b *Body) bool { return a.Species == b.Species }

// SpeciesCount is how many species the world has: the rows of the table,
// or one.
func (w *World) SpeciesCount() int { return max(len(w.cfg.Species), 1) }

// SpeciesName names species i, for reports; "human" without a table.
func (w *World) SpeciesName(i int) string {
	if i < len(w.cfg.Species) {
		return w.cfg.Species[i].Name
	}
	return "human"
}
