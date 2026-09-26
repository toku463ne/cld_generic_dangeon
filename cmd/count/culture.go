package main

import (
	"fmt"
	"math"

	"github.com/toku463ne/cld_generic_dangeon/engine"
	"github.com/toku463ne/cld_generic_dangeon/variant"
)

type cultureTally struct {
	decisions float64
	// Decisions whose best options would not include the body's action if
	// it knew what all the living together had seen: of the regions, of
	// the path, of both. And the truth table's risk of the body's action
	// less that of the best informed action (the cost of not knowing).
	changedRegion, changedPath, changedBoth float64
	excessRegion, excessPath, excessBoth    float64
	// The same loss judged by the informed valuation itself (its risk of
	// the body's action less its least): the truth table reads no path, so
	// it cannot price the path row.
	informedRegion, informedPath, informedBoth float64
	// Body-ticks, and those spent with less own evidence than a row's
	// weight: the estimate still more its parent's than its own.
	bodyTicks, ignorantRegion, ignorantPath float64
	// Per life (bodies born after tick 5000 that died): bodies met in sight
	// (the nearby route's chances), children had (the birth route's).
	lives, met, kids float64
	// Novelty summed over contacts and over births: for each row, the
	// sender's own evidence over it and the receiver's.
	metNovelRegion, metNovelPath, metN     float64
	birthNovelRegion, birthNovelPath, birN float64
}

// novelty is what a sender's own evidence n would add to a receiver who has
// m: n / (n + m).
func novelty(n, m float64) float64 {
	if n+m == 0 {
		return 0
	}
	return n / (n + m)
}

// own is a tally's own evidence (the heard part left out).
func own(t engine.Tally) float64 {
	n := t.N
	for _, c := range t.Heard {
		n -= c.N
	}
	return n
}

func regionN(b engine.Body, r int) float64 {
	if r < len(b.Memory.Regions) {
		return b.Memory.Regions[r].N
	}
	return 0
}

func regionOwn(b engine.Body, r int) float64 {
	if r < len(b.Memory.Regions) {
		return own(b.Memory.Regions[r])
	}
	return 0
}

