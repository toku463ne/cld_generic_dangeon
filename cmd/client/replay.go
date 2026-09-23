package main

import (
	"fmt"

	"github.com/toku463ne/cld_generic_dangeon/engine"
	"github.com/toku463ne/cld_generic_dangeon/variant"
	"github.com/toku463ne/cld_generic_dangeon/worldmap"
)

// options is what names the world to show and where to start showing it.
type options struct {
	mapName       string
	width, height int
	variant       string
	seed          int64
	fromTick      int
}

// prepare builds the world and runs it headless up to fromTick.
//
// Nothing is loaded from a saved state: the world is deterministic, so
// running it again from tick 0 with the same seed arrives at the state the
// experiment passed through (MEASURE.md "UI での再生").
func prepare(o options) (*engine.World, error) {
	if o.mapName == "" || o.width <= 0 || o.height <= 0 {
		return nil, fmt.Errorf("-map, -w and -h are required")
	}
	if o.fromTick < 0 {
		return nil, fmt.Errorf("-from-tick %d is negative", o.fromTick)
	}
	cfg, err := variant.Config(o.variant, o.seed)
	if err != nil {
		return nil, err
	}
	m, err := worldmap.Build(o.mapName, o.width, o.height)
	if err != nil {
		return nil, err
	}
	w, err := engine.NewWorld(cfg, m)
	if err != nil {
		return nil, err
	}
	for w.Tick() < int64(o.fromTick) {
		w.Step()
	}
	return w, nil
}
