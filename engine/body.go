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
	// Generation is how many births lie between it and the first bodies:
	// 0 for them, the greater parent's plus one for a child (stage 9-1).
	// No rule reads it; it is an instrument.
	Generation int `json:",omitempty"`
	// Sight is the tiles it sees plus one, drawn at birth with AllotSight
	// (stage 9-3); zero stands for the config's Sight, as a zero build
	// stands for the config's values.
	Sight int `json:",omitempty"`
	// Fert is its fertility level plus one, drawn at birth with
	// AllotFertility (stage 9-4); zero stands for the neutral level.
	Fert int `json:",omitempty"`
	// Size is its budget level plus one, drawn at birth with AllotBudget
	// (stage 9-5b); zero stands for the neutral level. Budget (below) is
	// what the level buys.
	Size int `json:",omitempty"`
	// Species is the index of its row in the config's species table
	// (stage 9-7a); zero for the first row, or a world of one species.
	Species int `json:",omitempty"`
	// Bold is its exploratory level plus one, drawn at birth with
	// AllotVenture (stage 9-6a); zero stands for the neutral level. It is
	// outside the budget and buys nothing: it scales what the body expects
	// of regions it has little evidence of.
	Bold int `json:",omitempty"`
	// LastSlept is the tile its last sleep began on, plus one; zero for
	// none yet (ReuseNights, stage 9-3b).
	LastSlept int `json:",omitempty"`
	// Trod is a bit per tile of the map, set for the tiles it has stood
	// on (its first step sets the tile it stepped from). No rule reads
	// it; it is an instrument (Stats.Steps, stage 9-6).
	Trod []uint64 `json:",omitempty"`
	// Sex is Female or Male, drawn when it comes into the world (with
	// Sexes); NoSex in a world without sexes.
	Sex Sex `json:",omitempty"`
	// Skill is the level of each skill it has (stage 8-1; 0 for none):
	// for now only the ones it was born with.
	Skill [NumSkills]float64 `json:",omitempty"`
	// Tent is the tent it carries (Tents, stage 8-2a).
	Tent Tent `json:",omitempty"`
	// Home is the ID of its settlement standing, zero for none, and Work
	// the one it is building (Building, stage 8-2b).
	Home int64 `json:",omitempty"`
	Work Work  `json:",omitempty"`
	// Rested is the tick a mother can mate again after a birth (with
	// FemaleBears); zero for none yet.
	Rested int64 `json:",omitempty"`
	// Partner is the father of a mother's last child (with FemaleBears),
	// -1 for none: whether he is near sets how likely she is provisioned.
	Partner int64 `json:",omitempty"`
	// Asks is how many units a resting mother has asked for and not yet
	// received (with ProvisionEach).
	Asks int `json:",omitempty"`
	// Requested is the tick its request to mate stays open until (with
	// Requests); zero or past for none.
	Requested int64 `json:",omitempty"`
	// Held is how many units of food it holds (with Carry).
	Held int `json:",omitempty"`
	// HeldBorn is the tick each held unit appeared, oldest first (with
	// FoodLife): a held unit decays like one on the ground. HeldPool is
	// each held unit's pool, in the same order (food.go).
	HeldBorn []int64 `json:",omitempty"`
	HeldPool []int   `json:",omitempty"`
	// Pressure is its sleep pressure (0 to 1), Debt its sleep debt, and
	// Asleep whether it sleeps now (Sleep, stage 7-3b).
	Pressure float64 `json:",omitempty"`
	Debt     float64 `json:",omitempty"`
	Asleep   bool    `json:",omitempty"`
	// Woke is the tick it wakes if nothing wakes it sooner (Sleep).
	Woke int64 `json:",omitempty"`
	// BurnFelt is the burn it last felt (planBurn) and BurnRise how fast
	// that has been rising per tick, smoothed over BurnSpan (stage 7-3a).
	BurnFelt float64 `json:",omitempty"`
	BurnRise float64 `json:",omitempty"`
	// LightKnown says it saw the light last tick, LightSeen what it was,
	// LightSince the tick the light came into its present step (-1: not
	// seen) and LightKey the key that made of it; Dusk is what it learned of when dark and light come
	// (DuskRead, stage 7-2b, dusk.go).
	LightKnown bool      `json:",omitempty"`
	LightSeen  float64   `json:",omitempty"`
	LightSince int64     `json:",omitempty"`
	LightKey   int32     `json:",omitempty"`
	Dusk       []DuskRow `json:",omitempty"`
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
	// ActPick picks up the unit underfoot (Carry, stage 4-1).
	ActPick
	// ActSleep falls asleep until the pressure falls to WakeAt (Sleep,
	// stage 7-3b).
	ActSleep
	// ActBuild is a tick of work on a settlement (Building, stage 8-2b).
	ActBuild
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
	// CauseAged is death at the end of a lifespan (Lifespan, stage 3-5).
	CauseAged
	// CauseSleepless is starving with a sleep debt above a quarter of
	// DebtBurn: the burn the debt raised outran the food (stage 7-3b).
	CauseSleepless
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
	// GenBirths counts the bodies that came into the world by generation:
	// the first bodies at 0, a child at the greater parent's generation
	// plus one (stage 9-1).
	GenBirths []int64 `json:",omitempty"`
	// GenBudget sums, by generation, the budgets of the bodies that came
	// into the world (stage 9-5b): over GenBirths it is the mean budget
	// born into a generation.
	GenBudget []float64 `json:",omitempty"`
	// BudgetIn and BudgetOut are the budgets brought in by the born and
	// taken out by the dead (BudgetLedger).
	BudgetIn, BudgetOut float64
	// RegionRows (indexed by RegionID), PathRow and MateRow count, for
	// each learned row, what bodies learned themselves and passed on.
	RegionRows       []RowCount `json:",omitempty"`
	PathRow, MateRow RowCount
	// Requests counts the requests to mate sent (Requests), and
	// RequestBirths the births a request brought about.
	Requests, RequestBirths int64
	// Matings counts the matings agreed on (stage 9-4), each a birth
	// unless the fertility dice say no; FertMatings and FertBirths count,
	// by fertility level, the parents in the matings and in the births.
	Matings                 int64   `json:",omitempty"`
	FertMatings, FertBirths []int64 `json:",omitempty"`
	// Provisioned counts, per region, the food that went straight to a
	// resting mother (Provision, food.go).
	Provisioned []int64 `json:",omitempty"`
	// EatenHeld counts the units bodies ate from what they held (Carry).
	EatenHeld int64 `json:",omitempty"`
	// SpotFound and SpotMissed count walks to a remembered unit that found
	// it there and that did not (stage 5-2).
	SpotFound, SpotMissed int64 `json:",omitempty"`
	// Slept counts sleeps begun, and SleptSheltered those begun on a place
	// to rest (stage 6-1b).
	Slept, SleptSheltered int64 `json:",omitempty"`
	// Built counts the places built (stage 8-1), BuiltSleeps the sleeps
	// begun on one by anyone but at its building, and Returns those of
	// them by its builder.
	Built, BuiltSleeps, Returns int64 `json:",omitempty"`
	// SleptAbroad counts the sleeps begun on a settlement of another
	// species (only with ClosedPlaces off; stage 9-7b-2b).
	SleptAbroad int64 `json:",omitempty"`
	// DiedOnWater counts the deaths of bodies standing in water (stage
	// 9-7b-2c), whatever the cause.
	DiedOnWater int64 `json:",omitempty"`
	// Begun, Fetched and Abandoned count the settlements begun, the times
	// a builder went for food in sight, and the sites given up (stage
	// 8-2b).
	Begun, Fetched, Abandoned int64 `json:",omitempty"`
	// TentsMade, TentNights and TentsWorn count the tents made, the nights
	// one was pitched and the tents worn out (stage 8-2a).
	TentsMade, TentNights, TentsWorn int64 `json:",omitempty"`
	// TentsDropped and TentsPicked count the tents left by the dead and
	// those taken up (stage 8-2a3).
	TentsDropped, TentsPicked int64 `json:",omitempty"`
	// SheltersPassed counts the places to rest passed from parent to child
	// that the child did not know (stage 6-5a).
	SheltersPassed int64 `json:",omitempty"`
	// Steps counts the steps that took a body onto another tile, and
	// NewSteps those onto a tile it had never stood on (Body.Trod, stage
	// 9-6): the untrodden-tile rate is NewSteps over Steps.
	Steps, NewSteps int64 `json:",omitempty"`
	// BoldSteps and BoldNewSteps count the same by the body's exploratory
	// level (AllotVenture, stage 9-6a).
	BoldSteps, BoldNewSteps []int64 `json:",omitempty"`
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
// random: Config.Bodies of them with drawn genomes, or, given genomes, one
// with each (NewWorldFrom).
func (w *World) placeBodies(genomes []Genome) {
	// Each species' first bodies land where its food comes back (Home),
	// or anywhere on land where it comes back nowhere.
	var land []int
	for i, t := range w.m.Terrain {
		if t == TerrainLand {
			land = append(land, i)
		}
	}
	if len(land) == 0 {
		return
	}
	homes := make([][]int, w.SpeciesCount())
	for s := range homes {
		var can []int
		for i, t := range w.m.Terrain {
			if w.specOf(s).enters(t) { // its own terrains, not wading
				can = append(can, i)
			}
		}
		if len(can) == 0 {
			can = land
		}
		for _, t := range can {
			if w.Home(s, w.m.Region[t]) {
				homes[s] = append(homes[s], t)
			}
		}
		if len(homes[s]) == 0 {
			homes[s] = can
		}
	}
	n := w.cfg.Bodies
	if genomes != nil {
		n = len(genomes)
	}
	for i := 0; i < n; i++ {
		species := 0
		if genomes != nil {
			species = genomes[i].Species
		} else {
			species = w.drawSpecies()
		}
		t := homes[species][w.rng.Intn(len(homes[species]))]
		b := Body{
			ID:      w.nextID,
			Species: species,
			X:       float64(t%w.m.Width) + 0.5,
			Y:       float64(t/w.m.Width) + 0.5,
			Energy:  w.cfg.EnergyMax,
			Born:    w.tick,
			Heading: -1,
			Goal:    -1,
			Decided: -1,
			Parents: [2]int64{-1, -1},
			Sex:     w.drawSex(),
			Partner: -1,
		}
		if genomes != nil {
			w.endow(&b, genomes[i])
		} else {
			w.allot(&b)
		}
		w.drawSkills(&b)
		b.Energy = w.maxOf(&b)
		w.noteGeneration(0, b.Budget)
		w.bodies = append(w.bodies, b)
		w.nextID++
	}
}

