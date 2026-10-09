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
// holds which food, the land of each region, the survival tables) is rebuilt
// rather than saved; of the survival tables, only which ones were in use is
// saved, so that a loaded world can build them before play (World.Warm).

// snapshotVersion changes whenever the format does.
const snapshotVersion = 53

type snapshot struct {
	Version int    `json:"version"`
	Config  Config `json:"config"`
	Map     Map    `json:"map"`
	Tick    int64  `json:"tick"`
	Draws   uint64 `json:"draws"`

	Foods    []Food  `json:"foods"`
	FoodBorn []int64 `json:"foodBorn"`
	// FoodOwner is each unit's owner (stage 6-4c), noOwner for none.
	FoodOwner []int64 `json:"foodOwner,omitempty"`
	// Pools is each pool's account (stage 9-7b-1): what it is owed per
	// region and what has happened to its units since the world was
	// built. What bodies hold now is in Bodies.
	Pools []poolSnapshot `json:"pools"`

	Bodies []Body `json:"bodies"`
	NextID int64  `json:"nextID"`
	// Places are the built places standing, NextPlace the last one's ID
	// (stage 8-1); their tiles are in the map's places to rest.
	Places    []Place `json:"places,omitempty"`
	NextPlace int64   `json:"nextPlace,omitempty"`
	// GroundTents are the tents lying on the ground (stage 8-2a3).
	GroundTents []GroundTent `json:"groundTents,omitempty"`
	Stats       Stats        `json:"stats"`

	// Tables are the survival tables the world was reading, for a loaded
	// world to build ahead (World.Warm). They are the only part of the
	// cache saved: the tables themselves can be rebuilt from the rest.
	Tables []TableKey `json:"tables,omitempty"`
}

// poolSnapshot is one pool's saved account (food.go).
type poolSnapshot struct {
	Owed        []float64 `json:"owed"`
	Appeared    int64     `json:"appeared"`
	Eaten       int64     `json:"eaten"`
	Provisioned int64     `json:"provisioned,omitempty"`
	Picked      int64     `json:"picked,omitempty"`
	Lost        int64     `json:"lost,omitempty"`
	Decayed     int64     `json:"decayed,omitempty"`
}

// Save writes the whole state of the world.
func (w *World) Save(out io.Writer) error {
	var pools []poolSnapshot
	for _, p := range w.food.pools {
		pools = append(pools, poolSnapshot{Owed: p.owed, Appeared: p.appeared, Eaten: p.eaten,
			Provisioned: p.provisioned, Picked: p.picked, Lost: p.lost, Decayed: p.decayed})
	}
	return json.NewEncoder(out).Encode(snapshot{
		Version:     snapshotVersion,
		Config:      w.cfg,
		Map:         w.m,
		Tick:        w.tick,
		Draws:       w.draws.draws,
		Foods:       w.food.foods,
		FoodBorn:    w.food.born,
		FoodOwner:   w.food.owner,
		Pools:       pools,
		Bodies:      w.bodies,
		NextID:      w.nextID,
		Places:      w.places,
		NextPlace:   w.nextPlace,
		GroundTents: w.groundTents,
		Stats:       w.stats,
		Tables:      w.Tables(),
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
	if err := validFood(s.Config.FoodKinds, s.Config.Foods); err != nil {
		return nil, err
	}
	w.initFood()
	if len(s.Pools) != len(w.food.pools) {
		return nil, fmt.Errorf("snapshot has %d food pools, config has %d", len(s.Pools), len(w.food.pools))
	}
	for i, ps := range s.Pools {
		p := &w.food.pools[i]
		if len(ps.Owed) != len(p.owed) {
			return nil, fmt.Errorf("snapshot owes food to %d regions, map has %d", len(ps.Owed), len(p.owed))
		}
		copy(p.owed, ps.Owed)
		p.appeared, p.eaten, p.provisioned = ps.Appeared, ps.Eaten, ps.Provisioned
		p.picked, p.lost, p.decayed = ps.Picked, ps.Lost, ps.Decayed
	}
	if len(s.FoodBorn) != len(s.Foods) {
		return nil, fmt.Errorf("snapshot has %d food ages for %d units", len(s.FoodBorn), len(s.Foods))
	}
	w.food.born = append(w.food.born, s.FoodBorn...)
	if len(s.FoodOwner) != len(s.Foods) {
		return nil, fmt.Errorf("snapshot has %d food owners for %d units", len(s.FoodOwner), len(s.Foods))
	}
	w.food.owner = append(w.food.owner, s.FoodOwner...)
	for _, b := range w.bodies {
		if len(b.HeldPool) != b.Held {
			return nil, fmt.Errorf("body %d holds %d units of %d pools", b.ID, b.Held, len(b.HeldPool))
		}
		for _, p := range b.HeldPool {
			if p < 0 || p >= len(w.food.pools) {
				return nil, fmt.Errorf("body %d holds food of pool %d, have %d", b.ID, p, len(w.food.pools))
			}
			w.food.pools[p].held++
		}
	}
	for _, f := range s.Foods {
		if !w.m.InBounds(f.X, f.Y) {
			return nil, fmt.Errorf("food at (%d,%d) is off the map", f.X, f.Y)
		}
		if f.ID < 0 || f.ID >= len(w.food.pools) {
			return nil, fmt.Errorf("food at (%d,%d) is of pool %d, have %d", f.X, f.Y, f.ID, len(w.food.pools))
		}
		w.food.foods = append(w.food.foods, f)
		w.food.foodAt[w.m.index(f.X, f.Y)] = int32(len(w.food.foods))
		p := &w.food.pools[f.ID]
		p.onGround[w.m.RegionAt(f.X, f.Y)]++
		p.count++
	}
	w.places, w.nextPlace = s.Places, s.NextPlace
	w.groundTents = s.GroundTents
	w.loadPlaces()
	w.initPredict()
	w.pred.pending = s.Tables
	w.buildGrid()
	return w, nil
}
