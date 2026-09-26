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
var Names = []string{"flat", "constrained", "seasons"}

// Build returns the named map at the given size.
func Build(name string, width, height int) (engine.Map, error) {
	switch name {
	case "flat":
		return Flat(width, height), nil
	case "constrained":
		return Constrained(width, height), nil
	case "seasons":
		return Seasons(width, height), nil
	}
	return engine.Map{}, fmt.Errorf("unknown map %q (known: %v)", name, Names)
}

// Flat is the calibration map: all land, one region.
func Flat(width, height int) engine.Map {
	return engine.NewMap(width, height)
}

// ConstrainedFood is each quadrant's share of the food on the constrained
// map, in region ID order. Provisional; PARAMETERS.md records it.
var ConstrainedFood = []float64{0.5, 0.1, 0.3, 0.1}

// Constrained is the map where food is meant to bind the population. It has
// four regions (the quadrants, IDs 0 to 3 in reading order) that take unequal
// shares of the food (ConstrainedFood), and a two-tile strip of water running
// the full height that cuts the land in two.
//
// The water sits at five eighths of the width, not on the region boundary at
// one half, so that terrain and region cut across each other: the two are
// separate maps, and a layout where they coincide could not show a rule
// confusing one for the other.
func Constrained(width, height int) engine.Map {
	m := engine.NewMap(width, height)
	m.RegionFood = append([]float64(nil), ConstrainedFood...)
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

// SeasonTicks is the length of a season on the seasons map: four of them,
// 4000 ticks, run over three median lives, so that one life cannot see the
// whole round. Provisional; PARAMETERS.md records it.
const SeasonTicks = 1000

// Seasons is the constrained map with its food shares moving: each season
// the shares move on by one region, 0 to 1 to 3 to 2 and round (clockwise
// through the quadrants), so the richest quadrant goes round the map once
// every four seasons. Which region will be rich next is a law of the map,
// not of where bodies are - but one life sees only part of the round.
func Seasons(width, height int) engine.Map {
	m := Constrained(width, height)
	round := []int{0, 1, 3, 2} // the quadrants clockwise
	for s := range round {
		shares := make([]float64, len(ConstrainedFood))
		for i, r := range round {
			// In season s the quadrant i steps on holds what the one s
			// steps back held in season 0.
			shares[r] = ConstrainedFood[round[(i-s+len(round))%len(round)]]
		}
		m.SeasonFood = append(m.SeasonFood, shares)
	}
	m.SeasonTicks = SeasonTicks
	return m
}