// possibleActions appends every action body b can take now.
func (w *World) possibleActions(dst []Action, b *Body) []Action {
	dst = append(dst, Action{Kind: ActWait})
	if t := w.tileOf(b.X, b.Y); t >= 0 && w.foodFor(b, t) >= 0 {
		dst = append(dst, Action{Kind: ActEat})
		if b.Held < w.cfg.Carry {
			dst = append(dst, Action{Kind: ActPick})
		}
	}
	if w.cfg.Sleep && b.Pressure > w.cfg.WakeAt {
		dst = append(dst, Action{Kind: ActSleep})
	}
	for d, v := range moveDirs {
		x, y := b.X+v[0]*w.speedOf(b), b.Y+v[1]*w.speedOf(b)
		t := w.tileOf(x, y)
		if t >= 0 && w.mayEnter(b, t) && !w.blocks(b, x, y) {
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
		if d, far := w.farMate(b, a.Mate); far {
			// A requester out of sight: a step towards it (Requests).
			w.stepTowards(i, b, d)
			return
		}
		if w.cfg.Learn {
			b.Memory.Asks++
			if w.cfg.AgeBand > 0 {
				w.observe(&b.Memory.Mate, 1, 0)
			}
			w.stats.MateRow.Learned++
		}
		w.mate(b, a.Mate)
	case ActEat:
		t := w.tileOf(b.X, b.Y)
		w.eatFood(w.foodFor(b, t))
		b.Energy = math.Min(b.Energy+w.cfg.FoodEnergy, w.maxOf(b))
	case ActSleep:
		w.noteReturn(b, w.tileOf(b.X, b.Y))
		w.fallAsleep(b)
	case ActBuild:
		w.buildTick(b)
	case ActPick:
		t := w.tileOf(b.X, b.Y)
		w.takeFood(w.foodFor(b, t), b)
		w.eatHeld(b) // where a meal fits, picking it up is eating it
	case ActMove:
		w.step(i, b, a.Dir)
	}
}

// fallAsleep has body b fall asleep where it is.
func (w *World) fallAsleep(b *Body) {
	b.Asleep, b.Woke = true, w.tick+int64(w.cfg.SleepTicks)
	w.stats.Slept++
	if w.m.ShelterAt(int(math.Floor(b.X)), int(math.Floor(b.Y))) {
		w.stats.SleptSheltered++
	}
	if t := w.tileOf(b.X, b.Y); t >= 0 && w.placeSp != nil && w.placeSp[t] != 0 && int(w.placeSp[t])-1 != b.Species {
		w.stats.SleptAbroad++
	}
}

// step moves body b, the i-th of the world's bodies, one step in direction
// d.
func (w *World) step(i int, b *Body, d int) {
	from := w.tileOf(b.X, b.Y)
	v := moveDirs[d]
	b.X += v[0] * w.speedOf(b)
	b.Y += v[1] * w.speedOf(b)
	b.Heading = d
	to := w.tileOf(b.X, b.Y)
	w.moved(i, from, to)
	w.stepped(b, from, to)
	w.trod(b, from, to)
}

// trod counts a step from tile from onto tile to, and whether the body had
// stood on to before (Body.Trod). An instrument: no rule reads it.
func (w *World) trod(b *Body, from, to int) {
	if to < 0 || to == from {
		return
	}
	if b.Trod == nil {
		b.Trod = make([]uint64, (len(w.m.Terrain)+63)/64)
		if from >= 0 {
			b.Trod[from/64] |= 1 << (from % 64)
		}
	}
	w.stats.Steps++
	fresh := b.Trod[to/64]&(1<<(to%64)) == 0
	if fresh {
		b.Trod[to/64] |= 1 << (to % 64)
		w.stats.NewSteps++
	}
	if w.cfg.AllotVenture {
		k := w.boldOf(b)
		for len(w.stats.BoldSteps) <= k {
			w.stats.BoldSteps = append(w.stats.BoldSteps, 0)
			w.stats.BoldNewSteps = append(w.stats.BoldNewSteps, 0)
		}
		w.stats.BoldSteps[k]++
		if fresh {
			w.stats.BoldNewSteps[k]++
		}
	}
}

// stepTowards moves body b a step in direction d if that step is open, and
// otherwise waits.
func (w *World) stepTowards(i int, b *Body, d int) {
	v := moveDirs[d]
	x, y := b.X+v[0]*w.speedOf(b), b.Y+v[1]*w.speedOf(b)
	if t := w.tileOf(x, y); t >= 0 && w.mayEnter(b, t) && !w.blocks(b, x, y) {
		w.step(i, b, d)
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
	s.Provisioned = append([]int64(nil), s.Provisioned...)
	// Every slice is copied: a copy taken at one tick and read against
	// the world's later must not share its counts (the experiment reads
	// the untrodden-tile rate by level as the difference since tick 5000).
	s.GenBirths = append([]int64(nil), s.GenBirths...)
	s.GenBudget = append([]float64(nil), s.GenBudget...)
	s.FertMatings = append([]int64(nil), s.FertMatings...)
	s.FertBirths = append([]int64(nil), s.FertBirths...)
	s.BoldSteps = append([]int64(nil), s.BoldSteps...)
	s.BoldNewSteps = append([]int64(nil), s.BoldNewSteps...)
	return s
}

// provisioned counts a unit given to a resting mother in region r.
func (w *World) provisioned(r RegionID) {
	for int(r) >= len(w.stats.Provisioned) {
		w.stats.Provisioned = append(w.stats.Provisioned, 0)
	}
	w.stats.Provisioned[r]++
}

// regionRow returns the counts of region r's row, growing the list to it.
func (w *World) regionRow(r RegionID) *RowCount {
	for int(r) >= len(w.stats.RegionRows) {
		w.stats.RegionRows = append(w.stats.RegionRows, RowCount{})
	}
	return &w.stats.RegionRows[r]
}
