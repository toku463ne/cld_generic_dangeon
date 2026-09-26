package main

import (
	"fmt"
	"math"

	"github.com/toku463ne/cld_generic_dangeon/engine"
	"github.com/toku463ne/cld_generic_dangeon/variant"
)

type genTally struct {
	meanGen   float64 // mean generation of the living at the end
	genTime   float64 // mean age of the parents at a birth
	selection float64 // cov(share, lifetime births) / mean lifetime births
	shareSD   float64 // standard deviation of the share among bodies born
	births    float64
}

// runGenerations reads, in the base world, the generation of each body (the
// first are 0, a child one more than its older parent), the age of parents
// when they have a child, and - over bodies born after tick 10000 that died
// before the end - the selection differential on the share of the budget
// for speed.
func runGenerations(cfg engine.Config, m engine.Map, ticks int) (genTally, error) {
	var t genTally
	w, err := engine.NewWorld(cfg, m)
	if err != nil {
		return t, err
	}
	gen := map[int64]int{}
	known := map[int64]engine.Body{}
	kids := map[int64]int{}
	for _, b := range w.Bodies() {
		gen[b.ID] = 0
		known[b.ID] = b
	}
	var shares, lives []float64
	parentAge, parents := 0.0, 0.0
	for tick := 1; tick <= ticks; tick++ {
		w.Step()
		now := w.Tick()
		alive := map[int64]bool{}
		for _, b := range w.Bodies() {
			alive[b.ID] = true
			if _, ok := known[b.ID]; ok {
				continue
			}
			known[b.ID] = b
			g := 0
			for _, p := range b.Parents {
				g = max(g, gen[p]+1)
				kids[p]++
				if pb, ok := known[p]; ok {
					parentAge += float64(now - pb.Born)
					parents++
				}
			}
			gen[b.ID] = g
			t.births++
		}
		for id, b := range known {
			if alive[id] {
				continue
			}
			if b.Born > 10000 {
				shares = append(shares, b.Share)
				lives = append(lives, float64(kids[id]))
			}
			delete(known, id)
			delete(kids, id)
			delete(gen, id)
		}
	}
	n := 0.0
	for _, b := range w.Bodies() {
		t.meanGen += float64(gen[b.ID])
		n++
	}
	t.meanGen /= n
	t.genTime = parentAge / parents
	ms, ml := 0.0, 0.0
	for i := range shares {
		ms += shares[i]
		ml += lives[i]
	}
	ms /= float64(len(shares))
	ml /= float64(len(lives))
	cov, vs := 0.0, 0.0
	for i := range shares {
		cov += (shares[i] - ms) * (lives[i] - ml)
		vs += (shares[i] - ms) * (shares[i] - ms)
	}
	cov /= float64(len(shares))
	t.selection = cov / ml
	t.shareSD = math.Sqrt(vs / float64(len(shares)))
	return t, nil
}

// generations counts, for stage 1-5, whether the run holds enough
// generations for selection on the share to show once it is inherited.
func generations(m engine.Map, name string, seeds int, seed0 int64, ticks int) {
	fmt.Printf("地図 %s (%dx%d)、%d シード、%d tick、base（1-4 ＋ 衝突）。\n\n", name, m.Width, m.Height, seeds, ticks)
	var ts []genTally
	for s := 0; s < seeds; s++ {
		cfg, err := variant.Config(variant.Base, seed0+int64(s))
		if err != nil {
			fail(err)
		}
		t, err := runGenerations(cfg, m, ticks)
		if err != nil {
			fail(err)
		}
		ts = append(ts, t)
	}
	cell := func(f func(genTally) float64, prec int) string {
		var xs []float64
		for _, t := range ts {
			xs = append(xs, f(t))
		}
		mm, se := meanSE(xs)
		return fmt.Sprintf("%.*f ± %.*f", prec, mm, prec, se)
	}
	fmt.Printf("| 量 | 値 |\n| --- | --- |\n")
	fmt.Printf("| 終わりに生きている身体の世代の平均 | %s |\n", cell(func(t genTally) float64 { return t.meanGen }, 1))
	fmt.Printf("| 子が生まれたときの親の年齢の平均（世代の長さ、tick） | %s |\n", cell(func(t genTally) float64 { return t.genTime }, 0))
	fmt.Printf("| 速さへの配分の選択差（共分散(配分, 生涯出生数) ÷ 生涯出生数の平均） | %s |\n", cell(func(t genTally) float64 { return t.selection }, 5))
	fmt.Printf("| 生まれた身体の配分の標準偏差 | %s |\n", cell(func(t genTally) float64 { return t.shareSD }, 4))
	fmt.Printf("| 出生（1シードあたり） | %s |\n\n", cell(func(t genTally) float64 { return t.births }, 0))
}
