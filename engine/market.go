package engine

// The market at the shop (stage 12-2, the user's choice 2026-10-12: "店の
// 周りを市場にして"): VISION's sign (看板), the call that stays where it
// was put, is the shop's board. A sign where a seller lives was put up
// three times in 8000 ticks (a seller keeps no place), and one a seller
// has to stand by fails the same way, since nothing in its valuation
// keeps it there; so the market is consignment. A body at a shop's door
// that holds a unit of food or a folded tent it would sell leaves it
// with the shop at its own price - the price it would ask a body in sight
// (sale.go) - and the board lists it: who, what, at what price. Any body
// at a door of that shop sees the board, and one that would take a thing
// at the price listed buys it there, paying the seller at once wherever
// it is (or, the seller dead, dropping the coins at the door). Nothing
// ages on the board, as nothing does in a shop (stage 11-2c): a unit is
// bought as old as it was left, a tent with the life it had left; a shop
// lists MarketCap things at most. A seller cannot buy its own back, so
// the board is no free store. The shop's own goods and
// prices are untouched, so the board's prices and the shop's are each
// other's outside option. Leaving a thing reads as sending one does
// (stage 11-2c): its worth gone, the price read at what coins buy,
// discounted by the sell row, which the payment teaches.

// Listing is a thing on a shop's board: Item (sale.go) left by Seller at
// Price; Age is how old a unit was when it was left, or the life a tent
// had left.
type Listing struct {
	Shop   int   `json:"shop"`
	Seller int64 `json:"seller"`
	Item   int   `json:"item"`
	Price  int   `json:"price"`
	Age    int64 `json:"age"`
}

// atShop is the shop whose door body b stands at, -1 for none.
func (w *World) atShop(b *Body) int {
	for i := range w.shops {
		if w.inReach(b, i) {
			return i
		}
	}
	return -1
}

// listed is how many things shop i's board holds.
func (w *World) listed(i int) int {
	n := 0
	for _, l := range w.listings {
		if l.Shop == i {
			n++
		}
	}
	return n
}

// listOptions appends a listing of each thing body b, at a shop's door,
// could leave on its board: food or a tent it would sell (callable), at
// its asking price, where the board has room.
func (w *World) listOptions(dst []Action, b *Body) []Action {
	if !w.cfg.Market || !w.mayOffer(b) {
		return dst
	}
	i := w.atShop(b)
	if i < 0 || w.listed(i) >= w.cfg.MarketCap {
		return dst
	}
	w.saleItems(b, func(item int) {
		if callable(item) && w.askOf(b, item) > 0 {
			dst = append(dst, Action{Kind: ActList, Mate: int64(i), Dir: item})
		}
	})
	return dst
}

// list has body b leave item on shop i's board at its asking price.
func (w *World) list(b *Body, i, item int) {
	ask := w.askOf(b, item)
	if !w.cfg.Market || i < 0 || i >= len(w.shops) || !w.inReach(b, i) || ask == 0 || !w.stillHas(b, item) || w.listed(i) >= w.cfg.MarketCap {
		return
	}
	l := Listing{Shop: i, Seller: b.ID, Item: item, Price: ask}
	if item == saleTent {
		l.Age = b.Tent.Until - w.tick
		b.Tent = Tent{}
	} else {
		p := item - 1
		k := heldOf(b, p)
		l.Age = w.tick - b.HeldBorn[k]
		b.HeldBorn = append(b.HeldBorn[:k], b.HeldBorn[k+1:]...)
		b.HeldPool = append(b.HeldPool[:k], b.HeldPool[k+1:]...)
		b.Held--
		w.food.pools[p].held--
		w.food.pools[p].stocked++ // on the board, as on its way to a shop
	}
	w.listings = append(w.listings, l)
	w.stats.Listed++
	if w.cfg.Learn {
		b.Memory.Sends++
		if w.cfg.AgeBand > 0 {
			w.observe(&b.Memory.Sell, 1, 0)
		}
	}
}

// boardOptions appends a buy of the cheapest listing of each item body b,
// at a shop's door, would take at the price listed.
func (w *World) boardOptions(dst []Action, b *Body) []Action {
	if !w.cfg.Market || len(w.listings) == 0 || w.spec(b).NoCoins {
		return dst
	}
	i := w.atShop(b)
	if i < 0 {
		return dst
	}
	best := map[int]int{} // item -> index of the cheapest listing
	for k, l := range w.listings {
		if l.Shop != i || l.Seller == b.ID || b.Coins < l.Price || !w.usesItem(b, l.Item) || !w.takesAt(b, l.Item, l.Price) {
			continue
		}
		if j, ok := best[l.Item]; !ok || l.Price < w.listings[j].Price {
			best[l.Item] = k
		}
	}
	for k, l := range w.listings {
		if j, ok := best[l.Item]; ok && j == k {
			dst = append(dst, Action{Kind: ActBuyListed, Mate: int64(k), Dir: l.Item})
		}
	}
	return dst
}

// buyListed has body b buy listing k: the price to its seller (or, the
// seller dead, dropped at the door), the thing to b.
func (w *World) buyListed(b *Body, k int) {
	if k < 0 || k >= len(w.listings) {
		return
	}
	l := w.listings[k]
	if !w.inReach(b, l.Shop) || b.Coins < l.Price || !w.usesItem(b, l.Item) {
		return
	}
	w.listings = append(w.listings[:k], w.listings[k+1:]...)
	b.Coins -= l.Price
	w.stats.ListedSales++
	w.stats.Sales++
	w.stats.SalesCoins += int64(l.Price)
	if s := w.bodyNamed(l.Seller); s != nil {
		s.Coins += l.Price
		if w.cfg.Learn {
			s.Memory.Paid++
			if w.cfg.AgeBand > 0 {
				w.observe(&s.Memory.Sell, 0, 1)
			}
		}
	} else if t := w.tileOf(b.X, b.Y); t >= 0 && w.coins.at != nil {
		w.coins.at[t] += int32(l.Price)
	}
	if l.Item == saleTent {
		b.Tent = Tent{Has: true, Until: w.tick + l.Age}
		return
	}
	p := l.Item - 1
	w.food.pools[p].stocked--
	w.food.pools[p].held++
	b.Held++
	born := w.tick - l.Age
	at := len(b.HeldBorn)
	for at > 0 && b.HeldBorn[at-1] > born {
		at--
	}
	b.HeldBorn = append(b.HeldBorn[:at], append([]int64{born}, b.HeldBorn[at:]...)...)
	b.HeldPool = append(b.HeldPool[:at], append([]int{p}, b.HeldPool[at:]...)...)
	w.eatHeld(b)
}

// Listings returns the things on the shops' boards.
func (w *World) Listings() []Listing { return append([]Listing(nil), w.listings...) }

// loadListings puts the saved boards back, their food in their pools.
func (w *World) loadListings(ls []Listing) {
	w.listings = append([]Listing(nil), ls...)
	for _, l := range w.listings {
		if l.Item != saleTent && l.Item-1 < len(w.food.pools) {
			w.food.pools[l.Item-1].stocked++
		}
	}
}
