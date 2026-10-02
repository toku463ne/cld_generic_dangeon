package main

import (
	"fmt"
	"math"

	"github.com/toku463ne/cld_generic_dangeon/engine"
	"github.com/toku463ne/cld_generic_dangeon/variant"
)

type carryRoomTally struct {
	// Decisions after tick 5000, per group (tiesGroups).
	decisions [3]float64
	// Of those: a meal does not fit (energy above the most less FoodEnergy),
	// and with food underfoot, or in sight but not underfoot.
	full, fullUnder, fullSight [3]float64
	// Of the full decisions with food underfoot or in sight: the best
	// option's risk is above zero, so a unit kept for later could lower it.
	fullRisk [3]float64
	// Units in sight, summed over full decisions with food in sight.
	seen [3]float64
	// Body-ticks after tick 5000 (sampled every 50 ticks) per group, and
	// those with a meal not fitting.
	ticks, ticksFull [3]float64
}

// runCarryRoom reads every decision after tick 5000 from the trace, and the
// bodies every 50 ticks.
func runCarryRoom(cfg engine.Config, m engine.Map, ticks int) (carryRoomTally, error) {
	var t carryRoomTally
	w, err := engine.NewWorld(cfg, m)
	if err != nil {
		return t, err
	}
	group := func(b engine.Body, now int64) int {
		switch {
		case b.Sex == engine.Female && now < b.Rested:
			return 0
		case now >= b.Mature:
			return 1
		}
		return 2
	}
	full := func(b engine.Body) bool { return b.Energy > w.MaxEnergy(b)-cfg.FoodEnergy }
	w.SetTrace(func(b engine.Body, v engine.Valuation, a engine.Action) {
		now := w.Tick()
		if now <= 5000 || len(v.Score) == 0 {
			return
		}
		g := group(b, now)
		t.decisions[g]++
		if !full(b) {
			return
		}
		t.full[g]++
		under := false
		for _, o := range v.Options {
			if o.Kind == engine.ActEat {
				under = true
			}
		}
		switch {
		case under:
			t.fullUnder[g]++
		case len(v.Seen) > 0:
			t.fullSight[g]++
			t.seen[g] += float64(len(v.Seen))
		default:
			return
		}
		best, risk := math.Inf(1), 0.0
		for j, s := range v.Score {
			if s < best {
				best, risk = s, v.Risk[0][j]
			}
		}
		if risk > 0 {
			t.fullRisk[g]++
		}
	})
	for tick := 1; tick <= ticks; tick++ {
		w.Step()
		if tick <= 5000 || tick%50 != 0 {
			continue
		}
		now := w.Tick()
		for _, b := range w.Bodies() {
			g := group(b, now)
			t.ticks[g]++
			if full(b) {
				t.ticksFull[g]++
			}
		}
	}
	return t, nil
}

// carryRoom counts, before stage 4-1 lets bodies carry food, how often a
// body that could not eat a whole meal has food underfoot or in sight: the
// room carrying has to change anything.
func carryRoom(m engine.Map, name string, seeds int, seed0 int64, ticks int, vname string) {
	var ts []carryRoomTally
	var foodEnergy float64
	for s := 0; s < seeds; s++ {
		cfg, err := variant.Config(vname, seed0+int64(s))
		if err != nil {
			fail(err)
		}
		foodEnergy = cfg.FoodEnergy
		t, err := runCarryRoom(cfg, m, ticks)
		if err != nil {
			fail(err)
		}
		ts = append(ts, t)
	}
	fmt.Printf("地図 %s (%dx%d)、%d シード、条件 %s、%d tick（tick 5000 より後）。満腹に近い＝1食（%.0f）が入りきらない（体力 > 体力の上限 − %.0f）。\n\n", name, m.Width, m.Height, seeds, vname, ticks, foodEnergy, foodEnergy)
	cell := func(f func(carryRoomTally) float64, prec int) string {
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
	fmt.Printf("| 量 | %s | %s | %s |\n| --- | --- | --- | --- |\n", tiesGroups[0], tiesGroups[1], tiesGroups[2])
	row := func(label string, f func(t carryRoomTally, g int) float64, prec int) {
		fmt.Printf("| %s |", label)
		for g := range tiesGroups {
			fmt.Printf(" %s |", cell(func(t carryRoomTally) float64 { return f(t, g) }, prec))
		}
		fmt.Println()
	}
	row("満腹に近い時間の割合（50 tick ごと）", func(t carryRoomTally, g int) float64 { return ratio(t.ticksFull[g], t.ticks[g]) }, 4)
	row("決定（1シードあたり）", func(t carryRoomTally, g int) float64 { return t.decisions[g] }, 0)
	row("満腹に近い決定の割合", func(t carryRoomTally, g int) float64 { return ratio(t.full[g], t.decisions[g]) }, 4)
	row("満腹に近く、足元に食料がある決定の割合", func(t carryRoomTally, g int) float64 { return ratio(t.fullUnder[g], t.decisions[g]) }, 4)
	row("満腹に近く、視界にだけ食料がある決定の割合", func(t carryRoomTally, g int) float64 { return ratio(t.fullSight[g], t.decisions[g]) }, 4)
	row("　そのときの視界の食料（平均）", func(t carryRoomTally, g int) float64 { return ratio(t.seen[g], t.fullSight[g]) }, 2)
	row("満腹に近く食料が見える決定のうち、最善の手の死ぬ確率が 0 より大きい割合", func(t carryRoomTally, g int) float64 {
		return ratio(t.fullRisk[g], t.fullUnder[g]+t.fullSight[g])
	}, 4)
	row("満腹に近く食料が見え、死ぬ確率が 0 より大きい決定（1シードあたり）", func(t carryRoomTally, g int) float64 { return t.fullRisk[g] }, 0)
	fmt.Println()
}
