package main

import (
	"math"
	"slices"

	"github.com/toku463ne/cld_generic_dangeon/engine"
	"github.com/toku463ne/cld_generic_dangeon/variant"
)

// The genotype instrument (stage 13-1): the levels of every slot of the
// genome among the living, by species, at tick 0 and at the end of the
// run. A training stage is worth running only if it moves them
// (VISION_CHECKS "巣から、ある程度進化した種が湧く"). Drift moves a small
// population's levels too, but in no one direction: the end less tick 0,
// seed by seed, is read for a direction, and the distance between the two
// distributions only describes how far they came apart.

// The same at half the run, the end less halfway, says whether the
// distribution was still moving when the run stopped: a training stage
// long enough has it settled.

// genoSlot is one slot of the genome the report reads: its name and its
// level read off a genome. A slot whose levels are kinds, not steps,
// has no mean (kind).
type genoSlot struct {
	name string
	of   func(engine.Genome) int
	kind bool
}

var genoSlots = []genoSlot{
	{"速さへの配分", func(g engine.Genome) int { return g.Level }, false},
	{"視野", func(g engine.Genome) int { return g.Sight }, false},
	{"繁殖能力", func(g engine.Genome) int { return g.Fert }, false},
	{"予算", func(g engine.Genome) int { return g.Size }, false},
	{"開拓", func(g engine.Genome) int { return g.Bold }, false},
	{"攻撃", func(g engine.Genome) int { return g.Attack }, false},
	{"防御", func(g engine.Genome) int { return g.Defense }, false},
	{"攻撃的", func(g engine.Genome) int { return g.Fierce }, false},
	{"好み", func(g engine.Genome) int { return g.Taste }, true},
}

// genoHist counts the living of each species at each level of each slot:
// [species][slot] level -> bodies.
func genoHist(w *engine.World, species int) [][]map[int]float64 {
	h := make([][]map[int]float64, species)
	for k := range h {
		h[k] = make([]map[int]float64, len(genoSlots))
		for s := range h[k] {
			h[k][s] = map[int]float64{}
		}
	}
	for _, b := range w.Bodies() {
		g := w.GenomeOf(b)
		for s, slot := range genoSlots {
			h[b.Species][s][slot.of(g)]++
		}
	}
	return h
}

// histMean is the mean level of a count by level; NaN with no body.
func histMean(c map[int]float64) float64 {
	x, n := 0.0, 0.0
	for l, k := range c {
		x += float64(l) * k
		n += k
	}
	if n == 0 {
		return math.NaN()
	}
	return x / n
}

// histDistance is the total variation distance between two counts by
// level, as shares: half the sum of the gaps, 0 for the same shares, 1
// for none in common. NaN where either has no body.
func histDistance(a, b map[int]float64) float64 {
	na, nb := 0.0, 0.0
	for _, k := range a {
		na += k
	}
	for _, k := range b {
		nb += k
	}
	if na == 0 || nb == 0 {
		return math.NaN()
	}
	d := 0.0
	for l, k := range a {
		d += math.Abs(k/na - b[l]/nb)
	}
	for l, k := range b {
		if _, ok := a[l]; !ok {
			d += k / nb
		}
	}
	return d / 2
}

// writeGenotype prints, for each variant with a genome and each species
// it begins with, every slot's mean level at tick 0 and at the end, the
// end less tick 0 and the end less halfway seed by seed, and the
// distance between the distributions at tick 0 and at the end. A seed with no body of the species at the end is left
// out of that species' end columns.
func writeGenotype(p func(string, ...any), o options, results map[string][]result) {
	for _, v := range o.variants {
		if cfg, _ := variant.Config(v, o.seed0); !cfg.Allot {
			continue
		}
		rs := results[v]
		if len(rs) == 0 || len(rs[0].genoStart) == 0 {
			continue
		}
		p("### 補助の表: 遺伝子型の分布（%s、tick 0 と終わり）\n\n", v)
		p("生きている身体の各遺伝子の段の平均（1シードあたり）。終わり − tick 0 と終わり − 中間（tick %d）は同じシードの対の差で、向きのある動き（選択）を読む。後者が 0 なら、分布は後半には落ち着いている。分布の隔たりは 2 つの分布の全変動距離で、漂流でも 0 より上になる。\n\n", o.ticks/2)
		p("| 種族 | 遺伝子 | tick 0 | 終わり | 終わり − tick 0 | 終わり − 中間 | 分布の隔たり |\n| --- | --- | --- | --- | --- | --- | --- |\n")
		for k, name := range rs[0].speciesNames {
			begun := func(r result) bool { return k < len(r.genoStart) && !math.IsNaN(histMean(r.genoStart[k][0])) }
			if !slices.ContainsFunc(rs, begun) {
				continue
			}
			ended := func(r result) bool { return begun(r) && !math.IsNaN(histMean(r.genoEnd[k][0])) }
			for s, slot := range genoSlots {
				dist := fmtMeanSE(collectWhere(rs, ended, func(r result) float64 { return histDistance(r.genoStart[k][s], r.genoEnd[k][s]) }), 3)
				if slot.kind {
					p("| %s | %s | — | — | — | — | %s |\n", name, slot.name, dist)
					continue
				}
				halved := func(r result) bool { return ended(r) && k < len(r.genoHalf) && !math.IsNaN(histMean(r.genoHalf[k][0])) }
				p("| %s | %s | %s | %s | %s | %s | %s |\n", name, slot.name,
					fmtMeanSE(collectWhere(rs, begun, func(r result) float64 { return histMean(r.genoStart[k][s]) }), 3),
					fmtMeanSE(collectWhere(rs, ended, func(r result) float64 { return histMean(r.genoEnd[k][s]) }), 3),
					fmtMeanSE(collectWhere(rs, ended, func(r result) float64 {
						return histMean(r.genoEnd[k][s]) - histMean(r.genoStart[k][s])
					}), 3),
					fmtMeanSE(collectWhere(rs, halved, func(r result) float64 {
						return histMean(r.genoEnd[k][s]) - histMean(r.genoHalf[k][s])
					}), 3),
					dist)
			}
		}
		p("\n")
	}
}
