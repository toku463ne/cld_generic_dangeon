package main

import (
	"fmt"
	"math"

	"github.com/toku463ne/cld_generic_dangeon/engine"
	"github.com/toku463ne/cld_generic_dangeon/variant"
)

// placeGaps bound the ticks since a body last met a tile, for the reads.
var placeGaps = []int64{50, 200, 1000, 1 << 40}

type placeRoomTally struct {
	// Tiles in sight at decisions after tick 5000, and those with food: the
	// base rate a tile holds food.
	sightTiles, sightFood float64
	// A tile where the body ate or picked up food, back in sight later: by
	// gap since then, how many times, and how many held food then.
	ateBack, ateFood [4]float64
	// A tile where the body saw food it did not eat, back in sight later:
	// by gap, how many times, and how many still held food.
	seenBack, seenFood [4]float64
	// Decisions after tick 5000 with no food in sight, and of them those
	// with a tile remembered (seen with food, not eaten, within 1000 ticks)
	// that still holds food: what a place row could point to.
	blind, blindRemembered float64
	// Units seen with food and not eaten, after tick 5000, and of them those
	// the body met again (in sight) at all.
	seenUnits, seenMet float64
}

// runPlaceRoom reads every decision after tick 5000: the tiles in sight,
// the food among them, and what became of tiles the body ate on or saw food
// on before.
func runPlaceRoom(cfg engine.Config, m engine.Map, ticks int) (placeRoomTally, error) {
	var t placeRoomTally
	w, err := engine.NewWorld(cfg, m)
	if err != nil {
		return t, err
	}
	type mark struct {
		tick int64
		met  bool
	}
	ate := map[int64]map[int]int64{}  // body -> tile -> tick it ate there
	seen := map[int64]map[int]*mark{} // body -> tile -> food seen there, not eaten
	foodAt := map[int]bool{}
	s := cfg.Sight
	gapOf := func(d int64) int {
		for k, g := range placeGaps {
			if d <= g {
				return k
			}
		}
		return len(placeGaps) - 1
	}
	w.SetTrace(func(b engine.Body, v engine.Valuation, a engine.Action) {
		now := w.Tick()
		bx, by := int(math.Floor(b.X)), int(math.Floor(b.Y))
		here := by*m.Width + bx
		if ate[b.ID] == nil {
			ate[b.ID], seen[b.ID] = map[int]int64{}, map[int]*mark{}
		}
		for k := range foodAt {
			delete(foodAt, k)
		}
		for _, f := range v.Seen {
			foodAt[f.Y*m.Width+f.X] = true
		}
		late := now > 5000
		for y := by - s; y <= by+s; y++ {
			for x := bx - s; x <= bx+s; x++ {
				if !m.InBounds(x, y) || m.TerrainAt(x, y) != engine.TerrainLand {
					continue
				}
				tile := y*m.Width + x
				food := foodAt[tile]
				if late {
					t.sightTiles++
					if food {
						t.sightFood++
					}
				}
				if when, ok := ate[b.ID][tile]; ok && now-when > 1 && late {
					k := gapOf(now - when)
					t.ateBack[k]++
					if food {
						t.ateFood[k]++
					}
					delete(ate[b.ID], tile)
				}
				if mk, ok := seen[b.ID][tile]; ok && now-mk.tick > 1 {
					// Back in sight: read it, then forget it unless it
					// still holds food (then it is seen afresh below).
					if late {
						k := gapOf(now - mk.tick)
						t.seenBack[k]++
						if food {
							t.seenFood[k]++
						}
						if !mk.met {
							t.seenMet++
						}
					}
					delete(seen[b.ID], tile)
				}
			}
		}
		if late && len(v.Seen) == 0 {
			t.blind++
			for tile, mk := range seen[b.ID] {
				if now-mk.tick <= 1000 && w.FoodOn(tile) {
					t.blindRemembered++
					break
				}
			}
		}
		for tile := range foodAt {
			if tile == here && (a.Kind == engine.ActEat || a.Kind == engine.ActPick) {
				continue
			}
			if _, ok := seen[b.ID][tile]; !ok && late {
				t.seenUnits++
			}
			seen[b.ID][tile] = &mark{tick: now}
		}
		if a.Kind == engine.ActEat || a.Kind == engine.ActPick {
			ate[b.ID][here] = now
			delete(seen[b.ID], here)
		}
	})
	for tick := 1; tick <= ticks; tick++ {
		w.Step()
		if tick%1000 == 0 {
			// Forget the dead.
			alive := map[int64]bool{}
			for _, b := range w.Bodies() {
				alive[b.ID] = true
			}
			for id := range ate {
				if !alive[id] {
					delete(ate, id)
					delete(seen, id)
				}
			}
		}
	}
	return t, nil
}

