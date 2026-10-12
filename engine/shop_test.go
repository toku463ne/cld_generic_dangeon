package engine

import (
	"bytes"
	"testing"
)

// shopMap is tempMap with a shop centred on tile (4, 5) - walls 3..5 x
// 4..6, doors (4,3), (6,5), (4,7) and (2,5) - selling the one pool of
// testConfig's food at price 2.
func shopMap(stock, fund int) Map {
	m := tempMap()
	m.Shops = []Shop{{X: 4, Y: 5, FundCap: fund, Goods: []Good{{Item: "food", Price: 2, Offer: 1, Stock: stock, StockCap: stock + 10}, {Item: TentGood, Price: 4, Offer: 2, StockCap: 5}}}}
	return m
}

// shopConfig is coinConfig with the shops open.
func shopConfig(seed int64) Config {
	cfg := coinConfig(seed)
	cfg.Shops = true
	return cfg
}

func shopper(id int64, x, y float64, coins int) Body {
	return Body{ID: id, X: x, Y: y, Energy: 40, Heading: -1, Goal: -1, Decided: -1, Parents: [2]int64{-1, -1}, Sight: 3, Born: -1500, Mature: -700, Coins: coins}
}

func hasBuy(w *World, b *Body) bool {
	for _, a := range w.possibleActions(nil, b) {
		if a.Kind == ActBuy {
			return true
		}
	}
	return false
}

// A shop stocks its units out of its pool's cap (stage 11-2b): the pool
// still closes over ground, hands, shops and vacancy. A body in reach
// with the price and room for a unit may buy: it pays, holds the unit
// fresh and eats it; the coins stay in the shop. Out of reach, without
// the price, or with the shop empty or full of coins, no unit passes; a
// try that fails is a buy in the row with nothing bought.
func TestShopBuy(t *testing.T) {
	cfg := shopConfig(5)
	cfg.Bodies = 0
	w, err := NewWorld(cfg, shopMap(1, 3))
	if err != nil {
		t.Fatal(err)
	}
	if l := w.FoodLedger(); l.Stocked != 1 || !l.Balanced() {
		t.Fatalf("food ledger %+v: want one unit stocked, balanced", l)
	}
	w.bodies = append(w.bodies, shopper(0, 4.5, 3.5, 5), shopper(1, 8.5, 1.5, 5), shopper(2, 2.5, 5.5, 1)) // at a door, away, at a door
	w.nextID = 3
	w.buildGrid()
	near, far, poor := &w.bodies[0], &w.bodies[1], &w.bodies[2]
	if !hasBuy(w, near) || hasBuy(w, far) || hasBuy(w, poor) {
		t.Fatalf("buy offered: near %v far %v poor %v; want only near", hasBuy(w, near), hasBuy(w, far), hasBuy(w, poor))
	}
	w.act(0, near, Action{Kind: ActBuy, Mate: 0, Dir: 0})
	if near.Coins != 3 || near.Energy != 40+cfg.FoodEnergy || w.shops[0].fund != 2 || w.shops[0].goods[0].stock != 0 {
		t.Fatalf("after a buy: coins %d energy %v fund %d stock %d", near.Coins, near.Energy, w.shops[0].fund, w.shops[0].goods[0].stock)
	}
	if near.Memory.Buys != 1 || near.Memory.Bought != 1 || w.Stats().Bought != 1 {
		t.Fatalf("buy row %v/%v, bought %d", near.Memory.Bought, near.Memory.Buys, w.Stats().Bought)
	}
	if l := w.FoodLedger(); !l.Balanced() || l.Bought != 1 || l.Stocked != 0 {
		t.Fatalf("food ledger after a buy %+v", l)
	}
	if l := w.CoinLedger(); l.Funds != 2 {
		t.Fatalf("coin ledger %+v: want 2 in the shop", l)
	}
	// Empty now: the try fails and is learned.
	near.Energy = 40
	w.act(0, near, Action{Kind: ActBuy, Mate: 0, Dir: 0})
	if near.Coins != 3 || near.Memory.Buys != 2 || near.Memory.Bought != 1 || w.Stats().BuysFailed != 1 {
		t.Fatalf("buy at an empty shop: coins %d row %v/%v failed %d", near.Coins, near.Memory.Bought, near.Memory.Buys, w.Stats().BuysFailed)
	}
	// Stocked again but its fund full (3, price 2): no sale.
	w.shops[0].goods[0].stock, w.food.pools[0].stocked = 1, 1
	w.food.pools[0].appeared++
	w.act(0, near, Action{Kind: ActBuy, Mate: 0, Dir: 0})
	if near.Coins != 3 || w.shops[0].goods[0].stock != 1 || w.Stats().BuysFailed != 2 {
		t.Fatalf("buy at a full shop: coins %d stock %d failed %d", near.Coins, w.shops[0].goods[0].stock, w.Stats().BuysFailed)
	}
}

