package main

import (
	"fmt"
	"math"

	"github.com/toku463ne/cld_generic_dangeon/engine"
	"github.com/toku463ne/cld_generic_dangeon/variant"
)

type collideTally struct {
	bodyTicks  float64
	crossing   [2]float64 // moves into another tile: [0] followed an intent, [1] decided
	blocked    [2]float64 // of them, into a tile another body stood on
	sharedTile float64    // body-ticks on a tile with another body
	foodTile   float64    // blocked moves into a tile with food
}

// runCollide reads, in the base world after tick 5000, every move that
// takes a body into another tile, and whether another body stood on that
// tile at the start of the tick: how often a rule that a body cannot enter
// an occupied tile would bite, and whether at a decision or while the body
// follows its intent.
func runCollide(cfg engine.Config, m engine.Map, ticks int) (collideTally, error) {
	var t collideTally
	w, err := engine.NewWorld(cfg, m)
	if err != nil {
		return t, err
	}
	decided := map[int64]bool{}
	w.SetTrace(func(b engine.Body, _ engine.Valuation, _ engine.Action) { decided[b.ID] = true })
	occ := make([]int32, m.Width*m.Height)
	tile := func(x, y float64) int { return int(math.Floor(y))*m.Width + int(math.Floor(x)) }
	for tick := 1; tick <= ticks; tick++ {
		before := w.Bodies()
		clear(occ)
		for _, b := range before {
			occ[tile(b.X, b.Y)]++
		}
		food := map[int]bool{}
		if tick > 5000 {
			for _, f := range w.Foods() {
				food[f.Y*m.Width+f.X] = true
			}
		}
		clear(decided)
		w.Step()
		if tick <= 5000 {
			continue
		}
		after := map[int64]engine.Body{}
		for _, b := range w.Bodies() {
			after[b.ID] = b
		}
		for _, b := range before {
			t.bodyTicks++
			if occ[tile(b.X, b.Y)] > 1 {
				t.sharedTile++
			}
			a, ok := after[b.ID]
			if !ok {
				continue
			}
			from, to := tile(b.X, b.Y), tile(a.X, a.Y)
			if from == to {
				continue
			}
			k := 0
			if decided[b.ID] {
				k = 1
			}
			t.crossing[k]++
			if occ[to] > 0 {
				t.blocked[k]++
				if food[to] {
					t.foodTile++
				}
			}
		}
		if len(w.Bodies()) == 0 {
			break
		}
	}
	return t, nil
}

// collide counts, before bodies collide, how often they would: moves into
// a tile another body stands on, at decisions and while following intents.
func collide(m engine.Map, name string, seeds int, seed0 int64, ticks int) {
	fmt.Printf("地図 %s (%dx%d)、%d シード、%d tick、base（1-4）。tick 5000 より後。「塞がっている」は tick の始めに他の身体が立っていたタイル。\n\n", name, m.Width, m.Height, seeds, ticks)
	var ts []collideTally
	for s := 0; s < seeds; s++ {
		cfg, err := variant.Config(variant.Base, seed0+int64(s))
		if err != nil {
			fail(err)
		}
		t, err := runCollide(cfg, m, ticks)
		if err != nil {
			fail(err)
		}
		ts = append(ts, t)
	}
	cell := func(f func(collideTally) float64) string {
		var xs []float64
		for _, t := range ts {
			xs = append(xs, f(t))
		}
		mm, se := meanSE(xs)
		return fmt.Sprintf("%.5f ± %.5f", mm, se)
	}
	fmt.Printf("| 量 | 値 |\n| --- | --- |\n")
	fmt.Printf("| 他の身体と同じタイルにいる身体×tick の割合 | %s |\n", cell(func(t collideTally) float64 { return t.sharedTile / t.bodyTicks }))
	fmt.Printf("| タイルをまたぐ移動の、身体×tick あたりの割合 | %s |\n", cell(func(t collideTally) float64 { return (t.crossing[0] + t.crossing[1]) / t.bodyTicks }))
	fmt.Printf("| そのうち決めた tick の移動の割合 | %s |\n", cell(func(t collideTally) float64 { return t.crossing[1] / (t.crossing[0] + t.crossing[1]) }))
	fmt.Printf("| 塞がったタイルへの移動 ÷ タイルをまたぐ移動（決めた tick） | %s |\n", cell(func(t collideTally) float64 { return t.blocked[1] / t.crossing[1] }))
	fmt.Printf("| 塞がったタイルへの移動 ÷ タイルをまたぐ移動（意図に従った tick） | %s |\n", cell(func(t collideTally) float64 { return t.blocked[0] / t.crossing[0] }))
	fmt.Printf("| 意図に従っていて塞がったタイルへ入る移動の、身体×tick あたりの割合 | %s |\n", cell(func(t collideTally) float64 { return t.blocked[0] / t.bodyTicks }))
	fmt.Printf("| 塞がったタイルへの移動のうち、そのタイルに食料があった割合 | %s |\n", cell(func(t collideTally) float64 { return t.foodTile / (t.blocked[0] + t.blocked[1]) }))
	fmt.Println()
}
