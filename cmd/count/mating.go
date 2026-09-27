package main

import (
	"fmt"
	"math"

	"github.com/toku463ne/cld_generic_dangeon/engine"
	"github.com/toku463ne/cld_generic_dangeon/variant"
)

// matingDist are the bins of distance, in tiles (the larger of the two
// axes), from an adult that can mate to the nearest one of the other sex
// that can: in sight, then farther.
var matingDist = [...][2]int{{0, 1}, {2, 5}, {6, 10}, {11, 20}, {21, 1 << 20}}

// matingLags are the spans, in ticks, a body's displacement is read over.
var matingLags = [...]int{25, 50, 100, 200}

type matingTally struct {
	// Offers after tick 5000, those that made a child, and the rest by
	// the partner's state when the offer was carried out: out of sight,
	// offering to another, not offering, offering back (yet no child: one
	// of them could not pay); not offering is split into partners that
	// could not have paid their share (poor) and the rest.
	offers, born, gone, other, none, back, poor float64
	// Samples of adults that could mate, by the distance to the nearest
	// other-sex adult that could (the last bin: none at all).
	near [len(matingDist) + 1]float64
	// Displacement summed over bodies alive across each lag, and those.
	moved, movedN [len(matingLags)]float64
}

// runMating reads why offers to mate fail, how far the nearest mate stands,
// and how far a body moves in a while - what remembering or passing on
// where a mate is could work with.
func runMating(cfg engine.Config, m engine.Map, ticks int) (matingTally, error) {
	var t matingTally
	w, err := engine.NewWorld(cfg, m)
	if err != nil {
		return t, err
	}
	type offer struct{ from, to int64 }
	var offers []offer
	var states []int // 0 gone, 1 other, 2 none, 3 back, 4 poor
	share := func(b engine.Body) float64 {
		if !cfg.FemaleBears || !cfg.Sexes {
			return cfg.BirthEnergy / 2
		}
		if b.Sex == engine.Female {
			return cfg.BirthEnergy - cfg.MateEnergy
		}
		return cfg.MateEnergy
	}
	w.SetTrace(func(b engine.Body, v engine.Valuation, a engine.Action) {
		if w.Tick() <= 5000 || a.Kind != engine.ActMate {
			return
		}
		state := 0
		for _, o := range w.Bodies() {
			if o.ID != a.Mate {
				continue
			}
			dx, dy := math.Abs(math.Floor(o.X)-math.Floor(b.X)), math.Abs(math.Floor(o.Y)-math.Floor(b.Y))
			switch {
			case math.Max(dx, dy) > float64(cfg.Sight):
				state = 0
			case o.Intent.Kind == engine.ActMate && o.Intent.Mate == b.ID:
				state = 3
			case o.Intent.Kind == engine.ActMate:
				state = 1
			case o.Energy-share(o) <= 0:
				state = 4
			default:
				state = 2
			}
		}
		offers = append(offers, offer{b.ID, a.Mate})
		states = append(states, state)
	})
	type pos struct{ x, y float64 }
	var history []map[int64]pos // samples every 25 ticks, newest last
	for tick := 1; tick <= ticks; tick++ {
		offers, states = offers[:0], states[:0]
		w.Step()
		now := w.Tick()
		bs := w.Bodies()
		if tick > 5000 {
			pairs := map[[2]int64]bool{}
			for _, b := range bs {
				if b.Born == now && b.Parents[0] >= 0 {
					pairs[[2]int64{b.Parents[0], b.Parents[1]}] = true
					pairs[[2]int64{b.Parents[1], b.Parents[0]}] = true
				}
			}
			for i, o := range offers {
				t.offers++
				switch {
				case pairs[[2]int64{o.from, o.to}]:
					t.born++
				case states[i] == 0:
					t.gone++
				case states[i] == 1:
					t.other++
				case states[i] == 2:
					t.none++
				case states[i] == 4:
					t.poor++
				default:
					t.back++
				}
			}
		}
		if tick <= 5000 || tick%25 != 0 {
			continue
		}
		// Displacement over each lag.
		cur := map[int64]pos{}
		for _, b := range bs {
			cur[b.ID] = pos{b.X, b.Y}
		}
		history = append(history, cur)
		if len(history) > 9 {
			history = history[1:]
		}
		for k, lag := range matingLags {
			back := lag / 25
			if back >= len(history) {
				continue
			}
			old := history[len(history)-1-back]
			for id, p := range cur {
				if q, ok := old[id]; ok {
					t.moved[k] += math.Max(math.Abs(p.x-q.x), math.Abs(p.y-q.y))
					t.movedN[k]++
				}
			}
		}
		if tick%50 != 0 {
			continue
		}
		// Nearest mate, among adults that can mate now.
		can := func(b engine.Body) bool {
			return now >= b.Mature && !(b.Sex == engine.Female && now < b.Rested)
		}
		for _, b := range bs {
			if !can(b) {
				continue
			}
			best := -1
			for _, o := range bs {
				if o.ID == b.ID || o.Sex == b.Sex || !can(o) {
					continue
				}
				d := int(math.Max(math.Abs(math.Floor(o.X)-math.Floor(b.X)), math.Abs(math.Floor(o.Y)-math.Floor(b.Y))))
				if best < 0 || d < best {
					best = d
				}
			}
			bin := len(matingDist)
			for k, r := range matingDist {
				if best >= r[0] && best <= r[1] {
					bin = k
				}
			}
			t.near[bin]++
		}
	}
	return t, nil
}

