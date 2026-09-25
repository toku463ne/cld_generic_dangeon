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
	// least risk include moving the way it last moved - reflected, where the
	// land ends that way - it moves that way, and it draws at random among
	// them only otherwise. It makes "keep
	// moving enters new tiles" (the table's third row) near true of the
	// body's own walk.
	KeepHeading bool
	// TurnOffReverse, with KeepHeading, keeps a bounce from sending a body
	// straight back the way it came: where the reflected heading is the
	// reverse of the last move, the body turns 45 degrees off it to either
	// side instead (drawn at random if both are among the options of least
	// risk). Without it a heading along an axis reflects into its own
	// reverse and the body walks one row or column back and forth.
	TurnOffReverse bool
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
}

// DefaultConfig returns the rules the measurements run under unless a
// variant rewrites them. The values are provisional; PARAMETERS.md records
// each one and what decided it.
func DefaultConfig() Config {
	return Config{
		Seed:           1,
		FoodCap:        300,
		FoodReturn:     0.002,
		FoodEnergy:     30,
		Bodies:         200,
		EnergyMax:      100,
		EnergyBurn:     0.1,
		Speed:          0.25,
		Window:         1000,
		Sight:          1,
		KeepHeading:    true,
		TurnOffReverse: true,
		Recheck:        300,
		Breed:          true,
		ChildWorth:     0.5,
		BirthEnergy:    50,
		MatureAge:      1000,
	}
}
