// Package variant names the rewrites of engine.Config that experiments
// compare. It is shared by cmd/experiment, which runs them, and cmd/client,
// which replays one, so that a replay command printed by an experiment builds
// the same world the experiment measured.
package variant

import (
	"fmt"
	"sort"
	"strings"

	"github.com/toku463ne/cld_generic_dangeon/engine"
)

// Base is the name of the variant that leaves the default config as it is.
const Base = "base"

// Blind is stage 1-2 as first run: valuation with no sight and no heading,
// so the body knows only the region rows.
const Blind = "blind"

// Restless is stage 1-2 with sight and no heading kept: ties are drawn at
// random, as in stage 1-2p.
const Restless = "restless"

// Straightback is stage 1-2q: the heading is kept and bounced, and a bounce
// may send a body straight back the way it came.
const Straightback = "straightback"

// Everytick is stage 1-2r: every body decides every tick.
const Everytick = "everytick"

// Random is the stage 1-1 control: no window and no heading, so every
// possible action is equally likely.
const Random = "random"

// rewrites maps a variant name to the rewrite of the default config it
// stands for.
var rewrites = map[string]func(*engine.Config){
	Base:         func(*engine.Config) {},
	Everytick:    func(c *engine.Config) { c.Recheck = 0 },
	Random:       func(c *engine.Config) { c.Window, c.KeepHeading, c.Recheck = 0, false, 0 },
	Blind:        func(c *engine.Config) { c.Sight, c.KeepHeading, c.Recheck = -1, false, 0 },
	Restless:     func(c *engine.Config) { c.KeepHeading, c.Recheck = false, 0 },
	Straightback: func(c *engine.Config) { c.TurnOffReverse, c.Recheck = false, 0 },
}

// Config returns the default config rewritten by the named variant, with the
// given seed.
func Config(name string, seed int64) (engine.Config, error) {
	f, ok := rewrites[name]
	if !ok {
		return engine.Config{}, fmt.Errorf("unknown variant %q (known: %s)", name, strings.Join(Names(), ", "))
	}
	cfg := engine.DefaultConfig()
	f(&cfg)
	cfg.Seed = seed
	return cfg, nil
}

// Names lists the known variants in order.
func Names() []string {
	names := make([]string, 0, len(rewrites))
	for n := range rewrites {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}
