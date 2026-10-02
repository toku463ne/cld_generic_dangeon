package main

import (
	"fmt"
	"strings"
	"sync"

	"github.com/toku463ne/cld_generic_dangeon/engine"
	"github.com/toku463ne/cld_generic_dangeon/variant"
)

// lockCollapse is the population at or below which a world counts as
// collapsed (MEASURE.md).
const lockCollapse = 20

// landParts labels each land tile with the land component it belongs to
// (eight neighbours, as bodies move), -1 for water. Bodies cannot cross
// water, so a component is a place bodies can never come back to once none
// is left in it.
func landParts(m engine.Map) ([]int, int) {
	part := make([]int, m.Width*m.Height)
	for i := range part {
		part[i] = -1
	}
	n := 0
	for y := 0; y < m.Height; y++ {
		for x := 0; x < m.Width; x++ {
			i := y*m.Width + x
			if part[i] >= 0 || m.TerrainAt(x, y) != engine.TerrainLand {
				continue
			}
			stack := []int{i}
			part[i] = n
			for len(stack) > 0 {
				t := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				tx, ty := t%m.Width, t/m.Width
				for dy := -1; dy <= 1; dy++ {
					for dx := -1; dx <= 1; dx++ {
						ux, uy := tx+dx, ty+dy
						if !m.InBounds(ux, uy) || m.TerrainAt(ux, uy) != engine.TerrainLand {
							continue
						}
						if u := uy*m.Width + ux; part[u] < 0 {
							part[u] = n
							stack = append(stack, u)
						}
					}
				}
			}
			n++
		}
	}
	return part, n
}

type lockSample struct {
	tick                  int
	bodies, births, deads int
	partBodies, partFood  []int
}

type lockTally struct {
	seed     int64
	collapse int // tick the world fell to lockCollapse, 0 for never
	// emptied is the first tick (sampled every 100) a land component held
	// no body, 0 for never; lockedMax the most food on the ground in
	// components with no body, and lockedAt the tick it was seen.
	emptied, lockedMax, lockedAt int
	samples                      []lockSample // every 1000 ticks
}

// runLock steps one world, sampling bodies and food per land component.
func runLock(cfg engine.Config, m engine.Map, ticks int, part []int, nparts int, seed int64) (lockTally, error) {
	t := lockTally{seed: seed}
	w, err := engine.NewWorld(cfg, m)
	if err != nil {
		return t, err
	}
	var pb, pd int64
	for tick := 1; tick <= ticks; tick++ {
		w.Step()
		if tick%100 != 0 {
			continue
		}
		bs := w.Bodies()
		pbod := make([]int, nparts)
		pfood := make([]int, nparts)
		for _, b := range bs {
			pbod[part[int(b.Y)*m.Width+int(b.X)]]++
		}
		for _, f := range w.Foods() {
			pfood[part[f.Y*m.Width+f.X]]++
		}
		locked := 0
		for p := 0; p < nparts; p++ {
			if pbod[p] == 0 {
				locked += pfood[p]
				if t.emptied == 0 {
					t.emptied = tick
				}
			}
		}
		if locked > t.lockedMax {
			t.lockedMax, t.lockedAt = locked, tick
		}
		if tick%1000 == 0 {
			st := w.Stats()
			dead := st.Deaths[engine.CauseStarved] + st.Deaths[engine.CauseAged]
			t.samples = append(t.samples, lockSample{tick, len(bs), int(st.Births - pb), int(dead - pd), pbod, pfood})
			pb, pd = st.Births, dead
		}
		if len(bs) <= lockCollapse {
			t.collapse = tick
			break
		}
	}
	return t, nil
}

