package engine

import "testing"

// ornamentConfig is shopConfig with the default tastes and ornaments.
func ornamentConfig(seed int64) Config {
	cfg := shopConfig(seed)
	cfg.Ornaments = DefaultOrnaments()
	return cfg
}

// ornamentMap is shopMap with the shop dealing in each ornament at 3,
// paying 2.
func ornamentMap() Map {
	m := shopMap(5, 100)
	for _, o := range DefaultOrnaments() {
		m.Shops[0].Goods = append(m.Shops[0].Goods, Good{Item: o.Name, Price: 3, Offer: 2, StockCap: 4})
	}
	return m
}

// A body is born with a taste, outside the budget, and a child takes a
// parent's (stage 11-4); a body with ornaments off has none.
func TestTasteDrawn(t *testing.T) {
	w, err := NewWorld(ornamentConfig(1), ornamentMap())
	if err != nil {
		t.Fatal(err)
	}
	seen := map[int]bool{}
	for _, b := range w.bodies {
		if b.Taste < 1 || b.Taste > 3 {
			t.Fatalf("body %d has taste %d", b.ID, b.Taste)
		}
		seen[b.Taste] = true
	}
	if len(seen) != 3 {
		t.Fatalf("tastes drawn %v, want all three", seen)
	}
	cfg := ornamentConfig(1)
	cfg.AllotMutation = 0
	w, _ = NewWorld(cfg, ornamentMap())
	a, b := w.genomeOf(&w.bodies[0]), w.genomeOf(&w.bodies[1])
	for i := 0; i < 20; i++ {
		if g := w.inherit(a, b); g.Taste != a.Taste && g.Taste != b.Taste {
			t.Fatalf("child's taste %d, parents %d and %d", g.Taste, a.Taste, b.Taste)
		}
	}
	w, _ = NewWorld(shopConfig(1), shopMap(5, 100))
	if w.bodies[0].Taste != 0 {
		t.Fatal("a taste without ornaments")
	}
	bad := ornamentConfig(1)
	bad.Ornaments = []OrnamentSpec{{Name: "x", Tastes: []int{3}}}
	if _, err := NewWorld(bad, ornamentMap()); err == nil {
		t.Fatal("an ornament answering to a taste that does not exist validated")
	}
}

// A body wants an ornament its taste answers to while it holds none it
// likes; a crafter falling asleep makes the kind it wants, for
// CraftEnergy, and one that wants none makes one only where a shop it
// knows pays more than the cost.
func TestCraft(t *testing.T) {
	cfg := ornamentConfig(2)
	cfg.Bodies = 0
	w, err := NewWorld(cfg, ornamentMap())
	if err != nil {
		t.Fatal(err)
	}
	b := shopper(0, 9.5, 5.5, 0)
	b.Taste = 2 // taste 1: shell and bead
	b.Skill[SkillCraft] = 1
	w.bodies = append(w.bodies, b)
	p := &w.bodies[0]
	if !w.wants(p, 0) || !w.wants(p, 1) || w.wants(p, 2) {
		t.Fatalf("wants bead %v shell %v feather %v", w.wants(p, 0), w.wants(p, 1), w.wants(p, 2))
	}
	w.craft(p, Action{Kind: ActSleep})
	if len(p.Ornaments) != 1 || !w.likes(p, p.Ornaments[0]) || p.Energy != 40-cfg.CraftEnergy {
		t.Fatalf("made %v, energy %v", p.Ornaments, p.Energy)
	}
	if w.wants(p, 0) || w.wants(p, 1) {
		t.Fatal("still wants one with one it likes")
	}
	w.craft(p, Action{Kind: ActSleep})
	if len(p.Ornaments) != 1 {
		t.Fatal("made one more with no shop to sell it to")
	}
	w.rememberShop(p, 0) // pays 2 coins, read at 15 grains each: more than the cost
	w.craft(p, Action{Kind: ActSleep})
	if len(p.Ornaments) != 2 || w.Stats().OrnamentsMade != 2 {
		t.Fatalf("with a shop that pays: %v", p.Ornaments)
	}
}

