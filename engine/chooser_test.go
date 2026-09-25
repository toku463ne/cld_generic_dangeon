package engine

import "testing"

// eastward takes one body and always walks it east.
type eastward struct{ id int64 }

func (e eastward) Takes(id int64) bool { return id == e.id }
func (e eastward) Choose(Body, Valuation) (Action, bool) {
	return Action{Kind: ActMove, Dir: 0}, true
}

// A chosen body decides every tick and does what it is told when it can,
// what the valuation takes when it cannot; the trace shows what it did.
func TestChooserTakesItsBody(t *testing.T) {
	w := newTestWorld(t, 2)
	id := w.bodies[0].ID
	w.SetChooser(eastward{id})
	traced := 0
	w.SetTrace(func(b Body, v Valuation, a Action) {
		if b.ID != id {
			return
		}
		traced++
		east := false
		for _, o := range v.Options {
			east = east || o == (Action{Kind: ActMove, Dir: 0})
		}
		if east && a != (Action{Kind: ActMove, Dir: 0}) {
			t.Fatalf("tick %d: could walk east, took %v", w.Tick(), a)
		}
		if !east && a == (Action{Kind: ActMove, Dir: 0}) {
			t.Fatalf("tick %d: walked east where it could not", w.Tick())
		}
	})
	ticks := 0
	for ; ticks < 200; ticks++ {
		alive := false
		for _, b := range w.Bodies() {
			alive = alive || b.ID == id
		}
		if !alive {
			break
		}
		w.Step()
	}
	if traced < ticks-1 {
		t.Fatalf("decided %d times in %d ticks, want every tick", traced, ticks)
	}
}

// A chooser that takes no body changes nothing.
func TestIdleChooserChangesNothing(t *testing.T) {
	a, b := newTestWorld(t, 4), newTestWorld(t, 4)
	b.SetChooser(eastward{-7})
	run(a, 600)
	run(b, 600)
	if a.Fingerprint() != b.Fingerprint() {
		t.Fatal("a chooser that takes no one changed the world")
	}
}
