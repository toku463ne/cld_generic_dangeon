package engine

import "sort"

// Passing evidence between bodies (stage 2-1).
//
// With Tell, when two bodies first come within each other's sight, each
// passes the other what it knows of the world: for each region and for the
// path, its own evidence and what it has heard, each piece under the body
// that first observed it. The receiver keeps, for each observer, the
// larger of what it had and what it is given - a later copy of the same
// observer's evidence holds more - so nothing is counted twice however
// many ways it arrives, and nothing is passed back to its own observer.
// Heard evidence counts toward an estimate as the body's own does.
//
// Only what a body knows of the world passes: not the mate row, and never
// a row about one individual (there are none yet). A body keeps at most
// HeardLimit observers' evidence per row, the largest: without a limit,
// evidence passed on and on would have every body carry every observer
// that ever lived. Memory capacity is not yet in the budget; the limit
// stands in for it.

// tell passes evidence between body b and every body in its sight it has
// not met.
func (w *World) tell(b *Body) {
	if !w.cfg.Learn || !w.cfg.Tell || w.grid == nil || w.cfg.Sight < 0 {
		return
	}
	bx, by := w.tileOf(b.X, b.Y)%w.m.Width, w.tileOf(b.X, b.Y)/w.m.Width
	for y := by - w.cfg.Sight; y <= by+w.cfg.Sight; y++ {
		for x := bx - w.cfg.Sight; x <= bx+w.cfg.Sight; x++ {
			if !w.m.InBounds(x, y) {
				continue
			}
			for _, j := range w.grid[w.m.index(x, y)] {
				o := &w.bodies[j]
				if o.ID == b.ID || b.Memory.Met[o.ID] {
					continue
				}
				if b.Memory.Met == nil {
					b.Memory.Met = map[int64]bool{}
				}
				if o.Memory.Met == nil {
					o.Memory.Met = map[int64]bool{}
				}
				b.Memory.Met[o.ID], o.Memory.Met[b.ID] = true, true
				w.pass(o, b)
				w.pass(b, o)
			}
		}
	}
}

// pass gives body to what body from knows of the world.
func (w *World) pass(from, to *Body) {
	for r := range from.Memory.Regions {
		for len(to.Memory.Regions) <= r {
			to.Memory.Regions = append(to.Memory.Regions, Tally{})
		}
		w.passTally(&to.Memory.Regions[r], from.Memory.Regions[r], from.ID, to.ID)
	}
	w.passTally(&to.Memory.Path, from.Memory.Path, from.ID, to.ID)
}

// passTally merges tally src, held by body srcID, into dst, held by dstID.
func (w *World) passTally(dst *Tally, src Tally, srcID, dstID int64) {
	take := func(id int64, c Count) {
		if id == dstID || c.N <= 0 {
			return
		}
		if dst.Heard == nil {
			dst.Heard = map[int64]Count{}
		}
		if c.N > dst.Heard[id].N {
			dst.Heard[id] = c
		}
	}
	take(srcID, Count{N: src.N - src.HN, K: src.K - src.HK})
	for id, c := range src.Heard {
		take(id, c)
	}
	if len(dst.Heard) > w.cfg.HeardLimit {
		type entry struct {
			id int64
			c  Count
		}
		es := make([]entry, 0, len(dst.Heard))
		for id, c := range dst.Heard {
			es = append(es, entry{id, c})
		}
		sort.Slice(es, func(i, j int) bool {
			if es[i].c.N != es[j].c.N {
				return es[i].c.N < es[j].c.N
			}
			return es[i].id < es[j].id
		})
		for _, e := range es[:len(es)-w.cfg.HeardLimit] {
			delete(dst.Heard, e.id)
		}
	}
	ownN, ownK := dst.N-dst.HN, dst.K-dst.HK
	dst.HN, dst.HK = 0, 0
	for _, c := range dst.Heard {
		dst.HN += c.N
		dst.HK += c.K
	}
	dst.N, dst.K = ownN+dst.HN, ownK+dst.HK
}
