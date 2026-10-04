package engine

import "math"

// Fathers sending food (stage 6-4a, the user's design).
//
// While a mother rests after a birth, she asks for food now and then (the
// asks of provisioning, ProvisionEach), and the father of that child,
// standing on a unit of food while she has an ask open, sends it to her
// if his own life does not need it:
// not having it would raise his chance of dying within the window by no
// more than SendGate (life before offspring, as in 7-3d). The unit leaves
// him and, after a tick for each Speed of the way, comes to the place to
// rest nearest the mother that she remembers. He spends no time on the
// way - sending is already weaker than provisioning was, which needed no
// one at all. The unit is the same unit: it keeps the tick it appeared and
// decays on the way like any other, so the food's total is kept.

// Sent is a unit on its way to a place to rest.
type Sent struct {
	Arrive int64
	Tile   int
	Born   int64
	Mother int64
}

// listMates notes, for every father of a resting mother, her index among
// the bodies this tick (the latest rest if he has more than one).
func (w *World) listMates() {
	if !w.cfg.FatherSends {
		return
	}
	if w.mateOf == nil {
		w.mateOf = map[int64]int{}
	}
	clear(w.mateOf)
	for i := range w.bodies {
		m := &w.bodies[i]
		if !w.resting(m) || m.Partner < 0 {
			continue
		}
		if j, ok := w.mateOf[m.Partner]; ok && w.bodies[j].Rested >= m.Rested {
			continue
		}
		w.mateOf[m.Partner] = i
	}
}

// restsAt is the tile of the place to rest nearest mother m that she
// remembers, and false if she knows none.
func (w *World) restsAt(m *Body) (int, bool) {
	best, bt := math.Inf(1), -1
	for t := range m.Memory.Shelters {
		x, y := t%w.m.Width, t/w.m.Width
		d := math.Max(math.Abs(float64(x)+0.5-m.X), math.Abs(float64(y)+0.5-m.Y))
		if d < best || d == best && t < bt {
			best, bt = d, t
		}
	}
	return bt, bt >= 0
}

// sendInstead is what father b, having decided a, does instead: send the
// unit underfoot to his resting mate, if he has one who knows a place to
// rest and his life does not need the unit, read from the decision just
// made.
func (w *World) sendInstead(b *Body, a Action) Action {
	if !w.cfg.FatherSends || b.Asleep {
		return a
	}
	mi, ok := w.mateOf[b.ID]
	if !ok || w.bodies[mi].Asks <= 0 {
		return a // she has not asked (6-4a: what she asks for, no more)
	}
	if t := w.tileOf(b.X, b.Y); t < 0 || w.foodOn(t) < 0 {
		return a
	}
	if _, ok := w.restsAt(&w.bodies[mi]); !ok {
		return a
	}
	v := &w.valuation
	took, without := math.Inf(1), math.Inf(1)
	for j, o := range v.Options {
		if j >= len(v.Risk[0]) {
			break
		}
		if o.Kind == ActEat || o.Kind == ActPick {
			took = math.Min(took, v.Risk[0][j])
		} else {
			without = math.Min(without, v.Risk[0][j])
		}
	}
	if math.IsInf(took, 1) || without-took > w.cfg.SendGate {
		return a
	}
	return Action{Kind: ActSend}
}

// send has father b send the unit underfoot to his resting mate.
func (w *World) send(b *Body) {
	t := w.tileOf(b.X, b.Y)
	i := w.foodOn(t)
	mi, ok := w.mateOf[b.ID]
	if i < 0 || !ok {
		return
	}
	m := &w.bodies[mi]
	dest, ok := w.restsAt(m)
	if !ok {
		return
	}
	f := &w.food
	born := f.born[i]
	w.liftFood(i)
	f.held++
	dx, dy := abs(dest%w.m.Width-t%w.m.Width), abs(dest/w.m.Width-t/w.m.Width)
	ticks := int64(math.Ceil(float64(max(dx, dy)) / w.cfg.Speed))
	f.sending = append(f.sending, Sent{Arrive: w.tick + ticks, Tile: dest, Born: born, Mother: m.ID})
	m.Asks--
	w.stats.Sent++
}

