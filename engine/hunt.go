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

// hunting reports whether hunting is on now (Hunt, from HuntFrom).
func (w *World) hunting() bool { return w.cfg.Hunt && w.tick >= w.cfg.HuntFrom }

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

// hungry reports whether body b is hungry enough to hunt (HuntHunger).
func (w *World) hungry(b *Body) bool {
	return b.Energy < w.cfg.HuntHunger*w.maxOf(b)
}

// preyIn calls f with every prey of body b standing on a tile within its
// sight, in the order of the grid; nothing to a hunter not hungry enough
// to hunt (stage 10-2g).
func (w *World) preyIn(b *Body, f func(*Body)) {
	sight := w.sightOf(b)
	if !w.hunting() || w.prey == nil || sight < 0 || w.grid == nil || !w.hungry(b) {
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

// adjacent reports whether bodies b and o stand within a tile of each
// other, as the crow flies on each axis: within reach of a blow. It is
// read on positions, not tiles, so that one step away is one step out of
// reach - the prey's reading of a step (threatAt) and the hunter's blow
// agree.
func adjacent(b, o *Body) bool {
	return math.Abs(b.X-o.X) <= 1 && math.Abs(b.Y-o.Y) <= 1
}

// huntOptions appends an attack on every prey body b sees that it can
// catch, and works out for each the ticks to the kill: the chase to
// within reach - the prey read as fleeing at its own speed, so the gap
// closes at the hunter's speed over the prey's (a sleeping prey does not
// flee) - then a blow a tick until its energy is gone. A prey out of
// reach that b cannot outrun is no option (stage 10-2b).
func (w *World) huntOptions(dst []Action, b *Body) []Action {
	clear(w.hunt)
	// Striking back, with its band, at a hunter that has just struck
	// (stages 10-2c, 10-2e).
	w.foesOf(b, func(h *Body) {
		dst = append(dst, Action{Kind: ActAttack, Mate: h.ID})
		if w.hunt == nil {
			w.hunt = map[int64]int{}
		}
		ticks, _, _ := w.fightLoss(b, h)
		w.hunt[h.ID] = ticks
	})
	w.preyIn(b, func(o *Body) {
		if _, down := w.huntLoss(b, o); down {
			return // the prey and its kind would down it first (Odds)
		}
		gap := math.Max(math.Abs(o.X-b.X), math.Abs(o.Y-b.Y)) - 1
		walk := 0
		if gap > 0 {
			closing := w.speedOf(b)
			if !o.Asleep {
				closing -= w.speedOf(o)
			}
			if closing <= 0 {
				return
			}
			walk = int(math.Ceil(gap / closing))
		}
		dst = append(dst, Action{Kind: ActAttack, Mate: o.ID})
		if w.hunt == nil {
			w.hunt = map[int64]int{}
		}
		blows := int(math.Ceil(o.Energy / w.damage(b, o)))
		w.hunt[o.ID] = walk + max(blows, 1)
	})
	return dst
}

// preyNamed returns the prey id among those body b sees, or nil.
func (w *World) preyNamed(b *Body, id int64) *Body {
	var p *Body
	w.preyIn(b, func(o *Body) {
		if o.ID == id {
			p = o
		}
	})
	return p
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
	back := false
	if o == nil {
		if foe := w.foeOf(b, id); foe != nil {
			o, back = foe, true // striking back: no chase
		}
	}
	if o == nil {
		return
	}
	if !adjacent(b, o) {
		w.stepTowards(i, b, dirTowards(o.X-b.X, o.Y-b.Y))
		return
	}
	o.Energy -= w.damage(b, o)
	o.Struck = true
	b.Striking = w.tick + 1
	w.stats.Blows++
	if back {
		w.stats.BlowsBack++
	}
	if !back {
		w.call(o, CallAttacked, b.ID) // the prey calls out (stage 10-4)
	}
	if o.Energy <= 0 {
		o.Slain, o.Slayer = true, b.ID
		if back {
			w.stats.HuntersSlain++
			w.call(b, CallDefeated, o.ID) // the slayer calls out (stage 10-4)
		}
		if o.Asleep {
			w.stats.KilledAsleep++
		}
		if t := w.tileOf(o.X, o.Y); t >= 0 && !w.Home(o.Species, w.m.Region[t]) {
			w.stats.KilledAway++
		}
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
		if !w.m.Terrain[t].Ground() || w.food.foodAt[t] != 0 || w.food.pools[p].count+w.food.pools[p].held >= w.food.pools[p].cap {
			continue
		}
		w.putFood(t, p)
		w.food.owner[len(w.food.foods)-1] = b.Slayer
		laid++
	}
	w.stats.Kills++
}

// Reading a threat (stage 10-2b, VISION: a node fights or flees a threat
// as its state allows). A prey perceives the hunters in its sight: while
// one is in sight, or when a blow lands, it decides every tick (the
// threat trigger, intent.go), and its valuation reads off every option
// the blows the hunters would land on it over the window from where the
// option leaves it (threatLoss): within reach, a blow a tick until it is
// dead; farther off, from when a faster hunter catches up; none from one
// it can outrun. There is no move of fleeing: with the blows read, the
// step that keeps the hunter off longest comes out best. The hunter, for
// its part, chases only a prey it can outrun, and keeps the chase up
// only while the kill still reads as a meal worth its ticks.

// hunters calls f with every body within body b's sight whose species
// preys on b's.
func (w *World) hunters(b *Body, f func(*Body)) {
	sight := w.sightOf(b)
	if !w.hunting() || !w.cfg.Fear || w.prey == nil || sight < 0 || w.grid == nil {
		return
	}
	bx, by := int(math.Floor(b.X)), int(math.Floor(b.Y))
	for y := by - sight; y <= by+sight; y++ {
		for x := bx - sight; x <= bx+sight; x++ {
			if !w.m.InBounds(x, y) || !w.seesFrom(b, bx, by, x, y) {
				continue
			}
			for _, j := range w.grid[w.m.index(x, y)] {
				if o := &w.bodies[j]; o.ID != b.ID && w.preysOn(o, b) {
					f(o)
				}
			}
		}
	}
}

// sawThreats hashes the hunters body b sees, as sawMates does the mates.
func (w *World) sawThreats(b *Body) uint64 {
	var h uint64
	w.hunters(b, func(o *Body) { h += mix(uint64(o.ID)) })
	return h
}

// threatLoss is the energy body b, left at (x, y) by an option, would
// lose to the hunters it sees now (the list threats, filled by valueInto)
// over the next ticks: a hunter within reach of (x, y) after a step of
// its own strikes every tick from now; one farther off, faster than b,
// catches up at the closing speed and strikes from then; one b can
// outrun never does. The blows stop at what kills b. The worst hunter
// counts: within reach of one, the option reads as dying there unless
// the window ends first.
func (w *World) threatLoss(b *Body, x, y float64, ticks int) float64 {
	worst := 0.0
	for _, h := range w.threats {
		reach := 1 + w.speedOf(h)
		d := math.Max(math.Abs(h.X-x), math.Abs(h.Y-y))
		catch := 0.0
		if d > reach {
			closing := w.speedOf(h) - w.speedOf(b)
			if closing <= 0 {
				continue
			}
			catch = (d - reach) / closing
		}
		blows := float64(ticks) - catch
		if blows <= 0 {
			continue
		}
		blow := w.damage(h, b)
		worst = math.Max(worst, math.Min(blows, math.Ceil(b.Energy/blow))*blow)
	}
	return worst
}

// Striking back, together (stages 10-2c and 10-2e, the user's choice: a
// prey fights as well as flees, and its kind joins in). A hunter that
// has just struck (Striking, within FightTicks) may be attacked by every
// body of the prey's species standing within its reach, whatever their
// row says of prey: the blows land as a hunter's do. Each reads the
// fight as the blows it takes until the hunter is down under the blows
// of all who stand within reach of it - the hunter's blows shared among
// them - or it is down first, which reads as death (valueInto). A
// hunter out of sight, or one that has not struck lately, is no option.

// hunted reports whether some species preys on species s.
func (w *World) hunted(s int) bool {
	if w.prey == nil || !w.hunting() {
		return false
	}
	for h := range w.prey {
		if w.prey[h][s] {
			return true
		}
	}
	return false
}

// foesOf calls f with every hunter of body b's species within b's sight
// and reach that has struck within FightTicks: those b may strike.
func (w *World) foesOf(b *Body, f func(*Body)) {
	if !w.cfg.Fight {
		return
	}
	w.hunters(b, func(h *Body) {
		if h.Striking > 0 && w.tick-(h.Striking-1) <= int64(w.cfg.FightTicks) && adjacent(b, h) {
			f(h)
		}
	})
}

// foeOf returns the hunter id, if body b may strike it now.
func (w *World) foeOf(b *Body, id int64) *Body {
	var foe *Body
	w.foesOf(b, func(h *Body) {
		if h.ID == id {
			foe = h
		}
	})
	return foe
}

// band counts the bodies of body b's species, b among them, standing
// within reach of hunter h: those who fight it together.
func (w *World) band(b, h *Body) int {
	n := 0
	hx, hy := int(math.Floor(h.X)), int(math.Floor(h.Y))
	for y := hy - 2; y <= hy+2; y++ {
		for x := hx - 2; x <= hx+2; x++ {
			if !w.m.InBounds(x, y) {
				continue
			}
			for _, j := range w.grid[w.m.index(x, y)] {
				if o := &w.bodies[j]; o.Species == b.Species && adjacent(o, h) {
					n++
				}
			}
		}
	}
	return max(n, 1)
}

// fightTicks is how many blows body b needs to down body o.
func (w *World) fightTicks(b, o *Body) int {
	return max(int(math.Ceil(o.Energy/w.damage(b, o))), 1)
}

// fightLoss is what body b reads of fighting hunter h with its band: the
// ticks until h is down under the band's blows, the energy h's blows
// take off b meanwhile (shared among the band, and read smaller by an
// aggressive body - fierceness, stage 10-3), and whether b is down
// first.
func (w *World) fightLoss(b, h *Body) (ticks int, loss float64, down bool) {
	n := w.band(b, h)
	ticks = max(int(math.Ceil(h.Energy/(float64(n)*w.damage(b, h)))), 1)
	loss = float64(ticks) * w.damage(h, b) / float64(n) / w.fierceness(b)
	return ticks, loss, loss >= b.Energy
}

// huntLoss is what hunter b reads of attacking prey o (Odds, stage 10-3):
// the blows o and its kind within reach of it would land on b over the
// blows b needs to down o, once they strike back, read smaller by an
// aggressive hunter; and whether that would down b first. Without Odds a
// hunter reads nothing.
func (w *World) huntLoss(b, o *Body) (loss float64, down bool) {
	if !w.cfg.Odds || !w.cfg.Fight {
		return 0, false
	}
	n := w.band(o, o) // o's kind within reach of o, o among them
	blows := w.fightTicks(b, o)
	loss = float64(n) * w.damage(o, b) * float64(max(blows-1, 0)) / w.fierceness(b)
	return loss, loss >= b.Energy
}

// safe reports whether tile t is a refuge for body b: a settlement of
// its own species (ClosedPlaces), or one of the map's places to rest
// that belongs to its species (ClosedShelters) - which its hunters may
// not enter.
func (w *World) safe(b *Body, t int) bool {
	if w.cfg.ClosedPlaces && w.placeSp != nil && w.placeSp[t] != 0 {
		return int(w.placeSp[t])-1 == b.Species
	}
	return w.ownerOf(t) == b.Species
}

// Calls (stage 10-4, VISION "発信"): a body struck calls out, and a body
// that has struck down a hunter calls out that it did. A call reaches
// the bodies of its species within CallRange tiles. One who hears a
// body struck remembers the alarm against the region (Memory.Alarms)
// and decides this tick (the call trigger, intent.go); one who hears a
// hunter struck down thinks better of the body that did it
// (Memory.Liking), which is read off the knowledge, not acted on yet -
// cooperation, trade and mating will read it. Nothing comes to help:
// a hearer within reach of the hunter fights it as its band anyway.

// CallKind is what a call tells.
type CallKind int

const (
	CallAttacked CallKind = iota
	CallDefeated
	NumCallKinds
)

// call has body b call out k; from is the body the call is about (the
// hunter struck down, for CallDefeated).
func (w *World) call(b *Body, k CallKind, about int64) {
	if !w.cfg.Calls || w.grid == nil {
		return
	}
	w.stats.Calls[k]++
	bx, by := int(math.Floor(b.X)), int(math.Floor(b.Y))
	r := w.m.Region[w.tileOf(b.X, b.Y)]
	n := w.cfg.CallRange
	for y := by - n; y <= by+n; y++ {
		for x := bx - n; x <= bx+n; x++ {
			if !w.m.InBounds(x, y) {
				continue
			}
			for _, j := range w.grid[w.m.index(x, y)] {
				o := &w.bodies[j]
				if o.ID == b.ID || o.Species != b.Species {
					continue
				}
				w.stats.CallsHeard++
				switch k {
				case CallAttacked:
					if o.Memory.Alarms == nil {
						o.Memory.Alarms = map[int]int64{}
					}
					o.Memory.Alarms[int(r)]++
					o.Called = true
				case CallDefeated:
					if o.Memory.Liking == nil {
						o.Memory.Liking = map[int64]float64{}
					}
					o.Memory.Liking[b.ID]++
				}
			}
		}
	}
}

// Liking is how well body b thinks of the body id, read off what it
// has heard: zero for a stranger.
func (w *World) Liking(b Body, id int64) float64 { return b.Memory.Liking[id] }

// Alarms is how many times body b has heard its kind struck in region r.
func (w *World) Alarms(b Body, r RegionID) int64 { return b.Memory.Alarms[int(r)] }

// threatened reports whether body b has a hunter in sight now.
func (w *World) threatened(b *Body) bool {
	seen := false
	w.hunters(b, func(*Body) { seen = true })
	return seen
}
