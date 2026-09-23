package engine

// Config holds the rules of the world. Every simulation constant lives here as
// a field rather than a package const, so that a test can switch one rule off
// and an experiment can put two variants into the same binary.
//
// Stage 1-0 has no rules yet, so the only field is the seed. Anything that
// only changes how a run is measured (seed counts, run lengths, the collapse
// floor) does not belong here: it is not a rule of the world.
type Config struct {
	// Seed is the seed of the world's single random source.
	Seed int64
}
