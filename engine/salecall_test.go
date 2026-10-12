package engine

import "testing"

// A body with a thing to sell and no buyer in sight calls it out with its
// price (stage 12-1), spending no turn; a body out of its sight within
// RequestRange that would take it at that price walks there, and once in
// sight pays and takes it - the call is the seller's agreement. A seller
// with a buyer in sight offers there instead and calls nothing.
func TestSaleCall(t *testing.T) {
	cfg := shopConfig(14)
	cfg.Bodies = 0
	w, err := NewWorld(cfg, shopMap(5, 100))
	if err != nil {
		t.Fatal(err)
	}
	seller := shopper(0, 9.5, 1.5, 0)
	seller.Sight = 2
	buyer := shopper(1, 9.5, 5.5, 4) // four tiles off: out of sight 2, within range 5
	buyer.Sight = 2
	w.bodies = append(w.bodies, seller, buyer)
	w.nextID = 2
	w.coins.issued = 4
	w.buildGrid()
	s, p := &w.bodies[0], &w.bodies[1]
	f := w.food.foods[0]
	w.takeFood(int(w.food.foodAt[w.m.index(f.X, f.Y)])-1, s)
	w.rememberShop(s, 0) // reads coins at what they buy: it asks 3
	w.listCalling()
	w.callSale(s)
	if s.Selling.Until != w.tick+int64(cfg.RequestTicks) || s.Selling.Item != 1 || s.Selling.Price != 3 || w.Stats().SaleCalls != 1 {
		t.Fatalf("call %+v", s.Selling)
	}
	var buy Action
	for _, a := range w.possibleActions(nil, p) {
		if a.Kind == ActBuyFrom {
			buy = a
		}
	}
	if buy != (Action{Kind: ActBuyFrom, Mate: 0, Dir: 1}) || w.toCaller[0] == 0 {
		t.Fatalf("buyer's option %+v, walk %d", buy, w.toCaller[0])
	}
	y := p.Y
	w.act(1, p, buy)
	if p.Y >= y || w.Stats().CallSales != 0 {
		t.Fatalf("out of sight the buyer steps towards the caller: y %v -> %v", y, p.Y)
	}
	p.Y = 2.5 // now in sight (by night it sees a tile)
	w.buildGrid()
	w.act(1, p, buy)
	if w.Stats().CallSales != 1 || s.Coins != 3 || p.Coins != 1 || s.Held != 0 || s.Selling.Until != 0 {
		t.Fatalf("after the sale: calls sold %d, coins %d/%d, held %d, call %+v", w.Stats().CallSales, s.Coins, p.Coins, s.Held, s.Selling)
	}
	if !w.FoodLedger().Balanced() || !w.CoinLedger().Balanced() {
		t.Fatal("ledgers do not balance")
	}
	// With a buyer in sight a seller offers there and calls nothing.
	f = w.food.foods[0]
	w.takeFood(int(w.food.foodAt[w.m.index(f.X, f.Y)])-1, s)
	p.Coins = 4
	w.coins.issued += 3
	w.callSale(s)
	if s.Selling.Until != 0 {
		t.Fatalf("called with a buyer in sight: %+v", s.Selling)
	}
}
