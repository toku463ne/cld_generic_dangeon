package engine

import (
	"math"
	"testing"
)

// pairWorld is the test map with no bodies but a and b, adults side by side
// unless the caller says otherwise.
func pairWorld(t *testing.T, a, b Body) *World {
	t.Helper()
	cfg := testConfig(1)
	cfg.Bodies = 0
	cfg.FemaleBears = false // each pays half (3-1); TestMotherBears turns it on
	w, err := NewWorld(cfg, testMap())
	if err != nil {
		t.Fatal(err)
	}
	for i, x := range []Body{a, b} {
		x.Heading, x.Goal, x.Decided, x.Parents = -1, -1, -1, [2]int64{-1, -1}
		x.Sex = Female + Sex(i) // one of each: they can mate
		w.bodies = append(w.bodies, x)
	}
	w.nextID = 2
	w.buildGrid()
	return w
}

func hasMate(opts []Action, id int64) bool {
	for _, o := range opts {
		if o == (Action{Kind: ActMate, Mate: id}) {
			return true
		}
	}
	return false
}

// A mate is offered with an adult in sight, to an adult that can pay its
// share and live; not to or with a child, and not out of sight.
func TestMateOptions(t *testing.T) {
	for _, c := range []struct {
		name   string
		a, b   Body
		offerA bool
	}{
		{"adults in sight", Body{ID: 0, X: 2.5, Y: 2.5, Energy: 100}, Body{ID: 1, X: 3.5, Y: 3.5, Energy: 100}, true},
		{"out of sight", Body{ID: 0, X: 2.5, Y: 2.5, Energy: 100}, Body{ID: 1, X: 4.5, Y: 2.5, Energy: 100}, false},
		{"partner a child", Body{ID: 0, X: 2.5, Y: 2.5, Energy: 100}, Body{ID: 1, X: 2.5, Y: 2.5, Energy: 100, Mature: 50}, false},
		{"body a child", Body{ID: 0, X: 2.5, Y: 2.5, Energy: 100, Mature: 50}, Body{ID: 1, X: 2.5, Y: 2.5, Energy: 100}, false},
		{"paying would kill", Body{ID: 0, X: 2.5, Y: 2.5, Energy: 25}, Body{ID: 1, X: 2.5, Y: 2.5, Energy: 100}, false},
		{"paying leaves a little", Body{ID: 0, X: 2.5, Y: 2.5, Energy: 25.5}, Body{ID: 1, X: 2.5, Y: 2.5, Energy: 100}, true},
	} {
		w := pairWorld(t, c.a, c.b)
		if got := hasMate(w.possibleActions(nil, &w.bodies[0]), 1); got != c.offerA {
			t.Errorf("%s: mate offered %v, want %v", c.name, got, c.offerA)
		}
	}
}

// A birth needs both: a mate the partner has not chosen does nothing, and
// one it has makes one child, paid for half by each, born where the body
// that carried the mate out stands, an adult MatureAge ticks on.
func TestBirthNeedsBoth(t *testing.T) {
	w := pairWorld(t, Body{ID: 0, X: 2.5, Y: 2.5, Energy: 80}, Body{ID: 1, X: 3.5, Y: 2.5, Energy: 60})
	a, b := &w.bodies[0], &w.bodies[1]
	a.Intent = Action{Kind: ActMove}
	w.act(1, b, Action{Kind: ActMate, Mate: 0})
	if len(w.born) != 0 || a.Energy != 80 || b.Energy != 60 {
		t.Fatalf("a mate the partner did not choose made %d children, energies %v %v", len(w.born), a.Energy, b.Energy)
	}
	a.Intent = Action{Kind: ActMate, Mate: 1}
	w.act(1, b, Action{Kind: ActMate, Mate: 0})
	if len(w.born) != 1 {
		t.Fatalf("%d children born, want 1", len(w.born))
	}
	share := w.cfg.BirthEnergy / 2
	c := w.born[0]
	if a.Energy != 80-share || b.Energy != 60-share || c.Energy != w.cfg.BirthEnergy {
		t.Fatalf("energies after the birth: parents %v %v, child %v", a.Energy, b.Energy, c.Energy)
	}
	if c.X != b.X || c.Y != b.Y || c.Mature != w.tick+int64(w.cfg.MatureAge) || c.Parents != [2]int64{0, 1} || c.ID != 2 {
		t.Fatalf("child %+v", c)
	}
	// Neither intends to mate after, so the partner's turn makes no second.
	w.act(0, a, Action{Kind: ActMate, Mate: 1})
	if len(w.born) != 1 || w.stats.Births != 1 {
		t.Fatalf("%d children after the partner's turn, want 1", len(w.born))
	}
}

