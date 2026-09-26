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
// judgement: a move onto water or off the map, or into a tile another body
// stands on (Collide), is not offered at all.

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
	// Sex is Female or Male, drawn when it comes into the world (with
	// Sexes); NoSex in a world without sexes.
	Sex Sex `json:",omitempty"`
	// Rested is the tick a mother can mate again after a birth (with
	// FemaleBears); zero for none yet.
	Rested int64 `json:",omitempty"`
	// Build is its own speed, most energy and burn (build.go), and Share
	// the share of its budget it was born with for speed (with Allot).
	Build Build
	Share float64 `json:",omitempty"`
	// Level is the share's level (0 to AllotLevels-1), which a child
	// inherits; Budget is the size of its budget (Config.Budget for now).
	Level  int     `json:",omitempty"`
	Budget float64 `json:",omitempty"`
	// Memory is what it has learned (learn.go), with Learn.
	Memory Memory
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
	// BudgetIn and BudgetOut are the budgets brought in by the born and
	// taken out by the dead (BudgetLedger).
	BudgetIn, BudgetOut float64
	// RegionRows (indexed by RegionID), PathRow and MateRow count, for
	// each learned row, what bodies learned themselves and passed on.
	RegionRows       []RowCount `json:",omitempty"`
	PathRow, MateRow RowCount
}

// RowCount counts, for one learned row, the observations bodies added to it
// themselves (Learned: tiles come into view, or mates asked) and the times a
// body gave another the evidence it held for it (Passed, tell.go).
type RowCount struct{ Learned, Passed int64 }

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
		b := Body{
			ID:      w.nextID,
			X:       float64(t%w.m.Width) + 0.5,
			Y:       float64(t/w.m.Width) + 0.5,
			Energy:  w.cfg.EnergyMax,
			Born:    w.tick,
			Heading: -1,
			Goal:    -1,
			Decided: -1,
			Parents: [2]int64{-1, -1},
			Sex:     w.drawSex(),
		}
		w.allot(&b)
		b.Energy = w.maxOf(&b)
		w.bodies = append(w.bodies, b)
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
		x, y := b.X+v[0]*w.speedOf(b), b.Y+v[1]*w.speedOf(b)
		t := w.tileOf(x, y)
		if t >= 0 && w.m.Terrain[t] == TerrainLand && !w.blocks(b, x, y) {
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
		if w.cfg.Learn {
			b.Memory.Asks++
			w.stats.MateRow.Learned++
		}
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
		to := w.tileOf(b.X, b.Y)
		w.moved(i, from, to)
		w.stepped(b, from, to)
	}
}

// Bodies returns a copy of the living bodies.
func (w *World) Bodies() []Body {
	return append([]Body(nil), w.bodies...)
}

// Stats returns what has happened since the world was built.
func (w *World) Stats() Stats {
	s := w.stats
	s.RegionRows = append([]RowCount(nil), s.RegionRows...)
	return s
}

// regionRow returns the counts of region r's row, growing the list to it.
func (w *World) regionRow(r RegionID) *RowCount {
	for int(r) >= len(w.stats.RegionRows) {
		w.stats.RegionRows = append(w.stats.RegionRows, RowCount{})
	}
	return &w.stats.RegionRows[r]
}
