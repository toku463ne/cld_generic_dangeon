package engine

import "fmt"

// Nests (stage 13-3, MAP.md "巣", NODE.md "トレーニングと本番"): the
// points where bodies come into a world. A nest is a map layer - a
// species and a tile - and follows one rule: every NestGap ticks it counts
// the living bodies of its species within NestRadius tiles, and where
// fewer than NestLow stand there it sends one out onto its tile. It
// refills; it is not a stream of immigrants (the user's choice,
// 2026-10-12): a species that holds its ground near a nest gets nothing
// from it.
//
// A world on a map with nests begins empty and its first bodies come out
// of the nests too (VISION.md: the stage begins mixed by the nests'
// doing); Config.Bodies is read only on a map without them. A body sent
// out is a first body: grown, full of energy, with no parents and no
// memory. Where the world was given a save (NewWorldNested), it is one of
// the save's stocks of its species, drawn uniformly, with that stock's
// skills; else its genome and skills are drawn as a first body's are.
//
// A nest sends at most NestQuota bodies in the world's life (the user's
// choice, 2026-10-14): it props a species up only so far, and a species
// can die out once its nests are empty.

// Nest is one nest: the species (by name) it sends out, and its tile.
type Nest struct {
	Species string
	X, Y    int
}

// NewWorldNested builds a world like NewWorld on a map with nests, whose
// nests send out bodies of the given stocks (stock.go): the stocks of a
// species are what its nests draw from; a species with none draws its
// bodies as on NewWorld.
func NewWorldNested(cfg Config, m Map, stocks []Stock) (*World, error) {
	if !cfg.Nests || len(m.Nests) == 0 {
		return nil, fmt.Errorf("a save for the nests needs a map with nests and Nests on")
	}
	return newWorld(cfg, m, append([]Stock{}, stocks...), true, true)
}

// nesting reports whether the world's bodies come from its nests.
func (w *World) nesting() bool { return w.cfg.Nests && len(w.m.Nests) > 0 }

// validNests reports whether the map's nests stand in bounds and name a
// species.
func (m Map) validNests() error {
	for i, n := range m.Nests {
		if !m.InBounds(n.X, n.Y) {
			return fmt.Errorf("nest %d at (%d,%d) is off the map", i, n.X, n.Y)
		}
		if n.Species == "" {
			return fmt.Errorf("nest %d at (%d,%d) names no species", i, n.X, n.Y)
		}
	}
	return nil
}

// initNests finds each nest's species and checks that it can stand on
// its tile, and sorts the stocks given by species.
func (w *World) initNests(stocks []Stock) error {
	if !w.nesting() {
		return nil
	}
	if w.cfg.NestGap < 1 || w.cfg.NestRadius < 0 || w.cfg.NestLow < 1 || w.cfg.NestQuota < 0 {
		return fmt.Errorf("Nests needs NestGap and NestLow at least one and NestRadius and NestQuota at least zero, got %d %d %d %d", w.cfg.NestGap, w.cfg.NestLow, w.cfg.NestRadius, w.cfg.NestQuota)
	}
	w.nestSpecies = make([]int, len(w.m.Nests))
	for i, n := range w.m.Nests {
		s := -1
		for k := 0; k < w.SpeciesCount(); k++ {
			if w.SpeciesName(k) == n.Species {
				s = k
			}
		}
		if s < 0 {
			return fmt.Errorf("nest %d names species %q, which the world does not have", i, n.Species)
		}
		t := w.m.index(n.X, n.Y)
		if !w.specOf(s).enters(w.m.Terrain[t]) || w.walled(t) {
			return fmt.Errorf("nest %d at (%d,%d): a %s cannot stand there", i, n.X, n.Y, n.Species)
		}
		w.nestSpecies[i] = s
	}
	w.nestStocks = make([][]Stock, w.SpeciesCount())
	for _, st := range stocks {
		w.nestStocks[st.Genome.Species] = append(w.nestStocks[st.Genome.Species], st)
	}
	return nil
}

// refill has each nest, on the ticks it looks, send out a body if fewer
// than NestLow of its species stand within NestRadius of it and it has
// not sent its NestQuota.
func (w *World) refill() {
	if !w.nesting() || w.tick%int64(w.cfg.NestGap) != 0 {
		return
	}
	r := w.cfg.NestRadius
	for i, n := range w.m.Nests {
		if w.nestEmpty(i) {
			continue
		}
		s := w.nestSpecies[i]
		near := 0
		for k := range w.bodies {
			b := &w.bodies[k]
			if b.Species == s && abs(int(b.X)-n.X) <= r && abs(int(b.Y)-n.Y) <= r {
				if near++; near >= w.cfg.NestLow {
					break
				}
			}
		}
		if near < w.cfg.NestLow {
			w.send(i)
		}
	}
}

// nestEmpty reports whether nest i has sent its NestQuota.
func (w *World) nestEmpty(i int) bool {
	return w.cfg.NestQuota > 0 && i < len(w.stats.NestOut) && w.stats.NestOut[i] >= int64(w.cfg.NestQuota)
}

// send puts a body out of nest i onto its tile: one of the save's stocks
// of its species, or a drawn one.
func (w *World) send(i int) {
	n, s := w.m.Nests[i], w.nestSpecies[i]
	var st *Stock
	if pool := w.nestStocks[s]; len(pool) > 0 {
		st = &pool[w.rng.Intn(len(pool))]
	}
	b := w.firstBody(s, w.m.index(n.X, n.Y), st, true)
	for len(w.stats.NestSent) <= s {
		w.stats.NestSent = append(w.stats.NestSent, 0)
	}
	w.stats.NestSent[s]++
	for len(w.stats.NestOut) <= i {
		w.stats.NestOut = append(w.stats.NestOut, 0)
	}
	w.stats.NestOut[i]++
	w.stats.BudgetNest += b.Budget
	w.bodies = append(w.bodies, b)
}
