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
// Only what a body knows of the world passes: not the mate row, never a
// row about one individual (there are none yet), and not the path row but
// with PassPath - what tiles walked lately hold is a fact of how their
// holder moves. With StableRows how rich a region is now, which changes
// with how many eat there, is each body's own too: passed on, the evidence
// of the days of plenty had everyone expect too much at once when food ran
// short. So with both, nothing passes yet. A body keeps at most
// HeardLimit observers' evidence per row, the largest: without a limit,
// evidence passed on and on would have every body carry every observer
// that ever lived. Memory capacity is not yet in the budget; the limit
// stands in for it.

// tell passes evidence between body b and every body in its sight it has
// not met.
func (w *World) tell(b *Body) {
	sight := w.sightOf(b)
	if !w.cfg.Learn || !w.cfg.Tell || w.grid == nil || sight < 0 {
		return
	}
	bx, by := w.tileOf(b.X, b.Y)%w.m.Width, w.tileOf(b.X, b.Y)/w.m.Width
	for y := by - sight; y <= by+sight; y++ {
		for x := bx - sight; x <= bx+sight; x++ {
			if !w.m.InBounds(x, y) || !w.seesFrom(b, bx, by, x, y) {
				continue
			}
			for _, j := range w.grid[w.m.index(x, y)] {
				o := &w.bodies[j]
				if w.cfg.Kin {
					if !w.tellChild(b, o) {
						w.tellFriend(b, o)
					}
					continue
				}
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
				if w.onMeet != nil {
					w.onMeet(*b, *o)
				}
			}
		}
	}
}

// tellChild passes body b's evidence to body o if o is b's child and b
// holds some it has not passed to it; it reports whether o is b's child.
func (w *World) tellChild(b, o *Body) bool {
	if o.Parents[0] != b.ID && o.Parents[1] != b.ID {
		return false
	}
	w.tellTo(b, o)
	return true
}

// tellFriend passes body b's evidence to body o, not its child, if b
// likes o (TellLiked, stage 12-4, VISION "好感度を上げたい相手には、自分に
// とって点数が高い情報を優先的に伝えること"): what b tells a child it tells
// a body it has gained by, as it would a child - once for each change to
// its rows. Nothing yet limits what is told, so "first what scores highest"
// has nothing to choose between: every row that passes passes. The order
// matters once telling has a budget (memory capacity, NODE.md "記憶の粒度
// の階段").
func (w *World) tellFriend(b, o *Body) {
	if !w.cfg.TellLiked || o.ID == b.ID || !sameSpecies(b, o) || w.Liking(*b, o.ID) <= 0 {
		return
	}
	if w.tellTo(b, o) {
		w.stats.FriendsTold++
	}
}

// tellTo passes body b's evidence to body o if b holds some it has not
// passed to it, and reports whether it did.
func (w *World) tellTo(b, o *Body) bool {
	m := &b.Memory
	if m.Ver == 0 || m.Told[o.ID] == m.Ver {
		return false
	}
	if m.Told == nil {
		m.Told = map[int64]int64{}
	}
	m.Told[o.ID] = m.Ver
	w.pass(b, o)
	if w.onMeet != nil {
		w.onMeet(*b, *o)
	}
	return true
}

// pass gives body to what body from knows of the world, both brought to
// how their evidence weighs now.
func (w *World) pass(from, to *Body) {
	for r := range from.Memory.Regions {
		if w.cfg.StableRows {
			break // how rich a region is now changes: each body's own
		}
		for len(to.Memory.Regions) <= r {
			to.Memory.Regions = append(to.Memory.Regions, Tally{T: w.tick})
		}
		w.age(&from.Memory.Regions[r])
		w.age(&to.Memory.Regions[r])
		if from.Memory.Regions[r].N > 0 {
			w.regionRow(RegionID(r)).Passed++
		}
		w.passTally(&to.Memory.Regions[r], from.Memory.Regions[r], from.ID, to.ID)
	}
	if w.cfg.Wary {
		// Where the hunters were seen, and how often a sighting became
		// a blow (stages 10-6, 10-6b): hearsay of the threats.
		for r := range from.Memory.Threats {
			row := w.threatRow(&to.Memory, RegionID(r))
			w.ageBy(&from.Memory.Threats[r])
			w.ageBy(row)
			w.passTally(row, from.Memory.Threats[r], from.ID, to.ID)
		}
		w.ageBy(&from.Memory.Blows)
		w.ageBy(&to.Memory.Blows)
		w.passTally(&to.Memory.Blows, from.Memory.Blows, from.ID, to.ID)
	}
	if w.cfg.PassShelters {
		// Where the places to rest are (stage 6-5a).
		for t := range from.Memory.Shelters {
			if to.Memory.Shelters[t] {
				continue
			}
			if to.Memory.Shelters == nil {
				to.Memory.Shelters = map[int]bool{}
			}
			to.Memory.Shelters[t] = true
			w.stats.SheltersPassed++
		}
		// And where the shops are (stage 11-2b).
		for _, i := range from.Memory.Shops {
			w.rememberShop(to, i)
		}
	}
	if w.cfg.PassPath {
		w.agePath(&from.Memory.Path)
		w.agePath(&to.Memory.Path)
		if from.Memory.Path.N > 0 {
			w.stats.PathRow.Passed++
		}
		w.passTally(&to.Memory.Path, from.Memory.Path, from.ID, to.ID)
	}
	to.Memory.Ver++
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

// SetMeet sets a function shown every meeting at which two bodies pass
// evidence (after they have). It draws nothing and changes nothing; nil
// turns it off.
func (w *World) SetMeet(f func(a, b Body)) { w.onMeet = f }
