package engine

import "testing"

// A body may offer a thing it holds to one of its kind in sight that
// holds coins and could use it (stage 11-3): it asks the thing's worth
// to it in coins, rounded up, plus SellMargin, and the buyer takes it
// where the thing is worth more to it than the coins. A seller that
// knows no shop reads coins at nothing and offers nothing; a buyer that
// knows none pays, one that trusts the shop to sell the same meal for
// less does not. Coins and the unit change hands; the food ledger and
// the coin ledger close.
func TestSellTo(t *testing.T) {
	cfg := shopConfig(12)
	cfg.Bodies = 0
	w, err := NewWorld(cfg, shopMap(5, 100))
	if err != nil {
		t.Fatal(err)
	}
	w.bodies = append(w.bodies, shopper(0, 9.5, 5.5, 0), shopper(1, 10.5, 5.5, 5))
	w.nextID = 2
	w.buildGrid()
	s, p := &w.bodies[0], &w.bodies[1]
	w.coins.issued = 5
	f := w.food.foods[0]
	w.takeFood(int(w.food.foodAt[w.m.index(f.X, f.Y)])-1, s)
	offers := func() []Action {
		var a []Action
		for _, o := range w.possibleActions(nil, s) {
			if o.Kind == ActSellTo {
				a = append(a, o)
			}
		}
		return a
	}
	if len(offers()) != 0 {
		t.Fatal("a seller that knows no shop offers")
	}
	w.rememberShop(s, 0)
	o := offers()
	if len(o) != 1 || o[0] != (Action{Kind: ActSellTo, Mate: 1, Dir: 1}) {
		t.Fatalf("offers %+v, want the unit to body 1", o)
	}
	if ask := w.askOf(s, 1); ask != 3 {
		t.Fatalf("asks %d, want a meal at 2 coins plus 1", ask)
	}
	// A buyer that trusts the shop reads 3 coins at a meal and a half.
	w.rememberShop(p, 0)
	w.act(0, s, o[0])
	if s.Coins != 0 || s.Held != 1 || w.Stats().SalesRefused != 1 || s.Memory.Offers != 1 {
		t.Fatalf("refused: seller coins %d held %d refused %d", s.Coins, s.Held, w.Stats().SalesRefused)
	}
	// One turned away from it reads coins at less, and pays.
	p.Memory.Buys = 6
	w.observe(&p.Memory.Buy, 6, 0)
	w.act(0, s, o[0])
	if s.Coins != 3 || p.Coins != 2 || s.Held != 0 || p.Energy != 40+cfg.FoodEnergy || w.Stats().Sales != 1 || s.Memory.Traded != 1 {
		t.Fatalf("sold: coins %d/%d held %d energy %v sales %d", s.Coins, p.Coins, s.Held, p.Energy, w.Stats().Sales)
	}
	if !w.FoodLedger().Balanced() || !w.CoinLedger().Balanced() {
		t.Fatalf("ledgers %+v %+v", w.FoodLedger(), w.CoinLedger())
	}
}