// Every child born in a world under way was born of two bodies whose last
// decision before the birth was to mate with the other; children come of
// age and are counted.
func TestBirthsAreAgreed(t *testing.T) {
	cfg := testConfig(3)
	cfg.Requests = false // with requests a request is consent (TestRequest)
	w, err := NewWorld(cfg, testMap())
	if err != nil {
		t.Fatal(err)
	}
	type choice struct {
		born int // children born this tick before the decision
		a    Action
	}
	last := map[int64]Action{}  // up to the end of the last tick
	now := map[int64][]choice{} // this tick, in order
	w.SetTrace(func(b Body, _ Valuation, a Action) { now[b.ID] = append(now[b.ID], choice{len(w.born), a}) })
	before := func(id int64, k int) Action {
		a := last[id]
		for _, c := range now[id] {
			if c.born <= k {
				a = c.a
			}
		}
		return a
	}
	births := 0
	for i := 0; i < 3000; i++ {
		ids := map[int64]bool{}
		for _, b := range w.Bodies() {
			ids[b.ID] = true
		}
		clear(now)
		w.Step()
		k := 0
		for _, b := range w.Bodies() {
			if ids[b.ID] {
				continue
			}
			p, q := b.Parents[0], b.Parents[1]
			if before(p, k) != (Action{Kind: ActMate, Mate: q}) || before(q, k) != (Action{Kind: ActMate, Mate: p}) {
				t.Fatalf("tick %d: child %d of %d and %d, who last chose %v and %v", w.Tick(), b.ID, p, q, before(p, k), before(q, k))
			}
			k++
			births++
		}
		for id, cs := range now {
			last[id] = cs[len(cs)-1].a
		}
	}
	st := w.Stats()
	if births == 0 || st.Matured == 0 {
		t.Fatalf("births %d, matured %d", births, st.Matured)
	}
}

// Mating is a comparison, not a threshold: a body whose risk the payment
// would raise by more than ChildWorth does not mate; raised by less, it
// does.
func TestMateIsValued(t *testing.T) {
	for _, energy := range []float64{100, 26, 40, 60} {
		w := pairWorld(t, Body{ID: 0, X: 2.5, Y: 2.5, Energy: energy}, Body{ID: 1, X: 2.5, Y: 2.5, Energy: 100})
		a := w.decide(&w.bodies[0])
		v := w.valuation
		best, mate := math.Inf(1), -1
		for j, o := range v.Options {
			if o.Kind == ActMate {
				mate = j
			} else {
				best = math.Min(best, v.Risk[0][j])
			}
		}
		if mate < 0 {
			t.Fatalf("energy %v: no mate offered", energy)
		}
		want := v.Risk[0][mate]-best < w.cfg.ChildWorth
		if got := a.Kind == ActMate; got != want {
			t.Errorf("energy %v: mated %v, risk %v against %v", energy, got, v.Risk[0][mate], best)
		}
		if energy == 100 && a.Kind != ActMate {
			t.Errorf("a full body did not mate: risk %v against %v", v.Risk[0][mate], best)
		}
	}
}

