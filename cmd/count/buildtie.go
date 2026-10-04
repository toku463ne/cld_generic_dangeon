package main

import (
	"fmt"
	"math"
	"math/rand"

	"github.com/toku463ne/cld_generic_dangeon/engine"
	"github.com/toku463ne/cld_generic_dangeon/variant"
)

// tieCosts are the energies a build might cost; tieLives the ticks a
// built place might last; tieSides the sides of the square it might cover.
var (
	tieCosts = []float64{5, 10, 20}
	tieLives = []int64{1000, 3000, 10000}
	tieSides = []int{1, 3}
)

type buildTieTally struct {
	// after tick 5000, adults' decisions, full (a meal does not fit) [0]
	// and not [1]; of them, those where building at each of tieCosts -
	// paying the cost and waiting, with nothing yet learned of what a
	// built place is worth - ties the best option, and the chance the
	// draw among ties would have taken it.
	decisions [2]float64
	ties      [2][3]float64
	chance    [2][3]float64
	// lives begun after tick 5000 and ended before the end, those that
	// came of age, and the builds the chance adds up to over them, per
	// cost.
	lives, adultLives float64
	lifeBuilds        [3]float64
	// places drawn with that chance (the count's own random source, not
	// the world's), per cost; and for each cost, side and life, the sleeps
	// begun on them by anyone, in the dark, and by the builder in the dark;
	// and the places whose builder slept on it in the dark at least once.
	places       [3]float64
	sleeps, dark [3][2][3]float64
	own          [3][2][3]float64
	learnt       [3][2][3]float64
}

type tiePlace struct {
	tick    int64
	builder int64
	x, y    int
	// whether the builder has slept on it in the dark, per side and life
	slept [2][3]bool
}

// runBuildTie reads, at every adult's decision, whether building (at a
// cost, valued at nothing yet) would tie the best option and be drawn, and
// follows the places such draws would make: who sleeps on them.
func runBuildTie(cfg engine.Config, m engine.Map, ticks int, seed int64) (buildTieTally, error) {
	var t buildTieTally
	w, err := engine.NewWorld(cfg, m)
	if err != nil {
		return t, err
	}
	rng := rand.New(rand.NewSource(seed))
	meal := cfg.FoodEnergy
	var places [3][]*tiePlace
	builds := map[int64]*[3]float64{}
	first := map[int64]int64{}
	mature := map[int64]int64{}
	w.SetTrace(func(b engine.Body, v engine.Valuation, a engine.Action) {
		now := w.Tick()
		if now <= 5000 || b.Asleep {
			return
		}
		x, y := int(math.Floor(b.X)), int(math.Floor(b.Y))
		if a.Kind == engine.ActSleep {
			for ci := range tieCosts {
				for _, p := range places[ci] {
					for si, side := range tieSides {
						if abs(x-p.x) > side/2 || abs(y-p.y) > side/2 {
							continue
						}
						for li, life := range tieLives {
							if now-p.tick >= life {
								continue
							}
							t.sleeps[ci][si][li]++
							if w.Night() {
								t.dark[ci][si][li]++
								if b.ID == p.builder {
									t.own[ci][si][li]++
									if !p.slept[si][li] {
										p.slept[si][li] = true
										t.learnt[ci][si][li]++
									}
								}
							}
						}
					}
				}
			}
			return
		}
		if b.Mature > now || len(v.Score) == 0 {
			return
		}
		g := 1
		if b.Energy > w.MaxEnergy(b)-meal {
			g = 0
		}
		t.decisions[g]++
		best := math.Inf(1)
		for _, s := range v.Score {
			best = math.Min(best, s)
		}
		drawn, n := w.LastTie()
		region, path := w.Rates(b)
		for ci, c := range tieCosts {
			bc := b
			bc.Energy -= c
			if bc.Energy <= 0 {
				continue
			}
			bv := w.ValueWith(bc, region, path)
			wait := math.Inf(1)
			for j, o := range bv.Options {
				if o.Kind == engine.ActWait {
					wait = bv.Risk[0][j]
				}
			}
			if wait != best {
				continue
			}
			t.ties[g][ci]++
			if !drawn {
				continue
			}
			p := 1 / float64(n+1)
			t.chance[g][ci] += p
			if builds[b.ID] == nil {
				builds[b.ID] = &[3]float64{}
			}
			builds[b.ID][ci] += p
			if rng.Float64() < p {
				places[ci] = append(places[ci], &tiePlace{tick: now, builder: b.ID, x: x, y: y})
				t.places[ci]++
			}
		}
	})
	for tick := 1; tick <= ticks; tick++ {
		before := map[int64]bool{}
		for _, b := range w.Bodies() {
			before[b.ID] = true
		}
		w.Step()
		bs := w.Bodies()
		if len(bs) <= lockCollapse {
			break
		}
		alive := map[int64]engine.Body{}
		for _, b := range bs {
			alive[b.ID] = b
			if _, ok := first[b.ID]; !ok {
				first[b.ID] = w.Tick()
			}
			mature[b.ID] = b.Mature
		}
		for id := range before {
			if _, ok := alive[id]; ok || first[id] <= 5000 {
				continue
			}
			t.lives++
			if mature[id] > w.Tick() {
				continue // died a child: only adults build
			}
			t.adultLives++
			if bb := builds[id]; bb != nil {
				for ci := range tieCosts {
					t.lifeBuilds[ci] += bb[ci]
				}
			}
		}
	}
	return t, nil
}

