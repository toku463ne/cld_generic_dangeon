package main

import (
	"fmt"
	"math"

	"github.com/toku463ne/cld_generic_dangeon/engine"
	"github.com/toku463ne/cld_generic_dangeon/variant"
)

// buildCosts are the energies a build might cost, read for each decision.
var buildCosts = []float64{0, 5, 10, 20, 40}

type buildRoomTally struct {
	// after tick 5000, decisions of awake bodies off any place to rest
	// with sleep among their options, for adults who learned a place's
	// warmth [0], adults who did not [1] and children [2]; and of them,
	// those where sleeping here warm (at the burn of the body's own body,
	// no cold) after paying each of buildCosts reads as less risk than
	// the best option now, and of those at cost 0, the ones in the dark
	// and the gain in risk.
	offered [3]float64
	beats   [3][5]float64
	dark    [3]float64
	gain    [3]float64
	// with the warmth the body learned (group 0 only), at cost 0
	learned float64
	// sleeps begun off a place to rest in the dark, and all sleeps begun
	coldSleeps, sleeps float64
	// adults alive at the end, and those who learned a place's warmth
	adults, warmAdults float64
	// sleeps begun off a place to rest (chosen or forced), and of them
	// those where building first at each of buildCosts and sleeping warm
	// reads as less risk than sleeping there in the cold, and the gain at
	// cost 0
	offSleeps  float64
	buildBeats [5]float64
	buildGain  float64
	// the cold's extra burn over body-ticks asleep off a place to rest,
	// and those body-ticks
	coldBurn, coldTicks float64
}

// runBuildRoom reads, at every decision, what a warm place where the body
// stands would be worth: the room for building one.
func runBuildRoom(cfg engine.Config, m engine.Map, ticks int) (buildRoomTally, error) {
	var t buildRoomTally
	w, err := engine.NewWorld(cfg, m)
	if err != nil {
		return t, err
	}
	w.SetTrace(func(b engine.Body, v engine.Valuation, a engine.Action) {
		if w.Tick() <= 5000 || b.Asleep || len(v.Score) == 0 {
			return
		}
		x, y := int(math.Floor(b.X)), int(math.Floor(b.Y))
		if a.Kind == engine.ActSleep {
			t.sleeps++
			if !w.ShelterAt(x, y) && w.Night() {
				t.coldSleeps++
			}
			if !w.ShelterAt(x, y) {
				t.offSleeps++
				own := w.OwnBurn(b)
				cold := w.SleepHereRisk(b, own*w.TempBurn(b), 0)
				for i, c := range buildCosts {
					if r := w.SleepHereRisk(b, own, c); r < cold {
						t.buildBeats[i]++
						if i == 0 {
							t.buildGain += cold - r
						}
					}
				}
			}
		}
		if w.ShelterAt(x, y) {
			return
		}
		sleep := false
		for _, o := range v.Options {
			sleep = sleep || o.Kind == engine.ActSleep
		}
		if !sleep {
			return
		}
		g := 2
		if b.Mature <= w.Tick() {
			g = 1
			if b.Memory.Warm[1] > 0 {
				g = 0
			}
		}
		best := math.Inf(1)
		for _, s := range v.Score {
			best = math.Min(best, s)
		}
		t.offered[g]++
		own := w.OwnBurn(b)
		for i, c := range buildCosts {
			r := w.SleepHereRisk(b, own, c)
			if r < best {
				t.beats[g][i]++
				if i == 0 {
					t.gain[g] += best - r
					if w.Night() {
						t.dark[g]++
					}
				}
			}
		}
		if g == 0 {
			if r := w.SleepHereRisk(b, own*b.Memory.Warm[0]/b.Memory.Warm[1], 0); r < best {
				t.learned++
			}
		}
	})
	for tick := 1; tick <= ticks; tick++ {
		w.Step()
		bs := w.Bodies()
		if len(bs) <= lockCollapse {
			break
		}
		if tick <= 5000 {
			continue
		}
		for _, b := range bs {
			if !b.Asleep || w.ShelterAt(int(math.Floor(b.X)), int(math.Floor(b.Y))) {
				continue
			}
			t.coldTicks++
			if f := w.TempBurn(b); f > 1 {
				t.coldBurn += b.BurnFelt * (1 - 1/f)
			}
		}
	}
	for _, b := range w.Bodies() {
		if b.Mature <= w.Tick() {
			t.adults++
			if b.Memory.Warm[1] > 0 {
				t.warmAdults++
			}
		}
	}
	return t, nil
}

