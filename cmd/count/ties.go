package main

import (
	"fmt"
	"math"
	"sort"

	"github.com/toku463ne/cld_generic_dangeon/engine"
	"github.com/toku463ne/cld_generic_dangeon/variant"
)

// tiesGroups split the decisions read by ties: resting mothers, other
// adults and children.
var tiesGroups = [3]string{"休んでいる母", "それ以外の成人", "子"}

// tiesBands split decisions by energy over the body's most energy.
var tiesBands = [3]string{"< 0.5", "0.5〜0.9", "≥ 0.9"}

// tiesGaps are the upper bounds of the gap between the best score and the
// next different one; the last bucket is everything wider.
var tiesGaps = []float64{0, 1e-12, 1e-9, 1e-6, 1e-3}

type tiesTally struct {
	// Decisions after tick 5000, per group.
	decisions [3]float64
	// Of those: more than one option shares the best score; the best
	// option's risk (first window) is exactly 0, or below 1e-9.
	tied, zero, tiny [3]float64
	// Options sharing the best score, summed over tied decisions.
	tiedWidth [3]float64
	// How tied decisions were settled: the heading kept (the body's last
	// move is among the tied and taken), or anything else (a draw, a
	// bounce or a turn off).
	kept [3]float64
	// Gap between the best score and the next different one, per bucket
	// (len(tiesGaps)+1), over decisions with at least two different scores.
	gaps [3][]float64
	// Decisions with food underfoot, per group and energy band: how many,
	// how many ate, how many had eating among the best, and how many had
	// eating strictly worse than the best.
	under, ate, eatTied, eatWorse [3][3]float64
	// Of decisions with food underfoot that did not eat although eating
	// was among the best: what was taken (wait, move, mate).
	skipped [3][3]float64
	// Of decisions with food underfoot where eating was strictly worse than
	// the best: the kind of the first best option (wait, move, mate).
	beaten [3][3]float64
}

// runTies reads every decision after tick 5000 from the trace.
func runTies(cfg engine.Config, m engine.Map, ticks int) (tiesTally, error) {
	var t tiesTally
	for g := range t.gaps {
		t.gaps[g] = make([]float64, len(tiesGaps)+1)
	}
	w, err := engine.NewWorld(cfg, m)
	if err != nil {
		return t, err
	}
	w.SetTrace(func(b engine.Body, v engine.Valuation, a engine.Action) {
		now := w.Tick()
		if now <= 5000 || len(v.Score) == 0 {
			return
		}
		g := 2
		switch {
		case b.Sex == engine.Female && now < b.Rested:
			g = 0
		case now >= b.Mature:
			g = 1
		}
		t.decisions[g]++
		best, bestRisk := math.Inf(1), math.Inf(1)
		for j, s := range v.Score {
			if s < best {
				best, bestRisk = s, v.Risk[0][j]
			}
		}
		width, next := 0, math.Inf(1)
		eat := -1
		for j, s := range v.Score {
			if s == best {
				width++
			} else if s < next {
				next = s
			}
			if v.Options[j].Kind == engine.ActEat {
				eat = j
			}
		}
		if width > 1 {
			t.tied[g]++
			t.tiedWidth[g] += float64(width)
			if a.Kind == engine.ActMove && a.Dir == b.Heading {
				t.kept[g]++
			}
		}
		if bestRisk == 0 {
			t.zero[g]++
		}
		if bestRisk < 1e-9 {
			t.tiny[g]++
		}
		if !math.IsInf(next, 1) {
			gap := next - best
			k := sort.SearchFloat64s(tiesGaps, gap)
			if k < len(tiesGaps) && gap > tiesGaps[k] {
				k++
			}
			t.gaps[g][k]++
		}
		if eat < 0 {
			return
		}
		e := b.Energy / w.MaxEnergy(b)
		band := 0
		switch {
		case e >= 0.9:
			band = 2
		case e >= 0.5:
			band = 1
		}
		t.under[g][band]++
		if a.Kind == engine.ActEat {
			t.ate[g][band]++
		}
		if v.Score[eat] == best {
			t.eatTied[g][band]++
			if a.Kind != engine.ActEat {
				switch a.Kind {
				case engine.ActWait:
					t.skipped[g][0]++
				case engine.ActMove:
					t.skipped[g][1]++
				default:
					t.skipped[g][2]++
				}
			}
		} else {
			t.eatWorse[g][band]++
			for j, s := range v.Score {
				if s != best {
					continue
				}
				switch v.Options[j].Kind {
				case engine.ActWait:
					t.beaten[g][0]++
				case engine.ActMove:
					t.beaten[g][1]++
				default:
					t.beaten[g][2]++
				}
				break
			}
		}
	})
	for tick := 1; tick <= ticks; tick++ {
		w.Step()
	}
	return t, nil
}

