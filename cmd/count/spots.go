package main

import (
	"fmt"
	"math"

	"github.com/toku463ne/cld_generic_dangeon/engine"
	"github.com/toku463ne/cld_generic_dangeon/variant"
)

type spotsTally struct {
	// Walks to a remembered unit after tick 5000 that found it there and
	// that did not.
	found, missed float64
	// Decisions after tick 5000, those with remembered units read, and those
	// whose action walks to one; and of the latter, those with no food in
	// sight.
	decisions, withRecalled, toRecalled, toRecalledBlind float64
	// Remembered tiles per living body (every 1000 ticks), summed, and the
	// samples.
	spots, samples float64
}

// runSpots reads every decision after tick 5000 from the trace.
func runSpots(cfg engine.Config, m engine.Map, ticks int) (spotsTally, error) {
	var t spotsTally
	w, err := engine.NewWorld(cfg, m)
	if err != nil {
		return t, err
	}
	w.SetTrace(func(b engine.Body, v engine.Valuation, a engine.Action) {
		if w.Tick() <= 5000 {
			return
		}
		t.decisions++
		if len(v.Recalled) == 0 {
			return
		}
		t.withRecalled++
		for j, o := range v.Options {
			if o == a && j < len(v.Plan) && v.Plan[j] >= len(v.Seen) {
				t.toRecalled++
				if len(v.Seen) == 0 {
					t.toRecalledBlind++
				}
				break
			}
		}
	})
	var found0, missed0 int64
	for tick := 1; tick <= ticks; tick++ {
		w.Step()
		if tick == 5000 {
			found0, missed0 = w.Stats().SpotFound, w.Stats().SpotMissed
		}
		if tick > 5000 && tick%1000 == 0 {
			for _, b := range w.Bodies() {
				t.spots += float64(len(b.Memory.Spots))
				t.samples++
			}
		}
	}
	st := w.Stats()
	t.found, t.missed = float64(st.SpotFound-found0), float64(st.SpotMissed-missed0)
	return t, nil
}

// spots measures, in a world where bodies remember food they saw and left
// (stage 5-2), how often they walk to a remembered unit and find it.
func spots(m engine.Map, name string, seeds int, seed0 int64, ticks int, vname string) {
	var ts []spotsTally
	for s := 0; s < seeds; s++ {
		cfg, err := variant.Config(vname, seed0+int64(s))
		if err != nil {
			fail(err)
		}
		t, err := runSpots(cfg, m, ticks)
		if err != nil {
			fail(err)
		}
		ts = append(ts, t)
	}
	fmt.Printf("地図 %s (%dx%d)、%d シード、条件 %s、%d tick（tick 5000 より後）。\n\n", name, m.Width, m.Height, seeds, vname, ticks)
	cell := func(f func(spotsTally) float64, prec int) string {
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
	fmt.Printf("| 量 | 値 |\n| --- | --- |\n")
	fmt.Printf("| 覚えている食料のタイル（生きている身体1体あたり） | %s |\n", cell(func(t spotsTally) float64 { return ratio(t.spots, t.samples) }, 2))
	fmt.Printf("| 覚えている食料を読んだ決定の割合 | %s |\n", cell(func(t spotsTally) float64 { return ratio(t.withRecalled, t.decisions) }, 4))
	fmt.Printf("| 覚えている食料へ歩く決定の割合 | %s |\n", cell(func(t spotsTally) float64 { return ratio(t.toRecalled, t.decisions) }, 4))
	fmt.Printf("| 　そのうち視界に食料が無い決定の割合 | %s |\n", cell(func(t spotsTally) float64 { return ratio(t.toRecalledBlind, t.toRecalled) }, 4))
	fmt.Printf("| 覚えている食料へ歩いた回数（1シードあたり） | %s |\n", cell(func(t spotsTally) float64 { return t.found + t.missed }, 0))
	fmt.Printf("| 　そのうち食料がまだあった割合 | %s |\n\n", cell(func(t spotsTally) float64 { return ratio(t.found, t.found+t.missed) }, 4))
}