// buildRoom counts the room for building a warm place to sleep where a
// body stands (stage 8): how often sleeping here warm would beat the best
// option now, at what cost of building it still would, and how many know
// the warmth.
func buildRoom(m engine.Map, name string, seeds int, seed0 int64, ticks int, vname string) {
	var ts []buildRoomTally
	w0, err := variant.Config(vname, seed0)
	if err != nil {
		fail(err)
	}
	for s := 0; s < seeds; s++ {
		cfg, err := variant.Config(vname, seed0+int64(s))
		if err != nil {
			fail(err)
		}
		t, err := runBuildRoom(cfg, m, ticks)
		if err != nil {
			fail(err)
		}
		ts = append(ts, t)
	}
	fmt.Printf("地図 %s (%dx%d)、%d シード、条件 %s、%d tick（tick 5000 より後の決定）。対象＝起きていて休める場所の外にいて、寝る手が候補にある決定。「ここで暖かく寝る」＝建てる代金を払ってから、その場で自分の身体の消耗（寒さなし）で `SleepTicks` 眠る手を、寝る手と同じ読み方で読んだ危険。勝つ＝今の一番よい手の点（危険 − 子の値打ち）より小さい。\n\n", name, m.Width, m.Height, seeds, vname, ticks)
	cell := func(f func(buildRoomTally) float64) string {
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
	fmt.Printf("| 身体 | 対象の決定（1シードあたり） | 代金 0 で勝つ | 5 | 10 | 20 | 40 | 勝つうち暗い | 勝つときの危険の差（平均） |\n| --- | --- | --- | --- | --- | --- | --- | --- | --- |\n")
	for g, label := range []string{"暖かさを学んだ成人", "学んでいない成人", "子"} {
		g := g
		row := fmt.Sprintf("| %s | %s", label, cell(func(t buildRoomTally) float64 { return t.offered[g] }))
		for i := range buildCosts {
			i := i
			row += " | " + cell(func(t buildRoomTally) float64 { return ratio(t.beats[g][i], t.offered[g]) })
		}
		row += " | " + cell(func(t buildRoomTally) float64 { return ratio(t.dark[g], t.beats[g][0]) })
		row += " | " + cell(func(t buildRoomTally) float64 { return ratio(t.gain[g], t.beats[g][0]) })
		fmt.Println(row + " |")
	}
	fmt.Printf("\n| 量 | 値 |\n| --- | --- |\n")
	fmt.Printf("| 学んだ暖かさで読んでも代金 0 で勝つ（暖かさを学んだ成人の対象の決定のうち） | %s |\n", cell(func(t buildRoomTally) float64 { return ratio(t.learned, t.offered[0]) }))
	fmt.Printf("| 暗いとき休める場所の外で始めた眠り（始めた眠りのうち） | %s |\n", cell(func(t buildRoomTally) float64 { return ratio(t.coldSleeps, t.sleeps) }))
	row := "| 休める場所の外で始めた眠りのうち、建ててから暖かく寝るほうがその場で寒く寝るより危険が小さい（代金 0 / 5 / 10 / 20 / 40） | "
	for i := range buildCosts {
		i := i
		if i > 0 {
			row += " / "
		}
		row += cell(func(t buildRoomTally) float64 { return ratio(t.buildBeats[i], t.offSleeps) })
	}
	fmt.Println(row + " |")
	fmt.Printf("| そのときの危険の差（代金 0、平均） | %s |\n", cell(func(t buildRoomTally) float64 { return ratio(t.buildGain, t.buildBeats[0]) }))
	fmt.Printf("| 休める場所の外で眠っている tick あたりの寒さの消耗 | %s |\n", cell(func(t buildRoomTally) float64 { return ratio(t.coldBurn, t.coldTicks) }))
	fmt.Printf("| 休める場所の外で始めた眠り1回あたりの寒さの消耗（`SleepTicks` 分） | %s |\n", cell(func(t buildRoomTally) float64 { return ratio(t.coldBurn, t.coldTicks) * float64(w0.SleepTicks) }))
	fmt.Printf("| 最後の tick の成人のうち暖かさを学んだ割合 | %s |\n", cell(func(t buildRoomTally) float64 { return ratio(t.warmAdults, t.adults) }))
	fmt.Println()
}
