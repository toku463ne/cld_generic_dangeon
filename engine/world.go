package engine

import (
	"encoding/binary"
	"hash/fnv"
	"math"
	"math/rand"
)

// World is the whole simulation. It knows nothing about drawing, networking,
// map files, or who is playing.
type World struct {
	cfg Config
	m   Map

	// rng is the single source of randomness of the simulation. Everything
	// random draws from it, so a given seed always gives the same run.
	rng   *rand.Rand
	draws *countingSource

	tick int64

	food   foodState
	bodies []Body
	nextID int64
	stats  Stats

	// grid lists the bodies on each tile (breed.go), and born the children
	// born this tick, who join the world at its end.
	grid [][]int32
	born []Body

	// pred is what valuing options needs and can be rebuilt from the rest,
	// so it is neither saved nor fingerprinted.
	pred predictor
	// valuation is scratch space for the decision of the body acting now.
	valuation Valuation
	ties      []int

	// trace, when set, is shown every decision as it is made.
	trace func(Body, Valuation, Action)
}

// NewWorld builds a world on the given map: the food cap laid out by region
// share, then the bodies. The map is copied, so the caller may reuse it.
func NewWorld(cfg Config, m Map) (*World, error) {
	if err := m.Validate(); err != nil {
		return nil, err
	}
	w := &World{cfg: cfg, m: m.Clone()}
	w.rng, w.draws = newCountingRand(cfg.Seed)
	w.initFood()
	w.initPredict()
	w.fillFood()
	w.placeBodies()
	w.buildGrid()
	return w, nil
}

// Step advances the world by one tick: vacant food comes back, then every
// body in turn follows its intent or decides (intent.go), acts and spends
// its energy, then the dead are removed and the children born this tick
// join.
func (w *World) Step() {
	w.tick++
	w.returnFood()
	for i := range w.bodies {
		b := &w.bodies[i]
		if b.Mature == w.tick && b.Mature > 0 {
			w.stats.Matured++
		}
		w.act(i, b, w.turn(b))
		burn := w.burnOf(b)
		b.Energy -= burn
		w.stats.EnergyBurned += burn
	}
	w.removeDead()
	w.bodies = append(w.bodies, w.born...)
	w.born = w.born[:0]
	w.buildGrid()
}

// removeDead drops the bodies that have run out of energy, keeping the order
// of the rest.
func (w *World) removeDead() {
	alive := w.bodies[:0]
	for _, b := range w.bodies {
		if b.Energy <= 0 {
			w.stats.Deaths[CauseStarved]++
			if w.tick < b.Mature {
				w.stats.DiedYoung++
			}
			continue
		}
		alive = append(alive, b)
	}
	w.bodies = alive
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
	putF := func(v float64) { put(math.Float64bits(v)) }
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
	for _, f := range w.food.foods {
		put(uint64(f.X))
		put(uint64(f.Y))
	}
	for _, o := range w.food.owed {
		putF(o)
	}
	put(uint64(w.food.appeared))
	put(uint64(w.food.eaten))
	for _, b := range w.bodies {
		put(uint64(b.ID))
		putF(b.X)
		putF(b.Y)
		putF(b.Energy)
		put(uint64(b.Born))
		// A build is state only where it is not the config's.
		if b.Build != (Build{}) {
			putF(b.Build.Speed)
			putF(b.Build.EnergyMax)
			putF(b.Build.EnergyBurn)
		}
		// The heading is state only where a rule reads it.
		if w.cfg.KeepHeading {
			put(uint64(int64(b.Heading)))
		}
		// So is the intent, where bodies follow one between decisions.
		if w.cfg.Recheck > 0 {
			put(uint64(b.Intent.Kind))
			put(uint64(b.Intent.Dir))
			put(uint64(int64(b.Goal)))
			put(uint64(b.Decided))
			if b.Under {
				put(1)
			} else {
				put(0)
			}
			put(b.Saw)
		}
		// And the age of coming of age, where bodies breed.
		if w.cfg.Breed {
			put(uint64(b.Mature))
			put(b.Mates)
			put(uint64(b.Parents[0]))
			put(uint64(b.Parents[1]))
		}
	}
	put(uint64(w.nextID))
	for _, d := range w.stats.Deaths {
		put(uint64(d))
	}
	if w.cfg.Breed {
		put(uint64(w.stats.Births))
	}
	return h.Sum64()
}
