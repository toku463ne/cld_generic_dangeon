package main

import (
	"fmt"
	"math"

	"github.com/toku463ne/cld_generic_dangeon/engine"
	"github.com/toku463ne/cld_generic_dangeon/variant"
)

// learnPriors are the chances per tick of meeting food a body might be born
// expecting everywhere, with nothing learned: from none to about three
// times what the maps hold once they are eaten down.
var learnPriors = []float64{0, 0.0005, 0.002, 0.008}

// learnAges are the ages, in ticks, at which the learning delay is read.
var learnAges = []int64{250, 500, 1000, 2000}

// learnRecent are how long ago, in ticks, a body left a tile, for the food
// it finds on walking onto it again - the way it came.
var learnRecent = []int64{50, 200, 1000}

type learnTally struct {
	decisions float64
	differ    [4]float64 // decisions a prior-only valuation takes elsewhere
	// Per life (bodies born after tick 5000 that died): tiles entered, the
	// regions among them, mates asked and children had.
	tiles, regions, asked, kids, lives float64
	// At each of learnAges, among bodies alive at that age: how many have
	// entered enough tiles of the region they are in for its rate to be
	// known to within 30% (binomially), and how many were read.
	known, read [4]float64
	// Tiles entered that the body had left within learnRecent ticks, and
	// how many held food; against all tiles entered.
	recentIn, recentFood [3]float64
	allIn, allFood       float64
}

type learnBody struct {
	n        map[engine.RegionID]float64 // tiles entered per region
	walked   map[int]int64               // tile -> last tick it was on it
	tile     int
	asked    float64
	kids     float64
	born     int64
	readAges int
}

// runLearn follows the base world and keeps, beside it, what each body
// could have learned: never feeding it back.
func runLearn(cfg engine.Config, m engine.Map, ticks int) (learnTally, error) {
	var t learnTally
	w, err := engine.NewWorld(cfg, m)
	if err != nil {
		return t, err
	}
	land := make([]float64, len(m.RegionFood))
	for i, tr := range m.Terrain {
		if tr == engine.TerrainLand {
			land[m.Region[i]]++
		}
	}
	// Survival tables for the priors, by prior (the config's build).
	tab := w.TruthTable()
	priorSurv := make([][]engine.Survival, len(learnPriors))
	for i, p := range learnPriors {
		s := tab.NewSurvival(p, []int{cfg.Window})
		priorSurv[i] = make([]engine.Survival, len(tab.Meet))
		for r := range priorSurv[i] {
			priorSurv[i][r] = s
		}
	}
	trueSurv := map[float64]engine.Survival{}
	bodies := map[int64]*learnBody{}
	food := map[int]bool{}
	w.SetTrace(func(b engine.Body, v engine.Valuation, a engine.Action) {
		if a.Kind == engine.ActMate {
			if lb := bodies[b.ID]; lb != nil {
				lb.asked++
			}
		}
		if w.Tick() <= 5000 {
			return
		}
		t.decisions++
		// The true choice, read as Value reads it (the config's build), and
		// the choices under each prior.
		tt := w.TruthTable()
		surv := make([]engine.Survival, len(tt.Meet))
		for r, q := range tt.Meet {
			s, ok := trueSurv[q]
			if !ok {
				s = tt.NewSurvival(q, []int{cfg.Window})
				trueSurv[q] = s
			}
			surv[r] = s
		}
		best := func(v engine.Valuation) map[engine.Action]bool {
			lo := math.Inf(1)
			sc := make([]float64, len(v.Options))
			for j := range v.Options {
				sc[j] = v.Risk[0][j] - cfg.ChildWorth*v.Child[j]
				lo = math.Min(lo, sc[j])
			}
			set := map[engine.Action]bool{}
			for j, o := range v.Options {
				if sc[j] == lo {
					set[o] = true
				}
			}
			return set
		}
		truth := best(w.Value(tt, surv, b))
		for i := range learnPriors {
			prior := best(w.Value(tt, priorSurv[i], b))
			shared := false
			for o := range truth {
				shared = shared || prior[o]
			}
			if !shared {
				t.differ[i]++
			}
		}
	})
	for tick := 1; tick <= ticks; tick++ {
		clear(food)
		for _, f := range w.Foods() {
			food[f.Y*m.Width+f.X] = true
		}
		before := w.Bodies()
		w.Step()
		now := w.Tick()
		alive := map[int64]engine.Body{}
		for _, b := range w.Bodies() {
			alive[b.ID] = b
			lb := bodies[b.ID]
			if lb == nil {
				lb = &learnBody{n: map[engine.RegionID]float64{}, walked: map[int]int64{}, tile: -1, born: b.Born}
				bodies[b.ID] = lb
				for _, p := range b.Parents {
					if pb := bodies[p]; pb != nil {
						pb.kids++
					}
				}
			}
			tx, ty := int(b.X), int(b.Y)
			tile := ty*m.Width + tx
			if tile != lb.tile && b.Goal >= 0 {
				// A step of a walk to food in sight: not evidence of what
				// keeping on the move meets.
				if lb.tile >= 0 {
					lb.walked[lb.tile] = now
				}
				lb.tile = tile
			} else if tile != lb.tile {
				// Entered a tile keeping on the move: an observation of its
				// region's rate, and a return to a tile it may have left.
				r := m.RegionAt(tx, ty)
				lb.n[r]++
				if now > 5000 {
					t.allIn++
					if food[tile] {
						t.allFood++
					}
					if when, ok := lb.walked[tile]; ok {
						for i, k := range learnRecent {
							if now-when <= k {
								t.recentIn[i]++
								if food[tile] {
									t.recentFood[i]++
								}
							}
						}
					}
				}
				if lb.tile >= 0 {
					lb.walked[lb.tile] = now
				}
				lb.tile = tile
			}
			// The learning delay, read once at each age.
			for lb.readAges < len(learnAges) && now-lb.born >= learnAges[lb.readAges] {
				if lb.born > 5000 {
					r := m.RegionAt(tx, ty)
					q := float64(0)
					for _, f := range w.Foods() {
						if m.RegionAt(f.X, f.Y) == r {
							q++
						}
					}
					p := q / land[r]
					if p > 0 {
						t.read[lb.readAges]++
						// Binomial relative error below 30%.
						if n := lb.n[r]; n > 0 && math.Sqrt((1-p)/(n*p)) < 0.3 {
							t.known[lb.readAges]++
						}
					}
				}
				lb.readAges++
			}
		}
		for _, b := range before {
			if _, ok := alive[b.ID]; ok {
				continue
			}
			if lb := bodies[b.ID]; lb != nil {
				if lb.born > 5000 {
					n := 0.0
					for _, x := range lb.n {
						n += x
					}
					t.tiles += n
					t.regions += float64(len(lb.n))
					t.asked += lb.asked
					t.kids += lb.kids
					t.lives++
				}
				delete(bodies, b.ID)
			}
		}
		if len(w.Bodies()) == 0 {
			break
		}
	}
	return t, nil
}