// A body that knows a shop and holds its price reads the walk there as a
// meal; one that cannot pay walks to coins in sight that make up the
// price (stage 11-2b). One that knows no shop does neither.
func TestShopDrawsTheHungry(t *testing.T) {
	cfg := shopConfig(6)
	cfg.Bodies = 0
	w, err := NewWorld(cfg, shopMap(5, 100))
	if err != nil {
		t.Fatal(err)
	}
	// No food in the body's sight, some beyond it.
	for i := len(w.food.foods) - 1; i >= 0; i-- {
		if f := w.food.foods[i]; f.X <= 6 && f.Y >= 3 {
			w.eatFood(i)
		}
	}
	w.bodies = append(w.bodies, shopper(0, 0.5, 8.5, 2)) // out of the shop's sight
	w.nextID = 1
	w.buildGrid()
	b := &w.bodies[0]
	b.Energy = 60
	w.turn(b)
	if b.Goal >= 0 {
		t.Fatalf("a body that knows no shop walks to tile %d", b.Goal)
	}
	w.rememberShop(b, 0)
	b.Decided = -1
	w.turn(b)
	if door := w.m.index(2, 5); b.Goal != door {
		t.Fatalf("goal %d, want the shop's west door %d", b.Goal, door)
	}
	// Without the price, a coin in sight that makes it up is the goal.
	b.Coins = 1
	w.coins.at[w.m.index(1, 7)] = 1
	w.coins.issued = 1
	b.Decided = -1
	w.turn(b)
	if b.Goal != w.m.index(1, 7) {
		t.Fatalf("goal %d, want the coin's tile %d", b.Goal, w.m.index(1, 7))
	}
}

