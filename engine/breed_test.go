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
	w := newTestWorld(t, 3)
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
