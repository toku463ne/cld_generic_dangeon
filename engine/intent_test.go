package engine

import "testing"

// Between decisions a body takes its intent, step for step; no body goes
// Recheck ticks without deciding.
func TestFollowsIntentBetweenDecisions(t *testing.T) {
	w := newTestWorld(t, 5)
	decided := map[int64]bool{}
	w.SetTrace(func(b Body, _ Valuation, _ Action) { decided[b.ID] = true })
	followed := 0
	for i := 0; i < 1000; i++ {
		before := map[int64]Body{}
		for _, b := range w.Bodies() {
			before[b.ID] = b
		}
		clear(decided)
		w.Step()
		for _, b := range w.Bodies() {
			if w.Tick()-b.Decided >= int64(w.cfg.Recheck) {
				t.Fatalf("tick %d body %d: last decided at %d", w.Tick(), b.ID, b.Decided)
			}
			if decided[b.ID] {
				if b.Decided != w.Tick() {
					t.Fatalf("tick %d body %d: decided but Decided is %d", w.Tick(), b.ID, b.Decided)
				}
				continue
			}
			p := before[b.ID]
			if b.Intent != p.Intent || b.Goal != p.Goal || b.Decided != p.Decided {
				t.Fatalf("tick %d body %d: intent changed without a decision", w.Tick(), b.ID)
			}
			x, y := p.X, p.Y
			if p.Intent.Kind == ActMove {
				d := moveDirs[p.Intent.Dir]
				x, y = x+d[0]*w.cfg.Speed, y+d[1]*w.cfg.Speed
			}
			if b.X != x || b.Y != y {
				t.Fatalf("tick %d body %d: at (%v,%v), intent %v from (%v,%v) leads to (%v,%v)", w.Tick(), b.ID, b.X, b.Y, p.Intent, p.X, p.Y, x, y)
			}
			followed++
		}
	}
	if followed == 0 {
		t.Fatal("no body ever followed its intent")
	}
}

// Every trigger but the one for deciding every tick fires in a world under
// way, and the decisions are fewer than the actions. (On the small, crowded
// test map food comes and goes in sight often; the share on the maps the
// experiments run is in the stage 1-2e report.)
func TestTriggersFire(t *testing.T) {
	w := newTestWorld(t, 2)
	run(w, 3000)
	st := w.Stats()
	for k := TriggerFirst; k < NumTriggers; k++ {
		if st.Decisions[k] == 0 {
			t.Errorf("trigger %s never fired", TriggerNames[k])
		}
	}
	if st.Decisions[TriggerEveryTick] != 0 {
		t.Errorf("%d decisions made every tick with Recheck %d", st.Decisions[TriggerEveryTick], w.cfg.Recheck)
	}
	var decisions, actions int64
	for _, n := range st.Decisions {
		decisions += n
	}
	for _, n := range st.Actions {
		actions += n
	}
	if decisions*2 > actions {
		t.Errorf("%d decisions for %d actions", decisions, actions)
	}
}

// Walking to food, a step that no longer shortens the way by a whole step
// is a trigger: the food is level on one axis and the step is diagonal, or
// the body is on it. A step along a shortest path is not.
func TestPathTrigger(t *testing.T) {
	cfg := testConfig(1)
	cfg.Bodies = 0
	w, err := NewWorld(cfg, testMap())
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		x, y   float64
		dir    int
		gx, gy int
		want   Trigger
	}{
		{2.5, 5.5, 1, 5, 7, noTrigger},   // SE toward food below and right
		{2.5, 5.5, 0, 5, 7, noTrigger},   // E along the longer axis is as short
		{2.5, 5.5, 1, 5, 5, TriggerPath}, // SE toward food level with it
		{2.5, 5.5, 2, 5, 7, TriggerPath}, // S along the shorter axis is longer
		{5.5, 7.5, 0, 5, 7, TriggerPath}, // on the food
		{2.5, 5.5, 0, -1, -1, noTrigger}, // keeping on the move: no path
	} {
		b := Body{X: c.x, Y: c.y, Decided: w.tick, Intent: Action{Kind: ActMove, Dir: c.dir}, Goal: -1}
		if c.gx >= 0 {
			b.Goal = w.m.index(c.gx, c.gy)
		}
		b.Under, b.Saw = w.perceive(&b)
		if got := w.trigger(&b, b.Under, b.Saw); got != c.want {
			t.Errorf("%+v: trigger %d, want %d", c, got, c.want)
		}
	}
}
