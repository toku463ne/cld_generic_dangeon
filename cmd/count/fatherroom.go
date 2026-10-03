package main

import (
	"fmt"
	"math"

	"github.com/toku463ne/cld_generic_dangeon/engine"
	"github.com/toku463ne/cld_generic_dangeon/variant"
)

type fatherRoomTally struct {
	// after tick 5000: births, and units of food fathers with a resting
	// mate took (ate or picked up), and of those the ones the gate would
	// send.
	births, taken, sent float64
	// and the units underfoot such fathers left where they were (food on
	// their tile, not eaten or picked up): more they could send.
	left float64
}

// runFatherRoom reads every decision of a father whose mate rests and who
// takes a unit of food: would the unit matter to his own life?
func runFatherRoom(cfg engine.Config, m engine.Map, ticks int) (fatherRoomTally, error) {
	var t fatherRoomTally
	w, err := engine.NewWorld(cfg, m)
	if err != nil {
		return t, err
	}
	resting := map[int64]bool{} // fathers of resting mothers, this tick
	w.SetTrace(func(b engine.Body, v engine.Valuation, a engine.Action) {
		if w.Tick() <= 5000 || !resting[b.ID] {
			return
		}
		if a.Kind != engine.ActEat && a.Kind != engine.ActPick {
			for _, o := range v.Options {
				if o.Kind == engine.ActEat {
					t.left++
					break
				}
			}
			return
		}
		t.taken++
		took, without := math.Inf(1), math.Inf(1)
		for j, o := range v.Options {
			r := v.Risk[0][j]
			if o == a {
				took = r
			}
			if o.Kind != engine.ActEat && o.Kind != engine.ActPick {
				without = math.Min(without, r)
			}
		}
		if without-took <= cfg.SendGate {
			t.sent++
		}
	})
	var b0 int64
	for tick := 1; tick <= ticks; tick++ {
		for k := range resting {
			delete(resting, k)
		}
		for _, b := range w.Bodies() {
			if w.Resting(b) {
				resting[b.Partner] = true
			}
		}
		w.Step()
		if tick == 5000 {
			b0 = w.Stats().Births
		}
		if len(w.Bodies()) <= lockCollapse {
			break
		}
	}
	t.births = float64(w.Stats().Births - b0)
	return t, nil
}

// fatherRoom counts the room for fathers sending food to a resting mother
// (stage 6-4a): per birth, the units a father takes while she rests, and
// those that would not matter to his own life.
func fatherRoom(m engine.Map, name string, seeds int, seed0 int64, ticks int, vname string) {
	var ts []fatherRoomTally
	for s := 0; s < seeds; s++ {
		cfg, err := variant.Config(vname, seed0+int64(s))
		if err != nil {
			fail(err)
		}
		t, err := runFatherRoom(cfg, m, ticks)
		if err != nil {
			fail(err)
		}
		ts = append(ts, t)
	}
	fmt.Printf("地図 %s (%dx%d)、%d シード、条件 %s、%d tick（tick 5000 より後）。休んでいる母の相手（最後の子の父）が食料を食べる・拾うとき、その手の危険と、食べも拾いもしない手の一番低い危険の差が SendGate 以下なら「送れる」。\n\n", name, m.Width, m.Height, seeds, vname, ticks)
	cell := func(f func(fatherRoomTally) float64) string {
		var xs []float64
		for _, t := range ts {
			if x := f(t); !math.IsNaN(x) {
				xs = append(xs, x)
			}
		}
		mm, se := meanSE(xs)
		return fmt.Sprintf("%.3f ± %.3f", mm, se)
	}
	ratio := func(a, b float64) float64 {
		if b == 0 {
			return math.NaN()
		}
		return a / b
	}
	fmt.Printf("| 量 | 値 |\n| --- | --- |\n")
	fmt.Printf("| 出生（1シードあたり） | %s |\n", cell(func(t fatherRoomTally) float64 { return t.births }))
	fmt.Printf("| 出生 1 回あたり、母の休みの間に父が取った食料 | %s |\n", cell(func(t fatherRoomTally) float64 { return ratio(t.taken, t.births) }))
	fmt.Printf("| 　そのうち送れる（父の命に効かない） | %s |\n", cell(func(t fatherRoomTally) float64 { return ratio(t.sent, t.births) }))
	fmt.Printf("| 出生 1 回あたり、父が足元に残した食料（決めた tick の数） | %s |\n", cell(func(t fatherRoomTally) float64 { return ratio(t.left, t.births) }))
	fmt.Println()
}
