package engine

import "math"

// Helping (stage 12-5, VISION "ノード同士で協力することもあること"): a body
// that hears one of its kind struck (CallAttacked, stage 10-4) may go to
// it and fight the hunter with it. Within reach of the hunter it already
// may strike back with the band (stage 10-2e); helping is the walk there
// from farther off, within CallRange, while the call is fresh (HelpTicks).
// There is no reward for it (VISION "交換や協力に専用の報酬を付けず"): the
// walk is valued in the two currencies as any option is. It costs the
// ticks of walking and the blows the hunter lands on the helper until the
// band, the helper among it, has it down - read as death where the hunter
// would down the helper first. It gains, if the band downs the hunter,
// a hunter less for itself (the user's choice, 2026-10-12): what that
// hunter would take off it over the window from where it stands, as fear
// reads a hunter in sight (threatLoss). And if the caller lives through
// it (the band downs the hunter before the hunter downs the caller), what
// the caller is worth to the helper: what the helper gained by it so far
// (its liking, stage 12-3) as energy, and, for its own child, the child
// itself - the second currency, as a child made is. A body the hunter
// could not catch, with no friend and no child in the fight, has nothing
// to gain and does not come.

// HelpCall is the call a body last heard and may go to: from Caller,
// struck by Hunter, open until tick Until.
type HelpCall struct {
	Caller int64 `json:",omitempty"`
	Hunter int64 `json:",omitempty"`
	Until  int64 `json:",omitempty"`
}

// heardStruck has body o, which heard caller struck by hunter, keep the
// call to help.
func (w *World) heardStruck(o *Body, caller, hunter int64) {
	if !w.cfg.Help {
		return
	}
	o.Help = HelpCall{Caller: caller, Hunter: hunter, Until: w.tick + int64(w.cfg.HelpTicks)}
}

// helpTarget returns the hunter and caller of body b's open call to help,
// both alive, b awake and not yet within reach of the hunter.
func (w *World) helpTarget(b *Body) (h, c *Body) {
	if !w.cfg.Help || b.Help.Until <= w.tick || b.Asleep {
		return nil, nil
	}
	h, c = w.bodyNamed(b.Help.Hunter), w.bodyNamed(b.Help.Caller)
	if h == nil || c == nil || h.Slain || c.Slain || adjacent(b, h) {
		return nil, nil
	}
	return h, c
}

// helpOptions appends the walk to help, if body b has a call open.
func (w *World) helpOptions(dst []Action, b *Body) []Action {
	if h, _ := w.helpTarget(b); h != nil {
		dst = append(dst, Action{Kind: ActHelp, Mate: h.ID})
	}
	return dst
}

// helpRead is what body b reads of going to help: the ticks of walking,
// the energy the hunter's blows take off it, whether it is down first,
// what the caller's living is worth to it in energy, and whether the
// caller is its child - both nothing where the caller would not live.
func (w *World) helpRead(b *Body) (walk int, loss float64, down bool, gain float64, child bool) {
	h, c := w.helpTarget(b)
	if h == nil {
		return 0, 0, true, 0, false
	}
	gap := math.Max(math.Abs(h.X-b.X), math.Abs(h.Y-b.Y)) - 1
	walk = int(math.Ceil(math.Max(gap, 0) / w.speedOf(b)))
	n := float64(w.band(c, h) + 1) // the caller's band, and b
	ticks := max(int(math.Ceil(h.Energy/(n*w.damage(b, h)))), 1)
	loss = float64(ticks) * w.damage(h, b) / n / w.fierceness(b)
	down = loss >= b.Energy
	if down {
		return walk, loss, true, 0, false
	}
	// The hunter down: what it would take off b over the window, gone.
	gain = w.lossFrom(b, h, b.X, b.Y, w.cfg.Window)
	// The caller lives if it outlasts the hunter's share of blows on it,
	// once b has come.
	if float64(walk+ticks)*w.damage(h, c)/n >= c.Energy {
		return walk, loss, false, gain, false
	}
	return walk, loss, false, gain + math.Max(w.Liking(*b, c.ID), 0), c.Parents[0] == b.ID || c.Parents[1] == b.ID
}

// help has body b take a step towards the hunter of its call.
func (w *World) help(i int, b *Body) {
	h, _ := w.helpTarget(b)
	if h == nil {
		return
	}
	w.stats.HelpSteps++
	w.stepTowards(i, b, dirTowards(h.X-b.X, h.Y-b.Y))
}
