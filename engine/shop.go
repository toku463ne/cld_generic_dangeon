package engine

import (
	"fmt"
	"math"
	"sort"
)

// Shops (stage 11-2b, the user's design 2026-10-12): a place the map
// sets, like a place to rest, where a body pays coins for a unit of
// food. What a shop sells, at what price, how much it holds at first
// and how many coins it can take are the map's (Map.Shops): the price is
// fixed, there being no one to set it. A shop makes nothing: its stock
// is units of a pool held off the ground (the pool's cap still closes
// over ground, hands, shops and vacancy), they do not rot while it holds
// them, and a unit bought is fresh in the buyer's hand. The coins paid
// stay in the shop, up to FundCap: a shop full of coins sells no more,
// so it cannot drain the coins out of the world. Stock comes in only
// from what bodies sell it (stage 11-2c); until then a shop sells what
// it starts with.
//
// A body knows a shop once it has seen it, and a child knows the shops
// its parent knew (with the places to rest). It buys standing on the
// shop's tile or next to it. What a coin is worth is not set anywhere: a
// body reads walking to a shop it knows, with the coins it holds, as a
// meal reached after the walk, with the chance its buy row gives (the
// buys that got a unit, as the trade row of stage 11-1b); and a coin in
// sight that makes up the price as that meal after the walk to the coin
// and on to the shop. So coins are worth a meal only to a body that
// knows where to spend them, and only as far as its buys have gone
// through.

// Shop is a shop the map sets on tile (X, Y): it sells units of the food
// pool named Food at Price coins each, holds Stock units at first, and
// takes up to FundCap coins.
type Shop struct {
	X, Y    int
	Food    string
	Price   int
	Stock   int
	FundCap int
}

// shopState is a shop as it stands: its pool, the units it holds and the
// coins it has taken.
type shopState struct {
	pool  int
	stock int
	fund  int
}

// ShopState is a shop as it stands, for the client and the saved world.
type ShopState struct {
	Stock int `json:"stock"`
	Fund  int `json:"fund"`
}

// validShops checks the map's shops.
func (m Map) validShops() error {
	for i, s := range m.Shops {
		if !m.InBounds(s.X, s.Y) || !m.Terrain[m.index(s.X, s.Y)].Ground() {
			return fmt.Errorf("shop %d at (%d,%d) is not on ground", i, s.X, s.Y)
		}
		if s.Price < 1 || s.Stock < 0 || s.FundCap < 0 {
			return fmt.Errorf("shop %d: price %d, stock %d, fund cap %d", i, s.Price, s.Stock, s.FundCap)
		}
	}
	return nil
}

// initShops stocks the map's shops from their pools, before the world
// is filled: the stock is part of each pool's cap.
func (w *World) initShops() {
	w.shops = nil
	if !w.cfg.Shops {
		return
	}
	for _, s := range w.m.Shops {
		p := w.poolNamed(s.Food)
		if p < 0 {
			// A world without that food (one pool for all, say): the
			// shop stays shut.
			w.shops = append(w.shops, shopState{pool: -1})
			continue
		}
		pl := &w.food.pools[p]
		stock := min(s.Stock, pl.cap-pl.used())
		pl.stocked += stock
		pl.appeared += int64(stock)
		w.shops = append(w.shops, shopState{pool: p, stock: stock})
	}
}

// poolNamed is the index of the pool named name, -1 for none.
func (w *World) poolNamed(name string) int {
	for p := range w.food.pools {
		if w.food.pools[p].name == name {
			return p
		}
	}
	return -1
}

// shopTile is the tile of shop i.
func (w *World) shopTile(i int) int { return w.m.index(w.m.Shops[i].X, w.m.Shops[i].Y) }

// mayBuyAt reports whether body b could pay shop i and use what it
// sells: the shops are on, it holds the price, eats the shop's food and
// has room to hold a unit. Whether the shop has stock or room for the
// coins b does not know until it tries.
func (w *World) mayBuyAt(b *Body, i int) bool {
	return w.cfg.Shops && w.cfg.Coins && w.shops[i].pool >= 0 && b.Coins >= w.m.Shops[i].Price && w.eats(b, w.shops[i].pool) && b.Held < w.cfg.Carry
}

// inReach reports whether body b stands on shop i's tile or next to it.
func (w *World) inReach(b *Body, i int) bool {
	s := w.m.Shops[i]
	return math.Abs(math.Floor(b.X)-float64(s.X)) <= 1 && math.Abs(math.Floor(b.Y)-float64(s.Y)) <= 1
}

// buyOptions appends a buy at each shop body b stands at and could pay.
func (w *World) buyOptions(dst []Action, b *Body) []Action {
	for i := range w.shops {
		if w.inReach(b, i) && w.mayBuyAt(b, i) {
			dst = append(dst, Action{Kind: ActBuy, Mate: int64(i)})
		}
	}
	return dst
}

// buy has body b buy a unit at shop i: if the shop holds one and has
// room for the coins, b pays the price and holds the unit, fresh, and
// eats it as it eats what it holds. The try and its outcome go into b's
// buy row.
func (w *World) buy(b *Body, i int) {
	if i < 0 || i >= len(w.shops) || !w.inReach(b, i) || !w.mayBuyAt(b, i) {
		return
	}
	s, price := &w.shops[i], w.m.Shops[i].Price
	if w.cfg.Learn {
		b.Memory.Buys++
		if w.cfg.AgeBand > 0 {
			w.observe(&b.Memory.Buy, 1, 0)
		}
	}
	if s.stock == 0 || s.fund+price > w.m.Shops[i].FundCap {
		w.stats.BuysFailed++
		return
	}
	if w.cfg.Learn {
		b.Memory.Bought++
		if w.cfg.AgeBand > 0 {
			w.observe(&b.Memory.Buy, 0, 1)
		}
	}
	w.stats.Bought++
	b.Coins -= price
	s.fund += price
	s.stock--
	pl := &w.food.pools[s.pool]
	pl.stocked--
	pl.held++
	pl.bought++
	b.Held++
	b.HeldBorn = append(b.HeldBorn, w.tick)
	b.HeldPool = append(b.HeldPool, s.pool)
	w.eatHeld(b)
}

