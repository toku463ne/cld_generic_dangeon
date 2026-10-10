package engine

import "math"

// Hunting (stage 10-2a, VISION "戦闘": a beast eats humans).
//
// A species' row names the species it preys on (SpeciesSpec.Preys). A
// body of such a species may attack a prey it sees: it walks up to it and
// strikes once a tick, each blow taking HitEnergy times its attack over
// attack plus defense off the prey's energy (at even strength, half a
// blow's worth: four blows for a body at EnergyMax). A prey struck down
// dies where it stands, and MeatUnits units of the kill pool (a pool with
// Kill, the raw meat, of a kind the hunter's species eats) lie on its tile
// and the tiles round it, owned by the hunter: no other body may take
// them (own.go), and they decay as any food does. The hunter values the
// attack as a meal reached after the ticks of walking and striking, in
// the same currency as a unit in sight. The prey does not strike back:
// humans do not prey on beasts. Fleeing is stage 10-2b.

// preysOn reports whether body b's species preys on body o's.
func (w *World) preysOn(b, o *Body) bool {
	return w.prey != nil && w.prey[b.Species][o.Species]
}

// initPrey builds the table of who preys on whom from the species' rows.
func (w *World) initPrey() {
	n := w.SpeciesCount()
	w.prey = nil
	any := false
	table := make([][]bool, n)
	for s := range table {
		table[s] = make([]bool, n)
		for _, name := range w.specOf(s).Preys {
			for t := 0; t < n; t++ {
				if w.SpeciesName(t) == name {
					table[s][t] = true
					any = true
				}
			}
		}
	}
	if any {
		w.prey = table
	}
}

// damage is what one blow of body b takes off body o: HitEnergy times
// b's attack over b's attack plus o's defense.
func (w *World) damage(b, o *Body) float64 {
	a, d := w.attackPower(b), w.defensePower(o)
	return w.cfg.HitEnergy * a / (a + d)
}

// preyIn calls f with every prey of body b standing on a tile within its
// sight, in the order of the grid.
func (w *World) preyIn(b *Body, f func(*Body)) {
	sight := w.sightOf(b)
	if !w.cfg.Hunt || w.prey == nil || sight < 0 || w.grid == nil {
		return
	}
	bx, by := int(math.Floor(b.X)), int(math.Floor(b.Y))
	for y := by - sight; y <= by+sight; y++ {
		for x := bx - sight; x <= bx+sight; x++ {
			if !w.m.InBounds(x, y) || !w.seesFrom(b, bx, by, x, y) {
				continue
			}
			for _, j := range w.grid[w.m.index(x, y)] {
				if o := &w.bodies[j]; o.ID != b.ID && w.preysOn(b, o) && !o.Slain {
					f(o)
				}
			}
		}
	}
}

// adjacent reports whether bodies b and o stand on the same tile or on
// tiles next to each other.
func adjacent(b, o *Body) bool {
	return math.Abs(math.Floor(b.X)-math.Floor(o.X)) <= 1 && math.Abs(math.Floor(b.Y)-math.Floor(o.Y)) <= 1
}

// huntOptions appends an attack on every prey body b sees, and works out
// for each the ticks to the kill: the walk to a tile next to it, then
// a blow a tick until its energy is gone.
func (w *World) huntOptions(dst []Action, b *Body) []Action {
	clear(w.hunt)
	w.preyIn(b, func(o *Body) {
		dst = append(dst, Action{Kind: ActAttack, Mate: o.ID})
		if w.hunt == nil {
			w.hunt = map[int64]int{}
		}
		gap := math.Max(math.Abs(o.X-b.X), math.Abs(o.Y-b.Y)) - 1
		walk := int(math.Ceil(math.Max(gap, 0) / w.speedOf(b)))
		blows := int(math.Ceil(o.Energy / w.damage(b, o)))
		w.hunt[o.ID] = walk + max(blows, 1)
	})
	return dst
}

// huntTicks is how many ticks the body whose options were last listed
// needs to kill prey id; huntOptions works it out once per decision.
func (w *World) huntTicks(id int64) int { return w.hunt[id] }

// attack carries out body b's attack on prey id: a blow if it stands
// next to it, else a step towards it. A prey struck to nothing is slain,
// and dies at the end of the tick (removeDead) leaving its meat to b.
func (w *World) attack(i int, b *Body, id int64) {
	var o *Body
	w.preyIn(b, func(p *Body) {
		if p.ID == id {
			o = p
		}
	})
	if o == nil {
		return
	}
	if !adjacent(b, o) {
		w.stepTowards(i, b, dirTowards(o.X-b.X, o.Y-b.Y))
		return
	}
	o.Energy -= w.damage(b, o)
	w.stats.Blows++
	if o.Energy <= 0 {
		o.Slain, o.Slayer = true, b.ID
	}
}

// dropMeat lays the meat of slain body b on its tile and the tiles round
// it: MeatUnits units of the kill pool, owned by its slayer. Where no
// tile is free the rest is not laid (nothing appears from nothing but a
// kill, so the ledger closes either way).
func (w *World) dropMeat(b *Body) {
	p := -1
	for i := range w.food.pools {
		if w.food.pools[i].kill {
			p = i
			break
		}
	}
	if p < 0 {
		return
	}
	bx, by := int(math.Floor(b.X)), int(math.Floor(b.Y))
	laid := 0
	for _, d := range [9][2]int{{0, 0}, {1, 0}, {-1, 0}, {0, 1}, {0, -1}, {1, 1}, {-1, 1}, {1, -1}, {-1, -1}} {
		if laid >= w.cfg.MeatUnits {
			break
		}
		x, y := bx+d[0], by+d[1]
		if !w.m.InBounds(x, y) {
			continue
		}
		t := w.m.index(x, y)
		if w.m.Terrain[t] != w.food.pools[p].terrain || w.food.foodAt[t] != 0 || w.food.pools[p].count+w.food.pools[p].held >= w.food.pools[p].cap {
			continue
		}
		w.putFood(t, p)
		w.food.owner[len(w.food.foods)-1] = b.Slayer
		laid++
	}
	w.stats.Kills++
}
