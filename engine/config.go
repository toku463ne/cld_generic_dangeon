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
}

// DefaultConfig returns the rules the measurements run under unless a
// variant rewrites them. The values are provisional; PARAMETERS.md records
// each one and what decided it.
func DefaultConfig() Config {
	return Config{
		Seed:       1,
		FoodCap:    300,
		FoodReturn: 0.002,
		FoodEnergy: 30,
		Bodies:     200,
		EnergyMax:  100,
		EnergyBurn: 0.1,
		Speed:      0.25,
	}
}
