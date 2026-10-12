package engine

import "math"

// Calls to sell (stage 12-1, VISION "発信ができること（…物を売っている…）"
// and "商売を行う人は、何を販売するか発信する…声を出す"): a body that
// holds a thing it would sell (sale.go) - a unit of food or a folded tent
// - and sees no one who would buy it calls out what it sells and at what
// price, the price it would ask in sight. It spends no turn, as a request
// to mate does not, and the call reaches RequestRange tiles and lasts
// RequestTicks, or until the thing has left its hands. A body out of its
// sight within the range that would take the thing at that price (the
// buyer's reading of stage 11-3) may walk to the caller, and the call is
// the caller's agreement: once in sight, the buyer pays the price and
// takes the thing (VISION: "発信そのものが同意とみなされ、受け手が到着して
// 申し出れば成立", said of mating and made the same here). The buyer reads
// the walk as the thing after the ticks of walking: a meal, or a tent's
// credit. Ornaments are not called: the want for one is read only against
// coins, never in the survival reading a walk is valued by.

// SaleCall is a body's open call to sell: the item (sale.go) at Price
// coins, until tick Until.
type SaleCall struct {
	Until int64 `json:",omitempty"`
	Item  int   `json:",omitempty"`
	Price int   `json:",omitempty"`
}

// callable reports whether item is a thing a body may call to sell.
func callable(item int) bool { return item < ornamentItem }

// callSale has body b call out a thing to sell if it has one, sees no one
// who would take it, and has no call open for something it still holds.
func (w *World) callSale(b *Body) {
	if !w.cfg.SaleCalls || !w.mayOffer(b) || b.Asleep {
		return
	}
	if b.Selling.Until > w.tick && w.stillHas(b, b.Selling.Item) {
		return
	}
	b.Selling = SaleCall{}
	w.saleItems(b, func(item int) {
		if b.Selling.Until > w.tick || !callable(item) {
			return
		}
		ask := w.askOf(b, item)
		if ask == 0 {
			return
		}
		seen := false
		w.buyersOf(b, item, func(o *Body) { seen = seen || w.takesAt(o, item, ask) })
		if seen {
			return // it offers in sight instead (stage 11-3)
		}
		b.Selling = SaleCall{Until: w.tick + int64(w.cfg.RequestTicks), Item: item, Price: ask}
		w.stats.SaleCalls++
		w.calling = append(w.calling, w.indexOf(b))
	})
}

// stillHas reports whether body b still holds item.
func (w *World) stillHas(b *Body, item int) bool {
	has := false
	w.saleItems(b, func(i int) { has = has || i == item })
	return has
}

// listCalling lists the bodies with a call to sell open, at the start of
// a tick.
func (w *World) listCalling() {
	w.calling = w.calling[:0]
	if !w.cfg.SaleCalls {
		return
	}
	for i := range w.bodies {
		if w.bodies[i].Selling.Until > w.tick {
			w.calling = append(w.calling, i)
		}
	}
}

// callers calls f with every body of b's kind within RequestRange of it,
// awake, whose call to sell is open for a thing it still holds and b
// would take at the price called - out of b's sight (far) or in it.
func (w *World) callers(b *Body, f func(o *Body, far bool)) {
	if !w.cfg.SaleCalls {
		return
	}
	bx, by := int(math.Floor(b.X)), int(math.Floor(b.Y))
	for _, i := range w.calling {
		o := &w.bodies[i]
		s := o.Selling
		if s.Until <= w.tick || o.ID == b.ID || !sameSpecies(o, b) || o.Asleep || !w.stillHas(o, s.Item) {
			continue
		}
		ox, oy := int(math.Floor(o.X)), int(math.Floor(o.Y))
		d := max(abs(ox-bx), abs(oy-by))
		if d > w.cfg.RequestRange || !w.usesItem(b, s.Item) || !w.takesAt(b, s.Item, s.Price) {
			continue
		}
		f(o, d > w.sightOf(b) || !w.seesFrom(b, bx, by, ox, oy))
	}
}

// callOptions appends a buy from each caller body b would buy from; one
// out of sight is walked to first. It notes the ticks of each walk.
func (w *World) callOptions(dst []Action, b *Body) []Action {
	clear(w.toCaller)
	w.callers(b, func(o *Body, far bool) {
		dst = append(dst, Action{Kind: ActBuyFrom, Mate: o.ID, Dir: o.Selling.Item})
		if !far {
			return
		}
		if w.toCaller == nil {
			w.toCaller = map[int64]int{}
		}
		gap := math.Max(math.Abs(o.X-b.X), math.Abs(o.Y-b.Y)) - float64(w.sightOf(b))
		w.toCaller[o.ID] = int(math.Ceil(math.Max(gap, 0) / w.speedOf(b)))
	})
	return dst
}

// sawCallers hashes the callers body b would buy from, so that a call
// coming in range makes it decide (the partner trigger).
func (w *World) sawCallers(b *Body) uint64 {
	var h uint64
	w.callers(b, func(o *Body, _ bool) { h += mix(uint64(o.ID) ^ 0x63616c6c) })
	return h
}

// callGain is what body b reads the thing of a call at, in grains: a meal,
// or a tent's credit.
func (w *World) callGain(b *Body, item int) int {
	if item == saleTent {
		return w.tentCredit(b)
	}
	return w.pred.meal
}

// buyFrom has body b buy the thing caller id calls, at the price called:
// a step towards the caller while it is out of sight, then, in sight, the
// coins to the caller and the thing to b.
func (w *World) buyFrom(i int, b *Body, id int64) {
	var p *Body
	far := false
	w.callers(b, func(o *Body, f bool) {
		if o.ID == id {
			p, far = o, f
		}
	})
	if p == nil {
		return
	}
	if far {
		w.stepTowards(i, b, dirTowards(p.X-b.X, p.Y-b.Y))
		return
	}
	item, price := p.Selling.Item, p.Selling.Price
	p.Selling = SaleCall{}
	w.stats.CallSales++
	w.hand(p, b, item, price)
}
