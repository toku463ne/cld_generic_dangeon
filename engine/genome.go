package engine

import "fmt"

// Genomes.
//
// A genome is what a body is born with and passes on: the level of each
// slot of its budget. Everything else about the body - its build, its
// share, the tiles it sees - is what the genome buys, and is read off it
// (endow). Keeping the two apart gives the world one more way to come by
// a body besides drawing it or breeding it: being handed a genome, which
// is what a nest sending saved bodies into a map, and a run that carries
// the genomes of one world into the next, both need.
//
// Only allot draws genomes; a slot joins the genome in the stage whose
// rules read it (build.go).

// Genome is the levels a body is born with and passes on. Level is its
// share of the budget for speed (0 to AllotLevels-1), Sight the tiles it
// sees (0 to SightMost; the config's Sight without AllotSight), Fert its
// fertility level (0 to FertMost; the neutral level without
// AllotFertility).
type Genome struct {
	Level int
	Sight int
	Fert  int
}

// GenomeOf is the genome body b was born with.
func (w *World) GenomeOf(b Body) Genome { return w.genomeOf(&b) }

func (w *World) genomeOf(b *Body) Genome {
	return Genome{Level: b.Level, Sight: w.sightOf(b), Fert: w.fertOf(b)}
}

// validGenome reports whether g lies within the levels the config has, in
// the slots that are in the budget.
func (w *World) validGenome(g Genome) error {
	if g.Level < 0 || g.Level >= max(w.cfg.AllotLevels, 1) {
		return fmt.Errorf("genome level %d is not within 0..%d", g.Level, max(w.cfg.AllotLevels, 1)-1)
	}
	if w.cfg.AllotSight && (g.Sight < 0 || g.Sight > w.cfg.SightMost) {
		return fmt.Errorf("genome sight %d is not within 0..%d", g.Sight, w.cfg.SightMost)
	}
	if w.cfg.AllotFertility && (g.Fert < 0 || g.Fert > w.cfg.FertMost) {
		return fmt.Errorf("genome fertility %d is not within 0..%d", g.Fert, w.cfg.FertMost)
	}
	return nil
}

// drawGenome is the genome of a body with no parents: every slot drawn
// uniformly, the share's level first, then sight, then fertility.
func (w *World) drawGenome() Genome {
	var g Genome
	if w.cfg.AllotLevels > 1 {
		g.Level = w.rng.Intn(w.cfg.AllotLevels)
	}
	if w.cfg.AllotSight {
		g.Sight = w.rng.Intn(w.cfg.SightMost + 1)
	} else {
		g.Sight = w.cfg.Sight
	}
	if w.cfg.AllotFertility {
		g.Fert = w.rng.Intn(w.cfg.FertMost + 1)
	} else {
		g.Fert = w.cfg.FertMost / 2
	}
	return g
}

// inherit is the genome of a child of parents with genomes a and b: each
// slot is one parent's, moved one level with chance AllotMutation
// (mutate). The parent is drawn once for the share and kept for the
// other slots, or with AllotMix drawn again for every slot (stage 9-5a).
// The draws come in the same order as drawGenome's.
func (w *World) inherit(a, b Genome) Genome {
	parents := [2]Genome{a, b}
	var from *Genome
	pick := func() {
		if from == nil || w.cfg.AllotMix {
			from = &parents[w.rng.Intn(2)]
		}
	}
	g := Genome{Sight: w.cfg.Sight, Fert: w.cfg.FertMost / 2}
	if n := w.cfg.AllotLevels; n > 1 {
		pick()
		g.Level = w.mutate(from.Level, n)
	}
	if w.cfg.AllotSight {
		pick()
		g.Sight = w.mutate(from.Sight, w.cfg.SightMost+1)
	}
	if w.cfg.AllotFertility {
		pick()
		g.Fert = w.mutate(from.Fert, w.cfg.FertMost+1)
	}
	return g
}

// endow gives body b its budget and what genome g buys of it: the share
// for speed its level stands for, its sight and fertility level, and the
// build (speed and most energy) the pair is left with after each tile of
// sight off the neutral costs or refunds SightTrade and each fertility
// level off the neutral FertTrade. Its energy is held to its most.
// Without Allot the body is the config's and g is not read.
func (w *World) endow(b *Body, g Genome) {
	b.Budget = w.cfg.Budget
	w.stats.BudgetIn += b.Budget
	if !w.cfg.Allot {
		return
	}
	n := w.cfg.AllotLevels
	s := 0.5
	if n > 1 {
		s += w.cfg.AllotSpread * (2*float64(g.Level)/float64(n-1) - 1)
	}
	b.Level, b.Share = g.Level, s
	r := 1.0
	if w.cfg.AllotSight {
		b.Sight = g.Sight + 1
		r += w.cfg.SightTrade * float64(w.cfg.Sight-g.Sight)
	}
	if w.cfg.AllotFertility {
		b.Fert = g.Fert + 1
		r += w.cfg.FertTrade * float64(w.cfg.FertMost/2-g.Fert)
	}
	r = max(r, 0.05)
	b.Build = w.buildOf([NumAbilities]float64{AbilitySpeed: s * r, AbilityEnergyMax: (1 - s) * r})
	b.Energy = min(b.Energy, w.maxOf(b))
}
