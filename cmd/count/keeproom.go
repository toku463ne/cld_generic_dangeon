package main

import (
	"fmt"
	"math"

	"github.com/toku463ne/cld_generic_dangeon/engine"
	"github.com/toku463ne/cld_generic_dangeon/variant"
)

type keepTally struct {
	// after tick 5000, per tick: units on the ground, those on a place
	// to rest, and the tiles of land and of places to rest.
	ticks, food, keptFood, land, shelterTiles float64
	// appeared and decayed from the ledger over the same stretch; of the
	// units watched from birth that vanished at FoodLife, those on a
	// place to rest and not (a vanish before that age was eaten or
	// picked, not decayed).
	appeared, decayed, decayedOn, decayedOff float64
	pop                                      float64
}

// runKeep runs the world and reads the room a slower decay on places to
// rest would have: how much food decays at all, and how much of it lies
// on such a tile when it does.
func runKeep(cfg engine.Config, m engine.Map, ticks int) (keepTally, error) {
	var t keepTally
	w, err := engine.NewWorld(cfg, m)
	if err != nil {
		return t, err
	}
	land := 0
	for _, k := range m.Terrain {
		if k == engine.TerrainLand {
			land++
		}
	}
	var led0 engine.FoodLedger
	// firstSeen is the tick a unit was first seen on its tile, -1 for a
	// unit already lying there when watching began (birth unknown).
	firstSeen := map[int]int64{}
	prev := map[int]bool{}
	for tick := 1; tick <= ticks; tick++ {
		w.Step()
		if len(w.Bodies()) <= lockCollapse {
			break
		}
		if tick == 5000 {
			led0 = w.FoodLedger()
			for _, f := range w.Foods() {
				firstSeen[f.Y*m.Width+f.X] = -1
				prev[f.Y*m.Width+f.X] = true
			}
		}
		if tick <= 5000 {
			continue
		}
		t.ticks++
		t.land += float64(land)
		for i := range m.Terrain {
			if m.Terrain[i] == engine.TerrainLand && w.ShelterAt(i%m.Width, i/m.Width) {
				t.shelterTiles++
			}
		}
		cur := map[int]bool{}
		for _, f := range w.Foods() {
			i := f.Y*m.Width + f.X
			cur[i] = true
			t.food++
			if w.ShelterAt(f.X, f.Y) {
				t.keptFood++
			}
			if !prev[i] {
				firstSeen[i] = int64(tick)
			}
		}
		for i := range prev {
			if cur[i] {
				continue
			}
			if born := firstSeen[i]; born >= 0 && int64(tick)-born >= int64(cfg.FoodLife) {
				if w.ShelterAt(i%m.Width, i/m.Width) {
					t.decayedOn++
				} else {
					t.decayedOff++
				}
			}
			delete(firstSeen, i)
		}
		prev = cur
	}
	led := w.FoodLedger()
	t.appeared = float64(led.Appeared - led0.Appeared)
	t.decayed = float64(led.Decayed - led0.Decayed)
	t.pop = float64(len(w.Bodies()))
	return t, nil
}

// keepRoom counts the room for stage 8-2c (food keeping longer on places
// to rest): what share of food decays, and where it lies when it does.
func keepRoom(m engine.Map, name string, seeds int, seed0 int64, ticks int, vname string) {
	var ts []keepTally
	for s := 0; s < seeds; s++ {
		cfg, err := variant.Config(vname, seed0+int64(s))
		if err != nil {
			fail(err)
		}
		t, err := runKeep(cfg, m, ticks)
		if err != nil {
			fail(err)
		}
		ts = append(ts, t)
	}
	fmt.Printf("地図 %s (%dx%d)、%d シード、条件 %s、%d tick（tick 5000 より後）。定住場所＝休める場所のタイル（世界が準備したものと建てたもの）。腐った単位は、現れてから消えるまでを見張れたもののうち FoodLife で消えたもの（途中から見た単位は数えない）。\n\n", name, m.Width, m.Height, seeds, vname, ticks)
	cell := func(f func(keepTally) float64) string {
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
	fmt.Printf("| 陸のタイルのうち定住場所の割合 | %s |\n", cell(func(t keepTally) float64 { return ratio(t.shelterTiles, t.land) }))
	fmt.Printf("| 地面の食料のうち定住場所の上の割合 | %s |\n", cell(func(t keepTally) float64 { return ratio(t.keptFood, t.food) }))
	fmt.Printf("| 現れた単位のうち腐った割合（帳簿） | %s |\n", cell(func(t keepTally) float64 { return ratio(t.decayed, t.appeared) }))
	fmt.Printf("| 見張れた腐りのうち定住場所の上の割合 | %s |\n", cell(func(t keepTally) float64 { return ratio(t.decayedOn, t.decayedOn+t.decayedOff) }))
	fmt.Printf("| 見張れた腐り（定住場所の上、1シード） | %s |\n", cell(func(t keepTally) float64 { return t.decayedOn }))
	fmt.Printf("| 見張れた腐り（外、1シード） | %s |\n", cell(func(t keepTally) float64 { return t.decayedOff }))
	fmt.Printf("| 最後の個体数 | %s |\n", cell(func(t keepTally) float64 { return t.pop }))
	fmt.Println()
}
