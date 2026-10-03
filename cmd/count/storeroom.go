package main

import (
	"fmt"
	"math"
	"sort"

	"github.com/toku463ne/cld_generic_dangeon/engine"
	"github.com/toku463ne/cld_generic_dangeon/variant"
)

type storeRoomTally struct {
	// decisions after tick 5000, and those with food in sight that the
	// body could neither eat (a meal does not fit) nor pick up (hand full):
	// a surplus it could only store.
	decisions, surplus float64
	// Followed surpluses (one open per body at a time, adults at their
	// full most), and of them those whose body fell to one meal or less of
	// energy within the window, and those whose body died within it, of
	// age or starved.
	followed, critical, starved, aged float64
	// ticks from the surplus to the fall, and how far (tiles, Chebyshev)
	// the body then was from where it saw the surplus.
	wait, dist []float64
	// by the light when decided (stage 7): decisions of adults at their
	// full most, and of them those with a surplus.
	adultBy, surplusBy [4]float64
}

// runStoreRoom reads every decision after tick 5000 for a surplus, and
// follows each body with one for a window.
func runStoreRoom(cfg engine.Config, m engine.Map, ticks int) (storeRoomTally, error) {
	var t storeRoomTally
	w, err := engine.NewWorld(cfg, m)
	if err != nil {
		return t, err
	}
	type open struct {
		tick, born int64
		x, y       int
		fell       bool
	}
	opens := map[int64]open{}
	w.SetTrace(func(b engine.Body, v engine.Valuation, a engine.Action) {
		if w.Tick() <= 5000 {
			return
		}
		t.decisions++
		if w.MaxEnergy(b) < cfg.EnergyMax {
			return // a child or an old body: one meal is a large part of its most
		}
		k := lightBand(w.Brightness())
		t.adultBy[k]++
		if len(v.Seen) == 0 || b.Energy <= w.MaxEnergy(b)-cfg.FoodEnergy || b.Held < cfg.Carry {
			return
		}
		t.surplus++
		t.surplusBy[k]++
		if _, ok := opens[b.ID]; ok {
			return
		}
		f := v.Seen[0]
		opens[b.ID] = open{tick: w.Tick(), born: b.Born, x: f.X, y: f.Y}
		t.followed++
	})
	for tick := 1; tick <= ticks; tick++ {
		w.Step()
		if len(opens) == 0 {
			continue
		}
		alive := map[int64]engine.Body{}
		for _, b := range w.Bodies() {
			alive[b.ID] = b
		}
		for id, o := range opens {
			b, ok := alive[id]
			switch {
			case !ok:
				// Died within the window: of age, or starved.
				if cfg.Lifespan > 0 && w.Tick()-o.born >= int64(cfg.Lifespan) {
					t.aged++
				} else {
					t.starved++
				}
				delete(opens, id)
			case w.Tick()-o.tick >= int64(cfg.Window):
				delete(opens, id)
			case !o.fell && b.Energy <= cfg.FoodEnergy:
				t.critical++
				t.wait = append(t.wait, float64(w.Tick()-o.tick))
				t.dist = append(t.dist, math.Max(math.Abs(math.Floor(b.X)-float64(o.x)), math.Abs(math.Floor(b.Y)-float64(o.y))))
				o.fell = true
				opens[id] = o
			}
		}
		if len(w.Bodies()) <= lockCollapse {
			break
		}
	}
	return t, nil
}

// storeRoom counts the room a store would have: food a body sees and can
// neither eat nor carry, and whether that body needs food within a window.
func storeRoom(m engine.Map, name string, seeds int, seed0 int64, ticks int, vname string) {
	var ts []storeRoomTally
	var wait, dist []float64
	for s := 0; s < seeds; s++ {
		cfg, err := variant.Config(vname, seed0+int64(s))
		if err != nil {
			fail(err)
		}
		t, err := runStoreRoom(cfg, m, ticks)
		if err != nil {
			fail(err)
		}
		ts = append(ts, t)
		wait = append(wait, t.wait...)
		dist = append(dist, t.dist...)
	}
	fmt.Printf("地図 %s (%dx%d)、%d シード、条件 %s、%d tick（tick 5000 より後の決定）。余り＝成人（体力の上限が満ち）で、視界に食料があり、1食が入りきらず、手持ちが一杯。追う窓は Window。\n\n", name, m.Width, m.Height, seeds, vname, ticks)
	cell := func(f func(storeRoomTally) float64) string {
		var xs []float64
		for _, t := range ts {
			if x := f(t); !math.IsNaN(x) {
				xs = append(xs, x)
			}
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
	fmt.Printf("| 量 | 値 |\n| --- | --- |\n")
	fmt.Printf("| 余りを見た決定の割合 | %s |\n", cell(func(t storeRoomTally) float64 { return ratio(t.surplus, t.decisions) }))
	fmt.Printf("| 追った余り（1シードあたり） | %s |\n", cell(func(t storeRoomTally) float64 { return t.followed }))
	fmt.Printf("| 　窓の中で体力が1食以下に落ちた割合 | %s |\n", cell(func(t storeRoomTally) float64 { return ratio(t.critical, t.followed) }))
	fmt.Printf("| 　窓の中で老いで死んだ割合 | %s |\n", cell(func(t storeRoomTally) float64 { return ratio(t.aged, t.followed) }))
	fmt.Printf("| 　窓の中で餓死した割合 | %s |\n", cell(func(t storeRoomTally) float64 { return ratio(t.starved, t.followed) }))
	fmt.Printf("\n| 明るさ（決めたとき） | 成人（上限が満ち）の決定（1シードあたり） | そのうち余りを見た割合 |\n| --- | --- | --- |\n")
	for k, l := range lightBands {
		k := k
		fmt.Printf("| %s | %s | %s |\n", l, cell(func(t storeRoomTally) float64 { return t.adultBy[k] }), cell(func(t storeRoomTally) float64 { return ratio(t.surplusBy[k], t.adultBy[k]) }))
	}
	q := func(xs []float64, p float64) float64 {
		if len(xs) == 0 {
			return math.NaN()
		}
		return xs[int(p*float64(len(xs)-1))]
	}
	sort.Float64s(wait)
	sort.Float64s(dist)
	fmt.Printf("\n1食以下に落ちるまでの tick: 中央値 %.0f / 25%% %.0f / 75%% %.0f。そのとき余りを見た場所までの距離（タイル）: 中央値 %.0f / 25%% %.0f / 75%% %.0f\n\n", q(wait, 0.5), q(wait, 0.25), q(wait, 0.75), q(dist, 0.5), q(dist, 0.25), q(dist, 0.75))
}
