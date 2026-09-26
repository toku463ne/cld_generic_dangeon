package main

import (
	"fmt"
	"math"

	"github.com/toku463ne/cld_generic_dangeon/engine"
	"github.com/toku463ne/cld_generic_dangeon/variant"
)

type regionsTally struct {
	decisions float64
	// Decisions whose best options would not include the body's action if
	// it knew each region's food per tile now (truth), or knew only how
	// rich each region is against its own (relative: the body's estimate
	// of the region it stands in, times the true ratios); and the loss of
	// its action by that valuation (its risk less the least).
	changedTruth, lossTruth       float64
	changedRelative, lossRelative float64
	// The same with the body's own estimates, read the same way: the floor
	// the method itself leaves (a valuation rebuilt from the rows need not
	// match the body's own to the last tie).
	changedOwn, lossOwn float64
	// Body-ticks, and those with less own evidence than RegionWeight of
	// some region other than the one the body stands in.
	bodyTicks, ignorant float64
	// Each region's food per land tile at every checkpoint, summed, and
	// the checkpoints: how unequal the regions are.
	perTile []float64
	samples float64
}

// runRegions reads, in a world where nothing passes, what knowing how rich
// the regions are would change: every decision after tick 5000 valued again
// with each region's true food per tile, and with the true ratios between
// regions set on the body's own estimate of where it stands.
func runRegions(cfg engine.Config, m engine.Map, ticks int) (regionsTally, error) {
	t := regionsTally{perTile: make([]float64, len(m.RegionFood))}
	w, err := engine.NewWorld(cfg, m)
	if err != nil {
		return t, err
	}
	nr := len(m.RegionFood)
	land := make([]float64, nr)
	for i, tt := range m.Terrain {
		if tt == engine.TerrainLand {
			land[m.Region[i]]++
		}
	}
	truth := make([]float64, nr)
	readTruth := func() {
		for r := range truth {
			truth[r] = 0
		}
		for _, f := range w.Foods() {
			truth[m.RegionAt(f.X, f.Y)]++
		}
		for r := range truth {
			truth[r] /= land[r]
		}
	}
	score := func(v engine.Valuation, j int) float64 { return v.Risk[0][j] - cfg.ChildWorth*v.Child[j] }
	// read values b's options with the region rates given (and the path
	// read as the body's ratio of them), and reports whether a is outside
	// the best, and a's loss.
	read := func(b engine.Body, a engine.Action, region []float64) (bool, float64) {
		ratio := w.Belief(b).PathRatio
		path := make([]float64, nr)
		for r := range path {
			path[r] = ratio * region[r]
		}
		v := w.ValueWith(b, region, path)
		lo, mine := math.Inf(1), math.NaN()
		for j, o := range v.Options {
			s := score(v, j)
			lo = math.Min(lo, s)
			if o == a {
				mine = s
			}
		}
		if math.IsNaN(mine) || mine == lo {
			return false, 0
		}
		return true, mine - lo
	}
	w.SetTrace(func(b engine.Body, v engine.Valuation, a engine.Action) {
		if w.Tick() <= 5000 {
			return
		}
		t.decisions++
		own := make([]float64, nr)
		landRate := w.Belief(b).Land
		for r := range own {
			var tl engine.Tally
			if r < len(b.Memory.Regions) {
				tl = w.Fresh(b.Memory.Regions[r])
			}
			own[r] = (tl.K + cfg.RegionWeight*landRate) / (tl.N + cfg.RegionWeight)
		}
		if c, l := read(b, a, own); c {
			t.changedOwn++
			t.lossOwn += l
		}
		if c, l := read(b, a, truth); c {
			t.changedTruth++
			t.lossTruth += l
		}
		here := m.RegionAt(int(b.X), int(b.Y))
		rel := make([]float64, nr)
		if truth[here] > 0 {
			own := w.Belief(b).Here
			for r := range rel {
				rel[r] = own * truth[r] / truth[here]
			}
			if c, l := read(b, a, rel); c {
				t.changedRelative++
				t.lossRelative += l
			}
		}
	})
	for tick := 1; tick <= ticks; tick++ {
		readTruth()
		w.Step()
		if tick <= 5000 {
			continue
		}
		for _, b := range w.Bodies() {
			t.bodyTicks++
			here := m.RegionAt(int(b.X), int(b.Y))
			for r := 0; r < nr; r++ {
				if r == int(here) {
					continue
				}
				n := 0.0
				if r < len(b.Memory.Regions) {
					n = w.Fresh(b.Memory.Regions[r]).N
				}
				if n < cfg.RegionWeight {
					t.ignorant++
					break
				}
			}
		}
		if tick%1000 == 0 {
			for r := range truth {
				t.perTile[r] += truth[r]
			}
			t.samples++
		}
	}
	return t, nil
}

