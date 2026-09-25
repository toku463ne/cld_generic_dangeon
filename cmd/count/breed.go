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
	alive    int // of them, alive at the end
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
				t.deathAge = append(t.deathAge, float64(w.Tick()-b.Born))
			}
		}
		if len(w.Bodies()) == 0 {
			break
		}
	}
	// The first bodies are the IDs below the starting count.
	for _, b := range w.Bodies() {
		if b.ID < int64(t.start) {
			t.alive++
		}
	}
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

type mateTally struct {
	offered, chose      float64 // decisions with a mate offered, and choosing one
	eChose, eDeclined   float64 // summed energy of those that chose and declined
	proposals, births   float64 // mate actions carried out, and births
	riskChose, riskDecl []float64
	// Dying decisions (the best option other than a mate still at risk 0.5
	// or more), with no food underfoot or in sight [0] and with some [1]:
	// how many had a mate offered, and how many took it.
	dyingOffered, dyingChose [2]float64
	// The same, dying read from the body's outlook without eating: keeping
	// on the move through its region, the table's third row alone.
	starvingOffered, starvingChose [2]float64
}

// runMate counts, in the base world after tick 5000, the decisions that
// had a mate among the options: the energy of those that took it and of
// those that did not, and the rise in risk paying brought; and how many
// mates were carried out per birth.
func runMate(cfg engine.Config, m engine.Map, ticks int) (mateTally, error) {
	var t mateTally
	w, err := engine.NewWorld(cfg, m)
	if err != nil {
		return t, err
	}
	cache := map[float64]engine.Survival{}
	w.SetTrace(func(b engine.Body, v engine.Valuation, a engine.Action) {
		if w.Tick() <= 5000 {
			return
		}
		mate, best := -1, math.Inf(1)
		for j, o := range v.Options {
			if o.Kind == engine.ActMate {
				mate = j
			} else {
				best = math.Min(best, v.Risk[0][j])
			}
		}
		if mate < 0 {
			return
		}
		t.offered++
		if best >= 0.5 {
			k := 0
			if len(v.Seen) > 0 {
				k = 1
			}
			t.dyingOffered[k]++
			if a.Kind == engine.ActMate {
				t.dyingChose[k]++
			}
		}
		tab := w.TruthTable()
		q := tab.Meet[m.RegionAt(int(math.Floor(b.X)), int(math.Floor(b.Y)))]
		sv, ok := cache[q]
		if !ok {
			sv = tab.NewSurvival(q, []int{cfg.Window})
			cache[q] = sv
		}
		if sv.Dead(0, int(math.Ceil(b.Energy/cfg.EnergyBurn-1e-9))-1) >= 0.5 {
			k := 0
			if len(v.Seen) > 0 {
				k = 1
			}
			t.starvingOffered[k]++
			if a.Kind == engine.ActMate {
				t.starvingChose[k]++
			}
		}
		rise := v.Risk[0][mate] - best
		if a.Kind == engine.ActMate {
			t.chose++
			t.eChose += b.Energy
			t.riskChose = append(t.riskChose, rise)
		} else {
			t.eDeclined += b.Energy
			t.riskDecl = append(t.riskDecl, rise)
		}
	})
	var births0 int64
	var mates0 int64
	for tick := 1; tick <= ticks; tick++ {
		w.Step()
		if tick == 5000 {
			births0, mates0 = w.Stats().Births, w.Stats().Actions[engine.ActMate]
		}
		if len(w.Bodies()) == 0 {
			break
		}
	}
	st := w.Stats()
	t.proposals = float64(st.Actions[engine.ActMate] - mates0)
	t.births = float64(st.Births - births0)
	return t, nil
}

