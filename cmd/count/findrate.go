package main

import (
	"fmt"
	"math"

	"github.com/toku463ne/cld_generic_dangeon/engine"
	"github.com/toku463ne/cld_generic_dangeon/variant"
)

// lightBands bound the light for the findrate count: dark, dim, bright, day.
var lightBands = []string{"0", "0〜0.5", "0.5〜1", "1"}

func lightBand(l float64) int {
	switch {
	case l <= 0:
		return 0
	case l < 0.5:
		return 1
	case l < 1:
		return 2
	}
	return 3
}

type findRateTally struct {
	// per light band, after tick 5000: ticks of hungry adults (a meal fits,
	// not resting, at their full most), of them those that ate, and the
	// model's chance per tick of standing on food in their region (the
	// truth table's keep-moving row), summed.
	ticks, ate, model [4]float64
}

// runFindRate follows each hungry adult tick by tick: whether its energy
// rose (it ate) against the chance the valuation's keep-moving row gives.
func runFindRate(cfg engine.Config, m engine.Map, ticks int) (findRateTally, error) {
	var t findRateTally
	w, err := engine.NewWorld(cfg, m)
	if err != nil {
		return t, err
	}
	type before struct {
		energy float64
		region engine.RegionID
	}
	prev := map[int64]before{}
	for tick := 1; tick <= ticks; tick++ {
		var meet []float64
		k := lightBand(w.Brightness())
		if tick > 5000 {
			meet = w.TruthTable().Meet
			clear(prev)
			for _, b := range w.Bodies() {
				if w.MaxEnergy(b) < cfg.EnergyMax || w.Resting(b) || b.Energy > w.MaxEnergy(b)-cfg.FoodEnergy || b.Held > 0 {
					continue
				}
				prev[b.ID] = before{b.Energy, m.RegionAt(int(b.X), int(b.Y))}
			}
		}
		w.Step()
		if tick <= 5000 {
			continue
		}
		// The light this tick's perceptions read is that of the new tick.
		k = lightBand(w.Brightness())
		for _, b := range w.Bodies() {
			p, ok := prev[b.ID]
			if !ok {
				continue
			}
			t.ticks[k]++
			t.model[k] += meet[p.region]
			if b.Energy > p.energy {
				t.ate[k]++
			}
		}
		if len(w.Bodies()) <= lockCollapse {
			break
		}
	}
	return t, nil
}

// findRate counts how often a hungry adult eats per tick, by light, against
// the chance the valuation reads for keeping on the move: how much sight
// adds to finding food, which the valuation does not read.
func findRate(m engine.Map, name string, seeds int, seed0 int64, ticks int, vname string) {
	var ts []findRateTally
	for s := 0; s < seeds; s++ {
		cfg, err := variant.Config(vname, seed0+int64(s))
		if err != nil {
			fail(err)
		}
		t, err := runFindRate(cfg, m, ticks)
		if err != nil {
			fail(err)
		}
		ts = append(ts, t)
	}
	fmt.Printf("地図 %s (%dx%d)、%d シード、条件 %s、%d tick（tick 5000 より後）。空腹の成人＝上限が満ち、1食が入りきり、休んでおらず、手持ちが無い。モデル＝真の表の「歩き続ける」行（その地域の地面の食料 ÷ 陸 × 速さ）。\n\n", name, m.Width, m.Height, seeds, vname, ticks)
	cell := func(f func(findRateTally) float64, prec int) string {
		var xs []float64
		for _, t := range ts {
			if x := f(t); !math.IsNaN(x) && !math.IsInf(x, 0) {
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
	fmt.Printf("| 明るさ | 空腹の成人の tick（1シードあたり） | 食べた割合（/tick） | モデルの見込み（/tick） | 食べた ÷ モデル |\n| --- | --- | --- | --- | --- |\n")
	for k, l := range lightBands {
		k := k
		fmt.Printf("| %s | %s | %s | %s | %s |\n", l,
			cell(func(t findRateTally) float64 { return t.ticks[k] }, 0),
			cell(func(t findRateTally) float64 { return ratio(t.ate[k], t.ticks[k]) }, 5),
			cell(func(t findRateTally) float64 { return ratio(t.model[k], t.ticks[k]) }, 5),
			cell(func(t findRateTally) float64 { return ratio(t.ate[k], t.model[k]) }, 2))
	}
	fmt.Println()
}