// regions counts the room for passing how rich the regions are: in the
// base world (nothing passes), how often a body does not know some other
// region, and how many decisions, at what loss, knowing the regions' food
// per tile - or only their ratios - would change.
func regions(m engine.Map, name string, seeds int, seed0 int64, ticks int, vname string) {
	fmt.Printf("地図 %s (%dx%d)、%d シード、条件 %s、%d tick。tick 5000 より後の決定を、各地域の今の陸のタイルあたりの食料（世界の値）で値付けし直したもの（真）と、身体が今いる地域の自分の推定に地域どうしの真の比を掛けたもの（比）で、身体の手が最善から外れる割合と損（その値付けでの、手の値 − 最小）。\n\n", name, m.Width, m.Height, seeds, vname, ticks)
	var ts []regionsTally
	for s := 0; s < seeds; s++ {
		cfg, err := variant.Config(vname, seed0+int64(s))
		if err != nil {
			fail(err)
		}
		t, err := runRegions(cfg, m, ticks)
		if err != nil {
			fail(err)
		}
		ts = append(ts, t)
	}
	cell := func(f func(regionsTally) float64, prec int) string {
		var xs []float64
		for _, t := range ts {
			xs = append(xs, f(t))
		}
		mm, se := meanSE(xs)
		return fmt.Sprintf("%.*f ± %.*f", prec, mm, prec, se)
	}
	fmt.Printf("| 知っているもの | 最善から外れる決定の割合 | 決定1回あたりの損 |\n| --- | --- | --- |\n")
	fmt.Printf("| 自分の推定（対照。方法そのものが残す床） | %s | %s |\n", cell(func(t regionsTally) float64 { return t.changedOwn / t.decisions }, 4),
		cell(func(t regionsTally) float64 { return t.lossOwn / t.decisions }, 6))
	fmt.Printf("| 各地域の食料 / タイル（真） | %s | %s |\n", cell(func(t regionsTally) float64 { return t.changedTruth / t.decisions }, 4),
		cell(func(t regionsTally) float64 { return t.lossTruth / t.decisions }, 6))
	fmt.Printf("| 地域どうしの比だけ（比） | %s | %s |\n\n", cell(func(t regionsTally) float64 { return t.changedRelative / t.decisions }, 4),
		cell(func(t regionsTally) float64 { return t.lossRelative / t.decisions }, 6))
	fmt.Printf("今いる地域以外のどれかを、自分の証拠が RegionWeight 未満でしか知らない身体×tick の割合: %s\n\n", cell(func(t regionsTally) float64 { return t.ignorant / t.bodyTicks }, 4))
	fmt.Printf("| 地域 | 食料の取り分 | 陸のタイルあたりの食料（1000 tick ごとの平均） |\n| --- | --- | --- |\n")
	for r := range m.RegionFood {
		fmt.Printf("| %d | %.2f | %s |\n", r, m.RegionFood[r], cell(func(t regionsTally) float64 { return t.perTile[r] / t.samples }, 5))
	}
	fmt.Println()
}