// mate counts, for stage 1-3, whether mating is a comparison: the energy of
// bodies that mate against those that had the chance and did not.
func mate(m engine.Map, name string, seeds int, seed0 int64, ticks int) {
	fmt.Printf("地図 %s (%dx%d)、%d シード、%d tick、base（1-3）。tick 5000 より後の、交配の手が候補にあった決定。\n\n", name, m.Width, m.Height, seeds, ticks)
	var ts []mateTally
	for s := 0; s < seeds; s++ {
		cfg, err := variant.Config(variant.Base, seed0+int64(s))
		if err != nil {
			fail(err)
		}
		t, err := runMate(cfg, m, ticks)
		if err != nil {
			fail(err)
		}
		ts = append(ts, t)
	}
	cell := func(f func(mateTally) float64, prec int) string {
		var xs []float64
		for _, t := range ts {
			xs = append(xs, f(t))
		}
		mm, se := meanSE(xs)
		return fmt.Sprintf("%.*f ± %.*f", prec, mm, prec, se)
	}
	fmt.Printf("| 量 | 値 |\n| --- | --- |\n")
	fmt.Printf("| 交配の手を選んだ割合 | %s |\n", cell(func(t mateTally) float64 { return t.chose / t.offered }, 4))
	fmt.Printf("| 選んだ決定の体力の平均 | %s |\n", cell(func(t mateTally) float64 { return t.eChose / t.chose }, 2))
	fmt.Printf("| 選ばなかった決定の体力の平均 | %s |\n", cell(func(t mateTally) float64 { return t.eDeclined / (t.offered - t.chose) }, 2))
	fmt.Printf("| 差（選んだ − 選ばなかった） | %s |\n", cell(func(t mateTally) float64 { return t.eChose/t.chose - t.eDeclined/(t.offered-t.chose) }, 2))
	for k, n := range []string{"食料が見えない", "食料が見える"} {
		fmt.Printf("| 死にかけ（交配以外の最善の死ぬ確率 0.5 以上）・%sの決定のうち交配を選んだ割合 | %s（1シードあたり %s 決定） |\n", n,
			cell(func(t mateTally) float64 { return t.dyingChose[k] / t.dyingOffered[k] }, 4),
			cell(func(t mateTally) float64 { return t.dyingOffered[k] }, 0))
	}
	fmt.Printf("| 差（見えない − 見える） | %s |\n", cell(func(t mateTally) float64 {
		return t.dyingChose[0]/t.dyingOffered[0] - t.dyingChose[1]/t.dyingOffered[1]
	}, 4))
	for k, n := range []string{"食料が見えない", "食料が見える"} {
		fmt.Printf("| 食べなければ死にかけ（地域を動き続ける死ぬ確率 0.5 以上）・%sの決定のうち交配を選んだ割合 | %s（1シードあたり %s 決定） |\n", n,
			cell(func(t mateTally) float64 { return t.starvingChose[k] / t.starvingOffered[k] }, 4),
			cell(func(t mateTally) float64 { return t.starvingOffered[k] }, 0))
	}
	fmt.Printf("| 差（見えない − 見える） | %s |\n", cell(func(t mateTally) float64 {
		return t.starvingChose[0]/t.starvingOffered[0] - t.starvingChose[1]/t.starvingOffered[1]
	}, 4))
	fmt.Printf("| 出生 1 体あたりの交配の手 | %s |\n", cell(func(t mateTally) float64 { return t.proposals / t.births }, 2))
	for _, g := range []struct {
		name string
		f    func(mateTally) []float64
	}{{"選んだ", func(t mateTally) []float64 { return t.riskChose }}, {"選ばなかった", func(t mateTally) []float64 { return t.riskDecl }}} {
		var all []float64
		for _, t := range ts {
			all = append(all, g.f(t)...)
		}
		sort.Float64s(all)
		if len(all) == 0 {
			continue
		}
		q := func(p float64) float64 { return all[int(p*float64(len(all)-1))] }
		fmt.Printf("| %s決定の、払うことによる死ぬ確率の上がり幅（最小・中央値・最大） | %.3g・%.3g・%.3g |\n", g.name, all[0], q(0.5), all[len(all)-1])
	}
	fmt.Println()
}
