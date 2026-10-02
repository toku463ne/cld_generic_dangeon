package main

import (
	"fmt"
	"math"
	"sort"

	"github.com/toku463ne/cld_generic_dangeon/engine"
	"github.com/toku463ne/cld_generic_dangeon/variant"
)

var actKindNames = map[engine.ActionKind]string{engine.ActWait: "待つ", engine.ActEat: "食べる", engine.ActMove: "移動", engine.ActMate: "交配", engine.ActPick: "拾う"}

type nightRoomTally struct {
	// per light band, after tick 5000: decisions of adults at their full
	// most, and of them those whose chosen option is not among the best
	// when the whole window is read in the dark (sight gain 1).
	decisions, changed [4]float64
	// what the choice was and what the dark reading would take instead
	// (lit bands only).
	moves map[string]float64
}

// runNightRoom revalues each adult's decision as if it were dark for the
// whole window, and counts the decisions whose best option changes: the
// most a reading of when dark comes (7-2b) could change.
func runNightRoom(cfg engine.Config, m engine.Map, ticks int) (nightRoomTally, error) {
	t := nightRoomTally{moves: map[string]float64{}}
	w, err := engine.NewWorld(cfg, m)
	if err != nil {
		return t, err
	}
	score := func(v engine.Valuation, j int) float64 { return v.Risk[0][j] - cfg.ChildWorth*v.Child[j] }
	w.SetTrace(func(b engine.Body, v engine.Valuation, a engine.Action) {
		if w.Tick() <= 5000 || w.MaxEnergy(b) < cfg.EnergyMax || len(v.Options) == 0 {
			return
		}
		k := lightBand(w.Brightness())
		t.decisions[k]++
		g := w.SightGain()
		if g <= 1 {
			return // already read in the dark
		}
		region, path := w.Rates(b)
		for r := range region {
			region[r] /= g
			path[r] /= g
		}
		dv := w.ValueWith(b, region, path)
		lo, chosen := math.Inf(1), math.Inf(1)
		var best engine.Action
		for j, o := range dv.Options {
			s := score(dv, j)
			if s < lo {
				lo, best = s, o
			}
			if o == a {
				chosen = s
			}
		}
		if chosen > lo {
			t.changed[k]++
			t.moves[actKindNames[a.Kind]+" → "+actKindNames[best.Kind]]++
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

// nightRoom counts, by light, the decisions whose best option a dark
// reading of the whole window would change.
func nightRoom(m engine.Map, name string, seeds int, seed0 int64, ticks int, vname string) {
	var ts []nightRoomTally
	moves := map[string]float64{}
	for s := 0; s < seeds; s++ {
		cfg, err := variant.Config(vname, seed0+int64(s))
		if err != nil {
			fail(err)
		}
		t, err := runNightRoom(cfg, m, ticks)
		if err != nil {
			fail(err)
		}
		ts = append(ts, t)
		for k, n := range t.moves {
			moves[k] += n
		}
	}
	fmt.Printf("地図 %s (%dx%d)、%d シード、条件 %s、%d tick（tick 5000 より後）。成人（上限が満ち）の決定を、窓の全体が暗闇（視界の倍率 1）として読み直し、選んだ手が最善から外れる割合。\n\n", name, m.Width, m.Height, seeds, vname, ticks)
	cell := func(f func(nightRoomTally) float64, prec int) string {
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
		return fmt.Sprintf("%.*f ± %.*f", prec, mm, prec, se)
	}
	ratio := func(a, b float64) float64 {
		if b == 0 {
			return math.NaN()
		}
		return a / b
	}
	fmt.Printf("| 明るさ | 決定（1シードあたり） | 暗闇として読むと最善が変わる割合 |\n| --- | --- | --- |\n")
	for k, l := range lightBands {
		k := k
		fmt.Printf("| %s | %s | %s |\n", l, cell(func(t nightRoomTally) float64 { return t.decisions[k] }, 0),
			cell(func(t nightRoomTally) float64 { return ratio(t.changed[k], t.decisions[k]) }, 4))
	}
	var keys []string
	total := 0.0
	for k, n := range moves {
		keys = append(keys, k)
		total += n
	}
	sort.Slice(keys, func(i, j int) bool { return moves[keys[i]] > moves[keys[j]] })
	fmt.Printf("\n変わった決定の内訳（選んだ手 → 暗闇の最善、全シード）\n\n| 変わり方 | 割合 |\n| --- | --- |\n")
	for _, k := range keys {
		fmt.Printf("| %s | %.3f |\n", k, moves[k]/total)
	}
	fmt.Println()
}