// ties counts, before stage 4 lets bodies carry food, how often the
// valuation cannot tell the best options apart: decisions whose best score
// is shared, whose best risk is zero or nearly, how wide the gap to the
// next option is, and what bodies with food underfoot do (stage 3-4 found
// resting mothers eating a third of the time).
func ties(m engine.Map, name string, seeds int, seed0 int64, ticks int, vname string) {
	var ts []tiesTally
	for s := 0; s < seeds; s++ {
		cfg, err := variant.Config(vname, seed0+int64(s))
		if err != nil {
			fail(err)
		}
		t, err := runTies(cfg, m, ticks)
		if err != nil {
			fail(err)
		}
		ts = append(ts, t)
	}
	fmt.Printf("地図 %s (%dx%d)、%d シード、条件 %s、%d tick（tick 5000 より後の決定）。点数＝窓の終わりに死んでいる確率 − ChildWorth × 子の確率。同点は点数が完全に等しいこと（engine の比較と同じ）。\n\n", name, m.Width, m.Height, seeds, vname, ticks)
	cell := func(f func(tiesTally) float64, prec int) string {
		var xs []float64
		for _, t := range ts {
			x := f(t)
			if math.IsNaN(x) {
				continue
			}
			xs = append(xs, x)
		}
		if len(xs) == 0 {
			return "—"
		}
		mm, se := meanSE(xs)
		return fmt.Sprintf("%.*f ± %.*f", prec, mm, prec, se)
	}
	ratio := func(a, b float64) float64 {
		if b == 0 {
			return math.NaN()
		}
		return a / b
	}
	fmt.Printf("| 量 | %s | %s | %s |\n| --- | --- | --- | --- |\n", tiesGroups[0], tiesGroups[1], tiesGroups[2])
	row := func(label string, f func(t tiesTally, g int) float64, prec int) {
		fmt.Printf("| %s |", label)
		for g := range tiesGroups {
			fmt.Printf(" %s |", cell(func(t tiesTally) float64 { return f(t, g) }, prec))
		}
		fmt.Println()
	}
	row("決定（1シードあたり）", func(t tiesTally, g int) float64 { return t.decisions[g] }, 0)
	row("最善の点数を複数の手が分ける割合", func(t tiesTally, g int) float64 { return ratio(t.tied[g], t.decisions[g]) }, 4)
	row("　そのときの同点の手の数（平均）", func(t tiesTally, g int) float64 { return ratio(t.tiedWidth[g], t.tied[g]) }, 2)
	row("　そのうち向きを保った割合", func(t tiesTally, g int) float64 { return ratio(t.kept[g], t.tied[g]) }, 4)
	row("最善の手の死ぬ確率がちょうど 0 の割合", func(t tiesTally, g int) float64 { return ratio(t.zero[g], t.decisions[g]) }, 4)
	row("最善の手の死ぬ確率が 1e-9 未満の割合", func(t tiesTally, g int) float64 { return ratio(t.tiny[g], t.decisions[g]) }, 4)
	fmt.Println()
	fmt.Printf("最善と次の手（点数の違う手）の差の分布（違う点数が2つ以上ある決定のうち）\n\n")
	fmt.Printf("| 差 | %s | %s | %s |\n| --- | --- | --- | --- |\n", tiesGroups[0], tiesGroups[1], tiesGroups[2])
	labels := []string{"≤ 1e-12", "≤ 1e-9", "≤ 1e-6", "≤ 1e-3", "> 1e-3"}
	for k := 1; k <= len(tiesGaps); k++ {
		k := k
		fmt.Printf("| %s |", labels[k-1])
		for g := range tiesGroups {
			fmt.Printf(" %s |", cell(func(t tiesTally) float64 {
				sum := 0.0
				for _, x := range t.gaps[g] {
					sum += x
				}
				return ratio(t.gaps[g][k], sum)
			}, 4))
		}
		fmt.Println()
	}
	fmt.Println()
	fmt.Printf("足元に食料がある決定（体力 ÷ 体力の上限の帯ごと）\n\n")
	fmt.Printf("| 群 | 帯 | 決定（1シードあたり） | 食べた | 食べるが最善に入る | 食べるが最善より悪い |\n| --- | --- | --- | --- | --- | --- |\n")
	for g := range tiesGroups {
		for band := range tiesBands {
			fmt.Printf("| %s | %s | %s | %s | %s | %s |\n", tiesGroups[g], tiesBands[band],
				cell(func(t tiesTally) float64 { return t.under[g][band] }, 0),
				cell(func(t tiesTally) float64 { return ratio(t.ate[g][band], t.under[g][band]) }, 4),
				cell(func(t tiesTally) float64 { return ratio(t.eatTied[g][band], t.under[g][band]) }, 4),
				cell(func(t tiesTally) float64 { return ratio(t.eatWorse[g][band], t.under[g][band]) }, 4))
		}
	}
	fmt.Println()
	fmt.Printf("食べるが最善に入っていたのに食べなかった決定の行き先\n\n")
	fmt.Printf("| 群 | 待つ | 移動 | 交配 |\n| --- | --- | --- | --- |\n")
	for g := range tiesGroups {
		fmt.Printf("| %s |", tiesGroups[g])
		for k := 0; k < 3; k++ {
			fmt.Printf(" %s |", cell(func(t tiesTally) float64 {
				return ratio(t.skipped[g][k], t.skipped[g][0]+t.skipped[g][1]+t.skipped[g][2])
			}, 4))
		}
		fmt.Println()
	}
	fmt.Println()
	fmt.Printf("食べるが最善より悪かった決定で、最善だった手\n\n")
	fmt.Printf("| 群 | 待つ | 移動 | 交配 |\n| --- | --- | --- | --- |\n")
	for g := range tiesGroups {
		fmt.Printf("| %s |", tiesGroups[g])
		for k := 0; k < 3; k++ {
			fmt.Printf(" %s |", cell(func(t tiesTally) float64 {
				return ratio(t.beaten[g][k], t.beaten[g][0]+t.beaten[g][1]+t.beaten[g][2])
			}, 4))
		}
		fmt.Println()
	}
	fmt.Println()
}
