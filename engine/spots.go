package engine

import (
	"math"
	"sort"
)

// Remembered food (stage 5-2).
//
// A unit of food a body saw and left stays on the ground until another
// eats it: a body remembers the tiles it saw food on (Memory.Spots) and
// forgets one when it sees the tile empty. When it decides it reads the
// SpotRead nearest it no longer sees as units it may walk to, each still
// there with the chance its own row (Memory.Spot) gives: what its walks to
// remembered units found. The row learns only from the body's own walks to
// one, not from tiles it happens to see again (VISION: what was merely
// there is not what it acted on). A tile tells nothing more than this -
// food comes back to tiles at random (docs/experiments/20261002-placeroom-
// count.md) - so this is the only place a body remembers.

// noteSpots updates body b's remembered food from what it sees now, before
// it decides: a remembered tile back in sight it walked to teaches the
// spot row; every tile in sight is remembered with food or forgotten.
func (w *World) noteSpots(b *Body) {
	if !w.cfg.Learn || w.cfg.SpotRead <= 0 || w.cfg.Sight < 0 {
		return
	}
	mem := &b.Memory
	if mem.Spots == nil {
		mem.Spots = map[int]int64{}
	}
	bx, by := int(math.Floor(b.X)), int(math.Floor(b.Y))
	s := w.cfg.Sight
	for y := by - s; y <= by+s; y++ {
		for x := bx - s; x <= bx+s; x++ {
			if !w.m.InBounds(x, y) {
				continue
			}
			t := w.m.index(x, y)
			food := w.foodOn(t) >= 0
			if _, ok := mem.Spots[t]; ok && b.Goal == t {
				k := 0.0
				if food {
					k = 1
					w.stats.SpotFound++
				} else {
					w.stats.SpotMissed++
				}
				w.observe(&mem.Spot, 1, k)
			}
			if food {
				mem.Spots[t] = w.tick
			} else {
				delete(mem.Spots, t)
			}
		}
	}
	// Spots past the last step of age are let go: they read the same as
	// any older, and a body would otherwise keep every tile it ever saw.
	if w.cfg.AgeBand > 0 {
		old := int64(w.cfg.AgeBand) * int64(len(w.cfg.AgeTrust))
		for t, when := range mem.Spots {
			if w.tick-when >= old {
				delete(mem.Spots, t)
			}
		}
	}
}

// recalled appends to dst the SpotRead remembered units nearest body b,
// out of sight (those in sight are in Seen or gone), nearest first, ties by
// tile.
func (w *World) recalled(dst []Food, b *Body) []Food {
	if !w.cfg.Learn || w.cfg.SpotRead <= 0 || len(b.Memory.Spots) == 0 {
		return dst
	}
	bx, by := int(math.Floor(b.X)), int(math.Floor(b.Y))
	s := w.cfg.Sight
	type cand struct {
		d    float64
		tile int
	}
	cs := make([]cand, 0, len(b.Memory.Spots))
	for t := range b.Memory.Spots {
		x, y := t%w.m.Width, t/w.m.Width
		if abs(x-bx) <= s && abs(y-by) <= s {
			continue
		}
		cs = append(cs, cand{octile(math.Abs(float64(x)+0.5-b.X), math.Abs(float64(y)+0.5-b.Y)), t})
	}
	sort.Slice(cs, func(i, j int) bool {
		if cs[i].d != cs[j].d {
			return cs[i].d < cs[j].d
		}
		return cs[i].tile < cs[j].tile
	})
	for i := 0; i < len(cs) && i < w.cfg.SpotRead; i++ {
		dst = append(dst, Food{X: cs[i].tile % w.m.Width, Y: cs[i].tile / w.m.Width})
	}
	return dst
}

// spotRate is body b's estimate that a remembered unit is still there.
func (w *World) spotRate(b *Body) float64 {
	return w.Fresh(b.Memory.Spot).estimate(w.cfg.PriorSpot, w.cfg.SpotWeight)
}
