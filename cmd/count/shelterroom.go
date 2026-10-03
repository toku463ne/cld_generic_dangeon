package main

import (
	"fmt"
	"math"
	"sort"

	"github.com/toku463ne/cld_generic_dangeon/engine"
	"github.com/toku463ne/cld_generic_dangeon/variant"
)

type shelterRoomTally struct {
	// starvations after tick 5000, of children and of adults, and of them
	// those in the night (light below 1) and those a warm night would have
	// carried to full light: the cold's extra burn since the light last
	// fell would have lasted until the light came back at the burn without
	// cold.
	starved, night, saved [2]float64
	// how far (tiles, Chebyshev) the night's dead were, when the light
	// began to fall, from the nearest quadrant centre: where a shelter
	// placed on the map might be.
	dist []float64
}

// runShelterRoom follows every body's extra burn from the cold since the
// light last fell, and reads each starvation in the night against it.
func runShelterRoom(cfg engine.Config, m engine.Map, ticks int) (shelterRoomTally, error) {
	var t shelterRoomTally
	w, err := engine.NewWorld(cfg, m)
	if err != nil {
		return t, err
	}
	type track struct {
		extra  float64 // the cold's burn since the light last fell
		x, y   int     // where the body was when it began to fall
		last   engine.Body
		marked bool
	}
	type death struct {
		tick        int64
		extra, warm float64
		child       int
	}
	centres := [][2]int{{m.Width / 4, m.Height / 4}, {3 * m.Width / 4, m.Height / 4}, {m.Width / 4, 3 * m.Height / 4}, {3 * m.Width / 4, 3 * m.Height / 4}}
	nearest := func(x, y int) float64 {
		d := math.Inf(1)
		for _, c := range centres {
			d = math.Min(d, math.Max(math.Abs(float64(x-c[0])), math.Abs(float64(y-c[1]))))
		}
		return d
	}
	tracks := map[int64]*track{}
	var waiting []death
	prevLight := 1.0
	for tick := 1; tick <= ticks; tick++ {
		w.Step()
		l := w.Brightness()
		falling := prevLight == 1 && l < 1
		alive := map[int64]bool{}
		for _, b := range w.Bodies() {
			alive[b.ID] = true
			tr := tracks[b.ID]
			if tr == nil {
				tr = &track{}
				tracks[b.ID] = tr
			}
			if falling || !tr.marked {
				tr.extra, tr.x, tr.y, tr.marked = 0, int(math.Floor(b.X)), int(math.Floor(b.Y)), true
			}
			if f := w.TempBurn(b); f > 1 {
				tr.extra += b.BurnFelt * (1 - 1/f)
			}
			tr.last = b
		}
		for id, tr := range tracks {
			if alive[id] {
				continue
			}
			b := tr.last
			delete(tracks, id)
			if w.Tick() <= 5000 || (cfg.Lifespan > 0 && w.Tick()-b.Born >= int64(cfg.Lifespan)) {
				continue
			}
			c := 1
			if w.Tick() < b.Mature {
				c = 0
			}
			t.starved[c]++
			if l < 1 {
				t.night[c]++
				t.dist = append(t.dist, nearest(tr.x, tr.y))
				warm := b.BurnFelt / math.Max(w.TempBurn(b), 1)
				waiting = append(waiting, death{tick: w.Tick(), extra: tr.extra, warm: warm, child: c})
			}
		}
		if l == 1 && len(waiting) > 0 {
			for _, d := range waiting {
				if d.extra >= float64(w.Tick()-d.tick)*d.warm {
					t.saved[d.child]++
				}
			}
			waiting = waiting[:0]
		}
		prevLight = l
		if len(w.Bodies()) <= lockCollapse {
			break
		}
	}
	return t, nil
}

// shelterRoom counts the room a warm place to rest would have: the
// starvations in the night a night without cold would have carried to
// full light, and how far the dying were from where shelters might be.
func shelterRoom(m engine.Map, name string, seeds int, seed0 int64, ticks int, vname string) {
	var ts []shelterRoomTally
	var dist []float64
	for s := 0; s < seeds; s++ {
		cfg, err := variant.Config(vname, seed0+int64(s))
		if err != nil {
			fail(err)
		}
		t, err := runShelterRoom(cfg, m, ticks)
		if err != nil {
			fail(err)
		}
		ts = append(ts, t)
		dist = append(dist, t.dist...)
	}
	fmt.Printf("地図 %s (%dx%d)、%d シード、条件 %s、%d tick（tick 5000 より後の死）。夜＝明るさ 1 未満。暖かい夜なら助かった＝明るさが落ち始めてからの寒さの分の消耗が、死んだ tick から明るさが 1 に戻るまでの寒さの無い消耗に足りる。\n\n", name, m.Width, m.Height, seeds, vname, ticks)
	cell := func(f func(shelterRoomTally) float64) string {
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
	fmt.Printf("| | 子 | 成人 |\n| --- | --- | --- |\n")
	fmt.Printf("| 餓死（1シードあたり） | %s | %s |\n", cell(func(t shelterRoomTally) float64 { return t.starved[0] }), cell(func(t shelterRoomTally) float64 { return t.starved[1] }))
	fmt.Printf("| 　そのうち夜の割合 | %s | %s |\n", cell(func(t shelterRoomTally) float64 { return ratio(t.night[0], t.starved[0]) }), cell(func(t shelterRoomTally) float64 { return ratio(t.night[1], t.starved[1]) }))
	fmt.Printf("| 　夜の餓死のうち暖かい夜なら明るくなるまでもった割合 | %s | %s |\n", cell(func(t shelterRoomTally) float64 { return ratio(t.saved[0], t.night[0]) }), cell(func(t shelterRoomTally) float64 { return ratio(t.saved[1], t.night[1]) }))
	q := func(xs []float64, p float64) float64 {
		if len(xs) == 0 {
			return math.NaN()
		}
		return xs[int(p*float64(len(xs)-1))]
	}
	sort.Float64s(dist)
	fmt.Printf("\n夜の餓死の、明るさが落ち始めたときの位置から最寄りの象限の中心までの距離（タイル）: 中央値 %.0f / 25%% %.0f / 75%% %.0f\n\n", q(dist, 0.5), q(dist, 0.25), q(dist, 0.75))
}
