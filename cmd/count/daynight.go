package main

import (
	"fmt"
	"math"

	"github.com/toku463ne/cld_generic_dangeon/engine"
	"github.com/toku463ne/cld_generic_dangeon/variant"
)

// dayBins is how many parts of a day the daynight count reads.
const dayBins = 10

type dayNightTally struct {
	// per part of the day, after tick 5000: body-ticks, meals eaten (on
	// the ground or held), starvations, and the energy summed over the
	// adults at their full most.
	bodyTicks, meals, picks, starved, energy, adults, light [dayBins]float64
}

// runDayNight steps one world and adds each tick after 5000 into the part
// of the day it falls in. A world that collapses stops.
func runDayNight(cfg engine.Config, m engine.Map, ticks, day int) (dayNightTally, error) {
	var t dayNightTally
	w, err := engine.NewWorld(cfg, m)
	if err != nil {
		return t, err
	}
	var pe, pp, pd int64
	for tick := 1; tick <= ticks; tick++ {
		w.Step()
		st := w.Stats()
		eaten := st.Actions[engine.ActEat] + st.EatenHeld
		dead := st.Deaths[engine.CauseStarved]
		picked := st.Actions[engine.ActPick]
		bs := w.Bodies()
		if tick > 5000 {
			k := (tick - 1) % day * dayBins / day
			t.bodyTicks[k] += float64(len(bs))
			t.light[k] += w.Brightness()
			// A pick that fits is eaten at once and counted in EatenHeld,
			// so meals are eats on the ground plus eats from the hand.
			t.meals[k] += float64(eaten - pe)
			t.starved[k] += float64(dead - pd)
			t.picks[k] += float64(picked - pp)
			for _, b := range bs {
				if w.MaxEnergy(b) >= cfg.EnergyMax {
					t.energy[k] += b.Energy
					t.adults[k]++
				}
			}
		}
		pe, pp, pd = eaten, picked, dead
		if len(bs) <= lockCollapse {
			break
		}
	}
	return t, nil
}

// dayNight counts eating, starving and energy by the part of the day.
func dayNight(m engine.Map, name string, seeds int, seed0 int64, ticks int, vname string) {
	cfg0, err := variant.Config(vname, seed0)
	if err != nil {
		fail(err)
	}
	day := cfg0.DayTicks
	if day <= 0 {
		day = 1000 // a world without days is read in the parts of a 1000-tick day
	}
	var ts []dayNightTally
	for s := 0; s < seeds; s++ {
		cfg, err := variant.Config(vname, seed0+int64(s))
		if err != nil {
			fail(err)
		}
		t, err := runDayNight(cfg, m, ticks, day)
		if err != nil {
			fail(err)
		}
		ts = append(ts, t)
	}
	fmt.Printf("地図 %s (%dx%d)、%d シード、条件 %s、%d tick（tick 5000 より後）。1日 %d tick（tick 0 を朝として）を %d 等分して読む。明るさはその区間の平均。\n\n", name, m.Width, m.Height, seeds, vname, ticks, day, dayBins)
	cell := func(f func(dayNightTally) float64, prec int) string {
		var xs []float64
		for _, t := range ts {
			if x := f(t); !math.IsNaN(x) {
				xs = append(xs, x)
			}
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
	fmt.Printf("| 1日の中の tick | 明るさ | 個体数 | 1000 体・tick あたりの食事 | 1000 体・tick あたりの拾う手 | 1000 体・tick あたりの餓死 | 成人（上限が満ち）の体力の平均 |\n| --- | --- | --- | --- | --- | --- | --- |\n")
	for k := 0; k < dayBins; k++ {
		k := k
		lo, hi := k*day/dayBins+1, (k+1)*day/dayBins
		perTick := float64(ticks-5000) / float64(day) * float64(day/dayBins)
		fmt.Printf("| %d〜%d | %s | %s | %s | %s | %s | %s |\n", lo, hi,
			cell(func(t dayNightTally) float64 { return t.light[k] / perTick }, 2),
			cell(func(t dayNightTally) float64 { return t.bodyTicks[k] / perTick }, 1),
			cell(func(t dayNightTally) float64 { return 1000 * ratio(t.meals[k], t.bodyTicks[k]) }, 2),
			cell(func(t dayNightTally) float64 { return 1000 * ratio(t.picks[k], t.bodyTicks[k]) }, 3),
			cell(func(t dayNightTally) float64 { return 1000 * ratio(t.starved[k], t.bodyTicks[k]) }, 3),
			cell(func(t dayNightTally) float64 { return ratio(t.energy[k], t.adults[k]) }, 1))
	}
	fmt.Println()
}