// lock counts, per seed, whether a land component was left with no body,
// how much food then lay there where no body could eat it, and whether the
// world collapsed. Seeds run in parallel; the output is in seed order.
func lock(m engine.Map, name string, seeds int, seed0 int64, ticks int, vname string) {
	part, nparts := landParts(m)
	land := make([]int, nparts)
	for _, p := range part {
		if p >= 0 {
			land[p]++
		}
	}
	ts := make([]lockTally, seeds)
	errs := make([]error, seeds)
	var wg sync.WaitGroup
	sem := make(chan bool, 10)
	for s := 0; s < seeds; s++ {
		wg.Add(1)
		go func(s int) {
			defer wg.Done()
			sem <- true
			defer func() { <-sem }()
			seed := seed0 + int64(s)
			cfg, err := variant.Config(vname, seed)
			if err != nil {
				errs[s] = err
				return
			}
			ts[s], errs[s] = runLock(cfg, m, ticks, part, nparts, seed)
		}(s)
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			fail(err)
		}
	}
	cap := 0
	if cfg, err := variant.Config(vname, seed0); err == nil {
		cap = cfg.FoodCap
	}
	sizes := make([]string, nparts)
	for p := range land {
		sizes[p] = fmt.Sprint(land[p])
	}
	fmt.Printf("地図 %s (%dx%d)、%d シード（%d〜）、条件 %s、%d tick（100 tick ごと）。陸の連結成分 %d 個（陸タイル %s）、食料の上限 %d。崩壊＝個体数 %d 以下。\n\n", name, m.Width, m.Height, seeds, seed0, vname, ticks, nparts, strings.Join(sizes, " / "), cap, lockCollapse)

	var both, onlyEmpty, onlyCollapse, neither int
	var lead []float64
	fmt.Printf("| シード | 身体のいない成分が出た tick | そこに積もった食料の最大（tick） | 崩壊した tick |\n| --- | --- | --- | --- |\n")
	for _, t := range ts {
		e, c := "—", "—"
		if t.emptied > 0 {
			e = fmt.Sprint(t.emptied)
		}
		if t.collapse > 0 {
			c = fmt.Sprint(t.collapse)
		}
		switch {
		case t.emptied > 0 && t.collapse > 0:
			both++
			lead = append(lead, float64(t.collapse-t.emptied))
		case t.emptied > 0:
			onlyEmpty++
		case t.collapse > 0:
			onlyCollapse++
		default:
			neither++
		}
		if t.emptied == 0 && t.collapse == 0 {
			continue
		}
		fmt.Printf("| %d | %s | %d（%d） | %s |\n", t.seed, e, t.lockedMax, t.lockedAt, c)
	}
	fmt.Printf("\n（どちらも起きなかったシードは表から省いた。）\n\n")
	fmt.Printf("| | 崩壊した | 崩壊しなかった |\n| --- | --- | --- |\n| 身体のいない成分が出た | %d | %d |\n| 出なかった | %d | %d |\n\n", both, onlyEmpty, onlyCollapse, neither)
	if len(lead) > 0 {
		mm, se := meanSE(lead)
		fmt.Printf("成分が空になってから崩壊までの tick: %.0f ± %.0f\n\n", mm, se)
	}

	// The time series of the first seed that collapsed and of the first
	// that did not, side by side in shape.
	shown := map[bool]bool{}
	for _, t := range ts {
		k := t.collapse > 0
		if shown[k] {
			continue
		}
		shown[k] = true
		fmt.Printf("シード %d の推移（1000 tick ごと。出生・死は直前の 1000 tick）\n\n", t.seed)
		fmt.Printf("| tick | 個体数 | 出生 | 死 |")
		for p := 0; p < nparts; p++ {
			fmt.Printf(" 成分 %d の身体 / 食料 |", p)
		}
		fmt.Printf("\n| --- | --- | --- | --- |%s\n", strings.Repeat(" --- |", nparts))
		for _, s := range t.samples {
			fmt.Printf("| %d | %d | %d | %d |", s.tick, s.bodies, s.births, s.deads)
			for p := 0; p < nparts; p++ {
				fmt.Printf(" %d / %d |", s.partBodies[p], s.partFood[p])
			}
			fmt.Println()
		}
		fmt.Println()
	}
}
