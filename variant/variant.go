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

// Random is the stage 1-1 control: no window, so every possible action is
// equally likely.
const Random = "random"

// rewrites maps a variant name to the rewrite of the default config it
// stands for.
var rewrites = map[string]func(*engine.Config){
	Base:   func(*engine.Config) {},
	Random: func(c *engine.Config) { c.Window = 0 },
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
