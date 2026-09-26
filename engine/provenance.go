package engine

import (
	"fmt"
	"sort"
)

// Where evidence came from (stage 2-0).
//
// A body's evidence for a row is what it observed itself and, once there
// is a way for it to pass between bodies (stage 2-1), what it received:
// Tally.Heard holds the received part by the body that first observed it,
// which stays the same however many times it is passed on. This is an
// instrument: it draws nothing and no rule reads it, so it changes nothing
// in the world. It answers whether knowledge outlives the bodies that
// gathered it - evidence whose observer is dead is an orphan, and its age
// is how long ago its observer died.
//
// The world keeps the tick each body died (died), for the ages. It is not
// saved: after a load, evidence of an observer who died before the save
// counts as an orphan of unknown age.

// Count is evidence received from one observer: K of N came out one way.
type Count struct{ N, K float64 }

// RowProvenance is where the evidence the living hold for one row came
// from.
type RowProvenance struct {
	Name string
	// Held is the evidence the living hold for the row; Heard the part they
	// received; Orphan the part whose first observer is dead.
	Held, Heard, Orphan float64
	// Ages are the orphan evidence by how many ticks ago its observer died
	// (evidence of unknown age is left out), as (age, amount) pairs.
	Ages [][2]float64
	// Hearers is the share of the living that hold any received evidence
	// for the row.
	Hearers float64
}

// Provenance reads where the evidence the living hold came from, row by
// row: each region's food, and the food of the body's own path. (The mate
// row is left out: passed on or not, it cannot move a choice - stage 1-6.)
func (w *World) Provenance() []RowProvenance {
	alive := make(map[int64]bool, len(w.bodies))
	for i := range w.bodies {
		alive[w.bodies[i].ID] = true
	}
	read := func(name string, tally func(*Body) (Tally, bool)) RowProvenance {
		rp := RowProvenance{Name: name}
		ages := map[float64]float64{}
		hearers := 0.0
		for i := range w.bodies {
			t, ok := tally(&w.bodies[i])
			if !ok {
				continue
			}
			rp.Held += t.N
			if len(t.Heard) > 0 {
				hearers++
			}
			for id, c := range t.Heard {
				rp.Heard += c.N
				if alive[id] {
					continue
				}
				rp.Orphan += c.N
				if when, ok := w.died[id]; ok {
					ages[float64(w.tick-when)] += c.N
				}
			}
		}
		for a, n := range ages {
			rp.Ages = append(rp.Ages, [2]float64{a, n})
		}
		sort.Slice(rp.Ages, func(i, j int) bool { return rp.Ages[i][0] < rp.Ages[j][0] })
		if len(w.bodies) > 0 {
			rp.Hearers = hearers / float64(len(w.bodies))
		}
		return rp
	}
	var rows []RowProvenance
	for r := range w.m.RegionFood {
		rows = append(rows, read(fmt.Sprintf("region %d", r), func(b *Body) (Tally, bool) {
			if r < len(b.Memory.Regions) {
				return b.Memory.Regions[r], true
			}
			return Tally{}, false
		}))
	}
	rows = append(rows, read("path", func(b *Body) (Tally, bool) { return b.Memory.Path, true }))
	return rows
}