// placeRoom counts, before stage 5-2 gives bodies rows for single tiles,
// whether a tile tells anything: whether food comes back to a tile a body
// ate on more often than to any tile in sight, and whether food a body saw
// and left is still there when it comes back.
func placeRoom(m engine.Map, name string, seeds int, seed0 int64, ticks int, vname string) {
	var ts []placeRoomTally
	for s := 0; s < seeds; s++ {
		cfg, err := variant.Config(vname, seed0+int64(s))
		if err != nil {
			fail(err)
		}
		t, err := runPlaceRoom(cfg, m, ticks)
		if err != nil {
			fail(err)
		}
		ts = append(ts, t)
	}
	fmt.Printf("地図 %s (%dx%d)、%d シード、条件 %s、%d tick（tick 5000 より後の決定）。タイルは決定のときの視界で読む。\n\n", name, m.Width, m.Height, seeds, vname, ticks)
	cell := func(f func(placeRoomTally) float64, prec int) string {
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
		return fmt.Sprintf("%.*f ± %.*f", prec, mm, prec, se)
	}
	ratio := func(a, b float64) float64 {
		if b == 0 {
			return math.NaN()
		}
		return a / b
	}
	fmt.Printf("| 量 | 値 |\n| --- | --- |\n")
	fmt.Printf("| 視界のタイルに食料がある割合 | %s |\n", cell(func(t placeRoomTally) float64 { return ratio(t.sightFood, t.sightTiles) }, 4))
	fmt.Printf("| 見て残した食料（1シードあたり） | %s |\n", cell(func(t placeRoomTally) float64 { return t.seenUnits }, 0))
	fmt.Printf("| 　そのうち同じ身体がまた視界に入れた割合 | %s |\n", cell(func(t placeRoomTally) float64 { return ratio(t.seenMet, t.seenUnits) }, 4))
	fmt.Printf("| 視界に食料が無い決定のうち、1000 tick 以内に見て残した食料がまだある割合 | %s |\n\n", cell(func(t placeRoomTally) float64 { return ratio(t.blindRemembered, t.blind) }, 4))
	labels := []string{"≤ 50", "51〜200", "201〜1000", "> 1000"}
	fmt.Printf("| 前に会ってからの tick | 食べたタイル: 戻った回数 | 食べたタイル: 食料がある割合 | 見て残した食料: 戻った回数 | 見て残した食料: まだある割合 |\n| --- | --- | --- | --- | --- |\n")
	for k, l := range labels {
		k := k
		fmt.Printf("| %s | %s | %s | %s | %s |\n", l,
			cell(func(t placeRoomTally) float64 { return t.ateBack[k] }, 0),
			cell(func(t placeRoomTally) float64 { return ratio(t.ateFood[k], t.ateBack[k]) }, 4),
			cell(func(t placeRoomTally) float64 { return t.seenBack[k] }, 0),
			cell(func(t placeRoomTally) float64 { return ratio(t.seenFood[k], t.seenBack[k]) }, 4))
	}
	fmt.Println()
}