// deliverFood puts the units whose way has ended on the tile sent to.
// With HomeRead (stage 6-4b) a unit finding food there waits on its way
// until the tile is free (decaying meanwhile like any unit), so delivered
// food lies only where the mother looks for it; without it (6-4a) the
// unit goes to the nearest free land within two of the tile, or is lost.
func (w *World) deliverFood() {
	f := &w.food
	kept := f.sending[:0]
	for _, s := range f.sending {
		if s.Arrive > w.tick || w.cfg.HomeRead && f.foodAt[s.Tile] != 0 {
			kept = append(kept, s)
			continue
		}
		f.held--
		at := s.Tile
		if !w.cfg.HomeRead {
			if at = w.freeNear(s.Tile, 2); at < 0 {
				f.lost++
				continue
			}
		}
		f.foods = append(f.foods, Food{X: at % w.m.Width, Y: at / w.m.Width})
		f.born = append(f.born, s.Born)
		f.foodAt[at] = int32(len(f.foods))
		w.foodMoved(w.m.Region[at], +1)
		if f.delivered == nil {
			f.delivered = map[int]int64{}
		}
		f.delivered[at] = s.Mother
		w.stats.Delivered++
	}
	f.sending = kept
}

// freeNear is the land tile without food nearest tile t, within r of it
// (rings out from t, rows in order), or -1.
func (w *World) freeNear(t, r int) int {
	tx, ty := t%w.m.Width, t/w.m.Width
	for d := 0; d <= r; d++ {
		for y := ty - d; y <= ty+d; y++ {
			for x := tx - d; x <= tx+d; x++ {
				if max(abs(x-tx), abs(y-ty)) != d || !w.m.InBounds(x, y) || w.m.TerrainAt(x, y) != TerrainLand {
					continue
				}
				if u := w.m.index(x, y); w.food.foodAt[u] == 0 {
					return u
				}
			}
		}
	}
	return -1
}

// noteDelivered counts who takes a delivered unit, before body b eats or
// picks up the unit on tile t.
func (w *World) noteDelivered(b *Body, t int) {
	m, ok := w.food.delivered[t]
	if !ok {
		return
	}
	if m == b.ID {
		w.stats.MotherAte++
	} else {
		w.stats.OthersAte++
	}
}

// noteHome has resting mother b, standing on the tile her food comes to
// after asking for it, learn whether food was there (HomeRead, stage
// 6-4b): what lies underfoot, which she sees.
func (w *World) noteHome(b *Body) {
	if !w.cfg.Learn || !w.cfg.HomeRead || !w.resting(b) || b.AskedAt <= b.HomeAt {
		return
	}
	home, ok := w.restsAt(b)
	if !ok || w.tileOf(b.X, b.Y) != home {
		return
	}
	found := 0.0
	if w.foodOn(home) >= 0 {
		found = 1
		w.stats.HomeFound++
	} else {
		w.stats.HomeMissed++
	}
	w.observe(&b.Memory.Home, 1, found)
	b.HomeAt = w.tick
}

// homeAhead returns the place resting mother b's food comes to, if she
// has asked since she was last there, and the chance she reads food there
// at (HomeRead, stage 6-4b).
func (w *World) homeAhead(b *Body) (Food, float64, bool) {
	if !w.cfg.Learn || !w.cfg.HomeRead || !w.resting(b) || b.AskedAt <= b.HomeAt {
		return Food{}, 0, false
	}
	home, ok := w.restsAt(b)
	if !ok {
		return Food{}, 0, false
	}
	return Food{X: home % w.m.Width, Y: home / w.m.Width}, w.Fresh(b.Memory.Home).estimate(w.cfg.PriorSpot, w.cfg.SpotWeight), true
}