// With a child worth nothing, paying for one is only a loss.
func TestWorthlessChildIsNotMated(t *testing.T) {
	w := pairWorld(t, Body{ID: 0, X: 2.5, Y: 2.5, Energy: 100}, Body{ID: 1, X: 2.5, Y: 2.5, Energy: 100})
	w.cfg.ChildWorth = 0
	if a := w.decide(&w.bodies[0]); a.Kind == ActMate {
		t.Fatalf("mated for a child worth nothing: %+v", w.valuation)
	}
}

// With Sexes only adults of the two sexes see each other as mates; bodies
// are born of one sex or the other alike; without Sexes none has a sex.
func TestMatesAreOfTheOtherSex(t *testing.T) {
	w := pairWorld(t, Body{ID: 0, X: 2.5, Y: 2.5, Energy: 100}, Body{ID: 1, X: 3.5, Y: 2.5, Energy: 100})
	a, b := &w.bodies[0], &w.bodies[1]
	if opts := w.mateOptions(nil, a); !hasMate(opts, 1) {
		t.Fatal("no mate offered with a body of the other sex in sight")
	}
	b.Sex = a.Sex
	if opts := w.mateOptions(nil, a); hasMate(opts, 1) {
		t.Fatal("a mate offered with a body of the same sex")
	}
	w.cfg.Sexes = false
	if opts := w.mateOptions(nil, a); !hasMate(opts, 1) {
		t.Fatal("without sexes, no mate offered")
	}
	cfg := testConfig(3)
	cfg.Bodies = 400
	w2, err := NewWorld(cfg, testMap())
	if err != nil {
		t.Fatal(err)
	}
	females := 0
	for _, x := range w2.Bodies() {
		switch x.Sex {
		case Female:
			females++
		case Male:
		default:
			t.Fatalf("body %d has sex %d", x.ID, x.Sex)
		}
	}
	if females < 160 || females > 240 {
		t.Fatalf("%d of 400 bodies female", females)
	}
	cfg.Sexes = false
	w3, err := NewWorld(cfg, testMap())
	if err != nil {
		t.Fatal(err)
	}
	for _, x := range w3.Bodies() {
		if x.Sex != NoSex {
			t.Fatalf("without sexes, body %d has sex %d", x.ID, x.Sex)
		}
	}
}

// With FemaleBears both parents pay MateEnergy and the mother the rest of
// what the child is born with; after the birth she can neither mate nor be
// mated with for RecoverTicks, and the father can mate again at once.
func TestMotherBears(t *testing.T) {
	w := pairWorld(t, Body{ID: 0, X: 2.5, Y: 2.5, Energy: 80}, Body{ID: 1, X: 3.5, Y: 2.5, Energy: 60})
	w.cfg.FemaleBears = true
	mother, father := &w.bodies[0], &w.bodies[1] // pairWorld: Female, then Male
	if mother.Sex != Female || father.Sex != Male {
		t.Fatalf("sexes %v %v", mother.Sex, father.Sex)
	}
	// A mother who could pay a father's share but not hers is offered no
	// mate; a father who can pay his is.
	mother.Energy = w.cfg.MateEnergy + 1
	if hasMate(w.mateOptions(nil, mother), 1) {
		t.Fatal("a mother who cannot pay the birth was offered a mate")
	}
	father.Energy = w.cfg.MateEnergy + 1
	if !hasMate(w.mateOptions(nil, father), 0) {
		t.Fatal("a father who can pay the mating was offered no mate")
	}
	mother.Energy, father.Energy = 80, 60
	mother.Intent = Action{Kind: ActMate, Mate: 1}
	w.act(1, father, Action{Kind: ActMate, Mate: 0})
	if len(w.born) != 1 {
		t.Fatalf("%d children born, want 1", len(w.born))
	}
	c := w.born[0]
	if mother.Energy != 80-(w.cfg.BirthEnergy-w.cfg.MateEnergy) || father.Energy != 60-w.cfg.MateEnergy || c.Energy != w.cfg.BirthEnergy {
		t.Fatalf("energies after the birth: mother %v father %v child %v", mother.Energy, father.Energy, c.Energy)
	}
	if mother.Rested != w.tick+int64(w.cfg.RecoverTicks) || father.Rested != 0 {
		t.Fatalf("rested: mother %d father %d", mother.Rested, father.Rested)
	}
	// Resting, she neither offers nor is offered; he still can with others.
	if hasMate(w.mateOptions(nil, mother), 1) || hasMate(w.mateOptions(nil, father), 0) {
		t.Fatal("a resting mother mates")
	}
	w.tick += int64(w.cfg.RecoverTicks)
	mother.Energy = 80 // fed again
	if !hasMate(w.mateOptions(nil, mother), 1) || !hasMate(w.mateOptions(nil, father), 0) {
		t.Fatal("rested, the mother still cannot mate")
	}
}

