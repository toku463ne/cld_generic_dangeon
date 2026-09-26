package main

import (
	"fmt"

	"github.com/toku463ne/cld_generic_dangeon/engine"
	"github.com/toku463ne/cld_generic_dangeon/variant"
)

// payersTally is read by sex (index Female-1, Male-1) after tick 5000.
type payersTally struct {
	// Adults that died: how many, how many within one full-to-starving span
	// of their last payment, how many never paid, and the payments of all.
	died, diedSoon, diedNever, paid [2]float64
	// Adult body-ticks, and payments made in them.
	adultTicks, payments [2]float64
	// Energy gained or lost by adults over ticks they paid nothing, summed,
	// and those ticks: the spare energy an adult has to pay with.
	drift, driftTicks [2]float64
	// After a payment, the ticks until energy was back where it was before
	// (recovered), or the payment was followed by death or another payment
	// first (cut).
	recovered []float64
	cut       [2]float64
}

// runPayers follows every adult of a world with sexes: when it paid for a
// child, how its energy came back, and how it died.
func runPayers(cfg engine.Config, m engine.Map, ticks int) (payersTally, error) {
	var t payersTally
	w, err := engine.NewWorld(cfg, m)
	if err != nil {
		return t, err
	}
	burnSpan := int64(cfg.EnergyMax / cfg.EnergyBurn)
	type state struct {
		last     int64   // tick of the last payment, -1 for none
		payments float64 // payments made
		target   float64 // energy before the last payment, while it is not yet back
		since    int64   // tick of the payment being recovered from, -1 for none
	}
	st := map[int64]*state{}
	prev := map[int64]engine.Body{}
	for tick := 1; tick <= ticks; tick++ {
		w.Step()
		now := w.Tick()
		cur := map[int64]engine.Body{}
		paidNow := map[int64]bool{}
		for _, b := range w.Bodies() {
			cur[b.ID] = b
			if _, ok := prev[b.ID]; !ok && b.Born == now && b.Parents[0] >= 0 {
				paidNow[b.Parents[0]], paidNow[b.Parents[1]] = true, true
			}
		}
		for id, b := range cur {
			if b.Sex == engine.NoSex || now < b.Mature {
				continue
			}
			s := st[id]
			if s == nil {
				s = &state{last: -1, since: -1}
				st[id] = s
			}
			k := int(b.Sex) - 1
			p, had := prev[id]
			if paidNow[id] {
				if s.since >= 0 && tick > 5000 {
					t.cut[k]++ // paid again before it was back
				}
				s.last, s.since = now, now
				s.payments++
				if had {
					s.target = p.Energy
				}
				if tick > 5000 {
					t.payments[k]++
				}
			} else if had && tick > 5000 {
				t.drift[k] += b.Energy - p.Energy
				t.driftTicks[k]++
			}
			if tick > 5000 {
				t.adultTicks[k]++
			}
			if s.since >= 0 && s.since != now && b.Energy >= s.target {
				if tick > 5000 {
					t.recovered = append(t.recovered, float64(now-s.since))
				}
				s.since = -1
			}
		}
		for id, b := range prev {
			if _, ok := cur[id]; ok {
				continue
			}
			s := st[id]
			delete(st, id)
			if tick <= 5000 || b.Sex == engine.NoSex || now < b.Mature || s == nil {
				continue
			}
			k := int(b.Sex) - 1
			t.died[k]++
			t.paid[k] += s.payments
			switch {
			case s.last < 0:
				t.diedNever[k]++
			case now-s.last <= burnSpan:
				t.diedSoon[k]++
			}
			if s.since >= 0 {
				t.cut[k]++ // died before it was back
			}
		}
		prev = cur
	}
	return t, nil
}

// payers counts, before stage 3-2, how paying for children kills adults, by
// sex, and what an adult can afford: its spare energy per tick, and how long
// its energy takes to come back after a payment.
func payers(m engine.Map, name string, seeds int, seed0 int64, ticks int, vname string) {
	cfg0, err := variant.Config(vname, seed0)
	if err != nil {
		fail(err)
	}
	fmt.Printf("地図 %s (%dx%d)、%d シード、条件 %s、%d tick（tick 5000 より後）。成人の身体を性別ごとに読む。代金は子が生まれた tick に両親が払う %.0f ずつ。「すぐに死んだ」は最後の代金から %d tick（満腹から餓死まで）以内。\n\n", name, m.Width, m.Height, seeds, vname, ticks, cfg0.BirthEnergy/2, int(cfg0.EnergyMax/cfg0.EnergyBurn))
	var ts []payersTally
	var rec []float64
	for s := 0; s < seeds; s++ {
		cfg, err := variant.Config(vname, seed0+int64(s))
		if err != nil {
			fail(err)
		}
		t, err := runPayers(cfg, m, ticks)
		if err != nil {
			fail(err)
		}
		ts = append(ts, t)
		rec = append(rec, t.recovered...)
	}
	cell := func(f func(payersTally) float64, prec int) string {
		var xs []float64
		for _, t := range ts {
			xs = append(xs, f(t))
		}
		mm, se := meanSE(xs)
		return fmt.Sprintf("%.*f ± %.*f", prec, mm, prec, se)
	}
	fmt.Printf("| 量 | 女 | 男 |\n| --- | --- | --- |\n")
	row := func(name string, prec int, f func(t payersTally, k int) float64) {
		fmt.Printf("| %s | %s | %s |\n", name, cell(func(t payersTally) float64 { return f(t, 0) }, prec), cell(func(t payersTally) float64 { return f(t, 1) }, prec))
	}
	row("死んだ成人（1シードあたり）", 0, func(t payersTally, k int) float64 { return t.died[k] })
	row("そのうち最後の代金から満腹から餓死まで以内に死んだ割合", 4, func(t payersTally, k int) float64 { return t.diedSoon[k] / t.died[k] })
	row("そのうち一度も払わなかった割合", 4, func(t payersTally, k int) float64 { return t.diedNever[k] / t.died[k] })
	row("死んだ成人が一生に払った回数", 3, func(t payersTally, k int) float64 { return t.paid[k] / t.died[k] })
	row("成人 1000 tick あたりの代金の回数", 3, func(t payersTally, k int) float64 { return 1000 * t.payments[k] / t.adultTicks[k] })
	row("払わなかった tick の体力の増減（1 tick あたり）", 4, func(t payersTally, k int) float64 { return t.drift[k] / t.driftTicks[k] })
	row("払ったあと、戻る前にまた払うか死んだ回数（1シードあたり）", 0, func(t payersTally, k int) float64 { return t.cut[k] })
	fmt.Printf("\n払ったあと体力が払う前の水準に戻るまでの tick（戻ったものだけ、全シード、25%%・50%%・75%%・95%%）: %s（%d 回）\n\n", quantiles(rec, 0.25, 0.5, 0.75, 0.95), len(rec))
}
