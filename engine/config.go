package engine

// Config holds the rules of the world. Every simulation constant lives here as
// a field rather than a package const, so that a test can switch one rule off
// and an experiment can put two variants into the same binary.
//
// Anything that only changes how a run is measured (seed counts, run lengths,
// the collapse floor) does not belong here: it is not a rule of the world.
type Config struct {
	// Seed is the seed of the world's single random source.
	Seed int64

	// FoodCap is how many units of food may exist on the map at once. The
	// world starts with the map full, and a unit that is eaten leaves a
	// vacancy that comes back somewhere later: food on the ground plus
	// vacancies is always FoodCap.
	FoodCap int
	// FoodReturn is the chance per tick that one vacancy comes back. Where it
	// comes back is decided by the map's RegionFood shares.
	FoodReturn float64
	// FoodEnergy is how much energy eating one unit gives.
	FoodEnergy float64

	// Bodies is how many bodies the world starts with, placed on land.
	Bodies int
	// EnergyMax is the most energy a body can hold. Bodies start full.
	EnergyMax float64
	// EnergyBurn is the energy every body spends per tick, moving or not.
	EnergyBurn float64
	// Speed is how far a move takes a body, in tiles per tick.
	Speed float64

	// Window is how many ticks ahead a body looks when it values its
	// options: each is valued by the chance of being dead at the end of the
	// window, and the body takes the least. Zero looks nowhere, so every
	// possible option ties; with KeepHeading off the choice is then the
	// uniform draw of stage 1-1, the control later stages pair against.
	Window int
	// Sight is how far a body sees food, in tiles: the square of tiles
	// within Sight of the one it stands on, which is 3x3 at 1. Zero sees
	// the tile it stands on only; a negative Sight sees nothing.
	Sight int
	// KeepHeading breaks ties by the body's last move: when the options of
	// least risk include moving the way it last moved, it moves that way,
	// and it draws at random among them otherwise - where the land ends,
	// water or another body stands, or the way has become worse, that move
	// is simply not among them. It makes "keep moving enters new tiles"
	// (the table's third row) near true of the body's own walk.
	KeepHeading bool
	// Bounce, with KeepHeading, reflects the heading off whatever makes it
	// worse than the best instead of drawing (bounce, stages 1-2q to 1-5):
	// an axis whose step alone is not among the options of least risk is
	// turned back. Off since 2026-09-26: a rule made to walk well, which
	// memory is to replace (docs/history/20260926.md).
	Bounce bool
	// TurnOffReverse, with Bounce, keeps a bounce from sending a body
	// straight back the way it came: where the reflected heading is the
	// reverse of the last move, the body turns 45 degrees off it to either
	// side instead (drawn at random if both are among the options of least
	// risk). Without it a heading along an axis reflects into its own
	// reverse and the body walks one row or column back and forth.
	TurnOffReverse bool
	// DrawOffReverse, with Bounce and TurnOffReverse, draws among the
	// options of least risk at random where the bounce would send the body
	// straight back, instead of turning 45 degrees off it.
	DrawOffReverse bool
	// Recheck is the most ticks a body follows the action it last chose
	// before it decides again when nothing has happened (intent.go). Other
	// than that it decides only when something it perceives changes, or
	// its step leaves the land or the region, or its walk to food turns.
	// Zero decides every tick, as stage 1-2r did.
	Recheck int

	// Breed lets adults mate and bear children (breed.go). Off, the world
	// is stage 1-2e.
	Breed bool
	// ChildWorth is what a sure child is worth against a sure death: an
	// option scores its risk minus ChildWorth times its chance of a child,
	// and the body takes the least. Below 1, life comes before descendants.
	ChildWorth float64
	// BirthEnergy is the energy a child is born with; each parent pays
	// half.
	BirthEnergy float64
	// MatureAge is how many ticks after its birth a child becomes an adult
	// and may mate.
	MatureAge int

	// Allot gives every body, at birth, a share of its budget for speed,
	// the rest going to its most energy (build.go); the share is drawn
	// uniformly from AllotLevels evenly spaced values within AllotSpread
	// of one half, and is not inherited. Off, every body has the config's
	// build: stage 1-3.
	Allot bool
	// AllotSpread is how far a body's share may lie from one half.
	AllotSpread float64
	// AllotLevels is how many shares there are to draw from, one half
	// among them when it is odd. Shares come in levels so that bodies share
	// survival tables, which cost about a window times a full body's ticks
	// each to build.
	AllotLevels int
	// AllotCurve bends what a share buys: an ability is the config's value
	// times (2 x share) to this power, so an even split buys the config's
	// values, and below 1 each further share buys less.
	AllotCurve float64
	// AllotInherit has a child take one parent's level of the share, drawn
	// between the two, instead of drawing it afresh (stage 1-5), and
	// AllotMutation is the chance it then moves one level.
	AllotInherit  bool
	AllotMutation float64
	// Learn has bodies read their own memory instead of the world's true
	// rules about the world (learn.go): the food of a region, of the tiles
	// they walked lately, and the chance a mate becomes a child. Off, they
	// read the truth table: the world before stage 1-6.
	Learn bool
	// PriorFood is the chance per tile of food a body is born expecting,
	// and PriorWeight how many tiles of evidence that expectation is worth
	// against the body's own (the land's pull).
	PriorFood, PriorWeight float64
	// RegionWeight is how many tiles pull a region's estimate toward the
	// land's, and PathWeight how many pull the path's toward its region's.
	RegionWeight, PathWeight float64
	// PathRecall is how many ticks a body remembers a tile it left, and
	// PathAhead how many tiles along a heading it reads for them.
	PathRecall, PathAhead int
	// PriorChild is the chance a body is born expecting a mate to make a
	// child, worth ChildWeight asks.
	PriorChild, ChildWeight float64
	// EvidenceHalfLife is how many ticks it takes evidence of the world -
	// a body's own and what it heard - to weigh half as much (learn.go).
	// The world changes; evidence of how it was misleads. Zero keeps all
	// evidence at full weight: the first run of stage 2-1.
	EvidenceHalfLife float64
	// StableRows keeps apart the rows of facts that do not change from those
	// that do (learn.go, tell.go): the path row is read as how much less
	// food tiles walked lately hold than their region - a ratio, which
	// neither ages nor changes with the region - and passes at full weight;
	// the regions' rows, how much food a region holds now, age and are not
	// passed. Off: both kinds pass and age (the second run of 2-1).
	StableRows bool
	// Tell has bodies pass what they know of the world to the bodies they
	// meet (tell.go), keeping at most HeardLimit observers' evidence per
	// row. Off: stage 2-0.
	Tell       bool
	HeardLimit int
	// Kin has evidence pass only from a parent to its own children in its
	// sight, whenever the parent holds some it has not passed to that child
	// (tell.go): a parent has a reason to tell, the worth of a child, where
	// a body telling everyone it meets has none. Off: every two bodies pass
	// to each other once, when they first meet (stage 2-1).
	Kin bool
	// BeliefStep is the ratio between the chances per tile survival tables
	// are built for: an estimate is rounded to a power of it, so that
	// bodies share tables.
	BeliefStep float64
	// Budget is the size of every body's budget. The budget ledger
	// (BudgetLedger) accounts for it.
	Budget float64

	// Collide keeps a body from stepping into a tile another body stands
	// on (breed.go keeps who stands where): the move is not among its
	// options, and a body whose intent would take it there decides again
	// (trigger blocked). Bodies already on one tile - a child and the
	// parent it was born beside - may stay. Off: stage 1-4.
	Collide bool
}

