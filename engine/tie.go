package engine

import "math"

// Ties (stage 12-3, VISION "点数・印象値・好感度・脅威度・物の値打ちは、知識
// から読み出す「表示」とすること" and "自分にとっての脅威を排除された発信を受け
// 取り、記憶すること（これはそれを行ったノードへの好感度の上昇につながる）"):
// a body keeps, for each body it has had dealings with, what it gained by
// them - in energy, as it read the gain at the time - and when it last
// did. How well it likes a body is read off that row, aged by
// EvidenceHalfLife as evidence is (Liking): a display, not a score kept.
// What goes into the row:
//   - a hunter struck down by that body, if the hunter was a threat to
//     this one - seen in its sight, or striking it, within the last
//     EvidenceHalfLife ticks - as one blow saved (HitEnergy). A hunter
//     that never threatened it is nothing to it (VISION_CHECKS: the call
//     tells who did it; whether it mattered is the hearer's own
//     observation);
//   - a trade, sale or barter with that body: the thing's worth less the
//     coins' (or the reverse for a seller), as it read them.
// A body keeps TieMemory rows at most, the weakest dropped. No rule reads
// the rows yet: telling (stage 12-4) and helping (12-5) will.

// Tie is what a body gained by its dealings with another, in energy, as
// of tick At.
type Tie struct {
	Gain float64 `json:",omitempty"`
	At   int64   `json:",omitempty"`
}

// SeenHunter is a hunter a body saw, or was struck by, at tick At.
type SeenHunter struct {
	ID int64 `json:",omitempty"`
	At int64 `json:",omitempty"`
}

// aged is tie t's gain read at tick now.
func (w *World) aged(t Tie, now int64) float64 {
	if w.cfg.EvidenceHalfLife <= 0 || now <= t.At {
		return t.Gain
	}
	return t.Gain * math.Exp2(-float64(now-t.At)/float64(w.cfg.EvidenceHalfLife))
}

// Liking is how well body b likes the body id, read off its row: the
// energy it gained by it, aged. Zero for a stranger.
func (w *World) Liking(b Body, id int64) float64 {
	t, ok := b.Memory.Ties[id]
	if !ok {
		return 0
	}
	return w.aged(t, w.tick)
}

// tie adds gain (energy) to body b's row of body id.
func (w *World) tie(b *Body, id int64, gain float64) {
	if !w.cfg.Ties || id == b.ID || gain == 0 {
		return
	}
	if b.Memory.Ties == nil {
		b.Memory.Ties = map[int64]Tie{}
	}
	t := b.Memory.Ties[id]
	b.Memory.Ties[id] = Tie{Gain: w.aged(t, w.tick) + gain, At: w.tick}
	w.stats.TiesNoted++
	if len(b.Memory.Ties) > w.cfg.TieMemory {
		// Drop the weakest, the lowest ID on a tie (maps have no order).
		weak, wv := int64(-1), math.Inf(1)
		for k, v := range b.Memory.Ties {
			if a := math.Abs(w.aged(v, w.tick)); a < wv || a == wv && k < weak {
				weak, wv = k, a
			}
		}
		delete(b.Memory.Ties, weak)
	}
}

// sawHunter has body b note hunter id as a threat now.
func (w *World) sawHunter(b *Body, id int64) {
	if !w.cfg.Ties {
		return
	}
	for k := range b.Memory.Hunters {
		if b.Memory.Hunters[k].ID == id {
			b.Memory.Hunters[k].At = w.tick
			return
		}
	}
	b.Memory.Hunters = append(b.Memory.Hunters, SeenHunter{ID: id, At: w.tick})
	if len(b.Memory.Hunters) > 4 {
		b.Memory.Hunters = b.Memory.Hunters[1:] // the oldest noted goes
	}
}

// noteHunters has body b note the hunters in its sight.
func (w *World) noteHunters(b *Body) {
	if !w.cfg.Ties {
		return
	}
	w.hunters(b, func(o *Body) { w.sawHunter(b, o.ID) })
}

// threatenedBy reports whether hunter id was a threat to body b within
// the last EvidenceHalfLife ticks.
func (w *World) threatenedBy(b *Body, id int64) bool {
	for _, h := range b.Memory.Hunters {
		if h.ID == id && w.tick-h.At <= int64(w.cfg.EvidenceHalfLife) {
			return true
		}
	}
	return false
}

// tieTrade adds to buyer p's and seller b's rows of each other what each
// gained by item passing from b to p for price coins, as each reads it.
func (w *World) tieTrade(b, p *Body, item, price int) {
	if !w.cfg.Ties {
		return
	}
	worth := float64(w.pred.meal)
	switch {
	case item == saleTent:
		worth = float64(w.tentCredit(p))
	case item >= ornamentItem:
		worth = float64(w.wantGrains())
	}
	g := w.cfg.Grain
	w.tie(p, b.ID, (worth-float64(price)*w.coinWorth(p))*g)
	w.tie(b, p.ID, (float64(price)*w.coinWorth(b)-float64(w.itemLoss(b, item)))*g)
}