// buyRate is body b's estimate of how likely a buy is to go through: its
// buy row, on the trade row's prior.
func (w *World) buyRate(b *Body) float64 {
	if w.cfg.AgeBand > 0 {
		return w.Fresh(b.Memory.Buy).estimate(w.cfg.PriorTrade, w.cfg.TradeWeight)
	}
	return Tally{N: b.Memory.Buys, K: b.Memory.Bought}.estimate(w.cfg.PriorTrade, w.cfg.TradeWeight)
}

// noteShops has body b remember the shops in its sight.
func (w *World) noteShops(b *Body) {
	if !w.cfg.Shops {
		return
	}
	bx, by := int(math.Floor(b.X)), int(math.Floor(b.Y))
	s := max(w.sightOf(b), 0)
	for i, sh := range w.m.Shops {
		if abs(sh.X-bx) <= s && abs(sh.Y-by) <= s && w.seesFrom(b, bx, by, sh.X, sh.Y) {
			w.rememberShop(b, i)
		}
	}
}

// rememberShop has body b know shop i.
func (w *World) rememberShop(b *Body, i int) {
	for _, k := range b.Memory.Shops {
		if k == i {
			return
		}
	}
	b.Memory.Shops = append(b.Memory.Shops, i)
	sort.Ints(b.Memory.Shops)
}

// shopAhead returns the nearest shop body b knows and could pay at, as a
// tile to walk to, and its index; ok is false for none.
func (w *World) shopAhead(b *Body) (f Food, i int, ok bool) {
	best := math.Inf(1)
	for _, k := range b.Memory.Shops {
		if k >= len(w.shops) || !w.mayBuyAt(b, k) {
			continue
		}
		s := w.m.Shops[k]
		if d := octile(gapTo(b.X, s.X), gapTo(b.Y, s.Y)); d < best {
			best, f, i, ok = d, Food{X: s.X, Y: s.Y}, k, true
		}
	}
	return f, i, ok
}

// coinsAhead calls f for each coin tile in body b's sight that would
// make up the price of the nearest shop it knows and could not pay at
// yet, with the ticks from that tile on to the shop: such a body walks
// to coins as to a meal further off. A body that can pay already reads
// no coin as worth more (a second meal waits on a second walk, which it
// reads when it gets there).
func (w *World) coinsAhead(b *Body, f func(x, y, rest, shop int)) {
	if !w.cfg.Shops || !w.cfg.Coins || len(b.Memory.Shops) == 0 || b.Held >= w.cfg.Carry || w.spec(b).NoCoins {
		return
	}
	sight := w.sightOf(b)
	if sight < 0 {
		return
	}
	k := -1
	best := math.Inf(1)
	for _, i := range b.Memory.Shops {
		if i >= len(w.shops) || w.shops[i].pool < 0 || !w.eats(b, w.shops[i].pool) || b.Coins >= w.m.Shops[i].Price {
			continue
		}
		s := w.m.Shops[i]
		if d := octile(gapTo(b.X, s.X), gapTo(b.Y, s.Y)); d < best {
			best, k = d, i
		}
	}
	if k < 0 {
		return
	}
	s := w.m.Shops[k]
	speed := w.speedOf(b)
	bx, by := int(math.Floor(b.X)), int(math.Floor(b.Y))
	for y := by - sight; y <= by+sight; y++ {
		for x := bx - sight; x <= bx+sight; x++ {
			if !w.m.InBounds(x, y) {
				continue
			}
			if n := int(w.coins.at[w.m.index(x, y)]); n > 0 && b.Coins+n >= s.Price && w.seesFrom(b, bx, by, x, y) {
				f(x, y, walkTicks(gapTo(float64(x)+0.5, s.X), gapTo(float64(y)+0.5, s.Y), speed), k)
			}
		}
	}
}

// sawCoins hashes the coin tiles in body b's sight it would walk to, so
// that one coming into sight makes it decide.
func (w *World) sawCoins(b *Body) uint64 {
	var h uint64
	w.coinsAhead(b, func(x, y, _, _ int) { h += mix(uint64(w.m.index(x, y)) ^ 0x636f696e) })
	return h
}

// Shops returns the map's shops as they stand.
func (w *World) Shops() []ShopState {
	var s []ShopState
	for _, sh := range w.shops {
		s = append(s, ShopState{Stock: sh.stock, Fund: sh.fund})
	}
	return s
}

// loadShops puts the saved shops back, their stock in their pools.
func (w *World) loadShops(saved []ShopState) error {
	if !w.cfg.Shops {
		if len(saved) > 0 {
			return fmt.Errorf("snapshot has shops without Shops")
		}
		return nil
	}
	if len(saved) != len(w.m.Shops) {
		return fmt.Errorf("snapshot has %d shops, map has %d", len(saved), len(w.m.Shops))
	}
	w.shops = nil
	for i, s := range w.m.Shops {
		p := w.poolNamed(s.Food)
		if p < 0 {
			w.shops = append(w.shops, shopState{pool: -1})
			continue
		}
		w.food.pools[p].stocked += saved[i].Stock
		w.shops = append(w.shops, shopState{pool: p, stock: saved[i].Stock, fund: saved[i].Fund})
	}
	return nil
}
