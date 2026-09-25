package engine

// Choosing from outside (stage 1-4).
//
// A Chooser takes the decisions of the bodies it names: it is how a person
// plays a body. The engine does not know who is behind it. A body a
// chooser takes decides every tick (trigger outside, unless another trigger
// is true), is valued as any body is, and does what the chooser returns if
// that is one of its options - the same options as any body's - and what
// the valuation takes otherwise. With no chooser set, nothing of this runs,
// and the world is the world without it, fingerprint for fingerprint.

// Chooser takes the decisions of some bodies.
type Chooser interface {
	// Takes reports whether the chooser decides for the body with this ID.
	Takes(id int64) bool
	// Choose returns the action for body b, given its valuation; false
	// leaves the decision to the valuation. The valuation's slices are
	// reused by the next decision.
	Choose(b Body, v Valuation) (Action, bool)
}

// SetChooser sets the chooser; nil takes it away.
func (w *World) SetChooser(c Chooser) { w.chooser = c }

// takes reports whether a chooser decides for body b.
func (w *World) takes(b *Body) bool { return w.chooser != nil && w.chooser.Takes(b.ID) }

// chosen returns the chooser's action for body b if it is one of the
// options just valued, and a otherwise.
func (w *World) chosen(b *Body, a Action) Action {
	c, ok := w.chooser.Choose(*b, w.valuation)
	if !ok {
		return a
	}
	for _, o := range w.valuation.Options {
		if o == c {
			return c
		}
	}
	return a
}
