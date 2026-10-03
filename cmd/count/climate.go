package main

import (
	"fmt"
	"math"

	"github.com/toku463ne/cld_generic_dangeon/engine"
	"github.com/toku463ne/cld_generic_dangeon/variant"
)

// climateFood and climateLows are the cells of the climate count: the
// food cap as a multiple of the config's, and the temperature in the dark
// (full light stays normal).
var (
	climateFood = []float64{1, 1.5, 2}
	climateLows = []float64{11, 14, 17, 20}
)

type climateTally struct {
	// after tick 5000: body-ticks, births, and body-ticks asleep in the
	// dark (light 0) and in full light, with the body-ticks of each.
	bodyTicks, births, nightAsleep, nightTicks, dayAsleep, dayTicks float64
	// sleeps begun after tick 5000, and those on a place to rest.
	slept, sheltered float64
	collapsed        bool
}

// runClimate steps one world and tallies it after tick 5000. A world that
// collapses stops.
func runClimate(cfg engine.Config, m engine.Map, ticks int) (climateTally, error) {
	var t climateTally
	w, err := engine.NewWorld(cfg, m)
	if err != nil {
		return t, err
	}
	var b0, s0, h0 int64
	for tick := 1; tick <= ticks; tick++ {
		w.Step()
		st := w.Stats()
		if tick == 5000 {
			b0, s0, h0 = st.Births, st.Slept, st.SleptSheltered
		}
		bs := w.Bodies()
		if tick > 5000 {
			n := float64(len(bs))
			t.bodyTicks += n
			asleep := 0.0
			for _, b := range bs {
				if b.Asleep {
					asleep++
				}
			}
			switch l := w.Brightness(); {
			case l <= 0:
				t.nightAsleep += asleep
				t.nightTicks += n
			case l >= 1:
				t.dayAsleep += asleep
				t.dayTicks += n
			}
			t.births, t.slept, t.sheltered = float64(st.Births-b0), float64(st.Slept-s0), float64(st.SleptSheltered-h0)
		}
		if len(bs) <= lockCollapse {
			t.collapsed = true
			break
		}
	}
	return t, nil
}

// climate counts, over a grid of food and cold, how many bodies the world
// keeps, how many are born, when they sleep and where: data for choosing a
// map's food and temperatures.
func climate(m engine.Map, name string, seeds int, seed0 int64, ticks int, vname string) {
	fmt.Printf("地図 %s (%dx%d)、%d シード、条件 %s、%d tick（tick 5000 より後）。食料の上限を設定の倍数に、全地域の暗闇の気温を変える（真昼は平温 20 度のまま）。崩壊＝個体数 %d 以下（そこで止める。崩壊したシードは個体数・出生の平均に入れない）。\n\n", name, m.Width, m.Height, seeds, vname, ticks, lockCollapse)
	fmt.Printf("| 食料 | 暗闇の気温 | 個体数 | 1000 tick あたりの出生 | 眠っている割合: 暗闇 | 真昼 | 休める場所で始まった眠り | 崩壊 |\n| --- | --- | --- | --- | --- | --- | --- | --- |\n")
	for _, food := range climateFood {
		for _, low := range climateLows {
			var ts []climateTally
			collapsed := 0
			for s := 0; s < seeds; s++ {
				cfg, err := variant.Config(vname, seed0+int64(s))
				if err != nil {
					fail(err)
				}
				cfg.FoodCap = int(math.Round(float64(cfg.FoodCap) * food))
				mm := m.Clone()
				for r := range mm.RegionTemp {
					mm.RegionTemp[r].Low = low
				}
				t, err := runClimate(cfg, mm, ticks)
				if err != nil {
					fail(err)
				}
				if t.collapsed {
					collapsed++
					continue
				}
				ts = append(ts, t)
			}
			cell := func(f func(climateTally) float64, prec int) string {
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
			span := float64(ticks - 5000)
			ratio := func(a, b float64) float64 {
				if b == 0 {
					return math.NaN()
				}
				return a / b
			}
			fmt.Printf("| ×%.1f | %.0f | %s | %s | %s | %s | %s | %d / %d |\n", food, low,
				cell(func(t climateTally) float64 { return t.bodyTicks / span }, 1),
				cell(func(t climateTally) float64 { return 1000 * t.births / span }, 2),
				cell(func(t climateTally) float64 { return ratio(t.nightAsleep, t.nightTicks) }, 3),
				cell(func(t climateTally) float64 { return ratio(t.dayAsleep, t.dayTicks) }, 3),
				cell(func(t climateTally) float64 { return ratio(t.sheltered, t.slept) }, 3),
				collapsed, seeds)
		}
	}
	fmt.Println()
}