// A resting mother burns RestBurn times what she would; a mother rested and
// a father burn as before.
func TestRestingMotherBurnsLess(t *testing.T) {
	w := pairWorld(t, Body{ID: 0, X: 2.5, Y: 2.5, Energy: 80}, Body{ID: 1, X: 3.5, Y: 2.5, Energy: 60})
	w.cfg.FemaleBears, w.cfg.RestBurn = true, 0.5
	mother, father := &w.bodies[0], &w.bodies[1]
	full := w.burnOf(mother)
	mother.Rested = w.tick + 10
	if got := w.burnOf(mother); got != full*0.5 {
		t.Fatalf("resting mother burns %v, want %v", got, full*0.5)
	}
	if got := w.burnOf(father); got != w.cfg.EnergyBurn {
		t.Fatalf("father burns %v", got)
	}
	w.tick += 10
	if got := w.burnOf(mother); got != full {
		t.Fatalf("rested mother burns %v, want %v", got, full)
	}
}

// With Requests a body that can mate and sees no mate broadcasts a request,
// spending no turn; a body of the other sex within RequestRange sees it as
// a mate out of sight, and a step to it is a step towards it; in sight, its
// offer makes a child without being named back.
func TestRequest(t *testing.T) {
	w := pairWorld(t, Body{ID: 0, X: 2.5, Y: 2.5, Energy: 80}, Body{ID: 1, X: 5.5, Y: 2.5, Energy: 60})
	w.cfg.Requests, w.cfg.RequestRange, w.cfg.RequestTicks = true, 5, 100
	she, he := &w.bodies[0], &w.bodies[1]
	w.request(she)
	if she.Requested != w.tick+100 {
		t.Fatalf("no request sent: %d", she.Requested)
	}
	// He sees her as a mate out of sight, and she him not (he sent none).
	if !hasMate(w.mateOptions(nil, he), 0) {
		t.Fatal("the requester is not among his mates")
	}
	d, far := w.farMate(he, 0)
	if !far || d != 4 {
		t.Fatalf("a step towards her: dir %d far %v, want west", d, far)
	}
	x := he.X
	w.act(1, he, Action{Kind: ActMate, Mate: 0})
	if he.X >= x || len(w.born) != 0 {
		t.Fatalf("he did not step towards her: x %v -> %v, born %d", x, he.X, len(w.born))
	}
	// In sight, his offer makes a child though she names no one.
	he.X = 3.5
	w.buildGrid()
	she.Intent = Action{Kind: ActWait}
	w.act(1, he, Action{Kind: ActMate, Mate: 0})
	if len(w.born) != 1 || she.Requested != 0 {
		t.Fatalf("born %d, her request %d", len(w.born), she.Requested)
	}
	// A body that cannot pay sends none; nor one with a mate in sight.
	w2 := pairWorld(t, Body{ID: 0, X: 2.5, Y: 2.5, Energy: 1}, Body{ID: 1, X: 3.5, Y: 2.5, Energy: 60})
	w2.cfg.Requests, w2.cfg.RequestTicks = true, 100
	w2.request(&w2.bodies[0])
	w2.request(&w2.bodies[1])
	if w2.bodies[0].Requested != 0 || w2.bodies[1].Requested != 0 {
		t.Fatalf("requests %d %d, want none", w2.bodies[0].Requested, w2.bodies[1].Requested)
	}
}

