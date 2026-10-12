package engine

import "math"

// Signs (stage 12-2, VISION "商売を行う人は、何を販売するか発信するが、看板を
// 立てる、声を出すなどの手段がある"): the call of stage 12-1 reaches only
// those near the seller while it lasts; a sign stays where it stands and
// tells whoever sees it later. A body that calls out a unit of food to
// sell and has a settlement standing puts up a sign at its middle -
// where it comes back to sleep, the one place in the world that is its
// own - saying what it sells and at what price. The sign stands until the
// seller dies, its settlement goes, the thing has left its hands, or
// SignTicks pass; a new call renews it. A body that sees a sign remembers
// it (SignMemory at most, the newest kept). One that would take the
// thing at that price and holds the coins reads walking to the sign as it
// reads walking to a shop: a meal reached after the walk, with the chance
// its buy row gives. When it gets there and the seller is in its sight,
// awake and still has the thing, the sign is the seller's agreement, as a
// call is, and the sale goes through without a turn; otherwise the walk
// counts in the buy row as a buy that failed, and the sign is forgotten.

// Sign is a sign standing on Tile: Seller sells Item (sale.go) at Price,
// until tick Until.
type Sign struct {
	Tile   int   `json:"tile"`
	Seller int64 `json:"seller"`
	Item   int   `json:"item"`
	Price  int   `json:"price"`
	Until  int64 `json:"until"`
}

// SignNote is a sign a body remembers: where, whose, what and at what
// price.
type SignNote struct {
	Tile   int   `json:",omitempty"`
	Seller int64 `json:",omitempty"`
	Item   int   `json:",omitempty"`
	Price  int   `json:",omitempty"`
}

// putSign has body b, which has just called item out at price, put up or
// renew its sign at the middle of its settlement, if it has one.
func (w *World) putSign(b *Body, item, price int) {
	if !w.cfg.Signs || b.Home == 0 || item == saleTent {
		return
	}
	var tile = -1
	for _, p := range w.places {
		if p.ID == b.Home && len(p.Tiles) > 0 {
			tile = p.Tiles[len(p.Tiles)/2]
		}
	}
	if tile < 0 {
		return
	}
	s := Sign{Tile: tile, Seller: b.ID, Item: item, Price: price, Until: w.tick + int64(w.cfg.SignTicks)}
	for i := range w.signs {
		if w.signs[i].Seller == b.ID {
			w.signs[i] = s
			return
		}
	}
	w.signs = append(w.signs, s)
	w.stats.Signs++
}

// wearSigns takes down the signs whose time is up, whose seller has died
// or lost its settlement, or that no longer has the thing.
func (w *World) wearSigns() {
	kept := w.signs[:0]
	for _, s := range w.signs {
		b := w.bodyNamed(s.Seller)
		if s.Until <= w.tick || b == nil || b.Home == 0 || !w.stillHas(b, s.Item) {
			continue
		}
		kept = append(kept, s)
	}
	w.signs = kept
}

// noteSigns has body b remember the signs in its sight, and settle a walk
// to a sign it has come to.
func (w *World) noteSigns(b *Body) {
	if !w.cfg.Signs || len(w.signs) == 0 && len(b.Memory.Signs) == 0 {
		return
	}
	bx, by := int(math.Floor(b.X)), int(math.Floor(b.Y))
	sight := max(w.sightOf(b), 0)
	for _, s := range w.signs {
		sx, sy := s.Tile%w.m.Width, s.Tile/w.m.Width
		if s.Seller == b.ID || abs(sx-bx) > sight || abs(sy-by) > sight || !w.seesFrom(b, bx, by, sx, sy) {
			continue
		}
		w.rememberSign(b, SignNote{Tile: s.Tile, Seller: s.Seller, Item: s.Item, Price: s.Price})
	}
	if b.Goal < 0 || b.Goal != w.tileOf(b.X, b.Y) {
		return
	}
	for k, n := range b.Memory.Signs {
		if n.Tile != b.Goal {
			continue
		}
		// Come to the sign: the seller in sight, awake, with the thing,
		// and b still taking it, or the walk was for nothing.
		var seller *Body
		if o := w.bodyNamed(n.Seller); o != nil && !o.Asleep && w.stillHas(o, n.Item) && w.usesItem(b, n.Item) && w.takesAt(b, n.Item, n.Price) {
			ox, oy := int(math.Floor(o.X)), int(math.Floor(o.Y))
			if abs(ox-bx) <= sight && abs(oy-by) <= sight && w.seesFrom(b, bx, by, ox, oy) {
				seller = o
			}
		}
		if seller != nil {
			w.stats.SignSales++
			w.hand(seller, b, n.Item, n.Price)
			if w.cfg.Learn {
				b.Memory.Buys++
				b.Memory.Bought++
				if w.cfg.AgeBand > 0 {
					w.observe(&b.Memory.Buy, 1, 1)
				}
			}
		} else {
			w.stats.SignMisses++
			if w.cfg.Learn {
				b.Memory.Buys++
				if w.cfg.AgeBand > 0 {
					w.observe(&b.Memory.Buy, 1, 0)
				}
			}
		}
		b.Memory.Signs = append(b.Memory.Signs[:k], b.Memory.Signs[k+1:]...)
		return
	}
}

// rememberSign has body b remember sign n, the newest first, renewing
// what it knew of the seller's sign and keeping SignMemory at most.
func (w *World) rememberSign(b *Body, n SignNote) {
	for k, o := range b.Memory.Signs {
		if o.Seller == n.Seller {
			b.Memory.Signs = append(b.Memory.Signs[:k], b.Memory.Signs[k+1:]...)
			break
		}
	}
	b.Memory.Signs = append([]SignNote{n}, b.Memory.Signs...)
	if len(b.Memory.Signs) > w.cfg.SignMemory {
		b.Memory.Signs = b.Memory.Signs[:w.cfg.SignMemory]
	}
}

// signAhead returns the nearest sign body b remembers whose thing it
// would take at the price signed and can pay for, as a tile to walk to.
func (w *World) signAhead(b *Body) (f Food, ok bool) {
	if !w.cfg.Signs || len(b.Memory.Signs) == 0 || !w.mayOffer(b) {
		return f, false
	}
	best := math.Inf(1)
	for _, n := range b.Memory.Signs {
		if b.Coins < n.Price || !w.usesItem(b, n.Item) || !w.takesAt(b, n.Item, n.Price) {
			continue
		}
		x, y := n.Tile%w.m.Width, n.Tile/w.m.Width
		if d := octile(gapTo(b.X, x), gapTo(b.Y, y)); d < best {
			best, f, ok = d, Food{X: x, Y: y}, true
		}
	}
	return f, ok
}

// Signs returns the signs standing.
func (w *World) Signs() []Sign { return append([]Sign(nil), w.signs...) }
