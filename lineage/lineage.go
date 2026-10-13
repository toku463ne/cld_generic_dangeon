// Package lineage keeps the saves of bred bodies (stage 13-2, NODE.md
// "トレーニングと本番"): a training stage is run, and the stocks of its
// living bodies - each one's genome and skills, nothing it learned - are
// written down with how they were bred. The nests of a production map
// send bodies out of such a save (stage 13-3). It lives outside the
// engine because the engine does not know files.
package lineage

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"github.com/toku463ne/cld_generic_dangeon/engine"
)

// File is one save: the stocks of each species by name, and the run they
// were bred in - the map and its size, the variant, the seed, the ticks
// and the commit of the code.
type File struct {
	Map     string
	Width   int
	Height  int
	Variant string
	Seed    int64
	Ticks   int
	Commit  string `json:",omitempty"`
	Species map[string][]engine.Stock
}

// Train runs the world cfg on m for ticks and saves the stocks of every
// species with a living body at the end. A species that began with
// bodies and has none left is an error: its training failed.
func Train(cfg engine.Config, m engine.Map, ticks int) (map[string][]engine.Stock, error) {
	w, err := engine.NewWorld(cfg, m)
	if err != nil {
		return nil, err
	}
	began := make([]bool, w.SpeciesCount())
	for _, b := range w.Bodies() {
		began[b.Species] = true
	}
	for t := 0; t < ticks; t++ {
		w.Step()
	}
	out := map[string][]engine.Stock{}
	for s := 0; s < w.SpeciesCount(); s++ {
		st := w.Stocks(s)
		if len(st) == 0 {
			if began[s] {
				return nil, fmt.Errorf("species %s died out by tick %d", w.SpeciesName(s), ticks)
			}
			continue
		}
		out[w.SpeciesName(s)] = st
	}
	return out, nil
}

// Write writes the save to path as JSON.
func (f File) Write(path string) error {
	b, err := json.MarshalIndent(f, "", " ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}

// Read reads a save written by Write.
func Read(path string) (File, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return File{}, err
	}
	f, err := Parse(b)
	if err != nil {
		return f, fmt.Errorf("%s: %w", path, err)
	}
	return f, nil
}

// Parse reads a save from the bytes Write wrote.
func Parse(b []byte) (File, error) {
	var f File
	err := json.Unmarshal(b, &f)
	return f, err
}

// Stocks returns the stocks of every species of the config that the
// saves hold, each stock's species set by name to the config's index
// (the save's index is the training world's), in the order of the saves
// and of the species' names. A species of a save that the config lacks is
// an error.
func Stocks(cfg engine.Config, saves ...File) ([]engine.Stock, error) {
	var out []engine.Stock
	for _, f := range saves {
		names := make([]string, 0, len(f.Species))
		for name := range f.Species {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			s := -1
			for k, spec := range cfg.Species {
				if spec.Name == name {
					s = k
				}
			}
			if s < 0 {
				return nil, fmt.Errorf("the save of %s (%s) has species %q, which the config does not", f.Map, f.Commit, name)
			}
			for _, st := range f.Species[name] {
				st.Genome.Species = s
				out = append(out, st)
			}
		}
	}
	return out, nil
}

// Stocks returns the stocks of the species named, in the order saved.
func (f File) Stocks(species string) ([]engine.Stock, error) {
	st, ok := f.Species[species]
	if !ok || len(st) == 0 {
		return nil, fmt.Errorf("the save holds no stock of species %q", species)
	}
	return st, nil
}
