package engine

import (
	"encoding/binary"
	"hash/fnv"
	"math/rand"
)

// World is the whole simulation. It knows nothing about drawing, networking,
// map files, or who is playing.
//
// Stage 1-0 is the container only: a clock, a map and the random source. No
// rule acts on it yet, so a tick changes nothing but the clock.
type World struct {
	cfg Config
	m   Map

	// rng is the single source of randomness of the simulation. Everything
	// random draws from it, so a given seed always gives the same run.
	rng   *rand.Rand
	draws *countingSource

	tick int64
}

// NewWorld builds a world on the given map. The map is copied, so the caller
// may reuse it.
func NewWorld(cfg Config, m Map) (*World, error) {
	if err := m.Validate(); err != nil {
		return nil, err
	}
	w := &World{cfg: cfg, m: m.Clone()}
	w.rng, w.draws = newCountingRand(cfg.Seed)
	return w, nil
}

// Step advances the world by one tick.
func (w *World) Step() {
	w.tick++
}

// Tick returns how many ticks the world has run.
func (w *World) Tick() int64 { return w.tick }

// Draws returns how many numbers have been taken from the random source.
func (w *World) Draws() uint64 { return w.draws.draws }

// Config returns the rules the world runs under.
func (w *World) Config() Config { return w.cfg }

// Map returns a copy of the world's map.
func (w *World) Map() Map { return w.m.Clone() }

// Fingerprint hashes everything the world's state consists of. Two worlds
// with the same fingerprint are, as far as any rule can tell, the same world;
// the behaviour fingerprint test pins its value for a fixed seed and run
// length, and it changes only when a rule is changed on purpose.
func (w *World) Fingerprint() uint64 {
	h := fnv.New64a()
	var buf [8]byte
	put := func(v uint64) {
		binary.LittleEndian.PutUint64(buf[:], v)
		h.Write(buf[:])
	}
	put(uint64(w.cfg.Seed))
	put(uint64(w.tick))
	put(w.draws.draws)
	put(uint64(w.m.Width))
	put(uint64(w.m.Height))
	for _, t := range w.m.Terrain {
		h.Write([]byte{byte(t)})
	}
	for _, r := range w.m.Region {
		binary.LittleEndian.PutUint16(buf[:2], uint16(r))
		h.Write(buf[:2])
	}
	return h.Sum64()
}