// buildTie counts how a build valued at nothing yet would come about by
// the draw among ties, and what the places so built would be used for
// (stage 8, the user's design for the first builds).
func buildTie(m engine.Map, name string, seeds int, seed0 int64, ticks int, vname string) {
	var ts []buildTieTally
	for s := 0; s < seeds; s++ {
		cfg, err := variant.Config(vname, seed0+int64(s))
		if err != nil {
			fail(err)
		}
		t, err := runBuildTie(cfg, m, ticks, seed0+int64(s))
		if err != nil {
			fail(err)
		}
		ts = append(ts, t)
	}
	fmt.Printf("地図 %s (%dx%d)、%d シード、条件 %s、%d tick（tick 5000 より後）。成人は皆スキルを持つとして数える（持つ割合が p なら、建てる回数は p 倍）。建てる＝代金を払ってその場で待つ手。建てた場所の値打ちはまだ学んでいない（中立）。同点＝その危険が今の一番よい手の点と等しい。抽選＝その決定が向きを保たず同点の中から引いた（`LastTie`）。引かれる見込み＝1 ÷ (同点の数 + 1)。建った場所はこの数えの乱数で引く（世界の乱数は使わない）。世界は建てる前のまま進むので、建てた身体が代金を払ってその場に留まる分は入らない。\n\n", name, m.Width, m.Height, seeds, vname, ticks)
	cell := func(f func(buildTieTally) float64) string {
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
		return fmt.Sprintf("%.4f ± %.4f", mm, se)
	}
	ratio := func(a, b float64) float64 {
		if b == 0 {
			return math.NaN()
		}
		return a / b
	}
	fmt.Printf("| 成人の決定 | 決定（1シードあたり） | 同点（代金 5 / 10 / 20） | 引かれる見込み（決定あたり、代金 5 / 10 / 20） |\n| --- | --- | --- | --- |\n")
	for g, label := range []string{"満腹", "満腹でない"} {
		g := g
		row := fmt.Sprintf("| %s | %s | ", label, cell(func(t buildTieTally) float64 { return t.decisions[g] }))
		for ci := range tieCosts {
			ci := ci
			if ci > 0 {
				row += " / "
			}
			row += cell(func(t buildTieTally) float64 { return ratio(t.ties[g][ci], t.decisions[g]) })
		}
		row += " | "
		for ci := range tieCosts {
			ci := ci
			if ci > 0 {
				row += " / "
			}
			row += cell(func(t buildTieTally) float64 { return ratio(t.chance[g][ci], t.decisions[g]) })
		}
		fmt.Println(row + " |")
	}
	fmt.Printf("\n| 代金 | 一生で建てる回数（成人まで生きた一生あたり、見込み） | 建った場所（1シードあたり） |\n| --- | --- | --- |\n")
	for ci, c := range tieCosts {
		ci := ci
		fmt.Printf("| %.0f | %s | %s |\n", c,
			cell(func(t buildTieTally) float64 { return ratio(t.lifeBuilds[ci], t.adultLives) }),
			cell(func(t buildTieTally) float64 { return t.places[ci] }))
	}
	fmt.Printf("\n建った場所1つあたり、もつ間に始まった眠り（代金ごと。広さ 1 / 3 四方、もつ tick ごと）。\n\n| 代金 | 広さ | もつ tick | 誰かの眠り | うち暗い | 建てた本人が暗いとき | 本人が暗いとき一度でも眠った場所の割合 |\n| --- | --- | --- | --- | --- | --- | --- |\n")
	for ci, c := range tieCosts {
		for si, side := range tieSides {
			for li, life := range tieLives {
				ci, si, li := ci, si, li
				per := func(f func(buildTieTally) float64) string {
					return cell(func(t buildTieTally) float64 { return ratio(f(t), t.places[ci]) })
				}
				fmt.Printf("| %.0f | %d | %d | %s | %s | %s | %s |\n", c, side, life,
					per(func(t buildTieTally) float64 { return t.sleeps[ci][si][li] }),
					per(func(t buildTieTally) float64 { return t.dark[ci][si][li] }),
					per(func(t buildTieTally) float64 { return t.own[ci][si][li] }),
					per(func(t buildTieTally) float64 { return t.learnt[ci][si][li] }))
			}
		}
	}
	fmt.Println()
}
