package engine

import "testing"

// A body holding a folded tent, with room for a meal, may offer the tent
// to one of its kind in sight that holds food and no tent (Barter, stage
// 11-1): the option is valued as the meal now less the tent's credit,
// and the holder of the food agrees only where the tent's credit, as it
// reads it, outweighs a meal. A trade moves the tent one way and the
// unit the other, and the offerer eats; a refusal moves nothing. No
// option without Barter, with the tent pitched, or with no room for a
// meal.
func TestBarter(t *testing.T) {
	cfg := testConfig(31)
	cfg.Bodies = 0
	w, err := NewWorld(cfg, testMap())
	if err != nil {
		t.Fatal(err)
	}
	for len(w.food.foods) > 1 {
		w.eatFood(len(w.food.foods) - 1)
	}
	// One unit left; the holder stands on it and picks it up, full.
	f := w.food.foods[0]
	holder := Body{ID: 0, X: float64(f.X) + 0.5, Y: float64(f.Y) + 0.5, Energy: 100, Heading: -1, Goal: -1, Decided: -1, Parents: [2]int64{-1, -1}, Sex: Female, Sight: 3, Born: -1500, Mature: -700}
	offerer := Body{ID: 1, X: float64(f.X) + 1.5, Y: float64(f.Y) + 0.5, Energy: 40, Heading: -1, Goal: -1, Decided: -1, Parents: [2]int64{-1, -1}, Sex: Male, Sight: 3, Born: -1500, Mature: -700, Tent: Tent{Has: true, Until: w.tick + 2000, Uses: 3}}
	if !w.m.InBounds(f.X+1, f.Y) {
		offerer.X = float64(f.X) - 0.5
	}
	w.bodies = append(w.bodies, holder, offerer)
	w.nextID = 2
	w.buildGrid()
	p, b := &w.bodies[0], &w.bodies[1]
	w.takeFood(w.foodFor(p, w.tileOf(p.X, p.Y)), p)
	if p.Held != 1 || p.Energy != 100 {
		t.Fatalf("the holder holds %d, energy %v", p.Held, p.Energy)
	}
	offered := func() bool {
		for _, a := range w.possibleActions(nil, b) {
			if a == (Action{Kind: ActTrade, Mate: 0}) {
				return true
			}
		}
		return false
	}
	if !offered() {
		t.Fatal("the offerer is not offered the trade")
	}
	for _, a := range w.possibleActions(nil, p) {
		if a.Kind == ActTrade {
			t.Fatal("the holder, with no tent, is offered a trade")
		}
	}
	// Valued: the meal now beats waiting hungry.
	w.decide(b)
	v := &w.valuation
	trade, wait := -1.0, -1.0
	for j, a := range v.Options {
		switch a.Kind {
		case ActTrade:
			trade = v.Risk[0][j]
		case ActWait:
			wait = v.Risk[0][j]
		}
	}
	if trade < 0 || !(trade < wait) {
		t.Fatalf("the trade reads %v, waiting %v", trade, wait)
	}
	// The holder has felt neither cold nor warmth: a tent is worth
	// nothing to it, and it refuses.
	w.act(1, b, Action{Kind: ActTrade, Mate: 0})
	if w.stats.TradesRefused != 1 || w.stats.Trades != 0 || !b.Tent.Has || p.Held != 1 {
		t.Fatalf("refused %d, traded %d, tent %v, held %d", w.stats.TradesRefused, w.stats.Trades, b.Tent.Has, p.Held)
	}
	// The offer refused goes into the trade row (stage 11-1b): the
	// chance of an offer taken falls below the prior, and the trade
	// option reads nearer to waiting.
	if b.Memory.Offers != 1 || b.Memory.Traded != 0 || !(w.tradeRate(b) < w.cfg.PriorTrade) {
		t.Fatalf("after a refusal the row is %v/%v, rate %v", b.Memory.Traded, b.Memory.Offers, w.tradeRate(b))
	}
	w.decide(b)
	v = &w.valuation
	again := -1.0
	for j, a := range v.Options {
		if a.Kind == ActTrade {
			again = v.Risk[0][j]
		}
	}
	if again < 0 || !(again > trade) || !(again <= wait) {
		t.Fatalf("after a refusal the trade reads %v, was %v, waiting %v", again, trade, wait)
	}
	// One that expects six nights in a tent, each saving a third of its
	// burn, reads it above a meal and agrees.
	p.Memory.Cold, p.Memory.Warm, p.Memory.TentUses = [2]float64{1.3, 1}, [2]float64{1, 1}, [2]float64{10, 1}
	if w.tentCredit(p) <= w.pred.meal {
		t.Fatalf("the holder reads the tent at %d grains, a meal at %d", w.tentCredit(p), w.pred.meal)
	}
	until := b.Tent.Until
	w.act(1, b, Action{Kind: ActTrade, Mate: 0})
	if w.stats.Trades != 1 || b.Tent.Has || !p.Tent.Has || p.Tent.Until != until || p.Tent.Pitched {
		t.Fatalf("traded %d, offerer's tent %v, holder's %+v", w.stats.Trades, b.Tent, p.Tent)
	}
	if p.Held != 0 || b.Held != 0 || b.Energy != 70 {
		t.Fatalf("after the trade the holder holds %d, the offerer holds %d at energy %v", p.Held, b.Held, b.Energy)
	}
	if b.Memory.Offers != 2 || b.Memory.Traded != 1 {
		t.Fatalf("after a trade the row is %v/%v", b.Memory.Traded, b.Memory.Offers)
	}
	checkFood(t, w)
	if !w.FoodLedger().Balanced() {
		t.Fatalf("ledger: %+v", w.FoodLedger())
	}
	// With the tent gone, nothing to offer; with a unit back in the
	// holder's hold and a tent in the offerer's, offered again - but not
	// with the tent pitched, nor with no room for a meal, nor without
	// Barter.
	if offered() {
		t.Fatal("offered with no tent")
	}
	p.Held, p.HeldBorn, p.HeldPool = 1, []int64{w.tick}, []int{0}
	w.food.pools[0].held++
	b.Tent, p.Tent = Tent{Has: true, Until: w.tick + 2000}, Tent{}
	if !offered() {
		t.Fatal("not offered again")
	}
	b.Tent.Pitched = true
	if offered() {
		t.Fatal("offered with the tent pitched")
	}
	b.Tent.Pitched = false
	b.Energy = 90
	if offered() {
		t.Fatal("offered with no room for a meal")
	}
	b.Energy = 40
	w.cfg.Barter = false
	if offered() {
		t.Fatal("offered without Barter")
	}
}
