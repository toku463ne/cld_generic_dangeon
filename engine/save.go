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
// (see rng.go).

// snapshotVersion changes whenever the format does.
const snapshotVersion = 1

type snapshot struct {
	Version int    `json:"version"`
	Config  Config `json:"config"`
	Map     Map    `json:"map"`
	Tick    int64  `json:"tick"`
	Draws   uint64 `json:"draws"`
}

// Save writes the whole state of the world.
func (w *World) Save(out io.Writer) error {
	enc := json.NewEncoder(out)
	return enc.Encode(snapshot{
		Version: snapshotVersion,
		Config:  w.cfg,
		Map:     w.m,
		Tick:    w.tick,
		Draws:   w.draws.draws,
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
	w := &World{cfg: s.Config, m: s.Map, tick: s.Tick}
	w.rng, w.draws = replayTo(s.Config.Seed, s.Draws)
	return w, nil
}
