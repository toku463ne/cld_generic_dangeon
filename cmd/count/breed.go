package main

import (
	"fmt"
	"math"
	"sort"

	"github.com/toku463ne/cld_generic_dangeon/engine"
	"github.com/toku463ne/cld_generic_dangeon/variant"
)

// breedRanges are the distances, in tiles on either axis (0 is the same
// tile, 1 the 3x3 square of Sight 1), at which another body is counted as
// near.
var breedRanges = []int{0, 1, 2}

// breedCosts are birth costs, in energy, whose effect on the risk is read.
var breedCosts = []float64{10, 25, 50}

type breedTally struct {
	decisions float64
	near      [3]float64 // decisions with another body within breedRanges[i]
	nearTicks [3]float64 // body-ticks with another body within breedRanges[i]
	bodyTicks float64
	// Over the decisions with another body within Sight: how often paying
	// breedCosts[i] leaves the risk of the best option exactly as it was,
	// and the rise in risk it brings.
	costed   float64
	tie      [3]float64
	rise     [3][]float64
	start    int // bodies at tick 0
	alive    int // bodies alive at the end
	deathAge []float64
}

// runBreed counts, in the base world after tick 5000, how often another
// body is near, and what paying the energy of a birth would do to the risk
// of the best option; and over the whole run, how long bodies live.
func runBreed(cfg engine.Config, m engine.Map, ticks int) (breedTally, error) {
	var t breedTally
	w, err := engine.NewWorld(cfg, m)
	if err != nil {
		return t, err
	}
	t.start = len(w.Bodies())
	occ := make([]int32, m.Width*m.Height)
	near := func(x, y float64, r int) bool {
		tx, ty := int(math.Floor(x)), int(math.Floor(y))
		n := int32(0)
		for yy := ty - r; yy <= ty+r; yy++ {
			for xx := tx - r; xx <= tx+r; xx++ {
				if m.InBounds(xx, yy) {
					n += occ[yy*m.Width+xx]
				}
			}
		}
		return n > 1 // the body itself is one
	}
	cache := map[float64]engine.Survival{}
	burnTicks := func(e float64) int { return int(math.Ceil(e/cfg.EnergyBurn - 1e-9)) }
	w.SetTrace(func(b engine.Body, v engine.Valuation, _ engine.Action) {
		if w.Tick() <= 5000 {
			return
		}
		t.decisions++
		for i, r := range breedRanges {
			if near(b.X, b.Y, r) {
				t.near[i]++
			}
		}
		if !near(b.X, b.Y, cfg.Sight) {
			return
		}
		// The risk of keeping on the move through the region, now and
		// after paying each cost: the survival table's own row.
		tab := w.TruthTable()
		q := tab.Meet[m.RegionAt(int(math.Floor(b.X)), int(math.Floor(b.Y)))]
		s, ok := cache[q]
		if !ok {
			s = tab.NewSurvival(q, []int{cfg.Window})
			cache[q] = s
		}
		n := burnTicks(b.Energy) - 1
		now := s.Dead(0, n)
		t.costed++
		for i, c := range breedCosts {
			after := s.Dead(0, n-burnTicks(c))
			if after == now {
				t.tie[i]++
			}
			t.rise[i] = append(t.rise[i], after-now)
		}
	})
	born := map[int64]int64{}
	for _, b := range w.Bodies() {
		born[b.ID] = b.Born
	}
	for tick := 1; tick <= ticks; tick++ {
		bodies := w.Bodies()
		clear(occ)
		for _, b := range bodies {
			occ[int(math.Floor(b.Y))*m.Width+int(math.Floor(b.X))]++
		}
		if tick > 5000 {
			for _, b := range bodies {
				t.bodyTicks++
				for i, r := range breedRanges {
					if near(b.X, b.Y, r) {
						t.nearTicks[i]++
					}
				}
			}
		}
		w.Step()
		alive := map[int64]bool{}
		for _, b := range w.Bodies() {
			alive[b.ID] = true
		}
		for _, b := range bodies {
			if !alive[b.ID] {
				t.deathAge = append(t.deathAge, float64(w.Tick()-born[b.ID]))
			}
		}
		if len(w.Bodies()) == 0 {
			break
		}
	}
	t.alive = len(w.Bodies())
	return t, nil
}

// breed counts, for stage 1-3, how often a body deciding has another body
// near (the chance a mate could be at hand), how long bodies live when
// starving is the only death, and how often paying for a birth would
// leave the risk of death exactly unchanged.
func breed(m engine.Map, name string, seeds int, seed0 int64, ticks int) {
	fmt.Printf("地図 %s (%dx%d)、%d シード、%d tick、base（1-2e）。近さ・コストは tick 5000 より後の決定、寿命は実行全体。\n\n", name, m.Width, m.Height, seeds, ticks)
	var ts []breedTally
	for s := 0; s < seeds; s++ {
		cfg, err := variant.Config(variant.Base, seed0+int64(s))
		if err != nil {
			fail(err)
		}
		t, err := runBreed(cfg, m, ticks)
		if err != nil {
			fail(err)
		}
		ts = append(ts, t)
	}
	cell := func(f func(breedTally) float64) string {
		var xs []float64
		for _, t := range ts {
			xs = append(xs, f(t))
		}
		mm, se := meanSE(xs)
		return fmt.Sprintf("%.4f ± %.4f", mm, se)
	}
	fmt.Printf("| 他の身体までの距離（タイル、各軸） | 決定に占める割合 | 身体×tick に占める割合 |\n| --- | --- | --- |\n")
	for i, r := range breedRanges {
		fmt.Printf("| %d 以内 | %s | %s |\n", r,
			cell(func(t breedTally) float64 { return t.near[i] / t.decisions }),
			cell(func(t breedTally) float64 { return t.nearTicks[i] / t.bodyTicks }))
	}
	fmt.Printf("\n視界の中に他の身体がいる決定で、体力を払ったときの「地域を歩き続ける」の死ぬ確率（窓 %d）\n\n", engine.DefaultConfig().Window)
	fmt.Printf("| 払う体力 | 死ぬ確率がちょうど変わらない割合 | 上がり幅の中央値 | 上がり幅の 90%% 点 |\n| --- | --- | --- | --- |\n")
	for i, c := range breedCosts {
		var all []float64
		for _, t := range ts {
			all = append(all, t.rise[i]...)
		}
		sort.Float64s(all)
		q := func(p float64) float64 { return all[int(p*float64(len(all)-1))] }
		fmt.Printf("| %.0f | %s | %.3g | %.3g |\n", c, cell(func(t breedTally) float64 { return t.tie[i] / t.costed }), q(0.5), q(0.9))
	}
	var ages []float64
	for _, t := range ts {
		ages = append(ages, t.deathAge...)
	}
	sort.Float64s(ages)
	fmt.Printf("\n寿命: 最初の身体のうち %d tick 後に生きている割合 %s。", ticks, cell(func(t breedTally) float64 { return float64(t.alive) / float64(t.start) }))
	if len(ages) > 0 {
		q := func(p float64) float64 { return ages[int(p*float64(len(ages)-1))] }
		fmt.Printf("死んだ身体の死んだ年齢（全シード）: 10%% 点 %.0f、中央値 %.0f、90%% 点 %.0f、最大 %.0f（%d 体）\n", q(0.1), q(0.5), q(0.9), ages[len(ages)-1], len(ages))
	} else {
		fmt.Println()
	}
}
