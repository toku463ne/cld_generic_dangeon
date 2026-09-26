package main

import (
	"fmt"

	"github.com/toku463ne/cld_generic_dangeon/engine"
	"github.com/toku463ne/cld_generic_dangeon/variant"
)

// walkedTally sums the tiles that came into view on a step, as the engine
// reads them for the path row (learn.go stepped): those the body left
// within PathRecall ticks (walked) and the rest (fresh), and how many held
// food; the walked ones split by whether food lay on the tile when the body
// left it; and the food per land tile of each region then, summed the same
// way, to read each against.
type walkedTally struct {
	walked, walkedFood, walkedTruth float64
	fresh, freshFood, freshTruth    float64
	// Walked tiles by what lay on them when the body left: food (left),
	// none (bare), and those left before the count began (unknown).
	left, leftFood, bare, bareFood float64
	// leaves counts the steps off a tile; leavesFood those off a tile with
	// food on it, left uneaten.
	leaves, leavesFood float64
}

// runWalked runs the world, and from tick 5000 on follows every step off a
// tile, reading the tiles that come into view as the path row does.
func runWalked(cfg engine.Config, m engine.Map, ticks int) (walkedTally, error) {
	var t walkedTally
	w, err := engine.NewWorld(cfg, m)
	if err != nil {
		return t, err
	}
	for i := 0; i < 5000; i++ {
		w.Step()
	}
	land := make([]float64, len(m.RegionFood))
	for i, tt := range m.Terrain {
		if tt == engine.TerrainLand {
			land[m.Region[i]]++
		}
	}
	tileOf := func(x, y float64) int { return int(y)*m.Width + int(x) }
	// leftWith[body][tile] is whether food lay on the tile when the body
	// last left it.
	leftWith := map[int64]map[int]bool{}
	s := cfg.Sight
	for tick := 5000; tick < ticks; tick++ {
		food := map[int]bool{}
		for _, f := range w.Foods() {
			food[f.Y*m.Width+f.X] = true
		}
		before := map[int64]int{}
		for _, b := range w.Bodies() {
			before[b.ID] = tileOf(b.X, b.Y)
		}
		w.Step()
		after := map[int]bool{}
		perRegion := make([]float64, len(land))
		for _, f := range w.Foods() {
			after[f.Y*m.Width+f.X] = true
			perRegion[m.Region[f.Y*m.Width+f.X]]++
		}
		rate := func(tile int) float64 { return perRegion[m.Region[tile]] / land[m.Region[tile]] }
		for _, b := range w.Bodies() {
			from, ok := before[b.ID]
			to := tileOf(b.X, b.Y)
			if !ok || from == to {
				continue
			}
			lw := leftWith[b.ID]
			if lw == nil {
				lw = map[int]bool{}
				leftWith[b.ID] = lw
			}
			tx, ty := to%m.Width, to/m.Width
			fx, fy := from%m.Width, from/m.Width
			for y := ty - s; y <= ty+s; y++ {
				for x := tx - s; x <= tx+s; x++ {
					if !m.InBounds(x, y) || m.TerrainAt(x, y) != engine.TerrainLand {
						continue
					}
					if abs(x-fx) <= s && abs(y-fy) <= s {
						continue
					}
					tile := y*m.Width + x
					has := 0.0
					if after[tile] {
						has = 1
					}
					when, walked := b.Memory.Walked[tile]
					if walked && w.Tick()-when <= int64(cfg.PathRecall) && tile != from {
						t.walked++
						t.walkedFood += has
						t.walkedTruth += rate(tile)
						if had, known := lw[tile]; known {
							if had {
								t.left++
								t.leftFood += has
							} else {
								t.bare++
								t.bareFood += has
							}
						}
					} else {
						t.fresh++
						t.freshFood += has
						t.freshTruth += rate(tile)
					}
				}
			}
			t.leaves++
			if food[from] {
				t.leavesFood++
			}
			lw[from] = food[from]
		}
	}
	return t, nil
}

// walked counts why the path row reads what it does: the food on walked
// tiles that come into view again against their region's food per tile,
// beside the same for tiles that were not walked, and how much of the
// walked tiles' food was lying there, uneaten, when the body left.
func walked(m engine.Map, name string, seeds int, seed0 int64, ticks int, vname string) {
	fmt.Printf("地図 %s (%dx%d)、%d シード、条件 %s、tick 5000〜%d。歩いて視界に入ったタイルを、通ったタイルの行と同じ決め方（%d tick 以内に離れたタイル）で分ける。比はその地域の、そのときの陸のタイルあたりの食料（世界の値）に対して。\n\n", name, m.Width, m.Height, seeds, vname, ticks, 200)
	var ts []walkedTally
	for s := 0; s < seeds; s++ {
		cfg, err := variant.Config(vname, seed0+int64(s))
		if err != nil {
			fail(err)
		}
		t, err := runWalked(cfg, m, ticks)
		if err != nil {
			fail(err)
		}
		ts = append(ts, t)
	}
	cell := func(f func(walkedTally) float64, prec int) string {
		var xs []float64
		for _, t := range ts {
			xs = append(xs, f(t))
		}
		mm, se := meanSE(xs)
		return fmt.Sprintf("%.*f ± %.*f", prec, mm, prec, se)
	}
	fmt.Printf("| タイル | 数 | 食料がある割合 | 世界の値に対する比 |\n| --- | --- | --- | --- |\n")
	fmt.Printf("| 通ったタイル | %s | %s | %s |\n", cell(func(t walkedTally) float64 { return t.walked }, 0),
		cell(func(t walkedTally) float64 { return t.walkedFood / t.walked }, 4),
		cell(func(t walkedTally) float64 { return t.walkedFood / t.walkedTruth }, 3))
	fmt.Printf("| 通っていないタイル | %s | %s | %s |\n", cell(func(t walkedTally) float64 { return t.fresh }, 0),
		cell(func(t walkedTally) float64 { return t.freshFood / t.fresh }, 4),
		cell(func(t walkedTally) float64 { return t.freshFood / t.freshTruth }, 3))
	fmt.Printf("| 通ったタイルのうち、離れたとき食料があった | %s | %s | — |\n", cell(func(t walkedTally) float64 { return t.left }, 0),
		cell(func(t walkedTally) float64 { return t.leftFood / t.left }, 4))
	fmt.Printf("| 通ったタイルのうち、離れたとき食料が無かった | %s | %s | — |\n\n", cell(func(t walkedTally) float64 { return t.bare }, 0),
		cell(func(t walkedTally) float64 { return t.bareFood / t.bare }, 4))
	fmt.Printf("通ったタイルの食料のうち、離れたときからあった（食べずに離れた）食料の割合: %s\n\n", cell(func(t walkedTally) float64 { return t.leftFood / t.walkedFood }, 3))
	fmt.Printf("タイルを離れる手のうち、食料の上から食べずに離れた割合: %s\n\n", cell(func(t walkedTally) float64 { return t.leavesFood / t.leaves }, 4))
}
