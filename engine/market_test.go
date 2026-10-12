package engine

import "testing"

// A body at a shop's door may leave food it would sell on the shop's
// board at its own asking price (stage 12-2); a body at a door that would
// take it at that price buys it there, paying the seller wherever it is.
// Nothing ages on the board, as nothing does in a shop; a board holds
// MarketCap things.
// The food and coin ledgers close throughout.
func TestMarket(t *testing.T) {
	cfg := shopConfig(16)
	cfg.Bodies = 0
	cfg.MarketCap = 1
	w, err := NewWorld(cfg, shopMap(5, 100))
	if err != nil {
		t.Fatal(err)
	}
	seller := shopper(0, 4.5, 3.5, 0) // at the north door
	buyer := shopper(1, 2.5, 5.5, 4)  // at the west door
	w.bodies = append(w.bodies, seller, buyer)
	w.nextID = 2
	w.coins.issued = 4
	w.buildGrid()
	s, p := &w.bodies[0], &w.bodies[1]
	f := w.food.foods[0]
	w.takeFood(int(w.food.foodAt[w.m.index(f.X, f.Y)])-1, s)
	w.rememberShop(s, 0)
	var list Action
	for _, a := range w.possibleActions(nil, s) {
		if a.Kind == ActList {
			list = a
		}
	}
	if list != (Action{Kind: ActList, Mate: 0, Dir: 1}) {
		t.Fatalf("list option %+v", list)
	}
	w.act(0, s, list)
	if len(w.listings) != 1 || w.listings[0].Price != 3 || s.Held != 0 || w.Stats().Listed != 1 {
		t.Fatalf("board %+v, held %d", w.listings, s.Held)
	}
	if l := w.FoodLedger(); !l.Balanced() || l.Stocked != 6 { // 5 in stock, 1 on the board
		t.Fatalf("ledger %+v", l)
	}
	// The board is full: nothing more is offered to list.
	f = w.food.foods[0]
	w.takeFood(int(w.food.foodAt[w.m.index(f.X, f.Y)])-1, s)
	for _, a := range w.possibleActions(nil, s) {
		if a.Kind == ActList {
			t.Fatal("listed on a full board")
		}
	}
	// The buyer, who knows no shop, takes it at 3; the seller is paid
	// where it has gone.
	s.X, s.Y = 9.5, 8.5
	var buy Action
	for _, a := range w.possibleActions(nil, p) {
		if a.Kind == ActBuyListed {
			buy = a
		}
	}
	if buy.Kind != ActBuyListed {
		t.Fatal("no buy off the board")
	}
	w.act(1, p, buy)
	if len(w.listings) != 0 || p.Coins != 1 || s.Coins != 3 || s.Memory.Paid != 1 || w.Stats().ListedSales != 1 {
		t.Fatalf("after the buy: board %d, coins %d/%d", len(w.listings), p.Coins, s.Coins)
	}
	if !w.FoodLedger().Balanced() || !w.CoinLedger().Balanced() {
		t.Fatal("ledgers do not balance")
	}
	// Nothing ages on the board: a unit left at age 100 is bought at age
	// 100, however long it waited.
	s.X, s.Y = 4.5, 3.5
	s.HeldBorn[0] = w.tick - 100
	w.act(0, s, list)
	w.tick += 5000
	p.Coins = 4
	w.coins.issued += 3
	w.act(1, p, Action{Kind: ActBuyListed, Mate: 0, Dir: 1})
	if len(w.listings) != 0 || (p.Held == 1 && p.HeldBorn[0] != w.tick-100) {
		t.Fatalf("aged on the board: held %d born %v at tick %d", p.Held, p.HeldBorn, w.tick)
	}
}
