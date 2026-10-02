package engine

import "math"

// Breeding (stage 1-3).
//
// With Breed set, an adult body may mate with an adult it sees: the action
// names the partner (ActMate), and it is offered only when paying the
// body's share leaves it energy - dying of the payment is not something a
// body can do. The partner's energy is not seen. A birth needs both: when a
// body carries out a mate with a partner whose intent is a mate with it,
// still in sight, and both can pay, each pays its share and a child is born
// where the body stands, with BirthEnergy (what the two paid), an adult
// MatureAge ticks later. The shares are half each, or with FemaleBears
// MateEnergy for the father and the rest for the mother, who then rests
// RecoverTicks before she can mate again. A mate the partner has not chosen does nothing;
// the tick is spent as a wait.
//
// The child currency is valued in the same comparison as survival
// (predict.go): an option scores its risk minus ChildWorth times its chance
// of a child, 1 for a mate and 0 for anything else.
//
// Who stands where is kept per tile (grid) so that a body finds the adults
// in sight, and a step finds whether its tile is taken (Collide), without
// looking at every body; it is rebuilt at the end of every
// tick, when the dead leave and the born join, and kept up to date as bodies
// move. It can be rebuilt from the bodies, so it is neither saved nor
// fingerprinted.

// fertile reports whether body b may mate now: of age, and with OldBarren
// not yet OldAge old (stage 3-8) - a baby and an old body alike cannot.
func (w *World) fertile(b *Body) bool {
	return w.tick >= b.Mature && !(w.cfg.OldBarren && w.cfg.OldAge > 0 && w.tick-b.Born >= int64(w.cfg.OldAge))
}

// Sex is a body's sex (with Sexes). It changes nothing a body can do but
// whom it can mate with.
type Sex int8

const (
	NoSex Sex = iota
	Female
	Male
)

// drawSex draws a sex, one or the other alike, with Sexes; NoSex without,
// drawing nothing.
func (w *World) drawSex() Sex {
	if !w.cfg.Sexes {
		return NoSex
	}
	return Female + Sex(w.rng.Intn(2))
}

// bears reports whether the mother bears the birth (FemaleBears, which
// needs Sexes).
func (w *World) bears() bool { return w.cfg.FemaleBears && w.cfg.Sexes }

// birthShare is the energy body b pays for a birth: half of BirthEnergy,
// or with FemaleBears MateEnergy, and for the mother the rest too.
func (w *World) birthShare(b *Body) float64 {
	if !w.bears() {
		return w.cfg.BirthEnergy / 2
	}
	if b.Sex == Female {
		return w.cfg.BirthEnergy - w.cfg.MateEnergy
	}
	return w.cfg.MateEnergy
}

// canPay reports whether body b can pay its share of a birth and live.
func (w *World) canPay(b *Body) bool { return b.Energy-w.birthShare(b) > 0 }

// resting reports whether body b is a mother that cannot mate yet.
func (w *World) resting(b *Body) bool { return w.bears() && w.tick < b.Rested }

// buildGrid puts every body on its tile.
func (w *World) buildGrid() {
	if !w.gridded() {
		return
	}
	if len(w.grid) != len(w.m.Terrain) {
		w.grid = make([][]int32, len(w.m.Terrain))
	}
	for t := range w.grid {
		w.grid[t] = w.grid[t][:0]
	}
	for i := range w.bodies {
		if t := w.tileOf(w.bodies[i].X, w.bodies[i].Y); t >= 0 {
			w.grid[t] = append(w.grid[t], int32(i))
		}
	}
}

// gridded reports whether a rule reads who stands where.
func (w *World) gridded() bool { return w.cfg.Breed || w.cfg.Collide }

// occupied reports whether a body other than b stands on tile t.
func (w *World) occupied(t int, b *Body) bool {
	for _, j := range w.grid[t] {
		if w.bodies[j].ID != b.ID {
			return true
		}
	}
	return false
}

// blocks reports whether a step of body b onto (x, y) is into a tile
// another body stands on: with Collide, a step within its own tile never
// is, and a step into another occupied tile always is.
func (w *World) blocks(b *Body, x, y float64) bool {
	if !w.cfg.Collide || w.grid == nil {
		return false
	}
	to := w.tileOf(x, y)
	return to >= 0 && to != w.tileOf(b.X, b.Y) && w.occupied(to, b)
}