// An old body cannot mate (stage 3-8): it has no mate to offer, sends no
// request, and is neither a mate in sight nor a requester to anyone.
func TestOldBarren(t *testing.T) {
	w := pairWorld(t, Body{ID: 0, X: 2.5, Y: 2.5, Energy: 80, Born: -100}, Body{ID: 1, X: 3.5, Y: 2.5, Energy: 80})
	w.cfg.OldAge, w.cfg.Lifespan = 100, 200
	w.cfg.Requests, w.cfg.RequestRange, w.cfg.RequestTicks = true, 5, 100
	she, he := &w.bodies[0], &w.bodies[1]
	if !w.cfg.OldBarren {
		t.Fatal("OldBarren is off by default")
	}
	if len(w.mateOptions(nil, she)) != 0 || hasMate(w.mateOptions(nil, he), 0) {
		t.Fatal("the old body is offered a mate or is one")
	}
	w.request(she)
	if she.Requested != 0 {
		t.Fatalf("the old body sent a request: %d", she.Requested)
	}
	// Out of his sight, an open request of hers reaches no one.
	she.X, she.Requested = 7.5, w.tick+100
	w.buildGrid()
	w.listAsking()
	if hasMate(w.mateOptions(nil, he), 0) {
		t.Fatal("the old body's request reached him")
	}
	// Off, the old mate on (stage M-3).
	w.cfg.OldBarren = false
	if !hasMate(w.mateOptions(nil, he), 0) {
		t.Fatal("with OldBarren off, the old body is no mate")
	}
}

// With RestAhead a resting mother values her options at the burn she goes
// back to, with what the rest of her rest saves as a reserve: well above
// half, she can still starve in the window, so eating the food underfoot
// is strictly better than waiting (a wait is read as eating it a tick
// later). Without it she reads the window at her resting burn, cannot
// starve in it, and eating ties with waiting.
func TestRestingMotherReadsHerRestEnding(t *testing.T) {
	for _, ahead := range []bool{true, false} {
		w := pairWorld(t, Body{ID: 0, X: 2.5, Y: 2.5, Energy: 50}, Body{ID: 1, X: 10.5, Y: 7.5, Energy: 60})
		w.cfg.FemaleBears, w.cfg.RestBurn, w.cfg.RestAhead = true, 0.5, ahead
		mother := &w.bodies[0]
		mother.Rested = w.tick + 300
		tile := w.tileOf(mother.X, mother.Y)
		if w.foodOn(tile) < 0 {
			w.food.foods = append(w.food.foods, Food{X: tile % w.m.Width, Y: tile / w.m.Width})
			w.food.foodAt[tile] = int32(len(w.food.foods))
			w.food.onGround[w.m.Region[tile]]++
		}
		if got, want := w.restReserve(mother), map[bool]int{true: 150, false: 0}[ahead]; got != want {
			t.Fatalf("ahead %v: reserve %d, want %d", ahead, got, want)
		}
		w.decide(mother)
		v := w.valuation
		eat, wait := -1, -1
		for j, o := range v.Options {
			switch o.Kind {
			case ActEat:
				eat = j
			case ActWait:
				wait = j
			}
		}
		if eat < 0 || wait < 0 {
			t.Fatalf("ahead %v: options %+v", ahead, v.Options)
		}
		re, rw := v.Risk[0][eat], v.Risk[0][wait]
		if ahead && !(re < rw) {
			t.Errorf("knowing her rest ends, eating risk %v is not below waiting %v", re, rw)
		}
		if !ahead && (re != 0 || rw != 0) {
			t.Errorf("at her resting burn, eating %v and waiting %v, want both 0", re, rw)
		}
	}
}
