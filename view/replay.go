package view

import (
	"fmt"
	"strconv"

	"github.com/toku463ne/cld_generic_dangeon/engine"
	"github.com/toku463ne/cld_generic_dangeon/variant"
	"github.com/toku463ne/cld_generic_dangeon/worldmap"
)

// Options is what names the world to show and where to start showing it.
type Options struct {
	Map           string
	Width, Height int
	Variant       string
	Seed          int64
	FromTick      int
	// Scale is the pixels per tile; zero is 12.
	Scale int
	// Follow is a body ID, "hungriest" for the body with the least energy
	// at FromTick, or empty for none.
	Follow string
}

// Prepare builds the world and its view and runs it headless up to
// FromTick, recording the history panel on the way.
//
// Nothing is loaded from a saved state: the world is deterministic, so
// running it again from tick 0 with the same seed arrives at the state the
// experiment passed through (MEASURE.md "UI での再生").
func Prepare(o Options) (*View, error) {
	if o.Map == "" || o.Width <= 0 || o.Height <= 0 {
		return nil, fmt.Errorf("-map, -w and -h are required")
	}
	if o.FromTick < 0 {
		return nil, fmt.Errorf("-from-tick %d is negative", o.FromTick)
	}
	if o.Scale <= 0 {
		o.Scale = 12
	}
	cfg, err := variant.Config(o.Variant, o.Seed)
	if err != nil {
		return nil, err
	}
	m, err := worldmap.Build(o.Map, o.Width, o.Height)
	if err != nil {
		return nil, err
	}
	w, err := engine.NewWorld(cfg, m)
	if err != nil {
		return nil, err
	}
	v := New(w, o.Scale)
	for w.Tick() < int64(o.FromTick) {
		v.Step(w.Tick() >= int64(o.FromTick-trailLen))
	}
	switch o.Follow {
	case "":
	case "hungriest":
		v.FollowHungriest()
	default:
		id, err := strconv.ParseInt(o.Follow, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("-follow %q is neither a body ID nor \"hungriest\"", o.Follow)
		}
		v.followID(id)
	}
	return v, nil
}
