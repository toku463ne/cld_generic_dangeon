package engine

import "sort"

// Passing evidence between bodies (stage 2-1).
//
// With Tell, when two bodies first come within each other's sight, each
// passes the other what it knows of the world: for each region and for the
// path, its own evidence and what it has heard, each piece under the body
// that first observed it. The receiver keeps, for each observer, the
// larger of what it had and what it is given, as they weigh now - a later
// copy of the same observer's evidence holds more - so nothing is counted
// twice however many ways it arrives, and nothing is passed back to its own
// observer.
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

// pass gives body to what body from knows of the world, both brought to
// how their evidence weighs now.
func (w *World) pass(from, to *Body) {
	for r := range from.Memory.Regions {
		for len(to.Memory.Regions) <= r {
			to.Memory.Regions = append(to.Memory.Regions, Tally{T: w.tick})
		}
		w.age(&from.Memory.Regions[r])
		w.age(&to.Memory.Regions[r])
		w.passTally(&to.Memory.Regions[r], from.Memory.Regions[r], from.ID, to.ID)
	}
	w.age(&from.Memory.Path)
	w.age(&to.Memory.Path)
	w.passTally(&to.Memory.Path, from.Memory.Path, from.ID, to.ID)
}

// passTally merges tally src, held by body srcID, into dst, held by dstID:
// for each observer the larger entry, as they weigh now, src's own evidence
// under srcID, none under dstID; then the HeardLimit largest are kept.
func (w *World) passTally(dst *Tally, src Tally, srcID, dstID int64) {
	dst.settle()
	ss := src.scale()
	// src's entries brought to scale, with its own evidence under srcID.
	in := make([]Heard, 0, len(src.Heard)+1)
	own := Heard{ID: srcID, N: src.N - src.HN, K: src.K - src.HK}
	placed := own.N <= 0
	for _, h := range src.Heard {
		if !placed && own.ID < h.ID {
			in, placed = append(in, own), true
		}
		in = append(in, Heard{ID: h.ID, N: ss * h.N, K: ss * h.K})
	}
	if !placed {
		in = append(in, own)
	}
	// Merge the two lists, both in the order of their observers.
	out := make([]Heard, 0, len(dst.Heard)+len(in))
	i, j := 0, 0
	for i < len(dst.Heard) || j < len(in) {
		var h Heard
		switch {
		case j == len(in) || i < len(dst.Heard) && dst.Heard[i].ID < in[j].ID:
			h, i = dst.Heard[i], i+1
		case i == len(dst.Heard) || in[j].ID < dst.Heard[i].ID:
			h, j = in[j], j+1
		default: // the same observer: the larger
			h = dst.Heard[i]
			if in[j].N > h.N {
				h = in[j]
			}
			i, j = i+1, j+1
		}
		if h.ID != dstID && h.N > 0 {
			out = append(out, h)
		}
	}
	if len(out) > w.cfg.HeardLimit {
		// Keep the HeardLimit largest; among equal ones, the lower IDs.
		ns := make([]float64, len(out))
		for k, h := range out {
			ns[k] = h.N
		}
		sort.Float64s(ns)
		cut := ns[len(ns)-w.cfg.HeardLimit]
		above := 0
		for _, n := range ns {
			if n > cut {
				above++
			}
		}
		atCut := w.cfg.HeardLimit - above
		kept := out[:0]
		for _, h := range out {
			switch {
			case h.N > cut:
				kept = append(kept, h)
			case h.N == cut && atCut > 0:
				kept = append(kept, h)
				atCut--
			}
		}
		out = kept
	}
	ownN, ownK := dst.N-dst.HN, dst.K-dst.HK
	dst.Heard = out
	dst.resum(ownN, ownK)
}