// runCulture reads, in the base world after tick 5000, how much a body
// would gain from what others have seen, and how often each route could
// carry it.
func runCulture(cfg engine.Config, m engine.Map, ticks int) (cultureTally, error) {
	var t cultureTally
	w, err := engine.NewWorld(cfg, m)
	if err != nil {
		return t, err
	}
	nr := len(m.RegionFood)
	pooledRegion := make([]float64, nr)
	pooledPath := make([]float64, nr)
	cache := map[float64]engine.Survival{}
	best := func(v engine.Valuation) map[engine.Action]bool {
		lo := math.Inf(1)
		for j := range v.Options {
			lo = math.Min(lo, v.Risk[0][j]-cfg.ChildWorth*v.Child[j])
		}
		set := map[engine.Action]bool{}
		for j, o := range v.Options {
			if v.Risk[0][j]-cfg.ChildWorth*v.Child[j] == lo {
				set[o] = true
			}
		}
		return set
	}
	w.SetTrace(func(b engine.Body, v engine.Valuation, a engine.Action) {
		if w.Tick() <= 5000 {
			return
		}
		t.decisions++
		var ownRates [2][]float64
		ownRates[0] = make([]float64, nr)
		ownRates[1] = make([]float64, nr)
		land := w.Belief(b).Land
		for r := 0; r < nr; r++ {
			var tl engine.Tally
			if r < len(b.Memory.Regions) {
				tl = b.Memory.Regions[r]
			}
			p := (tl.K + cfg.RegionWeight*land) / (tl.N + cfg.RegionWeight)
			ownRates[0][r] = p
			ownRates[1][r] = (b.Memory.Path.K + cfg.PathWeight*p) / (b.Memory.Path.N + cfg.PathWeight)
		}
		// The truth table's risk of each option, the config's build.
		tt := w.TruthTable()
		surv := make([]engine.Survival, len(tt.Meet))
		for r, q := range tt.Meet {
			s, ok := cache[q]
			if !ok {
				s = tt.NewSurvival(q, []int{cfg.Window})
				cache[q] = s
			}
			surv[r] = s
		}
		tv := w.Value(tt, surv, b)
		truthRisk := map[engine.Action]float64{}
		for j, o := range tv.Options {
			truthRisk[o] = tv.Risk[0][j]
		}
		read := func(region, path []float64) (bool, float64, float64) {
			cv := w.ValueWith(b, region, path)
			set := best(cv)
			if set[a] {
				return false, 0, 0
			}
			informed, lo2 := math.NaN(), math.Inf(1)
			for j, o := range cv.Options {
				sc := cv.Risk[0][j] - cfg.ChildWorth*cv.Child[j]
				lo2 = math.Min(lo2, sc)
				if o == a {
					informed = sc
				}
			}
			informed -= lo2
			if math.IsNaN(informed) {
				informed = 0
			}
			lo := math.Inf(1)
			for o := range set {
				if r, ok := truthRisk[o]; ok {
					lo = math.Min(lo, r)
				}
			}
			ra, ok := truthRisk[a]
			if !ok || math.IsInf(lo, 1) {
				return true, 0, informed
			}
			return true, ra - lo, informed
		}
		if c, e, i := read(pooledRegion, ownRates[1]); c {
			t.changedRegion++
			t.excessRegion += e
			t.informedRegion += i
		}
		if c, e, i := read(ownRates[0], pooledPath); c {
			t.changedPath++
			t.excessPath += e
			t.informedPath += i
		}
		if c, e, i := read(pooledRegion, pooledPath); c {
			t.changedBoth++
			t.excessBoth += e
			t.informedBoth += i
		}
	})
	type life struct {
		met  map[int64]bool
		born int64
	}
	lives := map[int64]*life{}
	grid := make([][]int, m.Width*m.Height)
	for tick := 1; tick <= ticks; tick++ {
		bs := w.Bodies()
		// What all the living have seen themselves, pooled.
		var pk, pn, rk, rn [16]float64
		for _, b := range bs {
			for r := 0; r < nr && r < len(b.Memory.Regions); r++ {
				rk[r] += b.Memory.Regions[r].K
				rn[r] += b.Memory.Regions[r].N
			}
			region := m.RegionAt(int(b.X), int(b.Y))
			pk[region] += b.Memory.Path.K
			pn[region] += b.Memory.Path.N
		}
		allK, allN := 0.0, 0.0
		for r := 0; r < nr; r++ {
			allK += pk[r]
			allN += pn[r]
		}
		for r := 0; r < nr; r++ {
			if rn[r] > 0 {
				pooledRegion[r] = rk[r] / rn[r]
			}
			if allN > 0 {
				// The path row is one row, not one per region.
				pooledPath[r] = allK / allN
			}
		}
		w.Step()
		now := w.Tick()
		bs = w.Bodies()
		for i := range grid {
			grid[i] = grid[i][:0]
		}
		byID := map[int64]int{}
		for i, b := range bs {
			byID[b.ID] = i
			grid[int(b.Y)*m.Width+int(b.X)] = append(grid[int(b.Y)*m.Width+int(b.X)], i)
			if lives[b.ID] == nil {
				lives[b.ID] = &life{met: map[int64]bool{}, born: b.Born}
				if now > 5000 && b.Parents[0] >= 0 {
					// A birth: what each parent's own evidence would add to
					// a child who has none.
					for _, pid := range b.Parents {
						j, ok := byID[pid]
						if !ok {
							continue
						}
						p := bs[j]
						for r := 0; r < nr; r++ {
							if regionOwn(p, r) > 0 {
								t.birthNovelRegion += 1.0 / float64(nr)
							}
						}
						if own(p.Memory.Path) > 0 {
							t.birthNovelPath++
						}
						t.birN++
					}
				}
			}
		}
		if now <= 5000 {
			continue
		}
		for _, b := range bs {
			t.bodyTicks++
			r := int(m.RegionAt(int(b.X), int(b.Y)))
			if regionN(b, r) < cfg.RegionWeight {
				t.ignorantRegion++
			}
			if b.Memory.Path.N < cfg.PathWeight {
				t.ignorantPath++
			}
			// Bodies coming into sight for the first time: a contact.
			lf := lives[b.ID]
			tx, ty := int(b.X), int(b.Y)
			for y := ty - cfg.Sight; y <= ty+cfg.Sight; y++ {
				for x := tx - cfg.Sight; x <= tx+cfg.Sight; x++ {
					if !m.InBounds(x, y) {
						continue
					}
					for _, j := range grid[y*m.Width+x] {
						o := bs[j]
						if o.ID == b.ID || lf.met[o.ID] {
							continue
						}
						lf.met[o.ID] = true
						nov := 0.0
						for rr := 0; rr < nr; rr++ {
							nov += novelty(regionOwn(o, rr), regionN(b, rr)) / float64(nr)
						}
						t.metNovelRegion += nov
						t.metNovelPath += novelty(own(o.Memory.Path), b.Memory.Path.N)
						t.metN++
					}
				}
			}
		}
		for id, lf := range lives {
			if _, ok := byID[id]; ok {
				continue
			}
			if lf.born > 5000 {
				t.lives++
				t.met += float64(len(lf.met))
			}
			delete(lives, id)
		}
		if len(bs) == 0 {
			break
		}
	}
	t.kids = t.birN / 2 // each birth read from both parents
	return t, nil
}

