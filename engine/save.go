package engine

import (
	"encoding/json"
	"fmt"
	"io"
)

// Saving a world and reading it back.
//
// The format is JSON, readable on purpose. The acceptance condition is the
// determinism one: save a world, load it, run both for the same number of
// ticks, and the two must not differ. That is also what proves nothing was
// left out - a field forgotten here shows up as a divergence there.
//
// The random source is saved as its seed and the number of draws taken from it
// (see rng.go). Anything that can be rebuilt from what is saved (which tile
// holds which food, the land of each region) is rebuilt rather than saved.

// snapshotVersion changes whenever the format does.
const snapshotVersion = 7

type snapshot struct {
	Version int    `json:"version"`
	Config  Config `json:"config"`
	Map     Map    `json:"map"`
	Tick    int64  `json:"tick"`
	Draws   uint64 `json:"draws"`

	Foods    []Food    `json:"foods"`
	FoodOwed []float64 `json:"foodOwed"`
	Appeared int64     `json:"appeared"`
	Eaten    int64     `json:"eaten"`

	Bodies []Body `json:"bodies"`
	NextID int64  `json:"nextID"`
	Stats  Stats  `json:"stats"`
}

// Save writes the whole state of the world.
func (w *World) Save(out io.Writer) error {
	return json.NewEncoder(out).Encode(snapshot{
		Version:  snapshotVersion,
		Config:   w.cfg,
		Map:      w.m,
		Tick:     w.tick,
		Draws:    w.draws.draws,
		Foods:    w.food.foods,
		FoodOwed: w.food.owed,
		Appeared: w.food.appeared,
		Eaten:    w.food.eaten,
		Bodies:   w.bodies,
		NextID:   w.nextID,
		Stats:    w.stats,
	})
}

// Load reads a world written by Save.
func Load(in io.Reader) (*World, error) {
	var s snapshot
	if err := json.NewDecoder(in).Decode(&s); err != nil {
		return nil, err
	}
	if s.Version != snapshotVersion {
		return nil, fmt.Errorf("snapshot version %d, want %d", s.Version, snapshotVersion)
	}
	if err := s.Map.Validate(); err != nil {
		return nil, err
	}
	w := &World{cfg: s.Config, m: s.Map, tick: s.Tick, bodies: s.Bodies, nextID: s.NextID, stats: s.Stats}
	w.rng, w.draws = replayTo(s.Config.Seed, s.Draws)
	w.initFood()
	if len(s.FoodOwed) != len(w.food.owed) {
		return nil, fmt.Errorf("snapshot owes food to %d regions, map has %d", len(s.FoodOwed), len(w.food.owed))
	}
	copy(w.food.owed, s.FoodOwed)
	w.food.appeared, w.food.eaten = s.Appeared, s.Eaten
	for _, f := range s.Foods {
		if !w.m.InBounds(f.X, f.Y) {
			return nil, fmt.Errorf("food at (%d,%d) is off the map", f.X, f.Y)
		}
		w.food.foods = append(w.food.foods, f)
		w.food.foodAt[w.m.index(f.X, f.Y)] = int32(len(w.food.foods))
		w.food.onGround[w.m.RegionAt(f.X, f.Y)]++
	}
	w.initPredict()
	w.buildGrid()
	return w, nil
}
