package main

import (
	"fmt"

	"github.com/toku463ne/cld_generic_dangeon/engine"
	"github.com/toku463ne/cld_generic_dangeon/variant"
)

type oldBirthsTally struct {
	// Births after tick 5000, and those with an old mother, an old father,
	// or either (a parent at OldAge or older on the tick of the birth).
	births, oldMother, oldFather, oldEither float64
	// Adult body-ticks after tick 5000 (sampled every 50 ticks), and those
	// of the old.
	adults, old float64
}

// runOldBirths reads how many births have an old parent: what a rule that
// the old cannot mate would take away.
func runOldBirths(cfg engine.Config, m engine.Map, ticks int) (oldBirthsTally, error) {
	var t oldBirthsTally
	w, err := engine.NewWorld(cfg, m)
	if err != nil {
		return t, err
	}
	old := func(b engine.Body, now int64) bool { return cfg.OldAge > 0 && now-b.Born >= int64(cfg.OldAge) }
	for tick := 1; tick <= ticks; tick++ {
		w.Step()
		if tick <= 5000 {
			continue
		}
		now := w.Tick()
		bs := w.Bodies()
		byID := make(map[int64]engine.Body, len(bs))
		for _, b := range bs {
			byID[b.ID] = b
		}
		for _, b := range bs {
			if b.Born != now || b.Parents[0] < 0 {
				continue
			}
			t.births++
			var mo, fa bool
			for _, id := range b.Parents {
				p, ok := byID[id]
				if !ok || !old(p, now) {
					continue
				}
				if p.Sex == engine.Male {
					fa = true
				} else {
					mo = true
				}
			}
			if mo {
				t.oldMother++
			}
			if fa {
				t.oldFather++
			}
			if mo || fa {
				t.oldEither++
			}
		}
		if tick%50 == 0 {
			for _, b := range bs {
				if now >= b.Mature {
					t.adults++
					if old(b, now) {
						t.old++
					}
				}
			}
		}
	}
	return t, nil
}

// oldBirths counts, before the old are kept from mating, the births with an
// old parent and the share of adults who are old.
func oldBirths(m engine.Map, name string, seeds int, seed0 int64, ticks int, vname string) {
	var ts []oldBirthsTally
	var oldAge int
	for s := 0; s < seeds; s++ {
		cfg, err := variant.Config(vname, seed0+int64(s))
		if err != nil {
			fail(err)
		}
		oldAge = cfg.OldAge
		t, err := runOldBirths(cfg, m, ticks)
		if err != nil {
			fail(err)
		}
		ts = append(ts, t)
	}
	fmt.Printf("地図 %s (%dx%d)、%d シード、条件 %s、%d tick（tick 5000 より後）。老人＝年齢 %d 以上。\n\n", name, m.Width, m.Height, seeds, vname, ticks, oldAge)
	cell := func(f func(oldBirthsTally) float64, prec int) string {
		var xs []float64
		for _, t := range ts {
			xs = append(xs, f(t))
		}
		mm, se := meanSE(xs)
		return fmt.Sprintf("%.*f ± %.*f", prec, mm, prec, se)
	}
	fmt.Printf("| 量 | 値 |\n| --- | --- |\n")
	fmt.Printf("| 出生（1シードあたり） | %s |\n", cell(func(t oldBirthsTally) float64 { return t.births }, 0))
	fmt.Printf("| 母が老人の出生の割合 | %s |\n", cell(func(t oldBirthsTally) float64 { return t.oldMother / t.births }, 4))
	fmt.Printf("| 父が老人の出生の割合 | %s |\n", cell(func(t oldBirthsTally) float64 { return t.oldFather / t.births }, 4))
	fmt.Printf("| どちらかが老人の出生の割合 | %s |\n", cell(func(t oldBirthsTally) float64 { return t.oldEither / t.births }, 4))
	fmt.Printf("| 成人のうち老人の割合（50 tick ごと） | %s |\n\n", cell(func(t oldBirthsTally) float64 { return t.old / t.adults }, 4))
}
