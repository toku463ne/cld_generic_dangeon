package engine

import "math"

// Sales between bodies (stage 11-3, the user's wish 2026-10-12 for a
// path from body to body besides the shop): a body that holds a unit of
// food or a folded tent may offer it to a body of its kind in sight that
// holds coins and could use it, for a price. There is no one to set the
// price, so the seller asks what the thing is worth to it in coins - the
// thing's worth (sellLoss: a meal, nothing for a unit that will rot in
// hand first, a tent's credit) over what it reads a coin at (coinWorth),
// rounded up - plus SellMargin coins; and the buyer takes it where the
// thing is worth more to it than the price, its coins read at what they
// buy for it. So the two read the same coins differently: a body that
// knows no shop, or has been turned away from an empty one, reads coins
// at little and pays more; a seller that knows no shop reads them at
// nothing and asks no price at all - it does not sell. The shop is the
// outside option in both readings. What the seller learns is how often
// its offers are taken: the trade row of stage 11-1b, shared with
// barter, since both are offers to a body in sight.

// The item a sale offers, in Action.Dir: a tent, or a unit of pool
// Dir-1.
const saleTent = 0

// saleItems calls f with each item body b could sell: the pools of the
// food it holds (each once) and a folded tent.
func (w *World) saleItems(b *Body, f func(item int)) {
	seen := 0
	for _, p := range b.HeldPool {
		if p < 62 && seen&(1<<p) == 0 {
			seen |= 1 << p
			f(p + 1)
		}
	}
	if w.cfg.Tents && b.Tent.Has && !b.Tent.Pitched && b.Tent.Until > w.tick {
		f(saleTent)
	}
	kinds := 0
	for _, k := range b.Ornaments {
		if k < 62 && kinds&(1<<k) == 0 {
			kinds |= 1 << k
			f(ornamentItem + k)
		}
	}
}

// itemLoss is what body b reads parting with item at, in grains (as
// sellLoss reads a thing sent to a shop).
func (w *World) itemLoss(b *Body, item int) int {
	if item == saleTent {
		return w.tentCredit(b)
	}
	if item >= ornamentItem {
		return w.ornamentLoss(b, item-ornamentItem)
	}
	k := heldOf(b, item-1)
	if k < 0 {
		return 0
	}
	if w.cfg.FoodLife > 0 {
		over := b.Energy - (w.maxOf(b) - w.cfg.FoodEnergy)
		if burn := w.planBurn(b); over > 0 && burn > 0 && b.HeldBorn[k]+int64(w.cfg.FoodLife)-w.tick <= int64(over/burn) {
			return 0
		}
	}
	return w.pred.meal
}

// askOf is the price body b asks for item, in coins: its worth to b over
// a coin's worth to b, rounded up, plus SellMargin; zero where b reads
// coins at nothing (it does not sell).
func (w *World) askOf(b *Body, item int) int {
	cw := w.coinWorth(b)
	if !(cw > 0) {
		return 0
	}
	return int(math.Ceil(float64(w.itemLoss(b, item))/cw-1e-9)) + w.cfg.SellMargin
}

// usesItem reports whether body o could take item: food it eats with
// room to hold it, or a tent where it has none and its kind has skills.
func (w *World) usesItem(o *Body, item int) bool {
	if item == saleTent {
		return !o.Tent.Has && !w.spec(o).NoSkills
	}
	if item >= ornamentItem {
		return w.wants(o, item-ornamentItem)
	}
	return w.eats(o, item-1) && o.Held < w.cfg.Carry
}

// buyersOf calls f for each body of b's kind in b's sight, awake, that
// holds coins and could take item.
func (w *World) buyersOf(b *Body, item int, f func(*Body)) {
	sight := w.sightOf(b)
	if sight < 0 || w.grid == nil {
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
				if o.ID != b.ID && sameSpecies(o, b) && o.Coins > 0 && !o.Asleep && w.usesItem(o, item) {
					f(o)
				}
			}
		}
	}
}

// mayOffer reports whether body b sells at all: sales are on, coins
// exist, and its kind takes coins.
func (w *World) mayOffer(b *Body) bool {
	return w.cfg.SellTo && w.cfg.Coins && !w.spec(b).NoCoins
}

