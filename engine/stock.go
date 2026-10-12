package engine

import "fmt"

// Stocks (stage 13-2, NODE.md "トレーニングと本番"): what a save of bred
// bodies carries from a training stage to the nests of a production map -
// each body's genome and its skills, and nothing it learned. A body's
// memory is the world it lived in; a body sent into another world knows
// nothing of it. Skills are not inherited (they are drawn at birth), so a
// stock keeps the skills of the very body saved, with its genome: the
// pair a nest sends out together.
//
// The engine hands stocks out and takes them in; writing them to a file
// is the caller's (the engine does not know files).

// Stock is one saved body: its genome and the level of each skill.
type Stock struct {
	Genome Genome
	Skill  [NumSkills]float64 `json:",omitempty"`
}

// Stocks returns a stock of every living body of species s, in the order
// of Bodies.
func (w *World) Stocks(s int) []Stock {
	var out []Stock
	for i := range w.bodies {
		b := &w.bodies[i]
		if b.Species == s {
			out = append(out, Stock{Genome: w.genomeOf(b), Skill: b.Skill})
		}
	}
	return out
}

// validStock reports whether stock st fits the config: its genome within
// the levels, and, where its skills are taken (skilled), no skill below
// zero and none at all for a species without skills.
func (w *World) validStock(st Stock, skilled bool) error {
	if err := w.validGenome(st.Genome); err != nil {
		return err
	}
	if !skilled {
		return nil
	}
	for k, v := range st.Skill {
		if !(v >= 0) {
			return fmt.Errorf("stock skill %d is %v", k, v)
		}
		if v > 0 && w.specOf(st.Genome.Species).NoSkills {
			return fmt.Errorf("stock of species %s has skill %d, which it cannot have", w.SpeciesName(st.Genome.Species), k)
		}
	}
	return nil
}
