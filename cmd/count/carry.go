package main

import (
	"fmt"
	"math"
	"sort"

	"github.com/toku463ne/cld_generic_dangeon/engine"
	"github.com/toku463ne/cld_generic_dangeon/variant"
)

type carryTally struct {
	// Units picked up after tick 5000, and of them how many were eaten
	// from the hold, and how many were held at death.
	picked, eaten, lost float64
	// Ticks from picking up to eating, and tiles between where the unit was
	// picked up and where it was eaten (octile distance), per unit eaten.
	ticks, dist []float64
	// Of the units eaten, those eaten at once (the tick they were picked up,
	// where a meal fitted).
	atOnce float64
	// Body-ticks after tick 5000 (every 50 ticks), and those holding food.
	bodyTicks, holding float64
	// Deaths after tick 5000, and those holding food.
	deaths, deathsHolding float64
}

// runCarry follows every unit picked up, first in first out per body: where
// and when it was picked up, and where and when it was eaten or lost.
func runCarry(cfg engine.Config, m engine.Map, ticks int) (carryTally, error) {
	var t carryTally
	w, err := engine.NewWorld(cfg, m)
	if err != nil {
		return t, err
	}
	type unit struct {
		x, y float64
		tick int64
	}
	held := map[int64][]unit{}
	prev := map[int64]engine.Body{}
	for tick := 1; tick <= ticks; tick++ {
		w.Step()
		now := w.Tick()
		late := tick > 5000
		cur := map[int64]engine.Body{}
		for _, b := range w.Bodies() {
			cur[b.ID] = b
			p, had := prev[b.ID]
			before := 0
			if had {
				before = p.Held
			}
			q := held[b.ID]
			switch {
			case b.Held > before:
				// Picked up this tick, where it stands now (a pick does not
				// move it).
				for k := 0; k < b.Held-before; k++ {
					q = append(q, unit{b.X, b.Y, now})
				}
				if late {
					t.picked += float64(b.Held - before)
				}
			case b.Held < before:
				// Eaten from the hold at the start of its turn, before it
				// moved: where it stood at the end of the last tick.
				for k := 0; k < before-b.Held && len(q) > 0; k++ {
					u := q[0]
					q = q[1:]
					if late && u.tick > 5000 {
						t.eaten++
						t.ticks = append(t.ticks, float64(now-u.tick))
						dx, dy := math.Abs(p.X-u.x), math.Abs(p.Y-u.y)
						t.dist = append(t.dist, math.Max(dx, dy)+(math.Sqrt2-1)*math.Min(dx, dy))
					}
				}
			}
			held[b.ID] = q
			// A unit picked up and eaten in the same tick never shows in
			// Held: count it from the stats below.
		}
		for id, p := range prev {
			if _, ok := cur[id]; ok {
				continue
			}
			if late {
				t.deaths++
				if p.Held > 0 {
					t.deathsHolding++
				}
				for _, u := range held[id] {
					if u.tick > 5000 {
						t.lost++
					}
				}
			}
			delete(held, id)
		}
		if late && tick%50 == 0 {
			for _, b := range cur {
				t.bodyTicks++
				if b.Held > 0 {
					t.holding++
				}
			}
		}
		prev = cur
		if tick == 5000 {
			st := w.Stats()
			t.atOnce = -float64(st.Actions[engine.ActPick])
		}
	}
	// Picks that never showed in Held were eaten at once: all picks less
	// those seen held.
	st := w.Stats()
	t.atOnce += float64(st.Actions[engine.ActPick])
	t.atOnce -= t.picked
	return t, nil
}

// carry measures, in a world where bodies hold food (stage 4-1), how far and
// how long food is carried before it is eaten, how much is lost with the
// dead, and how often bodies hold food.
func carry(m engine.Map, name string, seeds int, seed0 int64, ticks int, vname string) {
	var ts []carryTally
	var all, dists []float64
	for s := 0; s < seeds; s++ {
		cfg, err := variant.Config(vname, seed0+int64(s))
		if err != nil {
			fail(err)
		}
		t, err := runCarry(cfg, m, ticks)
		if err != nil {
			fail(err)
		}
		ts = append(ts, t)
		all = append(all, t.ticks...)
		dists = append(dists, t.dist...)
	}
	fmt.Printf("地図 %s (%dx%d)、%d シード、条件 %s、%d tick（tick 5000 より後に拾った単位）。手持ちは身体ごとに先に拾ったものから食べるとして追う。\n\n", name, m.Width, m.Height, seeds, vname, ticks)
	cell := func(f func(carryTally) float64, prec int) string {
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
	q := func(xs []float64, p float64) float64 {
		if len(xs) == 0 {
			return math.NaN()
		}
		s := append([]float64(nil), xs...)
		sort.Float64s(s)
		return s[int(p*float64(len(s)-1))]
	}
	fmt.Printf("| 量 | 値 |\n| --- | --- |\n")
	fmt.Printf("| 拾ってすぐ食べた単位（1シードあたり。1食が入る身体が拾った） | %s |\n", cell(func(t carryTally) float64 { return t.atOnce }, 0))
	fmt.Printf("| 拾って持った単位（1シードあたり） | %s |\n", cell(func(t carryTally) float64 { return t.picked }, 0))
	fmt.Printf("| 　そのうち手持ちから食べた割合 | %s |\n", cell(func(t carryTally) float64 { return ratio(t.eaten, t.picked) }, 4))
	fmt.Printf("| 　そのうち持ったまま死んだ割合 | %s |\n", cell(func(t carryTally) float64 { return ratio(t.lost, t.picked) }, 4))
	fmt.Printf("| 食料を持っている身体の割合（50 tick ごと） | %s |\n", cell(func(t carryTally) float64 { return ratio(t.holding, t.bodyTicks) }, 4))
	fmt.Printf("| 死んだ身体のうち食料を持っていた割合 | %s |\n", cell(func(t carryTally) float64 { return ratio(t.deathsHolding, t.deaths) }, 4))
	fmt.Printf("| 拾ってから食べるまでの tick（全シード、中央値 / 75%% / 95%%） | %.0f / %.0f / %.0f |\n", q(all, 0.5), q(all, 0.75), q(all, 0.95))
	fmt.Printf("| 拾った場所から食べた場所までのタイル（全シード、中央値 / 75%% / 95%%） | %.1f / %.1f / %.1f |\n", q(dists, 0.5), q(dists, 0.75), q(dists, 0.95))
	fmt.Printf("| 拾った場所から食べた場所までのタイル（平均） | %s |\n\n", cell(func(t carryTally) float64 {
		if len(t.dist) == 0 {
			return math.NaN()
		}
		sum := 0.0
		for _, d := range t.dist {
			sum += d
		}
		return sum / float64(len(t.dist))
	}, 2))
}
