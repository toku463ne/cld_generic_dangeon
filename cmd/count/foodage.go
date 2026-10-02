package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/toku463ne/cld_generic_dangeon/engine"
	"github.com/toku463ne/cld_generic_dangeon/variant"
)

// foodAgeMarks are the ages, in ticks, the foodage count reads its shares at.
var foodAgeMarks = []int{100, 300, 1000, 3000}

// runFoodAge steps one world and, after tick 5000, records how long each
// unit lay on the ground before it was taken (eaten or picked up), from the
// tile it lay on: a tile holds at most one unit. Units still on the ground
// at the end give their age so far, apart. A world that collapses stops.
func runFoodAge(cfg engine.Config, m engine.Map, ticks int) (taken, left []float64, err error) {
	w, err := engine.NewWorld(cfg, m)
	if err != nil {
		return nil, nil, err
	}
	since := map[int]int{} // tile -> tick its unit appeared (0: before tick 5000)
	now := map[int]bool{}
	for tick := 1; tick <= ticks; tick++ {
		w.Step()
		for k := range now {
			delete(now, k)
		}
		for _, f := range w.Foods() {
			t := f.Y*m.Width + f.X
			now[t] = true
			if _, ok := since[t]; !ok {
				since[t] = tick
				if tick <= 5000 {
					since[t] = 0
				}
			}
		}
		for t, s := range since {
			if !now[t] {
				if s > 0 {
					taken = append(taken, float64(tick-s))
				}
				delete(since, t)
			}
		}
		if len(w.Bodies()) <= lockCollapse {
			break
		}
	}
	for _, s := range since {
		if s > 0 {
			left = append(left, float64(ticks-s))
		}
	}
	return taken, left, nil
}

// foodAge counts how long food lies on the ground before it is taken: the
// room a decay rule has to work in without touching food bodies eat.
func foodAge(m engine.Map, name string, seeds int, seed0 int64, ticks int, vname string) {
	var taken, left []float64
	var perSeed []float64
	for s := 0; s < seeds; s++ {
		cfg, err := variant.Config(vname, seed0+int64(s))
		if err != nil {
			fail(err)
		}
		tk, lf, err := runFoodAge(cfg, m, ticks)
		if err != nil {
			fail(err)
		}
		taken = append(taken, tk...)
		left = append(left, lf...)
		perSeed = append(perSeed, float64(len(tk)))
	}
	fmt.Printf("地図 %s (%dx%d)、%d シード、条件 %s、%d tick（tick 5000 より後に現れた単位）。崩壊したシードはそこで打ち切る。\n\n", name, m.Width, m.Height, seeds, vname, ticks)
	sort.Float64s(taken)
	q := func(xs []float64, p float64) float64 { return xs[int(p*float64(len(xs)-1))] }
	mm, se := meanSE(perSeed)
	fmt.Printf("取られた単位（1シードあたり）: %.0f ± %.0f\n\n", mm, se)
	if len(taken) > 0 {
		fmt.Printf("地面にあった tick: 中央値 %.0f / 75%% %.0f / 90%% %.0f / 99%% %.0f\n\n", q(taken, 0.5), q(taken, 0.75), q(taken, 0.9), q(taken, 0.99))
		fmt.Printf("| 地面にあった tick | ")
		for _, a := range foodAgeMarks {
			fmt.Printf("> %d | ", a)
		}
		fmt.Printf("\n| --- |%s\n| 取られた単位の割合 | ", strings.Repeat(" --- |", len(foodAgeMarks)))
		for _, a := range foodAgeMarks {
			n := len(taken) - sort.SearchFloat64s(taken, float64(a)+0.5)
			fmt.Printf("%.4f | ", float64(n)/float64(len(taken)))
		}
		fmt.Println()
	}
	sort.Float64s(left)
	fmt.Printf("\n最後に地面に残っていた単位（全シード）: %d", len(left))
	if len(left) > 0 {
		fmt.Printf("、それまでの tick 中央値 %.0f / 90%% %.0f", q(left, 0.5), q(left, 0.9))
	}
	fmt.Printf("\n\n")
}