// culture counts, for stage 2-1, whether passing evidence between bodies
// has room to matter, and which route has the more of it.
func culture(m engine.Map, name string, seeds int, seed0 int64, ticks int) {
	fmt.Printf("地図 %s (%dx%d)、%d シード、%d tick、base（1-6）。tick 5000 より後。「皆の知識」は、生きている身体が自分で見た証拠を全部まとめた割合（伝承で届きうる上限）。\n\n", name, m.Width, m.Height, seeds, ticks)
	var ts []cultureTally
	for s := 0; s < seeds; s++ {
		cfg, err := variant.Config(variant.Base, seed0+int64(s))
		if err != nil {
			fail(err)
		}
		t, err := runCulture(cfg, m, ticks)
		if err != nil {
			fail(err)
		}
		ts = append(ts, t)
	}
	cell := func(f func(cultureTally) float64, prec int) string {
		var xs []float64
		for _, t := range ts {
			xs = append(xs, f(t))
		}
		mm, se := meanSE(xs)
		return fmt.Sprintf("%.*f ± %.*f", prec, mm, prec, se)
	}
	fmt.Printf("(1) 知らずに過ごす割合と、知らないことの代価（決定1回あたりの死ぬ確率の損。真の表で読んだものと、皆の知識の値付けで読んだもの）\n\n| 行 | 知らずに過ごす割合（証拠が重み未満の身体×tick） | 皆の知識なら最善から外れる決定 | 損（真の表） | 損（皆の知識の値付け） |\n| --- | --- | --- | --- | --- |\n")
	row := func(name string, ign func(cultureTally) float64, ch, ex, inf func(cultureTally) float64) {
		ic := "—"
		if ign != nil {
			ic = cell(ign, 4)
		}
		fmt.Printf("| %s | %s | %s | %s | %s |\n", name, ic,
			cell(func(t cultureTally) float64 { return ch(t) / t.decisions }, 4),
			cell(func(t cultureTally) float64 { return ex(t) / t.decisions }, 6),
			cell(func(t cultureTally) float64 { return inf(t) / t.decisions }, 6))
	}
	row("地域", func(t cultureTally) float64 { return t.ignorantRegion / t.bodyTicks },
		func(t cultureTally) float64 { return t.changedRegion }, func(t cultureTally) float64 { return t.excessRegion }, func(t cultureTally) float64 { return t.informedRegion })
	row("通ったタイル", func(t cultureTally) float64 { return t.ignorantPath / t.bodyTicks },
		func(t cultureTally) float64 { return t.changedPath }, func(t cultureTally) float64 { return t.excessPath }, func(t cultureTally) float64 { return t.informedPath })
	row("両方", nil,
		func(t cultureTally) float64 { return t.changedBoth }, func(t cultureTally) float64 { return t.excessBoth }, func(t cultureTally) float64 { return t.informedBoth })
	fmt.Printf("\n(3) 経路ごとの機会と新しさ\n\n| 量 | 親子（生まれたとき） | 近くの者（視界に初めて入ったとき） |\n| --- | --- | --- |\n")
	fmt.Printf("| 一生あたりの機会 | 親 2（子として1回） | %s |\n", cell(func(t cultureTally) float64 { return t.met / t.lives }, 1))
	fmt.Printf("| 新しさ: 地域の行 | %s | %s |\n",
		cell(func(t cultureTally) float64 { return t.birthNovelRegion / t.birN }, 4),
		cell(func(t cultureTally) float64 { return t.metNovelRegion / t.metN }, 4))
	fmt.Printf("| 新しさ: 通ったタイルの行 | %s | %s |\n",
		cell(func(t cultureTally) float64 { return t.birthNovelPath / t.birN }, 4),
		cell(func(t cultureTally) float64 { return t.metNovelPath / t.metN }, 4))
	// The products, seed by seed: chances x novelty x the cost per decision
	// of not knowing the row.
	prod := func(t cultureTally, parent bool) float64 {
		cr, cp := t.informedRegion/t.decisions, t.informedPath/t.decisions
		if parent {
			return 2 * (t.birthNovelRegion/t.birN*cr + t.birthNovelPath/t.birN*cp)
		}
		met := t.met / t.lives
		return met * (t.metNovelRegion/t.metN*cr + t.metNovelPath/t.metN*cp)
	}
	fmt.Printf("| 積（機会 × 新しさ × 決定1回あたりの損（皆の知識の値付け）、行の和） | %s | %s |\n",
		cell(func(t cultureTally) float64 { return prod(t, true) }, 6),
		cell(func(t cultureTally) float64 { return prod(t, false) }, 6))
	fmt.Printf("| 差（親子 − 近くの者） | %s | |\n\n", cell(func(t cultureTally) float64 { return prod(t, true) - prod(t, false) }, 6))
}