// DefaultConfig returns the rules the measurements run under unless a
// variant rewrites them. The values are provisional; PARAMETERS.md records
// each one and what decided it.
func DefaultConfig() Config {
	return Config{
		Seed:             1,
		FoodCap:          300,
		FoodReturn:       0.002,
		FoodEnergy:       30,
		Bodies:           200,
		EnergyMax:        100,
		EnergyBurn:       0.1,
		Speed:            0.25,
		Window:           1000,
		Sight:            1,
		KeepHeading:      true,
		TurnOffReverse:   true,
		Recheck:          300,
		Breed:            true,
		ChildWorth:       0.5,
		BirthEnergy:      50,
		MatureAge:        1000,
		Allot:            true,
		AllotSpread:      0.25,
		AllotLevels:      11,
		AllotCurve:       0.5,
		AllotInherit:     true,
		AllotMutation:    0.1,
		Budget:           1,
		Learn:            true,
		PriorFood:        0.01,
		PriorWeight:      50,
		RegionWeight:     50,
		PathWeight:       20,
		PathRecall:       200,
		PathAhead:        8,
		PriorChild:       1,
		ChildWeight:      2,
		BeliefStep:       1.1,
		Tell:             true,
		StableRows:       true,
		EvidenceHalfLife: 1200,
		HeardLimit:       64,
		Kin:              true,
		Collide:          true,
	}
}
