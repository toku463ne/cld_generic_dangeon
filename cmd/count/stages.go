package main

import (
	"fmt"

	"github.com/toku463ne/cld_generic_dangeon/engine"
	"github.com/toku463ne/cld_generic_dangeon/variant"
)

type stagesTally struct {
	// Body-ticks after tick 5000, and those spent as children (before
	// Mature).
	bodyTicks, childTicks float64
	// Bodies born after tick 5000 that died before the end: how many, how
	// many died children, their ages at death summed, and the ticks they
	// lived as children and as adults.
	died, diedChild, ages, lifeChild, lifeAdult float64
	agesAt                                      []float64
}

// runStages reads how the living's ticks and the lives of the dead divide
// between childhood and adulthood.
func runStages(cfg engine.Config, m engine.Map, ticks int) (stagesTally, error) {
	var t stagesTally
	w, err := engine.NewWorld(cfg, m)
	if err != nil {
		return t, err
	}
	last := map[int64]engine.Body{}
	for tick := 1; tick <= ticks; tick++ {
		w.Step()
		now := w.Tick()
		alive := map[int64]bool{}
		for _, b := range w.Bodies() {
			alive[b.ID] = true
			last[b.ID] = b
			if tick > 5000 {
				t.bodyTicks++
				if now < b.Mature {
					t.childTicks++
				}
			}
		}
		for id, b := range last {
			if alive[id] {
				continue
			}
			delete(last, id)
			if b.Born <= 5000 {
				continue
			}
			age := float64(now - b.Born)
			t.died++
			t.ages += age
			t.agesAt = append(t.agesAt, age)
			child := float64(min(now, b.Mature) - b.Born)
			t.lifeChild += child
			t.lifeAdult += age - child
			if now < b.Mature {
				t.diedChild++
			}
		}
	}
	return t, nil
}

// stages counts how much of the world is children: the share of the
// living's ticks spent before coming of age, and how the lives of bodies
// born and dead in the run divide.
func stages(m engine.Map, name string, seeds int, seed0 int64, ticks int, vname string) {
	cfg0, err := variant.Config(vname, seed0)
	if err != nil {
		fail(err)
	}
	fmt.Printf("地図 %s (%dx%d)、%d シード、条件 %s、%d tick（tick 5000 より後）。成人まで %d tick。\n\n", name, m.Width, m.Height, seeds, vname, ticks, cfg0.MatureAge)
	var ts []stagesTally
	var ages []float64
	for s := 0; s < seeds; s++ {
		cfg, err := variant.Config(vname, seed0+int64(s))
		if err != nil {
			fail(err)
		}
		t, err := runStages(cfg, m, ticks)
		if err != nil {
			fail(err)
		}
		ts = append(ts, t)
		ages = append(ages, t.agesAt...)
	}
	cell := func(f func(stagesTally) float64, prec int) string {
		var xs []float64
		for _, t := range ts {
			xs = append(xs, f(t))
		}
		mm, se := meanSE(xs)
		return fmt.Sprintf("%.*f ± %.*f", prec, mm, prec, se)
	}
	fmt.Printf("| 量 | 値 |\n| --- | --- |\n")
	fmt.Printf("| 生きている身体×tick のうち子（成人前）の割合 | %s |\n", cell(func(t stagesTally) float64 { return t.childTicks / t.bodyTicks }, 4))
	fmt.Printf("| 生まれて死んだ身体（1シードあたり） | %s |\n", cell(func(t stagesTally) float64 { return t.died }, 0))
	fmt.Printf("| そのうち成人前に死んだ割合 | %s |\n", cell(func(t stagesTally) float64 { return t.diedChild / t.died }, 4))
	fmt.Printf("| 一生の長さの平均（tick） | %s |\n", cell(func(t stagesTally) float64 { return t.ages / t.died }, 0))
	fmt.Printf("| 一生のうち子で過ごした tick の割合 | %s |\n", cell(func(t stagesTally) float64 { return t.lifeChild / (t.lifeChild + t.lifeAdult) }, 4))
	fmt.Printf("| 死んだ年齢（全シード、25%%・50%%・75%%・95%%） | %s |\n\n", quantiles(ages, 0.25, 0.5, 0.75, 0.95))
}