// mating counts, before the mating stage, why offers fail and what a body
// that remembered or heard where a mate was could work with.
func mating(m engine.Map, name string, seeds int, seed0 int64, ticks int, vname string) {
	fmt.Printf("地図 %s (%dx%d)、%d シード、条件 %s、%d tick（tick 5000 より後）。距離はタイルの縦横の大きいほう（視界は 1）。交配できる＝成人で休んでいる母でない。\n\n", name, m.Width, m.Height, seeds, vname, ticks)
	var ts []matingTally
	for s := 0; s < seeds; s++ {
		cfg, err := variant.Config(vname, seed0+int64(s))
		if err != nil {
			fail(err)
		}
		t, err := runMating(cfg, m, ticks)
		if err != nil {
			fail(err)
		}
		ts = append(ts, t)
	}
	cell := func(f func(matingTally) float64, prec int) string {
		var xs []float64
		for _, t := range ts {
			xs = append(xs, f(t))
		}
		mm, se := meanSE(xs)
		return fmt.Sprintf("%.*f ± %.*f", prec, mm, prec, se)
	}
	fmt.Printf("(a) 交配の申し出の行方\n\n| 量 | 値 |\n| --- | --- |\n")
	fmt.Printf("| 申し出（1シードあたり） | %s |\n", cell(func(t matingTally) float64 { return t.offers }, 0))
	for _, r := range []struct {
		name string
		f    func(matingTally) float64
	}{
		{"子になった", func(t matingTally) float64 { return t.born }},
		{"相手が視界にいなかった", func(t matingTally) float64 { return t.gone }},
		{"相手が別の身体に申し出ていた", func(t matingTally) float64 { return t.other }},
		{"相手が払えなかった（交配の手を持てない）", func(t matingTally) float64 { return t.poor }},
		{"相手が払えるのに交配を選んでいなかった", func(t matingTally) float64 { return t.none }},
		{"相手も申し出ていたが子にならなかった（払えない）", func(t matingTally) float64 { return t.back }},
	} {
		f := r.f
		fmt.Printf("| %s | %s |\n", r.name, cell(func(t matingTally) float64 { return f(t) / t.offers }, 4))
	}
	fmt.Printf("\n(b) 交配できる成人から、一番近い交配できる異性の成人までの距離（50 tick ごと）\n\n| 距離 | 割合 |\n| --- | --- |\n")
	for k := 0; k <= len(matingDist); k++ {
		label := "いない"
		if k < len(matingDist) {
			label = fmt.Sprintf("%d〜%d", matingDist[k][0], matingDist[k][1])
			if matingDist[k][1] > 1<<19 {
				label = fmt.Sprintf("%d〜", matingDist[k][0])
			}
		}
		fmt.Printf("| %s | %s |\n", label, cell(func(t matingTally) float64 {
			total := 0.0
			for _, x := range t.near {
				total += x
			}
			return t.near[k] / total
		}, 4))
	}
	fmt.Printf("\n(c) 身体が動く距離（縦横の大きいほう、タイル）\n\n| 間隔（tick） | 距離 |\n| --- | --- |\n")
	for k, lag := range matingLags {
		fmt.Printf("| %d | %s |\n", lag, cell(func(t matingTally) float64 { return t.moved[k] / t.movedN[k] }, 2))
	}
	fmt.Println()
}