// A world with shops saved and read back runs on as if it had not been.
func TestShopSaveLoad(t *testing.T) {
	a, err := NewWorld(shopConfig(11), shopMap(20, 100))
	if err != nil {
		t.Fatal(err)
	}
	run(a, 400)
	var buf bytes.Buffer
	if err := a.Save(&buf); err != nil {
		t.Fatal(err)
	}
	b, err := Load(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if a.Fingerprint() != b.Fingerprint() || a.FoodLedger().Stocked != b.FoodLedger().Stocked {
		t.Fatal("loaded shops differ")
	}
	run(a, 200)
	run(b, 200)
	if a.Fingerprint() != b.Fingerprint() {
		t.Fatal("loaded world with shops diverged")
	}
	if !a.FoodLedger().Balanced() || !a.CoinLedger().Balanced() {
		t.Fatalf("ledgers %+v %+v", a.FoodLedger(), a.CoinLedger())
	}
}

// With a shop, the world is stage 11-3: its fingerprint is pinned here
// (re-pinned at 11-2c: things are sent to the shop, which deals in tents;
// 11-2d: a body at the shop sees its stock; 11-3: bodies sell to each
// other; the shop made a building with four doors; and 12-1: calls to
// sell).
func TestShopFingerprint(t *testing.T) {
	const want = uint64(0xa5881a23aea8504f)
	w, err := NewWorld(shopConfig(1), shopMap(20, 100))
	if err != nil {
		t.Fatal(err)
	}
	run(w, 1000)
	if got := w.Fingerprint(); got != want {
		t.Fatalf("fingerprint = %#x, want %#x", got, want)
	}
}

// A body sends a thing to a shop it knows from wherever it is (stage
// 11-2c): it leaves its hands now and reaches the shop after the
// distance at CarrySpeed, when the shop stocks it and pays the offer out
// of its coins. A shop with no coins, or no room, takes nothing and the
// thing is lost; food that rots on the way is lost unpaid. The food
// ledger closes throughout (a unit on its way counts as stock).
func TestShopSell(t *testing.T) {
	cfg := shopConfig(7)
	cfg.Bodies = 0
	w, err := NewWorld(cfg, shopMap(0, 100))
	if err != nil {
		t.Fatal(err)
	}
	seller := shopper(0, 9.5, 2.5, 0)
	seller.Tent = Tent{Has: true, Until: 2000}
	w.bodies = append(w.bodies, seller)
	w.nextID = 1
	w.buildGrid()
	b := &w.bodies[0]
	sendable := func() (food, tent bool) {
		for _, a := range w.possibleActions(nil, b) {
			if a.Kind == ActSell {
				food, tent = food || a.Dir == 0, tent || a.Dir == 1
			}
		}
		return
	}
	if f, tn := sendable(); f || tn {
		t.Fatal("a body that knows no shop may send")
	}
	w.rememberShop(b, 0)
	if f, tn := sendable(); f || !tn {
		t.Fatalf("send food %v tent %v; want the tent only (no food held)", f, tn)
	}
	// The tent to a shop with no coins: lost on arrival.
	w.act(0, b, Action{Kind: ActSell, Mate: 0, Dir: 1})
	if b.Tent.Has || len(w.parcels) != 1 {
		t.Fatalf("after sending: tent %v, parcels %d", b.Tent.Has, len(w.parcels))
	}
	ticks := int64(w.sendTicks(b, 0)) // 4.5 by 2.5 tiles at 0.5
	if ticks != 12 || w.parcels[0].Arrive != w.tick+ticks {
		t.Fatalf("send ticks %d, arrives %d at tick %d", ticks, w.parcels[0].Arrive, w.tick)
	}
	w.tick += ticks
	w.deliver()
	if len(w.parcels) != 0 || w.Stats().SentRefused != 1 || w.shops[0].goods[1].stock != 0 || b.Memory.Paid != 0 {
		t.Fatalf("a shop without coins: parcels %d refused %d stock %d", len(w.parcels), w.Stats().SentRefused, w.shops[0].goods[1].stock)
	}
	// With coins in the shop, a unit of food is stocked and paid for.
	w.shops[0].fund = 3
	w.coins.issued = 3
	f := w.food.foods[0]
	w.takeFood(int(w.food.foodAt[w.m.index(f.X, f.Y)])-1, b)
	w.act(0, b, Action{Kind: ActSell, Mate: 0, Dir: 0})
	if l := w.FoodLedger(); !l.Balanced() || l.Stocked != 1 || b.Held != 0 {
		t.Fatalf("a unit on its way: ledger %+v held %d", l, b.Held)
	}
	w.tick += ticks
	w.deliver()
	if b.Coins != 1 || w.shops[0].fund != 2 || w.shops[0].goods[0].stock != 1 || b.Memory.Paid != 1 || b.Memory.Sends != 2 {
		t.Fatalf("paid: coins %d fund %d stock %d row %v/%v", b.Coins, w.shops[0].fund, w.shops[0].goods[0].stock, b.Memory.Paid, b.Memory.Sends)
	}
	if !w.FoodLedger().Balanced() || !w.CoinLedger().Balanced() {
		t.Fatalf("ledgers %+v %+v", w.FoodLedger(), w.CoinLedger())
	}
	// A unit that rots on the way is lost unpaid.
	f = w.food.foods[0]
	w.takeFood(int(w.food.foodAt[w.m.index(f.X, f.Y)])-1, b)
	b.HeldBorn[0] = w.tick - int64(cfg.FoodLife) + 5
	w.act(0, b, Action{Kind: ActSell, Mate: 0, Dir: 0})
	w.tick += ticks
	w.deliver()
	if b.Coins != 1 || w.Stats().SentSpoiled != 1 || w.shops[0].goods[0].stock != 1 || !w.FoodLedger().Balanced() {
		t.Fatalf("rotten on the way: coins %d spoiled %d stock %d", b.Coins, w.Stats().SentSpoiled, w.shops[0].goods[0].stock)
	}
}

// A tent sent to a shop keeps the life it had left, and a body without
// one may buy it there with the life it kept (stage 11-2c).
func TestShopTent(t *testing.T) {
	cfg := shopConfig(8)
	cfg.Bodies = 0
	w, err := NewWorld(cfg, shopMap(0, 100))
	if err != nil {
		t.Fatal(err)
	}
	w.shops[0].fund = 2
	w.coins.issued = 2
	seller := shopper(0, 3.5, 2.5, 0)
	seller.Tent = Tent{Has: true, Until: 1500}
	w.bodies = append(w.bodies, seller, shopper(1, 2.5, 5.5, 6)) // the buyer at the west door
	w.nextID = 2
	w.buildGrid()
	s, buyer := &w.bodies[0], &w.bodies[1]
	w.rememberShop(s, 0)
	w.act(0, s, Action{Kind: ActSell, Mate: 0, Dir: 1})
	arrive := w.parcels[0].Arrive
	w.tick = arrive
	w.deliver()
	if g := w.shops[0].goods[1]; g.stock != 1 || len(g.lives) != 1 || g.lives[0] != 1500-arrive || s.Coins != 2 {
		t.Fatalf("tent in the shop: %+v, seller's coins %d", g, s.Coins)
	}
	w.tick += 300 // nothing wears in a shop
	w.act(1, buyer, Action{Kind: ActBuy, Mate: 0, Dir: 1})
	if !buyer.Tent.Has || buyer.Tent.Until != w.tick+1500-arrive || buyer.Coins != 2 || w.Stats().TentsBought != 1 {
		t.Fatalf("bought tent %+v, coins %d", buyer.Tent, buyer.Coins)
	}
}

// A body at a shop sees whether it has a good to sell (ShopSight, stage
// 11-2d): an empty shop offers no buy, and a body that walked there for
// food counts the walk as a failed buy, once. Without the sight it may
// try, and learns only by being refused.
func TestShopSight(t *testing.T) {
	cfg := shopConfig(9)
	cfg.Bodies = 0
	w, err := NewWorld(cfg, shopMap(0, 100))
	if err != nil {
		t.Fatal(err)
	}
	w.bodies = append(w.bodies, shopper(0, 2.5, 5.5, 4)) // at the west door
	w.nextID = 1
	w.buildGrid()
	b := &w.bodies[0]
	for _, a := range w.possibleActions(nil, b) {
		if a.Kind == ActBuy {
			t.Fatalf("an empty shop offers %+v", a)
		}
	}
	b.Goal = w.m.index(2, 5)
	w.noteShops(b)
	if b.Memory.Buys != 1 || b.Memory.Bought != 0 || w.Stats().BuysFailed != 1 {
		t.Fatalf("arriving at an empty shop: row %v/%v failed %d", b.Memory.Bought, b.Memory.Buys, w.Stats().BuysFailed)
	}
	b.Goal = -1
	w.noteShops(b)
	if b.Memory.Buys != 1 {
		t.Fatal("standing at the shop counted again")
	}
	w.cfg.ShopSight = false
	if !hasBuy(w, b) {
		t.Fatal("without the sight, no buy is offered")
	}
}

// A shop is a building (the user's design): no body walks into its walls,
// nothing is put on them and no settlement covers them or its doors; a
// body buys standing at a door, and walks to the nearest door to buy.
func TestShopBuilding(t *testing.T) {
	cfg := shopConfig(13)
	w, err := NewWorld(cfg, shopMap(5, 100))
	if err != nil {
		t.Fatal(err)
	}
	b := shopper(99, 4.5, 3.5, 4)
	for y := 4; y <= 6; y++ {
		for x := 3; x <= 5; x++ {
			if w.mayEnter(&b, w.m.index(x, y)) || w.FoodOn(w.m.index(x, y)) {
				t.Fatalf("wall (%d,%d) entered or holds food", x, y)
			}
		}
	}
	for _, d := range shopDoors(4, 5) {
		tl := w.m.index(d[0], d[1])
		if !w.mayEnter(&b, tl) || !w.shopGround(tl) {
			t.Fatalf("door %v", d)
		}
	}
	run(w, 300)
	for _, bd := range w.Bodies() {
		if w.walled(w.tileOf(bd.X, bd.Y)) {
			t.Fatalf("body %d stands in a wall", bd.ID)
		}
	}
	for _, p := range w.Places() {
		for _, tl := range p.Tiles {
			if w.shopGround(tl) {
				t.Fatalf("a settlement covers shop tile %d", tl)
			}
		}
	}
	m := shopMap(5, 100)
	m.Shops[0].X, m.Shops[0].Y = 2, 2 // over the places to rest
	if m.Validate() == nil {
		t.Fatal("a shop over the places to rest validated")
	}
}
