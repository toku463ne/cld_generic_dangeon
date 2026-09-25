package view

import (
	"math"

	"github.com/toku463ne/cld_generic_dangeon/engine"
)

// Driver plays one body (engine.Chooser): what the person holds down picks
// its action every tick, from the same options as any body's. Holding
// nothing waits. The engine cannot tell the played body from the others.
type Driver struct {
	// ID is the played body, or noBody.
	ID int64
	// Dx and Dy are the direction held, each -1, 0 or 1 (y down); Eat and
	// Mate are held too, and win over a direction.
	Dx, Dy    int
	Eat, Mate bool
}

// Takes reports whether id is the played body.
func (d *Driver) Takes(id int64) bool { return d.ID != noBody && id == d.ID }

// Choose picks the action held: eat, a mate with the first adult offered,
// a move the way held, or a wait. An action the body cannot take now is
// passed to the engine all the same, which then leaves the tick to the
// valuation.
func (d *Driver) Choose(_ engine.Body, v engine.Valuation) (engine.Action, bool) {
	switch {
	case d.Eat:
		return engine.Action{Kind: engine.ActEat}, true
	case d.Mate:
		for _, o := range v.Options {
			if o.Kind == engine.ActMate {
				return o, true
			}
		}
		return engine.Action{Kind: engine.ActWait}, true
	case d.Dx != 0 || d.Dy != 0:
		return engine.Action{Kind: engine.ActMove, Dir: dirOf(d.Dx, d.Dy)}, true
	}
	return engine.Action{Kind: engine.ActWait}, true
}

// dirOf is the engine's direction of a move by (dx, dy): 0 east, then
// clockwise with y down.
func dirOf(dx, dy int) int {
	a := math.Atan2(float64(dy), float64(dx))
	return (int(math.Round(a/(math.Pi/4))) + 8) % 8
}

// Play starts playing the followed body, or stops if one is played.
func (v *View) Play() {
	if v.drive.ID != noBody || v.follow == noBody {
		v.drive.ID = noBody
		return
	}
	v.drive.ID = v.follow
}

// Driver returns the driver, for the client to set what is held.
func (v *View) Driver() *Driver { return &v.drive }
