package main

import (
	"fmt"

	"github.com/toku463ne/cld_generic_dangeon/engine"
	"github.com/toku463ne/cld_generic_dangeon/variant"
)

// driftTally is one seed's path row at the end of a run: what the living
// believe of it and whose evidence that is.
type driftTally struct {
	bodies float64
	ratio  float64 // mean PathRatio among the living
	// own is the share of the living's path evidence they saw themselves;
	// early is the share from observers born in the first quarter of the
	// run, and earlyRate and lateRate the food per expected food in the
	// evidence of those observers and of the rest.
	own, early, earlyRate, lateRate float64
}

// runDrift runs the world and reads, at the end, the living's belief about
// the path row and where its evidence came from.
func runDrift(cfg engine.Config, m engine.Map, ticks int) (driftTally, error) {
	var t driftTally
	w, err := engine.NewWorld(cfg, m)
	if err != nil {
		return t, err
	}
	born := map[int64]int64{}
	for i := 0; i < ticks; i++ {
		for _, b := range w.Bodies() {
			if _, ok := born[b.ID]; !ok {
				born[b.ID] = b.Born
			}
		}
		w.Step()
	}
	early := int64(ticks / 4)
	var total, own, eN, eK, lN, lK float64
	for _, b := range w.Bodies() {
		t.bodies++
		t.ratio += w.Belief(b).PathRatio
		p := b.Memory.Path
		s := p.S
		if s == 0 {
			s = 1
		}
		total += p.N
		own += p.N - p.HN
		for _, h := range p.Heard {
			bt, ok := born[h.ID]
			if ok && bt < early {
				eN, eK = eN+s*h.N, eK+s*h.K
			} else {
				lN, lK = lN+s*h.N, lK+s*h.K
			}
		}
	}
	if t.bodies > 0 {
		t.ratio /= t.bodies
	}
	if total > 0 {
		t.own, t.early = own/total, eN/total
	}
	if eN > 0 {
		t.earlyRate = eK / eN
	}
	if lN > 0 {
		t.lateRate = lK / lN
	}
	return t, nil
}

// drift prints, seed by seed, what the living believe walked tiles hold
// against their region at the end, and whose evidence it rests on: whether
// passed evidence settles on one value or drifts apart between worlds.
func drift(m engine.Map, name string, seeds int, seed0 int64, ticks int, vname string) {
	fmt.Printf("地図 %s (%dx%d)、%d シード、条件 %s、%d tick の終わりに生きている身体。比は通ったタイルの推定 ÷ 地域の推定（1 より小さいほど通ったタイルを避ける）。証拠は見込んだ食料に直した数で割合を出す。早い観測者は実験の最初の 1/4 に生まれた身体。\n\n", name, m.Width, m.Height, seeds, vname, ticks)
	fmt.Printf("| シード | 個体数 | 比の平均 | 自分の証拠の割合 | 早い観測者の証拠の割合 | 早い観測者の証拠の比 | それより後の観測者の証拠の比 |\n| --- | --- | --- | --- | --- | --- | --- |\n")
	for s := 0; s < seeds; s++ {
		cfg, err := variant.Config(vname, seed0+int64(s))
		if err != nil {
			fail(err)
		}
		t, err := runDrift(cfg, m, ticks)
		if err != nil {
			fail(err)
		}
		fmt.Printf("| %d | %.0f | %.3f | %.4f | %.3f | %.3f | %.3f |\n", cfg.Seed, t.bodies, t.ratio, t.own, t.early, t.earlyRate, t.lateRate)
	}
	fmt.Println()
}
