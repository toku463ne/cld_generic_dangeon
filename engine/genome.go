package engine

import (
	"fmt"
	"math"
)

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
// AllotFertility), Size its budget level (0 to BudgetLevels-1; the
// neutral level without AllotBudget), Bold its exploratory level (0 to
// VentureMost; the neutral level without AllotVenture - the one slot
// outside the budget).
type Genome struct {
	Level int
	Sight int
	Fert  int
	Size  int
	Bold  int
	// Attack and Defense are its combat levels (0 to CombatMost; the
	// neutral level without AllotCombat; stage 10-1, VISION "攻撃力・
	// 防御力"): each level off the neutral multiplies the strength by
	// CombatStep, and costs or refunds CombatTrade of the budget.
	Attack  int
	Defense int
	// Species is the index of the body's species (stage 9-7a): born
	// with and passed on, but not drawn or mutated - a child is of its
	// parents' species.
	Species int
}

// GenomeOf is the genome body b was born with.
func (w *World) GenomeOf(b Body) Genome { return w.genomeOf(&b) }

func (w *World) genomeOf(b *Body) Genome {
	return Genome{Level: b.Level, Sight: w.sightOf(b), Fert: w.fertOf(b), Size: w.sizeOf(b), Bold: w.boldOf(b), Attack: w.attackOf(b), Defense: w.defenseOf(b), Species: b.Species}
}

// sizeNeutral is the budget level that buys Config.Budget.
func (w *World) sizeNeutral() int { return (w.cfg.BudgetLevels - 1) / 2 }

// sizeOf is the budget level body b was born with; zero in the field
// stands for the neutral level.
func (w *World) sizeOf(b *Body) int {
	if b.Size > 0 {
		return b.Size - 1
	}
	return w.sizeNeutral()
}

// budgetOf is the budget that level size buys body b: Config.Budget scaled
// by BudgetStep for every level off the neutral, on the scale of b's
// species.
func (w *World) budgetOf(b *Body, size int) float64 {
	return w.cfg.Budget * w.speciesScale(b) * math.Pow(w.cfg.BudgetStep, float64(size-w.sizeNeutral()))
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
	if w.cfg.AllotBudget && (g.Size < 0 || g.Size >= w.cfg.BudgetLevels) {
		return fmt.Errorf("genome size %d is not within 0..%d", g.Size, w.cfg.BudgetLevels-1)
	}
	if w.cfg.AllotVenture && (g.Bold < 0 || g.Bold > w.cfg.VentureMost) {
		return fmt.Errorf("genome boldness %d is not within 0..%d", g.Bold, w.cfg.VentureMost)
	}
	if w.cfg.AllotCombat && (g.Attack < 0 || g.Attack > w.cfg.CombatMost || g.Defense < 0 || g.Defense > w.cfg.CombatMost) {
		return fmt.Errorf("genome attack %d / defense %d is not within 0..%d", g.Attack, g.Defense, w.cfg.CombatMost)
	}
	if g.Species < 0 || g.Species >= w.SpeciesCount() {
		return fmt.Errorf("genome species %d is not within 0..%d", g.Species, w.SpeciesCount()-1)
	}
	return nil
}

// drawGenome is the genome of a body with no parents: every slot drawn
// uniformly, the share's level first, then sight, fertility, size and
// boldness.
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
	g.Size = w.sizeNeutral()
	if w.cfg.AllotBudget {
		g.Size = w.rng.Intn(w.cfg.BudgetLevels)
	}
	g.Bold = w.cfg.VentureMost / 2
	if w.cfg.AllotVenture {
		g.Bold = w.rng.Intn(w.cfg.VentureMost + 1)
	}
	g.Attack, g.Defense = w.cfg.CombatMost/2, w.cfg.CombatMost/2
	if w.cfg.AllotCombat {
		g.Attack = w.rng.Intn(w.cfg.CombatMost + 1)
		g.Defense = w.rng.Intn(w.cfg.CombatMost + 1)
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
	g := Genome{Sight: w.cfg.Sight, Fert: w.cfg.FertMost / 2, Size: w.sizeNeutral(), Bold: w.cfg.VentureMost / 2, Attack: w.cfg.CombatMost / 2, Defense: w.cfg.CombatMost / 2, Species: a.Species}
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
	if w.cfg.AllotBudget {
		pick()
		g.Size = w.mutate(from.Size, w.cfg.BudgetLevels)
	}
	if w.cfg.AllotVenture {
		pick()
		g.Bold = w.mutate(from.Bold, w.cfg.VentureMost+1)
	}
	if w.cfg.AllotCombat {
		pick()
		g.Attack = w.mutate(from.Attack, w.cfg.CombatMost+1)
		pick()
		g.Defense = w.mutate(from.Defense, w.cfg.CombatMost+1)
	}
	return g
}

// endow gives body b its budget and what genome g buys of it: the budget
// its size stands for (AllotBudget, stage 9-5b), the share for speed its
// level stands for, its sight and fertility level, and the build (speed
// and most energy) the pair is left with after each tile of sight off the
// neutral costs or refunds SightTrade and each fertility level off the
// neutral FertTrade, all of it scaled by the budget. Its energy is held to
// its most. Its boldness (AllotVenture, stage 9-6a) is written down and
// buys nothing: it is outside the budget. Without Allot the body is the
// config's and g is not read.
func (w *World) endow(b *Body, g Genome) {
	b.Species = g.Species
	b.Budget = w.cfg.Budget
	if w.cfg.Allot {
		// The species' scale is a budget's: without Allot every body is
		// the config's, whatever its species.
		b.Budget *= w.speciesScale(b)
		if w.cfg.AllotBudget {
			b.Size = g.Size + 1
			b.Budget = w.budgetOf(b, g.Size)
		}
	}
	w.stats.BudgetIn += b.Budget
	if !w.cfg.Allot {
		return
	}
	if w.cfg.AllotVenture {
		b.Bold = g.Bold + 1
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
	if w.cfg.AllotCombat {
		b.Attack, b.Defense = g.Attack+1, g.Defense+1
		r += w.cfg.CombatTrade * float64(2*(w.cfg.CombatMost/2)-g.Attack-g.Defense)
	}
	r = max(r, 0.05) * b.Budget / w.cfg.Budget
	b.Build = w.buildOf([NumAbilities]float64{AbilitySpeed: s * r, AbilityEnergyMax: (1 - s) * r})
	b.Energy = min(b.Energy, w.maxOf(b))
}
