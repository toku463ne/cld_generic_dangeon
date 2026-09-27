package main

import (
	"fmt"
	"math"

	"github.com/toku463ne/cld_generic_dangeon/engine"
	"github.com/toku463ne/cld_generic_dangeon/variant"
)

// requestRanges are the ranges, in tiles, a request could reach.
var requestRanges = [...]int{1, 3, 5, 8, 12}

type requestTally struct {
	// Samples of able adults by sex (0 female, 1 male), and those with at
	// least one able adult of the other sex within each range.
	able   [2]float64
	within [2][len(requestRanges)]float64
	// Spells in which a female stayed able: how many ended, and their
	// lengths summed and listed; how many ended in a birth.
	spells, spellTicks, spellsBorn float64
	lengths                        []float64
}

// runRequest reads what a request to mate, sent by a body that can pay,
// could reach: how many bodies that can mate stand within a range of one
// that can, and how long a female stays able to.
func runRequest(cfg engine.Config, m engine.Map, ticks int) (requestTally, error) {
	var t requestTally
	w, err := engine.NewWorld(cfg, m)
	if err != nil {
		return t, err
	}
	share := func(b engine.Body) float64 {
		if !cfg.FemaleBears || !cfg.Sexes {
			return cfg.BirthEnergy / 2
		}
		if b.Sex == engine.Female {
			return cfg.BirthEnergy - cfg.MateEnergy
		}
		return cfg.MateEnergy
	}
	able := func(b engine.Body, now int64) bool {
		return now >= b.Mature && !(b.Sex == engine.Female && now < b.Rested) && b.Energy-share(b) > 0
	}
	since := map[int64]int64{} // female -> tick her spell of being able began
	for tick := 1; tick <= ticks; tick++ {
		w.Step()
		now := w.Tick()
		bs := w.Bodies()
		alive := map[int64]engine.Body{}
		mothers := map[int64]bool{}
		for _, b := range bs {
			alive[b.ID] = b
			if b.Born == now && b.Parents[0] >= 0 {
				mothers[b.Parents[0]], mothers[b.Parents[1]] = true, true
			}
		}
		// Spells: a female able now starts one if she has none; one who is
		// not ends hers (a birth this tick, or no longer able, or dead).
		for id, start := range since {
			b, ok := alive[id]
			if ok && able(b, now) {
				continue
			}
			if tick > 5000 {
				d := float64(now - start)
				t.spells++
				t.spellTicks += d
				t.lengths = append(t.lengths, d)
				if mothers[id] {
					t.spellsBorn++
				}
			}
			delete(since, id)
		}
		for _, b := range bs {
			if b.Sex == engine.Female && able(b, now) {
				if _, ok := since[b.ID]; !ok {
					since[b.ID] = now
				}
			}
		}
		if tick <= 5000 || tick%50 != 0 {
			continue
		}
		for _, b := range bs {
			if !able(b, now) {
				continue
			}
			k := 0
			if b.Sex == engine.Male {
				k = 1
			}
			t.able[k]++
			nearest := math.Inf(1)
			for _, o := range bs {
				if o.Sex == b.Sex || !able(o, now) {
					continue
				}
				d := math.Max(math.Abs(math.Floor(o.X)-math.Floor(b.X)), math.Abs(math.Floor(o.Y)-math.Floor(b.Y)))
				nearest = math.Min(nearest, d)
			}
			for r, rng := range requestRanges {
				if nearest <= float64(rng) {
					t.within[k][r]++
				}
			}
		}
	}
	return t, nil
}

// request counts, before requests to mate, what one sent by a body that can
// pay could reach and how long a female stays able to answer.
func request(m engine.Map, name string, seeds int, seed0 int64, ticks int, vname string) {
	fmt.Printf("地図 %s (%dx%d)、%d シード、条件 %s、%d tick（tick 5000 より後）。交配できる＝成人で、休んでいる母でなく、自分の分（雌 %v、雄 %v）を払える。距離はタイルの縦横の大きいほう（視界は 1）。\n\n", name, m.Width, m.Height, seeds, vname, ticks, 45, 5)
	var ts []requestTally
	var lengths []float64
	for s := 0; s < seeds; s++ {
		cfg, err := variant.Config(vname, seed0+int64(s))
		if err != nil {
			fail(err)
		}
		t, err := runRequest(cfg, m, ticks)
		if err != nil {
			fail(err)
		}
		ts = append(ts, t)
		lengths = append(lengths, t.lengths...)
	}
	cell := func(f func(requestTally) float64, prec int) string {
		var xs []float64
		for _, t := range ts {
			xs = append(xs, f(t))
		}
		mm, se := meanSE(xs)
		return fmt.Sprintf("%.*f ± %.*f", prec, mm, prec, se)
	}
	fmt.Printf("(a) 交配できる身体のうち、その範囲に交配できる異性がいる割合（50 tick ごと）\n\n| 範囲（タイル） | 女から見て | 男から見て |\n| --- | --- | --- |\n")
	for r, rng := range requestRanges {
		fmt.Printf("| %d | %s | %s |\n", rng,
			cell(func(t requestTally) float64 { return t.within[0][r] / t.able[0] }, 4),
			cell(func(t requestTally) float64 { return t.within[1][r] / t.able[1] }, 4))
	}
	fmt.Printf("| （交配できる身体の数、1シードあたりの標本） | %s | %s |\n\n", cell(func(t requestTally) float64 { return t.able[0] }, 0), cell(func(t requestTally) float64 { return t.able[1] }, 0))
	fmt.Printf("(b) 女が交配できる状態が続いた長さ\n\n| 量 | 値 |\n| --- | --- |\n")
	fmt.Printf("| 終わった期間（1シードあたり） | %s |\n", cell(func(t requestTally) float64 { return t.spells }, 0))
	fmt.Printf("| そのうち出産で終わった割合 | %s |\n", cell(func(t requestTally) float64 { return t.spellsBorn / t.spells }, 4))
	fmt.Printf("| 長さの分位（全シード、25%%・50%%・75%%・95%%） | %s |\n\n", quantiles(lengths, 0.25, 0.5, 0.75, 0.95))
}
