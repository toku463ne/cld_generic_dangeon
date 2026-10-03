package main

import (
	"fmt"
	"math"

	"github.com/toku463ne/cld_generic_dangeon/engine"
	"github.com/toku463ne/cld_generic_dangeon/variant"
)

type tieBurnTally struct {
	// after tick 5000, for all bodies [0] and resting mothers [1]:
	// decisions, those whose least risk more than one option shares, those
	// of them whose tied options spend differently, and those where the
	// option taken is not one that spends least.
	decisions, tied, differ, changed [2]float64
}

// runTieBurn reads every decision for the room a tie-break by spend
// (TieSpend) would have, without it on.
func runTieBurn(cfg engine.Config, m engine.Map, ticks int) (tieBurnTally, error) {
	var t tieBurnTally
	w, err := engine.NewWorld(cfg, m)
	if err != nil {
		return t, err
	}
	w.SetTrace(func(b engine.Body, v engine.Valuation, a engine.Action) {
		if w.Tick() <= 5000 || len(v.Spend) != len(v.Score) || len(v.Score) == 0 {
			return
		}
		groups := []int{0}
		if w.Resting(b) {
			groups = append(groups, 1)
		}
		best := math.Inf(1)
		for _, s := range v.Score {
			best = math.Min(best, s)
		}
		var ties []int
		taken := -1
		for j, s := range v.Score {
			if s == best {
				ties = append(ties, j)
				if v.Options[j] == a {
					taken = j
				}
			}
		}
		lo, hi := math.Inf(1), math.Inf(-1)
		for _, j := range ties {
			lo, hi = math.Min(lo, v.Spend[j]), math.Max(hi, v.Spend[j])
		}
		for _, g := range groups {
			t.decisions[g]++
			if len(ties) < 2 {
				continue
			}
			t.tied[g]++
			if hi > lo*(1+1e-9) {
				t.differ[g]++
				if taken < 0 || v.Spend[taken] > lo*(1+1e-9) {
					t.changed[g]++
				}
			}
		}
	})
	for tick := 1; tick <= ticks; tick++ {
		w.Step()
		if len(w.Bodies()) <= lockCollapse {
			break
		}
	}
	return t, nil
}

// tieBurn counts the room for breaking ties by what a plan spends: how
// often the least risk is shared, the tied options spend differently, and
// the one taken is not the cheapest.
func tieBurn(m engine.Map, name string, seeds int, seed0 int64, ticks int, vname string) {
	var ts []tieBurnTally
	for s := 0; s < seeds; s++ {
		cfg, err := variant.Config(vname, seed0+int64(s))
		if err != nil {
			fail(err)
		}
		t, err := runTieBurn(cfg, m, ticks)
		if err != nil {
			fail(err)
		}
		ts = append(ts, t)
	}
	fmt.Printf("地図 %s (%dx%d)、%d シード、条件 %s、%d tick（tick 5000 より後の決定）。同点＝一番低い（危険 − 子の値打ち）を2つ以上の手が分ける。費やす量＝その手の危険を読んだ続きが Window tick に費やす体力。\n\n", name, m.Width, m.Height, seeds, vname, ticks)
	cell := func(f func(tieBurnTally) float64) string {
		var xs []float64
		for _, t := range ts {
			if x := f(t); !math.IsNaN(x) {
				xs = append(xs, x)
			}
		}
		if len(xs) == 0 {
			return "—"
		}
		mm, se := meanSE(xs)
		return fmt.Sprintf("%.4f ± %.4f", mm, se)
	}
	ratio := func(a, b float64) float64 {
		if b == 0 {
			return math.NaN()
		}
		return a / b
	}
	fmt.Printf("| 身体 | 決定（1シードあたり） | 同点の割合 | 同点で費やす量が違う割合 | 選んだ手が一番費やさない手でない割合 |\n| --- | --- | --- | --- | --- |\n")
	for g, label := range []string{"全員", "休んでいる母"} {
		g := g
		fmt.Printf("| %s | %s | %s | %s | %s |\n", label,
			cell(func(t tieBurnTally) float64 { return t.decisions[g] }),
			cell(func(t tieBurnTally) float64 { return ratio(t.tied[g], t.decisions[g]) }),
			cell(func(t tieBurnTally) float64 { return ratio(t.differ[g], t.decisions[g]) }),
			cell(func(t tieBurnTally) float64 { return ratio(t.changed[g], t.decisions[g]) }))
	}
	fmt.Println()
}
