package main

import (
	"fmt"

	"github.com/toku463ne/cld_generic_dangeon/engine"
	"github.com/toku463ne/cld_generic_dangeon/variant"
)

type provisionTally struct {
	// Summed over ticks after tick 5000 and over regions: the food owed
	// back to the region that tick (what the engine returns, as vacancies
	// times FoodReturn times the region's share), the mothers resting in
	// it, and those whose partner (the father of her last child) stood in
	// the same region.
	owed, resting, withPartner float64
	// Ticks and regions with at least one mother resting, and the food owed
	// back in those, per resting mother.
	busy, owedPerMother float64
	// A mother's energy when her rest ended, summed, and how many rests
	// ended with her alive.
	endEnergy, ends float64
	// The food given to resting mothers (Provision), in units; the meals
	// resting mothers chose to eat themselves, and their decisions with
	// food underfoot, after tick 5000.
	given, ate, underfoot float64
}

// runProvision reads, in the world of stage 3-2, how much food comes back
// to each region while mothers rest there, how many rest, and whether their
// partner is in the same region: what provisioning a resting mother from
// the food that comes back would have to work with.
func runProvision(cfg engine.Config, m engine.Map, ticks int) (provisionTally, error) {
	var t provisionTally
	w, err := engine.NewWorld(cfg, m)
	if err != nil {
		return t, err
	}
	partner := map[int64]int64{} // mother -> father of her last child
	seen := map[int64]bool{}
	for _, b := range w.Bodies() {
		seen[b.ID] = true
	}
	nr := len(m.RegionFood)
	var given0 int64
	w.SetTrace(func(b engine.Body, v engine.Valuation, a engine.Action) {
		if w.Tick() <= 5000 || b.Sex != engine.Female || w.Tick() >= b.Rested {
			return
		}
		for _, o := range v.Options {
			if o.Kind == engine.ActEat {
				t.underfoot++
				break
			}
		}
		if a.Kind == engine.ActEat {
			t.ate++
		}
	})
	for tick := 1; tick <= ticks; tick++ {
		if tick == 5001 {
			given0 = w.FoodLedger().Provisioned
		}
		vacant := float64(cfg.FoodCap - len(w.Foods()))
		w.Step()
		now := w.Tick()
		bs := w.Bodies()
		region := map[int64]engine.RegionID{}
		for _, b := range bs {
			region[b.ID] = m.RegionAt(int(b.X), int(b.Y))
		}
		for _, b := range bs {
			if seen[b.ID] {
				continue
			}
			seen[b.ID] = true
			for i, p := range b.Parents {
				q := b.Parents[1-i]
				for _, x := range bs {
					if x.ID == p && x.Sex == engine.Female {
						partner[p] = q
					}
				}
			}
		}
		if tick <= 5000 {
			continue
		}
		shares := m.FoodShares(now)
		sum := 0.0
		for _, s := range shares {
			sum += s
		}
		resting := make([]float64, nr)
		with := make([]float64, nr)
		for _, b := range bs {
			if b.Sex != engine.Female {
				continue
			}
			if b.Rested == now && b.Rested > 0 {
				t.endEnergy += b.Energy
				t.ends++
			}
			if now >= b.Rested {
				continue
			}
			r := region[b.ID]
			resting[r]++
			if f, ok := partner[b.ID]; ok {
				if fr, alive := region[f]; alive && fr == r {
					with[r]++
				}
			}
		}
		for r := 0; r < nr; r++ {
			owed := cfg.FoodReturn * vacant * shares[r] / sum
			t.owed += owed
			t.resting += resting[r]
			t.withPartner += with[r]
			if resting[r] > 0 {
				t.busy++
				t.owedPerMother += owed / resting[r]
			}
		}
	}
	t.given = float64(w.FoodLedger().Provisioned - given0)
	return t, nil
}

// provision counts, before stage 3-3, what the food that comes back could
// give a resting mother.
func provision(m engine.Map, name string, seeds int, seed0 int64, ticks int, vname string) {
	cfg0, err := variant.Config(vname, seed0)
	if err != nil {
		fail(err)
	}
	fmt.Printf("地図 %s (%dx%d)、%d シード、条件 %s、%d tick（tick 5000 より後）。休んでいる母＝出産して %d tick のあいだの雌。相手＝最後の子の父。戻る食料＝その tick に地域へ戻る分（空き × 戻る確率 × 取り分）。\n\n", name, m.Width, m.Height, seeds, vname, ticks, cfg0.RecoverTicks)
	var ts []provisionTally
	for s := 0; s < seeds; s++ {
		cfg, err := variant.Config(vname, seed0+int64(s))
		if err != nil {
			fail(err)
		}
		t, err := runProvision(cfg, m, ticks)
		if err != nil {
			fail(err)
		}
		ts = append(ts, t)
	}
	cell := func(f func(provisionTally) float64, prec int) string {
		var xs []float64
		for _, t := range ts {
			xs = append(xs, f(t))
		}
		mm, se := meanSE(xs)
		return fmt.Sprintf("%.*f ± %.*f", prec, mm, prec, se)
	}
	n := float64(ticks - 5000)
	fmt.Printf("| 量 | 値 |\n| --- | --- |\n")
	fmt.Printf("| 地図全体に戻る食料（1 tick あたりの単位） | %s |\n", cell(func(t provisionTally) float64 { return t.owed / n }, 4))
	fmt.Printf("| 休んでいる母（1 tick あたりの数） | %s |\n", cell(func(t provisionTally) float64 { return t.resting / n }, 3))
	fmt.Printf("| そのうち相手が同じ地域にいる割合 | %s |\n", cell(func(t provisionTally) float64 { return t.withPartner / t.resting }, 4))
	fmt.Printf("| 休んでいる母のいる地域で、母1体あたりに戻る食料（1 tick あたりの単位） | %s |\n", cell(func(t provisionTally) float64 { return t.owedPerMother / t.busy }, 5))
	fmt.Printf("| 休み明けの母の体力 | %s |\n", cell(func(t provisionTally) float64 { return t.endEnergy / t.ends }, 1))
	fmt.Printf("| 休んでいる母1体が休みのあいだに自分で食べた回数 | %s |\n", cell(func(t provisionTally) float64 {
		return t.ate / t.resting * float64(cfg0.RecoverTicks)
	}, 2))
	fmt.Printf("| 休んでいる母が足元に食料のある決定で食べた割合 | %s |\n", cell(func(t provisionTally) float64 { return t.ate / t.underfoot }, 3))
	fmt.Printf("| 休んでいる母1体が休みのあいだに配られた体力（配られた食料 × 食料の体力 ÷ 休んでいる母の数 × 休みの tick 数） | %s |\n\n", cell(func(t provisionTally) float64 {
		return t.given * cfg0.FoodEnergy / t.resting * float64(cfg0.RecoverTicks)
	}, 1))
}
