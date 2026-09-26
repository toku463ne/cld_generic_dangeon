package main

import (
	"fmt"
	"math"

	"github.com/toku463ne/cld_generic_dangeon/engine"
	"github.com/toku463ne/cld_generic_dangeon/variant"
)

// seasonBins is how many bins a season is read in.
const seasonBins = 10

type seasonsTally struct {
	// By bin of the ticks since the season began: the food per land tile
	// of the region richest this season and of the one richest last
	// season, each over the map's; the relative error of the living's
	// estimates of the region they stand in, and the share of the living
	// in the richest region; and the samples.
	richNow, richLast, err, inRich, n [seasonBins]float64
}

// runSeasons reads, after tick 5000, how the food on the ground and the
// living's estimates follow the seasons.
func runSeasons(cfg engine.Config, m engine.Map, ticks int) (seasonsTally, error) {
	var t seasonsTally
	w, err := engine.NewWorld(cfg, m)
	if err != nil {
		return t, err
	}
	nr := len(m.RegionFood)
	land := make([]float64, nr)
	total := 0.0
	for i, tt := range m.Terrain {
		if tt == engine.TerrainLand {
			land[m.Region[i]]++
			total++
		}
	}
	richest := func(shares []float64) int {
		best := 0
		for r, s := range shares {
			if s > shares[best] {
				best = r
			}
		}
		return best
	}
	ns := len(m.SeasonFood)
	for tick := 1; tick <= ticks; tick++ {
		w.Step()
		if tick <= 5000 || tick%10 != 0 {
			continue
		}
		now := w.Tick()
		s := m.Season(now)
		bin := int(now%int64(m.SeasonTicks)) * seasonBins / m.SeasonTicks
		rNow := richest(m.SeasonFood[s])
		rLast := richest(m.SeasonFood[(s-1+ns)%ns])
		food := make([]float64, nr)
		all := 0.0
		for _, f := range w.Foods() {
			food[m.RegionAt(f.X, f.Y)]++
			all++
		}
		mean := all / total
		if mean <= 0 {
			continue
		}
		t.richNow[bin] += food[rNow] / land[rNow] / mean
		t.richLast[bin] += food[rLast] / land[rLast] / mean
		bs := w.Bodies()
		errSum, errN, rich := 0.0, 0.0, 0.0
		for _, b := range bs {
			r := m.RegionAt(int(b.X), int(b.Y))
			if int(r) == rNow {
				rich++
			}
			truth := food[r] / land[r]
			if truth <= 0 {
				continue
			}
			errSum += math.Abs(w.Belief(b).Here-truth) / truth
			errN++
		}
		if errN > 0 {
			t.err[bin] += errSum / errN
		}
		if len(bs) > 0 {
			t.inRich[bin] += rich / float64(len(bs))
		}
		t.n[bin]++
	}
	return t, nil
}

// seasons counts, on a map with seasons, whether the food on the ground
// follows the seasons (or bodies eat the difference away), how late, and
// how far behind the living's own estimates fall after a season turns.
func seasons(m engine.Map, name string, seeds int, seed0 int64, ticks int, vname string) {
	if len(m.SeasonFood) == 0 {
		fail(fmt.Errorf("map %s has no seasons", name))
	}
	fmt.Printf("地図 %s (%dx%d)、季節 %d tick × %d、%d シード、条件 %s、tick 5000〜%d（10 tick ごと）。季節が始まってからの tick の帯ごと。食料は地域の陸のタイルあたり ÷ 地図全体の陸のタイルあたり。\n\n", name, m.Width, m.Height, m.SeasonTicks, len(m.SeasonFood), seeds, vname, ticks)
	var ts []seasonsTally
	for s := 0; s < seeds; s++ {
		cfg, err := variant.Config(vname, seed0+int64(s))
		if err != nil {
			fail(err)
		}
		t, err := runSeasons(cfg, m, ticks)
		if err != nil {
			fail(err)
		}
		ts = append(ts, t)
	}
	cell := func(f func(seasonsTally) float64, prec int) string {
		var xs []float64
		for _, t := range ts {
			xs = append(xs, f(t))
		}
		mm, se := meanSE(xs)
		return fmt.Sprintf("%.*f ± %.*f", prec, mm, prec, se)
	}
	fmt.Printf("| 季節が始まってから | 今の季節に一番豊かな地域の食料 | 前の季節に一番豊かだった地域の食料 | 今いる地域の推定の相対誤差 | 今の季節に一番豊かな地域にいる身体の割合 |\n| --- | --- | --- | --- | --- |\n")
	for b := 0; b < seasonBins; b++ {
		lo, hi := b*m.SeasonTicks/seasonBins, (b+1)*m.SeasonTicks/seasonBins
		fmt.Printf("| %d〜%d | %s | %s | %s | %s |\n", lo, hi,
			cell(func(t seasonsTally) float64 { return t.richNow[b] / t.n[b] }, 3),
			cell(func(t seasonsTally) float64 { return t.richLast[b] / t.n[b] }, 3),
			cell(func(t seasonsTally) float64 { return t.err[b] / t.n[b] }, 3),
			cell(func(t seasonsTally) float64 { return t.inRich[b] / t.n[b] }, 3))
	}
	fmt.Println()
}
