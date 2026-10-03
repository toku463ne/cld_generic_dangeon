package main

import (
	"fmt"
	"math"

	"github.com/toku463ne/cld_generic_dangeon/engine"
	"github.com/toku463ne/cld_generic_dangeon/variant"
)

// knowEvery is how often, in ticks, the shelterknow count reads the world.
const knowEvery = 4000

type shelterKnowTally struct {
	// per reading: adults and children, those of them who know at least
	// one place to rest, and the places' tiles they know.
	adults, children, adultsKnow, childrenKnow, tiles []float64
}

// runShelterKnow steps one world and reads who knows a place to rest every
// knowEvery ticks. A world that collapses stops (its later readings are
// left out).
func runShelterKnow(cfg engine.Config, m engine.Map, ticks int) (shelterKnowTally, error) {
	var t shelterKnowTally
	w, err := engine.NewWorld(cfg, m)
	if err != nil {
		return t, err
	}
	for tick := 1; tick <= ticks; tick++ {
		w.Step()
		bs := w.Bodies()
		if tick%knowEvery == 0 {
			var a, c, ak, ck, tl float64
			for _, b := range bs {
				known := len(b.Memory.Shelters)
				if w.Tick() < b.Mature {
					c++
					if known > 0 {
						ck++
					}
					continue
				}
				a++
				tl += float64(known)
				if known > 0 {
					ak++
				}
			}
			t.adults, t.children = append(t.adults, a), append(t.children, c)
			t.adultsKnow, t.childrenKnow, t.tiles = append(t.adultsKnow, ak), append(t.childrenKnow, ck), append(t.tiles, tl)
		}
		if len(bs) <= lockCollapse {
			break
		}
	}
	return t, nil
}

// shelterKnow counts, through a run, how many adults and children know a
// place to rest: whether knowing where they are outlives the bodies that
// found them (stage 6-5a).
func shelterKnow(m engine.Map, name string, seeds int, seed0 int64, ticks int, vname string) {
	var ts []shelterKnowTally
	for s := 0; s < seeds; s++ {
		cfg, err := variant.Config(vname, seed0+int64(s))
		if err != nil {
			fail(err)
		}
		t, err := runShelterKnow(cfg, m, ticks)
		if err != nil {
			fail(err)
		}
		ts = append(ts, t)
	}
	fmt.Printf("地図 %s (%dx%d)、%d シード、条件 %s、%d tick。%d tick ごとに、休める場所を1つ以上覚えている成人・子の割合と、成人1体あたりの覚えているタイルの数。\n\n", name, m.Width, m.Height, seeds, vname, ticks, knowEvery)
	fmt.Printf("| tick | 覚えている成人の割合 | 覚えている子の割合 | 成人1体あたりのタイル |\n| --- | --- | --- | --- |\n")
	for i := 0; i < ticks/knowEvery; i++ {
		cell := func(f func(shelterKnowTally) float64) string {
			var xs []float64
			for _, t := range ts {
				if i < len(t.adults) {
					if x := f(t); !math.IsNaN(x) {
						xs = append(xs, x)
					}
				}
			}
			if len(xs) == 0 {
				return "—"
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
		fmt.Printf("| %d | %s | %s | %s |\n", (i+1)*knowEvery,
			cell(func(t shelterKnowTally) float64 { return ratio(t.adultsKnow[i], t.adults[i]) }),
			cell(func(t shelterKnowTally) float64 { return ratio(t.childrenKnow[i], t.children[i]) }),
			cell(func(t shelterKnowTally) float64 { return ratio(t.tiles[i], t.adults[i]) }))
	}
	fmt.Println()
}
