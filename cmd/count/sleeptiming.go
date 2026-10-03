package main

import (
	"fmt"
	"math"

	"github.com/toku463ne/cld_generic_dangeon/engine"
	"github.com/toku463ne/cld_generic_dangeon/variant"
	"github.com/toku463ne/cld_generic_dangeon/worldmap"
)

type sleepTimingTally struct {
	// per light band, after tick 5000: decisions of adults that may sleep
	// and whose sleep is not yet forced; the summed sight gain over the
	// waking part of the window if they sleep now and if they sleep when it
	// is forced; the summed cold factor over the sleep, now and then; and
	// the decisions where sleeping now leaves the waking part at least 10%
	// more sight.
	decisions, gainNow, gainLater, coldNow, coldLater, better [4]float64
}

// runSleepTiming reads, for each such decision, when the sleep would be
// forced (ForceAt) and how the true light and cold of the window fall
// around a sleep now and one then.
func runSleepTiming(cfg engine.Config, m engine.Map, ticks int) (sleepTimingTally, error) {
	var t sleepTimingTally
	w, err := engine.NewWorld(cfg, m)
	if err != nil {
		return t, err
	}
	gain := func(l float64) float64 {
		return float64(2*cfg.NightSight+1) + 2*float64(cfg.Sight-cfg.NightSight)*l
	}
	cold := func(l float64) float64 {
		if !cfg.Temperature {
			return 1
		}
		r := worldmap.DayNightTemp // every region's on the maps here
		temp := r.Low + (r.High-r.Low)*l
		return 1 + math.Abs(temp-cfg.NormalTemp)/cfg.TempBurn
	}
	win, sleep := int64(cfg.Window), int64(cfg.SleepTicks)
	w.SetTrace(func(b engine.Body, v engine.Valuation, a engine.Action) {
		if w.Tick() <= 5000 || w.MaxEnergy(b) < cfg.EnergyMax || b.Asleep || b.Pressure <= cfg.WakeAt || b.Pressure >= cfg.ForceAt {
			return
		}
		now := w.Tick()
		k := lightBand(w.Brightness())
		due := int64(math.Ceil(cfg.PressureAwake * math.Log((1-b.Pressure)/(1-cfg.ForceAt))))
		// The waking part of the window and the sleep, for a sleep that
		// starts s ticks from now.
		read := func(s int64) (g, c float64) {
			n := 0.0
			for i := int64(0); i < win; i++ {
				l := w.BrightnessAt(now + i)
				if i >= s && i < s+sleep {
					c += cold(l)
				} else {
					g += gain(l)
					n++
				}
			}
			return g / math.Max(n, 1), c / float64(sleep)
		}
		gn, cn := read(0)
		gl, cl := read(due)
		t.decisions[k]++
		t.gainNow[k] += gn
		t.gainLater[k] += gl
		t.coldNow[k] += cn
		t.coldLater[k] += cl
		if gn >= 1.1*gl {
			t.better[k]++
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

// sleepTiming counts the room for a valuation that foresees a forced sleep:
// how the light of the waking hours and the cold of the sleep differ
// between sleeping now and sleeping when the sleep is forced.
func sleepTiming(m engine.Map, name string, seeds int, seed0 int64, ticks int, vname string) {
	var ts []sleepTimingTally
	for s := 0; s < seeds; s++ {
		cfg, err := variant.Config(vname, seed0+int64(s))
		if err != nil {
			fail(err)
		}
		t, err := runSleepTiming(cfg, m, ticks)
		if err != nil {
			fail(err)
		}
		ts = append(ts, t)
	}
	fmt.Printf("地図 %s (%dx%d)、%d シード、条件 %s、%d tick（tick 5000 より後の決定）。眠れて（睡眠圧が WakeAt を超え）まだ強制されていない成人（上限が満ち）の決定ごとに、今眠る場合と強制される時刻に眠る場合で、窓（Window tick）の起きている部分の真の視界の倍率の平均と、眠っている間の寒さの倍率の平均。\n\n", name, m.Width, m.Height, seeds, vname, ticks)
	cell := func(f func(sleepTimingTally) float64, prec int) string {
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
	fmt.Printf("| 明るさ | 決定（1シードあたり） | 起きている部分の視界: 今眠る | 強制で眠る | 今眠るほうが 10%% 以上明るい割合 | 眠りの寒さ: 今眠る | 強制で眠る |\n| --- | --- | --- | --- | --- | --- | --- |\n")
	for k, l := range lightBands {
		k := k
		fmt.Printf("| %s | %s | %s | %s | %s | %s | %s |\n", l,
			cell(func(t sleepTimingTally) float64 { return t.decisions[k] }, 0),
			cell(func(t sleepTimingTally) float64 { return ratio(t.gainNow[k], t.decisions[k]) }, 3),
			cell(func(t sleepTimingTally) float64 { return ratio(t.gainLater[k], t.decisions[k]) }, 3),
			cell(func(t sleepTimingTally) float64 { return ratio(t.better[k], t.decisions[k]) }, 3),
			cell(func(t sleepTimingTally) float64 { return ratio(t.coldNow[k], t.decisions[k]) }, 3),
			cell(func(t sleepTimingTally) float64 { return ratio(t.coldLater[k], t.decisions[k]) }, 3))
	}
	fmt.Println()
}