// moved keeps the grid up to date after body i went from tile from to tile
// to.
func (w *World) moved(i, from, to int) {
	if !w.gridded() || i < 0 || from == to {
		return
	}
	g := w.grid[from]
	for k, j := range g {
		if int(j) == i {
			g[k] = g[len(g)-1]
			w.grid[from] = g[:len(g)-1]
			break
		}
	}
	if to >= 0 {
		w.grid[to] = append(w.grid[to], int32(i))
	}
}

// mates calls f with every fertile body other than b standing on a tile
// within Sight of b's tile, in the order of the grid.
func (w *World) mates(b *Body, f func(*Body)) {
	if !w.cfg.Breed || w.cfg.Sight < 0 || w.grid == nil {
		return
	}
	bx, by := int(math.Floor(b.X)), int(math.Floor(b.Y))
	for y := by - w.cfg.Sight; y <= by+w.cfg.Sight; y++ {
		for x := bx - w.cfg.Sight; x <= bx+w.cfg.Sight; x++ {
			if !w.m.InBounds(x, y) {
				continue
			}
			for _, j := range w.grid[w.m.index(x, y)] {
				o := &w.bodies[j]
				if o.ID != b.ID && w.fertile(o) && (!w.cfg.Sexes || o.Sex != b.Sex) && !w.resting(o) {
					f(o)
				}
			}
		}
	}
}

// sawMates hashes the fertile bodies body b sees. The hash does not depend on the
// order they are found in.
func (w *World) sawMates(b *Body) uint64 {
	var h uint64
	w.mates(b, func(o *Body) { h += mix(uint64(o.ID)) })
	if w.fertile(b) && w.canPay(b) && !w.resting(b) {
		// Only a body that can mate is moved by a request coming in
		// range: to any other it is no option.
		w.requesters(b, func(o *Body) { h += mix(uint64(o.ID)) })
	}
	return h
}

// mix scrambles x (the finalizer of SplitMix64), so that a sum of mixed IDs
// tells sets of IDs apart.
func mix(x uint64) uint64 {
	x += 0x9e3779b97f4a7c15
	x = (x ^ (x >> 30)) * 0xbf58476d1ce4e5b9
	x = (x ^ (x >> 27)) * 0x94d049bb133111eb
	return x ^ (x >> 31)
}

// mateOptions appends a mate with every fertile body b sees, if b is
// fertile and can pay.
func (w *World) mateOptions(dst []Action, b *Body) []Action {
	if !w.cfg.Breed || !w.fertile(b) || !w.canPay(b) || w.resting(b) {
		return dst
	}
	w.mates(b, func(o *Body) { dst = append(dst, Action{Kind: ActMate, Mate: o.ID}) })
	clear(w.far)
	w.requesters(b, func(o *Body) {
		dst = append(dst, Action{Kind: ActMate, Mate: o.ID})
		if w.far == nil {
			w.far = map[int64]int{}
		}
		gap := math.Max(math.Abs(o.X-b.X), math.Abs(o.Y-b.Y)) - float64(w.cfg.Sight)
		w.far[o.ID] = int(math.Ceil(math.Max(gap, 0) / w.speedOf(b)))
	})
	return dst
}

// request has body b broadcast a request to mate if it can mate -
// fertile, not resting, able to pay its share - sees no mate and has none
// open (stage M-3). It spends no turn: sent as a choice among moves, read
// as a wait that may bring a child, it outvalued eating.
func (w *World) request(b *Body) {
	if !w.cfg.Requests || !w.cfg.Breed || b.Requested > w.tick || !w.fertile(b) || !w.canPay(b) || w.resting(b) {
		return
	}
	seen := false
	w.mates(b, func(*Body) { seen = true })
	if !seen {
		b.Requested = w.tick + int64(w.cfg.RequestTicks)
		w.stats.Requests++
		w.asking = append(w.asking, w.indexOf(b))
	}
}

// indexOf is the index of body b among the world's bodies.
func (w *World) indexOf(b *Body) int {
	for i := range w.bodies {
		if &w.bodies[i] == b {
			return i
		}
	}
	return -1
}