// A body that wants an ornament the shop it stands at sells buys it
// there, without a turn, where its want outweighs the price; an ornament
// sent to a shop is stocked and paid for; ornaments are dropped where
// their holder dies and taken up; and an ornament it does not like is
// sold to a body in sight that wants it.
func TestOrnamentsChangeHands(t *testing.T) {
	cfg := ornamentConfig(3)
	cfg.Bodies = 0
	w, err := NewWorld(cfg, ornamentMap())
	if err != nil {
		t.Fatal(err)
	}
	seller := shopper(0, 9.5, 5.5, 0)
	seller.Taste = 3               // taste 2: shell and feather
	seller.Ornaments = []int{0, 2} // a bead it does not like, a feather it does
	buyer := shopper(1, 10.5, 5.5, 5)
	buyer.Taste = 1 // taste 0: bead and feather
	w.bodies = append(w.bodies, seller, buyer)
	w.nextID = 2
	w.buildGrid()
	s, p := &w.bodies[0], &w.bodies[1]
	w.coins.issued = 5
	w.rememberShop(s, 0)
	if w.itemLoss(s, ornamentItem+0) != 0 || w.itemLoss(s, ornamentItem+2) != w.wantGrains() {
		t.Fatalf("losses: bead %d feather %d", w.itemLoss(s, ornamentItem), w.itemLoss(s, ornamentItem+2))
	}
	var offer Action
	for _, a := range w.possibleActions(nil, s) {
		// The feather it likes is offered too, at its want's price.
		if a.Kind == ActSellTo && a.Dir == ornamentItem+0 && a.Mate == 1 {
			offer = a
		}
	}
	if offer.Kind != ActSellTo {
		t.Fatal("no offer of the bead")
	}
	w.act(0, s, offer)
	if heldOrnament(p, 0) < 0 || heldOrnament(s, 0) >= 0 || w.Stats().OrnamentSales != 1 || p.Coins != 5-w.askOf(s, ornamentItem) {
		t.Fatalf("after the sale: seller %v buyer %v coins %d", s.Ornaments, p.Ornaments, p.Coins)
	}
	// The feather to the shop, which has coins to pay.
	w.shops[0].fund = 10
	w.coins.issued += 10
	n := w.OrnamentCount()
	g := 2 + 2 // food, tent, bead, shell, feather
	w.act(0, s, Action{Kind: ActSell, Mate: 0, Dir: g})
	w.tick = w.parcels[0].Arrive
	w.deliver()
	if w.shops[0].goods[g].stock != 1 || s.Coins != 1+2 || w.OrnamentCount() != n {
		t.Fatalf("feather sent: stock %d coins %d count %d of %d", w.shops[0].goods[g].stock, s.Coins, w.OrnamentCount(), n)
	}
	// A body that wants a feather, at the shop, buys it there.
	fan := shopper(2, 2.5, 5.5, 4) // at the west door
	fan.Taste = 3
	w.bodies = append(w.bodies, fan)
	f := &w.bodies[2]
	w.shopOrnaments(f)
	if heldOrnament(f, 2) < 0 || f.Coins != 1 || w.Stats().OrnamentsBought != 1 {
		t.Fatalf("at the shop: %v coins %d", f.Ornaments, f.Coins)
	}
	// Dropped where it dies, taken up by the next to stand there.
	f.Energy = 0
	w.removeDead()
	if len(w.groundOrnaments) != 1 || w.OrnamentCount() != n {
		t.Fatalf("on the ground %v, count %d", w.groundOrnaments, w.OrnamentCount())
	}
	p = &w.bodies[1]
	p.X, p.Y = 2.5, 5.5
	w.pickOrnaments(p)
	if len(w.groundOrnaments) != 0 || len(p.Ornaments) != 2 {
		t.Fatalf("picked: %v", p.Ornaments)
	}
}

// With ornaments, the world is stage 11-4: its fingerprint is pinned here.
func TestOrnamentFingerprint(t *testing.T) {
	const want = uint64(0x70b1f4d9ec0af070)
	w, err := NewWorld(ornamentConfig(1), ornamentMap())
	if err != nil {
		t.Fatal(err)
	}
	run(w, 1000)
	if got := w.Fingerprint(); got != want {
		t.Fatalf("fingerprint = %#x, want %#x (made %d)", got, want, w.Stats().OrnamentsMade)
	}
}

// A maker for a shop draws among the kinds the shop pays alike for: every
// kind gets made, the added ones too.
func TestCraftDrawsAmongTies(t *testing.T) {
	cfg := ornamentConfig(4)
	cfg.Ornaments = SixOrnaments()
	cfg.Bodies = 0
	m := ornamentMap()
	for _, o := range SixOrnaments()[3:] {
		m.Shops[0].Goods = append(m.Shops[0].Goods, Good{Item: o.Name, Price: 3, Offer: 2, StockCap: 4})
	}
	w, err := NewWorld(cfg, m)
	if err != nil {
		t.Fatal(err)
	}
	made := map[int]bool{}
	for i := 0; i < 60; i++ {
		b := shopper(int64(i), 9.5, 5.5, 0)
		b.Skill[SkillCraft] = 1
		b.Taste = 1            // likes bead and feather
		b.Ornaments = []int{0} // holds a bead: wants none, has room for one
		w.rememberShop(&b, 0)
		w.craft(&b, Action{Kind: ActSleep})
		if len(b.Ornaments) == 2 {
			made[b.Ornaments[1]] = true
		}
	}
	if len(made) != 6 {
		t.Fatalf("kinds made for the shop %v, want all six", made)
	}
}
