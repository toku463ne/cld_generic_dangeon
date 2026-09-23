package engine

import "math/rand"

// The world has one random source, and everything that is random draws from
// it, so the same seed always gives the same run.
//
// The state of math/rand's source cannot be written out, so a snapshot stores
// the seed and the number of draws taken from it, and loading replays that
// many. Int63 and Uint64 advance the source identically, so the count is all
// that matters.

// countingSource wraps the world's generator to count what has been taken from
// it. It forwards Source64 so that the stream is exactly the underlying one.
type countingSource struct {
	src   rand.Source64
	draws uint64
}

func (c *countingSource) Int63() int64 {
	c.draws++
	return c.src.Int63()
}

func (c *countingSource) Uint64() uint64 {
	c.draws++
	return c.src.Uint64()
}

func (c *countingSource) Seed(seed int64) {
	c.src.Seed(seed)
	c.draws = 0
}

// newCountingRand is how a world gets its generator.
func newCountingRand(seed int64) (*rand.Rand, *countingSource) {
	c := &countingSource{src: rand.NewSource(seed).(rand.Source64)}
	return rand.New(c), c
}

// replayTo winds a fresh source forward to where a saved one had got to.
func replayTo(seed int64, draws uint64) (*rand.Rand, *countingSource) {
	r, c := newCountingRand(seed)
	for i := uint64(0); i < draws; i++ {
		c.src.Uint64()
	}
	c.draws = draws
	return r, c
}