// listAsking lists the bodies with a request open, at the start of a tick
// (the indices change only when the dead leave and the born join).
func (w *World) listAsking() {
	w.asking = w.asking[:0]
	if !w.cfg.Requests {
		return
	}
	for i := range w.bodies {
		if w.bodies[i].Requested > w.tick {
			w.asking = append(w.asking, i)
		}
	}
}

// requesters calls f with every body of the other sex, fertile and not
// resting, out of b's sight but within RequestRange of it, whose request to
// mate is open (Requests).
func (w *World) requesters(b *Body, f func(*Body)) {
	if !w.cfg.Requests {
		return
	}
	bx, by := int(math.Floor(b.X)), int(math.Floor(b.Y))
	for _, i := range w.asking {
		o := &w.bodies[i]
		if o.Requested <= w.tick || o.ID == b.ID || o.Sex == b.Sex || !w.fertile(o) || w.resting(o) {
			continue
		}
		d := max(abs(int(math.Floor(o.X))-bx), abs(int(math.Floor(o.Y))-by))
		if d > w.cfg.Sight && d <= w.cfg.RequestRange {
			f(o)
		}
	}
}

// farMate reports whether the body id is a requester out of b's sight, and
// the direction of a step towards it.
func (w *World) farMate(b *Body, id int64) (int, bool) {
	d, far := 0, false
	w.requesters(b, func(o *Body) {
		if o.ID == id {
			d, far = dirTowards(o.X-b.X, o.Y-b.Y), true
		}
	})
	return d, far
}

// farTicks is how many ticks the body whose options were last listed walks
// to reach the sight of body id, a requester out of its sight; 0 if id is
// in sight. mateOptions works it out once per decision.
func (w *World) farTicks(id int64) int { return w.far[id] }

// dirTowards is the move direction nearest to (dx, dy): 0 east, then
// clockwise with y down.
func dirTowards(dx, dy float64) int {
	return (int(math.Round(math.Atan2(dy, dx)/(math.Pi/4))) + 8) % 8
}

// mate carries out body b's mate with the body whose ID is id: a birth if
// the partner's intent is a mate with b, it is still in sight, and both can
// pay. Afterwards neither intends to mate, so the partner's own turn this
// tick does not make a second child.
func (w *World) mate(b *Body, id int64) {
	var p *Body
	w.mates(b, func(o *Body) {
		if o.ID == id {
			p = o
		}
	})
	// The partner agrees by naming b too, or by an open request: a
	// request is itself consent (stage M-3).
	requested := p != nil && w.cfg.Requests && p.Requested > w.tick
	if p == nil || (p.Intent != (Action{Kind: ActMate, Mate: b.ID}) && !requested) || !w.canPay(b) || !w.canPay(p) {
		return
	}
	if requested {
		w.stats.RequestBirths++
	}
	b.Requested, p.Requested = 0, 0
	b.Energy -= w.birthShare(b)
	p.Energy -= w.birthShare(p)
	if w.bears() {
		for _, x := range []*Body{b, p} {
			if x.Sex == Female {
				x.Rested = w.tick + int64(w.cfg.RecoverTicks)
				x.Partner = b.ID + p.ID - x.ID // the other
			}
		}
	}
	if w.cfg.Learn {
		b.Memory.Kids++
		p.Memory.Kids++
		if w.cfg.AgeBand > 0 {
			w.observe(&b.Memory.Mate, 0, 1)
			w.observe(&p.Memory.Mate, 0, 1)
		}
	}
	b.Intent, p.Intent = Action{Kind: ActWait}, Action{Kind: ActWait}
	child := Body{
		ID:      w.nextID,
		X:       b.X,
		Y:       b.Y,
		Energy:  w.cfg.BirthEnergy,
		Born:    w.tick,
		Heading: -1,
		Goal:    -1,
		Decided: -1,
		Mature:  w.tick + int64(w.cfg.MatureAge),
		Parents: [2]int64{p.ID, b.ID},
		Sex:     w.drawSex(),
		Partner: -1,
	}
	w.allot(&child, p, b)
	w.born = append(w.born, child)
	w.nextID++
	w.stats.Births++
}
