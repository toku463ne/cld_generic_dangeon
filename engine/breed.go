package engine

import "math"

// Breeding (stage 1-3).
//
// With Breed set, an adult body may mate with an adult it sees: the action
// names the partner (ActMate), and it is offered only when paying the
// body's half of BirthEnergy leaves it energy - dying of the payment is not
// something a body can do. The partner's energy is not seen. A birth needs
// both: when a body carries out a mate with a partner whose intent is a
// mate with it, still in sight, and both can pay, each pays half and a
// child is born where the body stands, with BirthEnergy, an adult
// MatureAge ticks later. A mate the partner has not chosen does nothing;
// the tick is spent as a wait.
//
// The child currency is valued in the same comparison as survival
// (predict.go): an option scores its risk minus ChildWorth times its chance
// of a child, 1 for a mate and 0 for anything else.
//
// Who stands where is kept per tile (grid) so that a body finds the adults
// in sight without looking at every body; it is rebuilt at the end of every
// tick, when the dead leave and the born join, and kept up to date as bodies
// move. It can be rebuilt from the bodies, so it is neither saved nor
// fingerprinted.

// adult reports whether body b may mate now.
func (w *World) adult(b *Body) bool { return w.tick >= b.Mature }

// birthShare is the energy each parent pays for a birth.
func (w *World) birthShare() float64 { return w.cfg.BirthEnergy / 2 }

// canPay reports whether body b can pay its share of a birth and live.
func (w *World) canPay(b *Body) bool { return b.Energy-w.birthShare() > 0 }

// buildGrid puts every body on its tile.
func (w *World) buildGrid() {
	if !w.cfg.Breed {
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

// moved keeps the grid up to date after body i went from tile from to tile
// to.
func (w *World) moved(i, from, to int) {
	if !w.cfg.Breed || i < 0 || from == to {
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

// mates calls f with every adult other than body b standing on a tile
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
				if o.ID != b.ID && w.adult(o) {
					f(o)
				}
			}
		}
	}
}

// sawMates hashes the adults body b sees. The hash does not depend on the
// order they are found in.
func (w *World) sawMates(b *Body) uint64 {
	var h uint64
	w.mates(b, func(o *Body) { h += mix(uint64(o.ID)) })
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

// mateOptions appends a mate with every adult body b sees, if b is an adult
// and can pay.
func (w *World) mateOptions(dst []Action, b *Body) []Action {
	if !w.cfg.Breed || !w.adult(b) || !w.canPay(b) {
		return dst
	}
	w.mates(b, func(o *Body) { dst = append(dst, Action{Kind: ActMate, Mate: o.ID}) })
	return dst
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
	if p == nil || p.Intent != (Action{Kind: ActMate, Mate: b.ID}) || !w.canPay(b) || !w.canPay(p) {
		return
	}
	b.Energy -= w.birthShare()
	p.Energy -= w.birthShare()
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
	}
	w.allot(&child)
	w.born = append(w.born, child)
	w.nextID++
	w.stats.Births++
}