// learn counts, for stage 1-6, whether learning from experience has room
// to change decisions, how much evidence a life gathers, how late a body
// learns its region, and how much poorer the tiles it saw lately are.
func learn(m engine.Map, name string, seeds int, seed0 int64, ticks int) {
	fmt.Printf("地図 %s (%dx%d)、%d シード、%d tick、base（跳ね返しなし）。決定は tick 5000 より後、一生は tick 5000 より後に生まれて死んだ身体。\n\n", name, m.Width, m.Height, seeds, ticks)
	var ts []learnTally
	for s := 0; s < seeds; s++ {
		cfg, err := variant.Config(variant.Base, seed0+int64(s))
		if err != nil {
			fail(err)
		}
		t, err := runLearn(cfg, m, ticks)
		if err != nil {
			fail(err)
		}
		ts = append(ts, t)
	}
	cell := func(f func(learnTally) float64, prec int) string {
		var xs []float64
		for _, t := range ts {
			xs = append(xs, f(t))
		}
		mm, se := meanSE(xs)
		return fmt.Sprintf("%.*f ± %.*f", prec, mm, prec, se)
	}
	fmt.Printf("(A) 生まれつきの見込みだけで値付けしたとき、真の表と最善の手が1つも重ならない決定の割合\n\n| 生まれつきの見込み（食料に出会う確率 / tick） | 割合 |\n| --- | --- |\n")
	for i, p := range learnPriors {
		fmt.Printf("| %g | %s |\n", p, cell(func(t learnTally) float64 { return t.differ[i] / t.decisions }, 4))
	}
	fmt.Printf("\n(B) 一生で集める証拠（1体あたり）\n\n| 量 | 値 |\n| --- | --- |\n")
	fmt.Printf("| 入ったタイル（地域の食料の証拠） | %s |\n", cell(func(t learnTally) float64 { return t.tiles / t.lives }, 1))
	fmt.Printf("| 入った地域の数 | %s |\n", cell(func(t learnTally) float64 { return t.regions / t.lives }, 2))
	fmt.Printf("| 交配の申し出 | %s |\n", cell(func(t learnTally) float64 { return t.asked / t.lives }, 2))
	fmt.Printf("| 子の数 | %s |\n", cell(func(t learnTally) float64 { return t.kids / t.lives }, 2))
	fmt.Printf("| 子 ÷ 申し出（交配の行の真の値） | %s |\n", cell(func(t learnTally) float64 { return t.kids / t.asked }, 4))
	fmt.Printf("\n(C) 自力で学ぶ遅れ: その年齢で、今いる地域に入ったタイルの数が、その地域の食料の割合を相対誤差 30%% 以内で知るのに足りている身体の割合\n\n| 年齢 | 割合 |\n| --- | --- |\n")
	for i, a := range learnAges {
		fmt.Printf("| %d | %s |\n", a, cell(func(t learnTally) float64 {
			if t.read[i] == 0 {
				return math.NaN()
			}
			return t.known[i] / t.read[i]
		}, 4))
	}
	fmt.Printf("\n(D) 自分が通ってきたタイルにもう一度入ったとき、食料がある割合（全体と比べる）\n\n| 離れてから | 入った回数（全体に対する割合） | 食料がある割合 | 全体の食料がある割合 |\n| --- | --- | --- | --- |\n")
	for i, k := range learnRecent {
		fmt.Printf("| %d tick 以内 | %s | %s | %s |\n", k,
			cell(func(t learnTally) float64 { return t.recentIn[i] / t.allIn }, 4),
			cell(func(t learnTally) float64 { return t.recentFood[i] / t.recentIn[i] }, 5),
			cell(func(t learnTally) float64 { return t.allFood / t.allIn }, 5))
	}
	fmt.Println()
}
