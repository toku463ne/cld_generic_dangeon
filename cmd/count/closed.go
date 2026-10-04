package main

import (
	"fmt"
	"math"

	"github.com/toku463ne/cld_generic_dangeon/engine"
	"github.com/toku463ne/cld_generic_dangeon/variant"
)

type closedTally struct {
	// after tick 5000, per tick: tiles of settlements closed to others,
	// food on them, all food on the ground; and the moves left out of
	// bodies' options by a closed settlement, against all decisions.
	ticks, closedTiles, closedFood, food float64
	movesClosed, decisions               float64
	pop                                  float64
}

// runClosed reads what settlements closed to others take from everyone
// else: the food on them and the moves they close off.
func runClosed(cfg engine.Config, m engine.Map, ticks int) (closedTally, error) {
	var t closedTally
	w, err := engine.NewWorld(cfg, m)
	if err != nil {
		return t, err
	}
	var st0 engine.Stats
	for tick := 1; tick <= ticks; tick++ {
		w.Step()
		if len(w.Bodies()) <= lockCollapse {
			break
		}
		if tick == 5000 {
			st0 = w.Stats()
		}
		if tick <= 5000 {
			continue
		}
		t.ticks++
		for i := 0; i < m.Width*m.Height; i++ {
			if w.ClosedTile(i) {
				t.closedTiles++
			}
		}
		for _, f := range w.Foods() {
			t.food++
			if w.ClosedTile(f.Y*m.Width + f.X) {
				t.closedFood++
			}
		}
	}
	st := w.Stats()
	t.movesClosed = float64(st.MovesClosed - st0.MovesClosed)
	for k := range st.Decisions {
		t.decisions += float64(st.Decisions[k] - st0.Decisions[k])
	}
	t.pop = float64(len(w.Bodies()))
	return t, nil
}

// closedCount counts the cost of settlements closed to others (stage
// 8-2b): the food closed in and the ways closed off.
func closedCount(m engine.Map, name string, seeds int, seed0 int64, ticks int, vname string) {
	var ts []closedTally
	for s := 0; s < seeds; s++ {
		cfg, err := variant.Config(vname, seed0+int64(s))
		if err != nil {
			fail(err)
		}
		t, err := runClosed(cfg, m, ticks)
		if err != nil {
			fail(err)
		}
		ts = append(ts, t)
	}
	fmt.Printf("地図 %s (%dx%d)、%d シード、条件 %s、%d tick（tick 5000 より後）。閉じた定住場所＝持ち主が生きていて、家族以外が入れない場所。\n\n", name, m.Width, m.Height, seeds, vname, ticks)
	cell := func(f func(closedTally) float64) string {
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
	fmt.Printf("| 量 | 値 |\n| --- | --- |\n")
	fmt.Printf("| 閉じた定住場所のタイル（同時、平均） | %s |\n", cell(func(t closedTally) float64 { return ratio(t.closedTiles, t.ticks) }))
	fmt.Printf("| その上の食料（同時、平均） | %s |\n", cell(func(t closedTally) float64 { return ratio(t.closedFood, t.ticks) }))
	fmt.Printf("| 地面の食料のうち閉じた場所の上の割合 | %s |\n", cell(func(t closedTally) float64 { return ratio(t.closedFood, t.food) }))
	fmt.Printf("| 閉じた場所のせいで消えた移動の手（決定あたり） | %s |\n", cell(func(t closedTally) float64 { return ratio(t.movesClosed, t.decisions) }))
	fmt.Printf("| 最後の個体数 | %s |\n", cell(func(t closedTally) float64 { return t.pop }))
	fmt.Println()
}
