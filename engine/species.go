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
	sum := 0.0
	for _, s := range w.cfg.Species {
		sum += s.Share
	}
	x := w.rng.Float64() * sum
	for i, s := range w.cfg.Species {
		x -= s.Share
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
