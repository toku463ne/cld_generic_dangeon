package engine

import (
	"fmt"
	"math"
	"sort"
)

// Shops (stages 11-2b and 11-2c, the user's design 2026-10-12): a place
// the map sets, like a place to rest, that deals in goods for coins.
// What a shop deals in, at what prices, how much it holds at first and
// at most, and how many coins it can take are the map's (Map.Shops): the
// prices are fixed, there being no one to set them. A shop makes
// nothing: it sells what it was stocked with and what bodies sent it.
// Its food is units of a pool held off the ground (the pool's cap still
// closes over ground, hands, shops and vacancy); nothing rots while a
// shop holds it, and a unit bought is fresh in the buyer's hand, a tent
// bought has the life it had left when it came in. The coins paid stay
// in the shop, up to FundCap: a shop full of coins sells no more, so it
// cannot drain the coins out of the world; a shop pays for what it is
// sent out of the same coins, so one with none buys nothing.
//
// A body knows a shop once it has seen it, and a child knows the shops
// its parent knew (with the places to rest). It buys standing on the
// shop's tile or next to it (stage 11-2b). It sells by sending (stage
// 11-2c): from wherever it is, the thing leaves its hands and reaches
// the shop after the ticks the distance takes at CarrySpeed; the shop
// pays the Offer then, if it has room for the thing and coins to pay -
// otherwise the thing is lost - and food that rots on the way is lost
// unpaid. The asymmetry is the user's: buying takes the walk, so the
// shop is a place to go to; selling does not, so anyone can.
//
// What a coin is worth is not set anywhere. A body reads walking to a
// shop it knows, with the price of food it eats, as a meal reached after
// the walk, with the chance its buy row gives (the buys that went
// through, as the trade row of stage 11-1b); and a coin in sight that
// makes up the price as that meal after the walk to the coin and on to
// the shop. Where a choice trades coins against something else - selling
// a thing, buying a tent - a coin is read at what it buys: the meal the
// cheapest food it can buy is, over its price, times the buy row
// (coinWorth); and what it sends is paid with the chance its sell row
// gives.

// TentGood is the item name of a tent in a shop's goods.
const TentGood = "tent"

// Good is one thing a shop deals in: Item names a food pool or TentGood;
// the shop sells one at Price coins and pays Offer coins for one sent
// to it (zero: it takes none), holds Stock at first and StockCap at
// most.
type Good struct {
	Item     string
	Price    int
	Offer    int `json:",omitempty"`
	Stock    int `json:",omitempty"`
	StockCap int `json:",omitempty"`
}

// Shop is a shop the map sets on tile (X, Y), dealing in Goods and
// taking up to FundCap coins.
type Shop struct {
	X, Y    int
	Goods   []Good
	FundCap int
}

// The kinds of good a shop state holds besides a pool's index.
const (
	goodTent = -1
	goodShut = -2 // a good the world has no such thing for
)

// goodState is one good of a shop as it stands: its pool (or goodTent,
// goodShut), how many it holds and, for tents, the life each has left.
type goodState struct {
	pool  int
	stock int
	lives []int64
}

// shopState is a shop as it stands: its goods and the coins it holds.
type shopState struct {
	goods []goodState
	fund  int
}

// ShopState is a shop as it stands, for the client and the saved world.
type ShopState struct {
	Goods []GoodState `json:"goods"`
	Fund  int         `json:"fund"`
}

// GoodState is one good of a shop as it stands.
type GoodState struct {
	Stock int     `json:"stock"`
	Lives []int64 `json:"lives,omitempty"`
}

// Parcel is a thing on its way to a shop (stage 11-2c): good Good of shop
// Shop, sent by Seller, arriving at tick Arrive. Born is when a unit of
// food appeared (it rots on the way as in a hand), or when a tent wears
// out.
type Parcel struct {
	Shop, Good int
	Seller     int64
	Arrive     int64
	Born       int64
}

// validShops checks the map's shops.
func (m Map) validShops() error {
	for i, s := range m.Shops {
		if !m.InBounds(s.X, s.Y) || !m.Terrain[m.index(s.X, s.Y)].Ground() {
			return fmt.Errorf("shop %d at (%d,%d) is not on ground", i, s.X, s.Y)
		}
		if s.FundCap < 0 {
			return fmt.Errorf("shop %d: fund cap %d", i, s.FundCap)
		}
		for _, g := range s.Goods {
			// The shop's margin: paying as much as it asks would make it a
			// free store, where nothing rots.
			if g.Price < 1 || g.Offer < 0 || g.Offer >= g.Price || g.Stock < 0 || g.StockCap < g.Stock {
				return fmt.Errorf("shop %d, %s: price %d, offer %d, stock %d of %d", i, g.Item, g.Price, g.Offer, g.Stock, g.StockCap)
			}
		}
	}
	return nil
}

