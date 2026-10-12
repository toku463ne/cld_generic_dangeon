package engine

import "testing"

// A body that hears one of its kind struck may walk to fight the hunter
// with it (stage 12-5). It reads the walk as worth something only where
// the caller lives through it and is its child (the second currency) or
// a body it likes (its liking, in energy), or where the hunter is a
// threat to itself (what it would take off it, a gain once it is down);
// a stranger's call against a hunter that cannot catch it is worth
// nothing to it, and a caller it cannot reach before it is down is worth
// only the hunter less. Two of a band striking the same hunter within
// FightTicks is counted as fighting together.
func TestHelp(t *testing.T) {
	cfg := testConfig(19)
	cfg.FoodKinds, cfg.Foods = DefaultConfig().FoodKinds, append([]FoodSpec(nil), DefaultConfig().Foods...)
	cfg.Foods[1].Terrain = TerrainLand
	cfg.Bodies, cfg.AllotCombat, cfg.HuntHunger, cfg.CallRange = 0, false, 1, 5
	cfg.Help, cfg.Ties = true, true
	w, err := NewWorld(cfg, testMap())
	if err != nil {
		t.Fatal(err)
	}
	mk := func(id int64, x, y float64, species int, energy float64) Body {
		return Body{ID: id, X: x, Y: y, Energy: energy, Species: species, Heading: -1, Goal: -1, Decided: 0, Parents: [2]int64{-1, -1}, Sex: Female, Sight: 3}
	}
	beast := mk(0, 2.5, 2.5, 1, 40) // hungry enough to hunt
	child := mk(1, 3.5, 2.5, 0, 100)
	parent := mk(2, 4.5, 2.5, 0, 100) // a tile beyond reach: there in time
	stranger := mk(3, 2.5, 5.5, 0, 100)
	friend := mk(4, 3.5, 4.5, 0, 100)
	child.Parents = [2]int64{2, -1}
	w.bodies = append(w.bodies, beast, child, parent, stranger, friend)
	w.nextID = 5
	w.buildGrid()
	b, p, s, f := &w.bodies[0], &w.bodies[2], &w.bodies[3], &w.bodies[4]
	w.tie(f, 1, 40) // the friend gained by the child before
	w.act(0, b, Action{Kind: ActAttack, Mate: 1})
	for _, o := range []*Body{p, s, f} {
		if o.Help.Caller != 1 || o.Help.Hunter != 0 {
			t.Fatalf("body %d heard %+v", o.ID, o.Help)
		}
		has := false
		for _, a := range w.possibleActions(nil, o) {
			has = has || a == (Action{Kind: ActHelp, Mate: 0})
		}
		if !has {
			t.Fatalf("body %d has no walk to help", o.ID)
		}
	}
	if _, _, down, gain, kid := w.helpRead(p); down || !kid || gain != 0 {
		t.Fatalf("parent reads down %v kid %v gain %v", down, kid, gain)
	}
	if _, _, _, gain, kid := w.helpRead(s); kid || gain != 0 {
		t.Fatalf("stranger reads kid %v gain %v", kid, gain)
	}
	if _, _, _, gain, _ := w.helpRead(f); gain != 40 {
		t.Fatalf("friend reads gain %v, want its liking 40", gain)
	}
	// A hunter faster than the stranger would catch it: downing it is a
	// gain to the stranger itself, a hunter less (the user's choice).
	b.Build = Build{Speed: 2 * w.speedOf(s), EnergyMax: w.cfg.EnergyMax, EnergyBurn: w.cfg.EnergyBurn}
	if _, _, _, gain, kid := w.helpRead(s); kid || !(gain > 0) {
		t.Fatalf("stranger reads kid %v gain %v from a faster hunter", kid, gain)
	}
	b.Build = Build{}
	far := *p
	far.X = 6.5 // four tiles off: the child is down before it comes
	if _, _, _, _, kid := w.helpRead(&far); kid {
		t.Fatal("a parent too far off reads its child saved")
	}
	x := p.X
	w.act(2, p, Action{Kind: ActHelp, Mate: 0})
	if p.X >= x || w.Stats().HelpSteps != 1 {
		t.Fatalf("the parent did not step towards the hunter: %v -> %v", x, p.X)
	}
	// Within reach, the child and the parent strike back together.
	p.X, p.Y = 2.5, 3.5
	w.buildGrid()
	c := &w.bodies[1]
	w.act(1, c, Action{Kind: ActAttack, Mate: 0})
	w.act(2, p, Action{Kind: ActAttack, Mate: 0})
	if w.Stats().JointBlows != 1 {
		t.Fatalf("joint blows %d, want 1", w.Stats().JointBlows)
	}
}