// saleOptions appends an offer of each item body b could sell, at its
// asking price, to each body in sight that could take it.
func (w *World) saleOptions(dst []Action, b *Body) []Action {
	if !w.mayOffer(b) {
		return dst
	}
	w.saleItems(b, func(item int) {
		if w.askOf(b, item) == 0 {
			return
		}
		w.buyersOf(b, item, func(o *Body) { dst = append(dst, Action{Kind: ActSellTo, Mate: o.ID, Dir: item}) })
	})
	return dst
}

// sawBuyers hashes the bodies body b could sell to, so that one coming
// into sight makes it decide (the partner trigger).
func (w *World) sawBuyers(b *Body) uint64 {
	var h uint64
	if !w.mayOffer(b) {
		return 0
	}
	w.saleItems(b, func(item int) {
		if w.askOf(b, item) == 0 {
			return
		}
		w.buyersOf(b, item, func(o *Body) { h += mix(uint64(o.ID)^0x73616c65) + uint64(item) })
	})
	return h
}

// takesAt reports whether body p, offered item at price coins, buys: it
// holds the price, and the item is worth more to it than the coins, read
// at what they buy for it.
func (w *World) takesAt(p *Body, item, price int) bool {
	if p.Coins < price {
		return false
	}
	worth := w.pred.meal
	switch {
	case item == saleTent:
		worth = w.tentCredit(p)
	case item >= ornamentItem:
		worth = w.wantGrains() // the want, read only against the coins
	}
	return float64(worth) > float64(price)*w.coinWorth(p)
}

// sellTo has body b offer item to body id at its asking price: if the
// partner is still in sight and could take it, and takes it, the coins
// pass to b and the item to the partner (a unit with the tick it
// appeared, eaten as held food is; a tent as it was). The offer and its
// outcome go into b's trade row.
func (w *World) sellTo(b *Body, id int64, item int) {
	ask := w.askOf(b, item)
	if !w.mayOffer(b) || ask == 0 {
		return
	}
	var p *Body
	w.buyersOf(b, item, func(o *Body) {
		if o.ID == id {
			p = o
		}
	})
	if p == nil {
		return
	}
	has := false
	w.saleItems(b, func(i int) { has = has || i == item })
	if !has {
		return
	}
	if w.cfg.Learn {
		b.Memory.Offers++
		if w.cfg.AgeBand > 0 {
			w.observe(&b.Memory.Trade, 1, 0)
		}
	}
	if !w.takesAt(p, item, ask) {
		w.stats.SalesRefused++
		return
	}
	if w.cfg.Learn {
		b.Memory.Traded++
		if w.cfg.AgeBand > 0 {
			w.observe(&b.Memory.Trade, 0, 1)
		}
	}
	w.hand(b, p, item, ask)
}

// hand passes item from seller b to buyer p for price coins: a unit with
// the tick it appeared, eaten as held food is; a tent as it was; an
// ornament.
func (w *World) hand(b, p *Body, item, price int) {
	w.stats.Sales++
	w.stats.SalesCoins += int64(price)
	p.Coins -= price
	b.Coins += price
	if item == saleTent {
		p.Tent, b.Tent = Tent{Has: true, Until: b.Tent.Until, Uses: b.Tent.Uses}, Tent{}
		return
	}
	if item >= ornamentItem {
		dropOrnament(b, item-ornamentItem)
		p.Ornaments = append(p.Ornaments, item-ornamentItem)
		w.stats.OrnamentSales++
		return
	}
	k := heldOf(b, item-1)
	born := b.HeldBorn[k]
	b.HeldBorn = append(b.HeldBorn[:k], b.HeldBorn[k+1:]...)
	b.HeldPool = append(b.HeldPool[:k], b.HeldPool[k+1:]...)
	b.Held--
	p.Held++
	// Kept oldest first: a unit bought may be older than one held.
	at := len(p.HeldBorn)
	for at > 0 && p.HeldBorn[at-1] > born {
		at--
	}
	p.HeldBorn = append(p.HeldBorn[:at], append([]int64{born}, p.HeldBorn[at:]...)...)
	p.HeldPool = append(p.HeldPool[:at], append([]int{item - 1}, p.HeldPool[at:]...)...)
	w.eatHeld(p)
}