// initShops stocks the map's shops, before the world is filled: food is
// part of its pool's cap; tents come out of nothing, as a tent a body is
// born with does.
func (w *World) initShops() {
	w.shops = nil
	if !w.cfg.Shops {
		return
	}
	for _, s := range w.m.Shops {
		st := shopState{}
		for _, g := range s.Goods {
			gs := w.goodKind(g.Item)
			switch {
			case gs.pool >= 0:
				pl := &w.food.pools[gs.pool]
				gs.stock = min(g.Stock, pl.cap-pl.used())
				pl.stocked += gs.stock
				pl.appeared += int64(gs.stock)
			case gs.pool == goodTent:
				gs.stock = g.Stock
				for k := 0; k < g.Stock; k++ {
					gs.lives = append(gs.lives, int64(w.cfg.TentLife))
				}
			}
			st.goods = append(st.goods, gs)
		}
		w.shops = append(w.shops, st)
	}
}

// goodKind is an empty good of item: its pool, a tent, or shut where the
// world has no such thing (a food it lacks; tents without Tents).
func (w *World) goodKind(item string) goodState {
	if item == TentGood {
		if w.cfg.Tents {
			return goodState{pool: goodTent}
		}
		return goodState{pool: goodShut}
	}
	if p := w.poolNamed(item); p >= 0 {
		return goodState{pool: p}
	}
	return goodState{pool: goodShut}
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

// trades reports whether the shops are open and coins exist.
func (w *World) trades() bool { return w.cfg.Shops && w.cfg.Coins }

// foodGood is the good of shop i that is food body b eats, -1 for none.
func (w *World) foodGood(b *Body, i int) int {
	for g, gs := range w.shops[i].goods {
		if gs.pool >= 0 && w.eats(b, gs.pool) {
			return g
		}
	}
	return -1
}

// mayBuyAt reports whether body b could pay for good g of shop i and use
// it: it holds the price, and eats the food and has room to hold a unit,
// or has no tent and is of a species with skills. Whether the shop has
// any, or room for the coins, b does not know until it tries.
func (w *World) mayBuyAt(b *Body, i, g int) bool {
	if !w.trades() || b.Coins < w.m.Shops[i].Goods[g].Price {
		return false
	}
	switch p := w.shops[i].goods[g].pool; {
	case p >= 0:
		return w.eats(b, p) && b.Held < w.cfg.Carry
	case p == goodTent:
		return !b.Tent.Has && !w.spec(b).NoSkills
	}
	return false
}

// inReach reports whether body b stands on shop i's tile or next to it.
func (w *World) inReach(b *Body, i int) bool {
	s := w.m.Shops[i]
	return math.Abs(math.Floor(b.X)-float64(s.X)) <= 1 && math.Abs(math.Floor(b.Y)-float64(s.Y)) <= 1
}

// onSale reports whether good g of shop i can be sold now: the shop
// holds one and has room for the price. A body at the shop sees it
// (ShopSight, stage 11-2d).
func (w *World) onSale(i, g int) bool {
	return w.shops[i].goods[g].stock > 0 && w.shops[i].fund+w.m.Shops[i].Goods[g].Price <= w.m.Shops[i].FundCap
}

// buyOptions appends a buy of each good body b could pay for at each
// shop it stands at - with ShopSight, of those it sees on sale.
func (w *World) buyOptions(dst []Action, b *Body) []Action {
	for i := range w.shops {
		if !w.inReach(b, i) {
			continue
		}
		for g := range w.shops[i].goods {
			if w.mayBuyAt(b, i, g) && (!w.cfg.ShopSight || w.onSale(i, g)) {
				dst = append(dst, Action{Kind: ActBuy, Mate: int64(i), Dir: g})
			}
		}
	}
	return dst
}

// buy has body b buy good g at shop i: if the shop holds one and has
// room for the coins, b pays the price and takes it - a unit fresh, which
// it eats as it eats what it holds; a tent with the life it had left. The
// try and its outcome go into b's buy row.
func (w *World) buy(b *Body, i, g int) {
	if i < 0 || i >= len(w.shops) || g < 0 || g >= len(w.shops[i].goods) || !w.inReach(b, i) || !w.mayBuyAt(b, i, g) {
		return
	}
	s, gs, price := &w.shops[i], &w.shops[i].goods[g], w.m.Shops[i].Goods[g].Price
	if w.cfg.Learn {
		b.Memory.Buys++
		if w.cfg.AgeBand > 0 {
			w.observe(&b.Memory.Buy, 1, 0)
		}
	}
	if gs.stock == 0 || s.fund+price > w.m.Shops[i].FundCap {
		w.stats.BuysFailed++
		return
	}
	if w.cfg.Learn {
		b.Memory.Bought++
		if w.cfg.AgeBand > 0 {
			w.observe(&b.Memory.Buy, 0, 1)
		}
	}
	b.Coins -= price
	s.fund += price
	gs.stock--
	if gs.pool == goodTent {
		w.stats.TentsBought++
		life := gs.lives[len(gs.lives)-1]
		gs.lives = gs.lives[:len(gs.lives)-1]
		b.Tent = Tent{Has: true, Until: w.tick + life}
		return
	}
	w.stats.Bought++
	pl := &w.food.pools[gs.pool]
	pl.stocked--
	pl.held++
	pl.bought++
	b.Held++
	b.HeldBorn = append(b.HeldBorn, w.tick)
	b.HeldPool = append(b.HeldPool, gs.pool)
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

// sellRate is body b's estimate of how likely a thing sent is to be paid
// for: its sell row, on the trade row's prior.
func (w *World) sellRate(b *Body) float64 {
	if w.cfg.AgeBand > 0 {
		return w.Fresh(b.Memory.Sell).estimate(w.cfg.PriorTrade, w.cfg.TradeWeight)
	}
	return Tally{N: b.Memory.Sends, K: b.Memory.Paid}.estimate(w.cfg.PriorTrade, w.cfg.TradeWeight)
}

// coinWorth is what body b reads a coin at, in grains: the meal the
// cheapest food it eats at a shop it knows buys, over its price, times
// its buy row. Zero where it knows no shop with food for it.
func (w *World) coinWorth(b *Body) float64 {
	if !w.trades() {
		return 0
	}
	price := 0
	for _, i := range b.Memory.Shops {
		if i >= len(w.shops) {
			continue
		}
		if g := w.foodGood(b, i); g >= 0 && (price == 0 || w.m.Shops[i].Goods[g].Price < price) {
			price = w.m.Shops[i].Goods[g].Price
		}
	}
	if price == 0 {
		return 0
	}
	return float64(w.pred.meal) * w.buyRate(b) / float64(price)
}

// heldOf is the index in body b's hold of the oldest unit of pool p, -1
// for none.
func heldOf(b *Body, p int) int {
	for k, q := range b.HeldPool {
		if q == p {
			return k
		}
	}
	return -1
}

// maySell reports whether body b could send good g to shop i: the shop
// takes the good, and b has one - a unit of the food in hand, or a
// folded tent.
func (w *World) maySell(b *Body, i, g int) bool {
	if !w.cfg.Sell || !w.trades() || w.m.Shops[i].Goods[g].Offer == 0 || w.spec(b).NoCoins {
		return false
	}
	switch p := w.shops[i].goods[g].pool; {
	case p >= 0:
		return heldOf(b, p) >= 0
	case p == goodTent:
		return b.Tent.Has && !b.Tent.Pitched && b.Tent.Until > w.tick
	}
	return false
}

// sellOptions appends a sale of each good body b could send to each shop
// it knows.
func (w *World) sellOptions(dst []Action, b *Body) []Action {
	if !w.cfg.Sell {
		return dst
	}
	for _, i := range b.Memory.Shops {
		if i >= len(w.shops) {
			continue
		}
		for g := range w.shops[i].goods {
			if w.maySell(b, i, g) {
				dst = append(dst, Action{Kind: ActSell, Mate: int64(i), Dir: g})
			}
		}
	}
	return dst
}

// sendTicks is the ticks a thing body b sends takes to reach shop i: the
// distance at CarrySpeed, at least one.
func (w *World) sendTicks(b *Body, i int) int {
	s := w.m.Shops[i]
	return max(1, int(math.Ceil(octile(gapTo(b.X, s.X), gapTo(b.Y, s.Y))/w.cfg.CarrySpeed-1e-9)))
}

// sell has body b send good g to shop i: the thing leaves its hands and
// is on its way (a unit of food still counts in its pool, as stock).
func (w *World) sell(b *Body, i, g int) {
	if i < 0 || i >= len(w.shops) || g < 0 || g >= len(w.shops[i].goods) || !w.maySell(b, i, g) {
		return
	}
	pc := Parcel{Shop: i, Good: g, Seller: b.ID, Arrive: w.tick + int64(w.sendTicks(b, i))}
	if p := w.shops[i].goods[g].pool; p >= 0 {
		k := heldOf(b, p)
		pc.Born = b.HeldBorn[k]
		b.HeldBorn = append(b.HeldBorn[:k], b.HeldBorn[k+1:]...)
		b.HeldPool = append(b.HeldPool[:k], b.HeldPool[k+1:]...)
		b.Held--
		w.food.pools[p].held--
		w.food.pools[p].stocked++
	} else {
		pc.Born = b.Tent.Until
		b.Tent = Tent{}
	}
	w.parcels = append(w.parcels, pc)
	w.stats.Sent++
	if w.cfg.Learn {
		b.Memory.Sends++
		if w.cfg.AgeBand > 0 {
			w.observe(&b.Memory.Sell, 1, 0)
		}
	}
}

// deliver hands the shops the parcels that reach them this tick, in the
// order they were sent: one rotten or worn out on the way is lost
// unpaid; one the shop has no room or no coins for is lost; any other is
// stocked and paid for, to its sender if alive (a dead sender's is kept
// unpaid).
func (w *World) deliver() {
	kept := w.parcels[:0]
	for _, pc := range w.parcels {
		if pc.Arrive > w.tick {
			kept = append(kept, pc)
			continue
		}
		s, gs, good := &w.shops[pc.Shop], &w.shops[pc.Shop].goods[pc.Good], w.m.Shops[pc.Shop].Goods[pc.Good]
		food := gs.pool >= 0
		switch {
		case food && w.cfg.FoodLife > 0 && pc.Born <= w.tick-int64(w.cfg.FoodLife), !food && pc.Born <= w.tick:
			w.stats.SentSpoiled++
			if food {
				w.food.pools[gs.pool].stocked--
				w.food.pools[gs.pool].decayed++
			}
		case gs.stock >= good.StockCap || s.fund < good.Offer:
			w.stats.SentRefused++
			if food {
				w.food.pools[gs.pool].stocked--
				w.food.pools[gs.pool].lost++
			}
		default:
			gs.stock++
			if !food {
				gs.lives = append(gs.lives, pc.Born-w.tick)
			}
			if seller := w.bodyNamed(pc.Seller); seller != nil {
				s.fund -= good.Offer
				seller.Coins += good.Offer
				w.stats.Sold++
				if w.cfg.Learn {
					seller.Memory.Paid++
					if w.cfg.AgeBand > 0 {
						w.observe(&seller.Memory.Sell, 0, 1)
					}
				}
			}
		}
	}
	w.parcels = kept
}

// bodyNamed is the living body with id, nil for none.
func (w *World) bodyNamed(id int64) *Body {
	for i := range w.bodies {
		if w.bodies[i].ID == id {
			return &w.bodies[i]
		}
	}
	return nil
}

// noteShops has body b remember the shops in its sight. A body that
// walked to a shop for food and finds it has none to sell it (ShopSight,
// stage 11-2d) counts the walk in its buy row as a buy that failed:
// once, as it arrives, where without the sight it tried every tick.
func (w *World) noteShops(b *Body) {
	if !w.cfg.Shops {
		return
	}
	if w.cfg.ShopSight && b.Goal >= 0 {
		for i := range w.shops {
			if b.Goal != w.shopTile(i) || !w.inReach(b, i) {
				continue
			}
			if g := w.foodGood(b, i); g >= 0 && w.mayBuyAt(b, i, g) && !w.onSale(i, g) {
				w.stats.BuysFailed++
				if w.cfg.Learn {
					b.Memory.Buys++
					if w.cfg.AgeBand > 0 {
						w.observe(&b.Memory.Buy, 1, 0)
					}
				}
			}
		}
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

// shopAhead returns the nearest shop body b knows that sells food it
// eats at a price it holds, as a tile to walk to.
func (w *World) shopAhead(b *Body) (f Food, ok bool) {
	best := math.Inf(1)
	for _, k := range b.Memory.Shops {
		if k >= len(w.shops) {
			continue
		}
		if g := w.foodGood(b, k); g < 0 || !w.mayBuyAt(b, k, g) {
			continue
		}
		s := w.m.Shops[k]
		if d := octile(gapTo(b.X, s.X), gapTo(b.Y, s.Y)); d < best {
			best, f, ok = d, Food{X: s.X, Y: s.Y}, true
		}
	}
	return f, ok
}

// coinsAhead calls f for each coin tile in body b's sight that would
// make up the price of food at the nearest shop it knows and could not
// pay at yet, with the ticks from that tile on to the shop: such a body
// walks to coins as to a meal further off. A body that can pay already
// reads no coin as worth more (a second meal waits on a second walk,
// which it reads when it gets there).
func (w *World) coinsAhead(b *Body, f func(x, y, rest, shop int)) {
	if !w.trades() || len(b.Memory.Shops) == 0 || b.Held >= w.cfg.Carry || w.spec(b).NoCoins {
		return
	}
	sight := w.sightOf(b)
	if sight < 0 {
		return
	}
	k, price := -1, 0
	best := math.Inf(1)
	for _, i := range b.Memory.Shops {
		if i >= len(w.shops) {
			continue
		}
		g := w.foodGood(b, i)
		if g < 0 || b.Coins >= w.m.Shops[i].Goods[g].Price {
			continue
		}
		s := w.m.Shops[i]
		if d := octile(gapTo(b.X, s.X), gapTo(b.Y, s.Y)); d < best {
			best, k, price = d, i, s.Goods[g].Price
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
			if n := int(w.coins.at[w.m.index(x, y)]); n > 0 && b.Coins+n >= price && w.seesFrom(b, bx, by, x, y) {
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
		st := ShopState{Fund: sh.fund}
		for _, g := range sh.goods {
			st.Goods = append(st.Goods, GoodState{Stock: g.stock, Lives: append([]int64(nil), g.lives...)})
		}
		s = append(s, st)
	}
	return s
}

// Parcels returns the things on their way to the shops.
func (w *World) Parcels() []Parcel { return append([]Parcel(nil), w.parcels...) }

// loadShops puts the saved shops and parcels back, their food in their
// pools.
func (w *World) loadShops(saved []ShopState, parcels []Parcel) error {
	if !w.cfg.Shops {
		if len(saved) > 0 || len(parcels) > 0 {
			return fmt.Errorf("snapshot has shops without Shops")
		}
		return nil
	}
	if len(saved) != len(w.m.Shops) {
		return fmt.Errorf("snapshot has %d shops, map has %d", len(saved), len(w.m.Shops))
	}
	w.shops = nil
	for i, s := range w.m.Shops {
		if len(saved[i].Goods) != len(s.Goods) {
			return fmt.Errorf("snapshot shop %d has %d goods, map has %d", i, len(saved[i].Goods), len(s.Goods))
		}
		st := shopState{fund: saved[i].Fund}
		for g, item := range s.Goods {
			gs := w.goodKind(item.Item)
			gs.stock = saved[i].Goods[g].Stock
			gs.lives = append([]int64(nil), saved[i].Goods[g].Lives...)
			if gs.pool >= 0 {
				w.food.pools[gs.pool].stocked += gs.stock
			}
			st.goods = append(st.goods, gs)
		}
		w.shops = append(w.shops, st)
	}
	for _, pc := range parcels {
		if pc.Shop < 0 || pc.Shop >= len(w.shops) || pc.Good < 0 || pc.Good >= len(w.shops[pc.Shop].goods) {
			return fmt.Errorf("parcel to shop %d good %d", pc.Shop, pc.Good)
		}
		if p := w.shops[pc.Shop].goods[pc.Good].pool; p >= 0 {
			w.food.pools[p].stocked++
		}
	}
	w.parcels = append([]Parcel(nil), parcels...)
	return nil
}

// sellLoss is what body b reads sending the thing of sale a costs it, in
// grains: a tent's credit, or a meal for a unit of food - nothing for a
// unit that will rot in its hand before a meal fits (itemLoss).
func (w *World) sellLoss(b *Body, a Action) int {
	if p := w.shops[a.Mate].goods[a.Dir].pool; p >= 0 {
		return w.itemLoss(b, p+1)
	}
	return w.itemLoss(b, saleTent)
}
