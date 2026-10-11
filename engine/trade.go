package engine

import "math"

// Barter (stage 11-1, VISION "ノード同士で物々交換をすること"): a body
// that holds a tent and has room for a meal may offer the tent to a body
// of its kind in sight that holds food and has no tent, for one unit.
// Each side reads the trade with the worth it already reads things at -
// a unit is a meal (predict.go), a tent is the nights it expects to use
// one times a night's saving (tentCredit) - so the two value the same
// things differently by their state and what they have learned, and a
// trade happens only where both come out ahead by their own reading:
// the offerer takes the trade option where the meal now outweighs the
// tent's credit, and the holder of the food agrees where the tent's
// credit, as it reads it, outweighs a meal. Nothing is learned of the
// trade itself: the things traded are read as they always are (the
// coin of stage 11-2 is what has to be learned). A body that has felt
// neither the cold nor the warmth of a place to rest reads a tent as
// worth nothing and refuses: the difference in experience is what makes
// a trade.

// traders calls f for each body of b's kind in b's sight that holds food
// and no tent, awake: those b may offer its tent to.
func (w *World) traders(b *Body, f func(*Body)) {
	sight := w.sightOf(b)
	if !w.cfg.Barter || !w.cfg.Tents || sight < 0 || w.grid == nil {
		return
	}
	bx, by := int(math.Floor(b.X)), int(math.Floor(b.Y))
	for y := by - sight; y <= by+sight; y++ {
		for x := bx - sight; x <= bx+sight; x++ {
			if !w.m.InBounds(x, y) || !w.seesFrom(b, bx, by, x, y) {
				continue
			}
			for _, j := range w.grid[w.m.index(x, y)] {
				o := &w.bodies[j]
				if o.ID != b.ID && sameSpecies(o, b) && o.Held > 0 && !o.Tent.Has && !o.Asleep {
					f(o)
				}
			}
		}
	}
}

// canOffer reports whether body b has a tent to offer and room for the
// meal it would get: a folded tent, and a meal that fits.
func (w *World) canOffer(b *Body) bool {
	return w.cfg.Barter && b.Tent.Has && !b.Tent.Pitched && b.Energy <= w.maxOf(b)-w.cfg.FoodEnergy
}

// tradeOptions appends the trade options body b has: its tent for a unit
// of each trader in sight.
func (w *World) tradeOptions(dst []Action, b *Body) []Action {
	if !w.canOffer(b) {
		return dst
	}
	w.traders(b, func(o *Body) { dst = append(dst, Action{Kind: ActTrade, Mate: o.ID}) })
	return dst
}

// sawTraders hashes the traders body b could offer its tent to, so that
// one coming into sight makes it decide (the partner trigger).
func (w *World) sawTraders(b *Body) uint64 {
	var h uint64
	if !w.canOffer(b) {
		return 0
	}
	w.traders(b, func(o *Body) { h += mix(uint64(o.ID) ^ 0x7261646572) })
	return h
}

// accepts reports whether body p, offered a tent for one of its units,
// takes it: the tent's credit as p reads it outweighs a meal.
func (w *World) accepts(p *Body) bool {
	return w.tentCredit(p) > w.pred.meal
}

// trade has body b offer its tent to body id for a unit: if the partner
// is still a trader in sight and agrees, the tent passes to it (folded,
// wearing out when it would have) and the oldest unit it holds passes to
// b, which eats it as it eats what it holds.
func (w *World) trade(b *Body, id int64) {
	var p *Body
	w.traders(b, func(o *Body) {
		if o.ID == id {
			p = o
		}
	})
	if p == nil || !w.canOffer(b) {
		return
	}
	if !w.accepts(p) {
		w.stats.TradesRefused++
		return
	}
	w.stats.Trades++
	p.Tent, b.Tent = Tent{Has: true, Until: b.Tent.Until, Uses: b.Tent.Uses}, Tent{}
	born := p.HeldBorn[0]
	pool := w.dropHeld(p)
	b.Held++
	b.HeldBorn = append(b.HeldBorn, born)
	b.HeldPool = append(b.HeldPool, pool)
	w.eatHeld(b)
}
