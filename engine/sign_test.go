package engine

import "testing"

// A body that calls out food to sell and has a settlement puts up a sign
// at its middle (stage 12-2); a body that sees the sign remembers it and,
// if it would buy at that price, reads a walk to it as a walk to a shop.
// Come to the sign with the seller in sight, it buys at the price signed;
// with the seller away, the walk counts as a failed buy and the sign is
// forgotten. A sign comes down when the seller no longer has the thing.
func TestSign(t *testing.T) {
	cfg := shopConfig(15)
	cfg.Bodies = 0
	w, err := NewWorld(cfg, shopMap(5, 100))
	if err != nil {
		t.Fatal(err)
	}
	seller := shopper(0, 9.5, 1.5, 0)
	seller.Sight = 2
	w.bodies = append(w.bodies, seller)
	w.nextID = 1
	s := &w.bodies[0]
	s.Work = Work{Site: w.m.index(9, 1) + 1}
	w.settle(s)
	s.Work = Work{}
	f := w.food.foods[0]
	w.takeFood(int(w.food.foodAt[w.m.index(f.X, f.Y)])-1, s)
	w.rememberShop(s, 0)
	w.buildGrid()
	w.listCalling()
	w.callSale(s)
	if len(w.signs) != 1 || w.signs[0].Seller != 0 || w.signs[0].Item != 1 || w.signs[0].Price != 3 {
		t.Fatalf("signs %+v", w.signs)
	}
	tile := w.signs[0].Tile
	// A buyer sees the sign later, when the seller is elsewhere (by night
	// it sees a tile).
	w.bodies = append(w.bodies, shopper(1, 10.5, 2.5, 4))
	w.nextID = 2
	s, p := &w.bodies[0], &w.bodies[1]
	p.Sight = 3
	s.X, s.Y = 10.5, 8.5 // away from its settlement
	w.buildGrid()
	w.noteSigns(p)
	if len(p.Memory.Signs) != 1 || p.Memory.Signs[0].Tile != tile {
		t.Fatalf("buyer remembers %+v", p.Memory.Signs)
	}
	if f, ok := w.signAhead(p); !ok || w.m.index(f.X, f.Y) != tile {
		t.Fatalf("sign ahead %v %v", f, ok)
	}
	// Come to the sign with the seller away: a failed buy, forgotten.
	p.X, p.Y = float64(tile%w.m.Width)+0.5, float64(tile/w.m.Width)+0.5
	p.Goal = tile
	p.Sight = 1
	w.noteSigns(p)
	if w.Stats().SignMisses != 1 || len(p.Memory.Signs) != 0 || p.Memory.Buys != 1 {
		t.Fatalf("miss %d, signs %+v", w.Stats().SignMisses, p.Memory.Signs)
	}
	// Seen again, and come to with the seller there: bought at 3.
	p.Goal = -1
	w.noteSigns(p)
	s.X, s.Y = p.X+1, p.Y
	w.buildGrid()
	p.Goal = tile
	p.Sight = 3
	w.noteSigns(p)
	if len(w.signs) == 0 {
		t.Fatal("the sign came down")
	}
	if w.Stats().SignSales != 1 || p.Coins != 1 || s.Coins != 3 || s.Held != 0 {
		t.Fatalf("sale %d, coins %d/%d, held %d", w.Stats().SignSales, p.Coins, s.Coins, s.Held)
	}
	w.wearSigns()
	if len(w.signs) != 0 {
		t.Fatal("the sign stands with nothing left to sell")
	}
}
