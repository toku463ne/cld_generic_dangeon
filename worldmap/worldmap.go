// Package worldmap builds the maps that stages 1 and 2 measure on. It lives
// outside the engine because the engine does not know where maps come from.
//
// The maps are laid out by rule, not drawn at random: the world owns the only
// random source, and a map generator with its own would be a second one.
package worldmap

import (
	"fmt"

	"github.com/toku463ne/cld_generic_dangeon/engine"
)

// Names lists the maps Build knows.
var Names = []string{"flat", "constrained"}

// Build returns the named map at the given size.
func Build(name string, width, height int) (engine.Map, error) {
	switch name {
	case "flat":
		return Flat(width, height), nil
	case "constrained":
		return Constrained(width, height), nil
	}
	return engine.Map{}, fmt.Errorf("unknown map %q (known: %v)", name, Names)
}

// Flat is the calibration map: all land, one region.
func Flat(width, height int) engine.Map {
	return engine.NewMap(width, height)
}

// Constrained is the map where food is meant to bind the population. It has
// four regions (the quadrants, IDs 0 to 3 in reading order), which stage 1-1
// gives different food rates, and a two-tile strip of water running the full
// height that cuts the land in two.
//
// The water sits at five eighths of the width, not on the region boundary at
// one half, so that terrain and region cut across each other: the two are
// separate maps, and a layout where they coincide could not show a rule
// confusing one for the other.
func Constrained(width, height int) engine.Map {
	m := engine.NewMap(width, height)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			r := 0
			if x >= width/2 {
				r++
			}
			if y >= height/2 {
				r += 2
			}
			m.SetRegion(x, y, engine.RegionID(r))
		}
	}
	water := width * 5 / 8
	for y := 0; y < height; y++ {
		for x := water; x < water+2 && x < width; x++ {
			m.SetTerrain(x, y, engine.TerrainWater)
		}
	}
	return m
}
