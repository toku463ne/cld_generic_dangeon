package engine

import "math"

// Bodies.
//
// A body has a position, an amount of energy and the tick it came into the
// world. Every tick it takes one action and then spends EnergyBurn, moving or
// not; a body whose energy reaches zero starves. Eating a unit of food on the
// tile it stands on gives FoodEnergy, up to EnergyMax. That is the recovery
// path: without it every body would only run down.
//
// Which actions a body can take is a matter of what is possible, not of a
// judgement: a move onto water or off the map is not offered at all.

// Body is one individual.
type Body struct {
	ID     int64
	X, Y   float64 // in tiles; the tile is the integer part
	Energy float64
	Born   int64 // tick it came into the world
	// Heading is the direction of the body's last move, -1 before its
	// first. Only KeepHeading reads it.
	Heading int

	// Between decisions (Recheck above zero, intent.go) the body follows
	// Intent, the action its last decision took. Goal is the tile of the
	// food that action walks to, -1 for none; Decided is the tick of that
	// decision, -1 before the first. Under and Saw are what it perceived
	// on its last turn: food on its tile, and a hash of the food in sight.
	Intent  Action
	Goal    int
	Decided int64
	Under   bool
	Saw     uint64

	// Mature is the tick the body becomes an adult (breed.go); the first
	// bodies are adults from the start. Mates is a hash of the adults in
	// sight on its last turn.
	Mature int64
	Mates  uint64
	// Parents are the IDs of the two bodies it was born of, -1 for the
	// first bodies.
	Parents [2]int64
	// Build is its own speed, most energy and burn (build.go).
	Build Build
}

// ActionKind is what an action does, for counting.
type ActionKind int

const (
	ActWait ActionKind = iota
	ActEat
	ActMove
	ActMate
	NumActionKinds
)

// Action is one choice a body can make. Dir is used by moves only, and
// Mate, the ID of the body mated with, by ActMate only.
type Action struct {
	Kind ActionKind
	Dir  int
	Mate int64
}

// Cause is why a body died.
type Cause int

const (
	CauseStarved Cause = iota
	NumCauses
)

// The eight directions of a move, as unit vectors.
var moveDirs = func() [8][2]float64 {
	var d [8][2]float64
	for i := range d {
		a := float64(i) * math.Pi / 4
		d[i] = [2]float64{math.Cos(a), math.Sin(a)}
	}
	// Cos and Sin of multiples of pi/4 are not exactly 0 and 1; make the
	// axis-aligned ones exact so that a straight move stays on its line.
	for i := 0; i < 8; i += 2 {
		d[i] = [2]float64{math.Round(d[i][0]), math.Round(d[i][1])}
	}
	return d
}()

// Stats counts what has happened in the world since it was built.
type Stats struct {
	Deaths       [NumCauses]int64
	EnergyBurned float64
	Actions      [NumActionKinds]int64
	// Decisions counts the ticks a body valued its options, by what made
	// it decide. The ticks it followed its intent are the rest of Actions.
	Decisions [NumTriggers]int64
	// Births counts the bodies born (breed.go). Of them, Matured became
	// adults and DiedYoung died before.
	Births, Matured, DiedYoung int64
}

// tileOf returns the index of the tile under (x, y), or -1 off the map.
func (w *World) tileOf(x, y float64) int {
	tx, ty := int(math.Floor(x)), int(math.Floor(y))
	if !w.m.InBounds(tx, ty) {
		return -1
	}
	return w.m.index(tx, ty)
}

// placeBodies puts the starting bodies on the centres of land tiles drawn at
// random.
func (w *World) placeBodies() {
	var land []int
	for i, t := range w.m.Terrain {
		if t == TerrainLand {
			land = append(land, i)
		}
	}
	if len(land) == 0 {
		return
	}
	for i := 0; i < w.cfg.Bodies; i++ {
		t := land[w.rng.Intn(len(land))]
		w.bodies = append(w.bodies, Body{
			ID:      w.nextID,
			X:       float64(t%w.m.Width) + 0.5,
			Y:       float64(t/w.m.Width) + 0.5,
			Energy:  w.cfg.EnergyMax,
			Born:    w.tick,
			Heading: -1,
			Goal:    -1,
			Decided: -1,
			Parents: [2]int64{-1, -1},
		})
		w.nextID++
	}
}

// possibleActions appends every action body b can take now.
func (w *World) possibleActions(dst []Action, b *Body) []Action {
	dst = append(dst, Action{Kind: ActWait})
	if t := w.tileOf(b.X, b.Y); t >= 0 && w.foodOn(t) >= 0 {
		dst = append(dst, Action{Kind: ActEat})
	}
	for d, v := range moveDirs {
		t := w.tileOf(b.X+v[0]*w.speedOf(b), b.Y+v[1]*w.speedOf(b))
		if t >= 0 && w.m.Terrain[t] == TerrainLand {
			dst = append(dst, Action{Kind: ActMove, Dir: d})
		}
	}
	return w.mateOptions(dst, b)
}

// act carries out action a of body b, the i-th of the world's bodies; i is
// -1 for a body that is not one of them (tests).
func (w *World) act(i int, b *Body, a Action) {
	w.stats.Actions[a.Kind]++
	switch a.Kind {
	case ActMate:
		w.mate(b, a.Mate)
	case ActEat:
		t := w.tileOf(b.X, b.Y)
		w.eatFood(w.foodOn(t))
		b.Energy = math.Min(b.Energy+w.cfg.FoodEnergy, w.maxOf(b))
	case ActMove:
		from := w.tileOf(b.X, b.Y)
		v := moveDirs[a.Dir]
		b.X += v[0] * w.speedOf(b)
		b.Y += v[1] * w.speedOf(b)
		b.Heading = a.Dir
		w.moved(i, from, w.tileOf(b.X, b.Y))
	}
}

// Bodies returns a copy of the living bodies.
func (w *World) Bodies() []Body {
	return append([]Body(nil), w.bodies...)
}

// Stats returns what has happened since the world was built.
func (w *World) Stats() Stats { return w.stats }
