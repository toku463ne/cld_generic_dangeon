package engine

import (
	"bytes"
	"testing"
)

// shopMap is tempMap with a shop on tile (2, 2) selling the one pool of
// testConfig's food at price 2.
func shopMap(stock, fund int) Map {
	m := tempMap()
	m.Shops = []Shop{{X: 2, Y: 2, Food: "food", Price: 2, Stock: stock, FundCap: fund}}
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
	w.bodies = append(w.bodies, shopper(0, 3.5, 3.5, 5), shopper(1, 6.5, 6.5, 5), shopper(2, 2.5, 1.5, 1))
	w.nextID = 3
	w.buildGrid()
	near, far, poor := &w.bodies[0], &w.bodies[1], &w.bodies[2]
	if !hasBuy(w, near) || hasBuy(w, far) || hasBuy(w, poor) {
		t.Fatalf("buy offered: near %v far %v poor %v; want only near", hasBuy(w, near), hasBuy(w, far), hasBuy(w, poor))
	}
	w.act(0, near, Action{Kind: ActBuy, Mate: 0})
	if near.Coins != 3 || near.Energy != 40+cfg.FoodEnergy || w.shops[0].fund != 2 || w.shops[0].stock != 0 {
		t.Fatalf("after a buy: coins %d energy %v fund %d stock %d", near.Coins, near.Energy, w.shops[0].fund, w.shops[0].stock)
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
	w.act(0, near, Action{Kind: ActBuy, Mate: 0})
	if near.Coins != 3 || near.Memory.Buys != 2 || near.Memory.Bought != 1 || w.Stats().BuysFailed != 1 {
		t.Fatalf("buy at an empty shop: coins %d row %v/%v failed %d", near.Coins, near.Memory.Bought, near.Memory.Buys, w.Stats().BuysFailed)
	}
	// Stocked again but its fund full (3, price 2): no sale.
	w.shops[0].stock, w.food.pools[0].stocked = 1, 1
	w.food.pools[0].appeared++
	w.act(0, near, Action{Kind: ActBuy, Mate: 0})
	if near.Coins != 3 || w.shops[0].stock != 1 || w.Stats().BuysFailed != 2 {
		t.Fatalf("buy at a full shop: coins %d stock %d failed %d", near.Coins, w.shops[0].stock, w.Stats().BuysFailed)
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
	w.bodies = append(w.bodies, shopper(0, 2.5, 7.5, 2)) // out of the shop's sight
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
	if b.Goal != w.shopTile(0) {
		t.Fatalf("goal %d, want the shop's tile %d", b.Goal, w.shopTile(0))
	}
	// Without the price, a coin in sight that makes it up is the goal.
	b.Coins = 1
	w.coins.at[w.m.index(3, 7)] = 1
	w.coins.issued = 1
	b.Decided = -1
	w.turn(b)
	if b.Goal != w.m.index(3, 7) {
		t.Fatalf("goal %d, want the coin's tile %d", b.Goal, w.m.index(3, 7))
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

// With a shop, the world is stage 11-2b: its fingerprint is pinned here.
func TestShopFingerprint(t *testing.T) {
	const want = uint64(0xdff1752926dce4cc)
	w, err := NewWorld(shopConfig(1), shopMap(20, 100))
	if err != nil {
		t.Fatal(err)
	}
	run(w, 1000)
	if got := w.Fingerprint(); got != want {
		t.Fatalf("fingerprint = %#x, want %#x", got, want)
	}
}
