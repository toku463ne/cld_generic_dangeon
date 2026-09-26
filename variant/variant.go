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

// Kin is stage 2-2: evidence passes from parent to child, and the path
// row never ages.
const Kin = "kin"

// Near is stage 2-1 as closed: every two bodies pass evidence to each
// other once, when they first meet.
const Near = "near"

// Mixed is the second run of stage 2-1: every row of the world passes and
// ages, the regions' and the path's alike.
const Mixed = "mixed"

// Pathless is the base world with the path row read nowhere: evidence
// passes and ages, but keeping on the move is read by the region alone.
const Pathless = "pathless"

// Undecayed is the first run of stage 2-1: evidence passes between bodies
// and keeps its full weight however old.
const Undecayed = "undecayed"

// Alone is the base world with nothing passed: evidence ages, but each body
// has only its own.
const Alone = "alone"

// Untold is stage 2-0: bodies learn but pass nothing on.
const Untold = "untold"

// Truth is the world before stage 1-6: bodies read the truth table.
const Truth = "truth"

// Bounced is stage 1-5: a body whose heading is blocked or worse than the
// best bounces off it, turning 45 degrees where that would be straight
// back.
const Bounced = "bounce"

// DrawReverse is stage 1-5 where a bounce that would send a body straight
// back draws among the best at random instead of turning 45 degrees off.
const DrawReverse = "drawreverse"

// Tight is the base world with the top of the budget pinched harder: the
// diminishing return of AllotCurve halved, 0.5 to 0.25 (stage 1-5 asks
// whether speed can be held down for free).
const Tight = "tight"

// Drawn is the world before stage 1-5: every body draws its share of the
// budget afresh (with collisions).
const Drawn = "drawn"

// Overlap is stage 1-4: bodies walk through each other.
const Overlap = "overlap"

// Fixed is stage 1-3: every body has the config's build.
const Fixed = "fixed"

// Nobreed is stage 1-2e: no one mates.
const Nobreed = "nobreed"

// Everytick is stage 1-2r: every body decides every tick.
const Everytick = "everytick"

// Random is the stage 1-1 control: no window and no heading, so every
// possible action is equally likely.
const Random = "random"

// Each stage's variant is the next stage's with one more rule taken out,
// so a rule added later is off in every earlier stage by construction.
func kin(c *engine.Config)       { c.AgePath = false }
func near(c *engine.Config)      { kin(c); c.Kin = false }
func mixed(c *engine.Config)     { near(c); c.StableRows = false }
func undecayed(c *engine.Config) { mixed(c); c.EvidenceHalfLife = 0 }
func untold(c *engine.Config)    { undecayed(c); c.Tell = false }
func truth(c *engine.Config)     { untold(c); c.Learn = false }
func bounced(c *engine.Config)   { truth(c); c.Bounce = true }
func drawn(c *engine.Config)     { bounced(c); c.AllotInherit = false }
func overlap(c *engine.Config)   { drawn(c); c.Collide = false }
func fixed(c *engine.Config)     { overlap(c); c.Allot = false }
func nobreed(c *engine.Config)   { fixed(c); c.Breed = false }
func everytick(c *engine.Config) { nobreed(c); c.Recheck = 0 }

// rewrites maps a variant name to the rewrite of the default config it
// stands for.
var rewrites = map[string]func(*engine.Config){
	Base:         func(*engine.Config) {},
	Tight:        func(c *engine.Config) { c.AllotCurve /= 2 },
	Kin:          kin,
	Near:         near,
	Mixed:        mixed,
	Pathless:     func(c *engine.Config) { mixed(c); c.PathAhead = 0 },
	Undecayed:    undecayed,
	Alone:        func(c *engine.Config) { mixed(c); c.Tell = false },
	Untold:       untold,
	Truth:        truth,
	Bounced:      bounced,
	DrawReverse:  func(c *engine.Config) { bounced(c); c.DrawOffReverse = true },
	Drawn:        drawn,
	Overlap:      overlap,
	Fixed:        fixed,
	Nobreed:      nobreed,
	Everytick:    everytick,
	Straightback: func(c *engine.Config) { everytick(c); c.TurnOffReverse = false },
	Restless:     func(c *engine.Config) { everytick(c); c.KeepHeading = false },
	Blind:        func(c *engine.Config) { everytick(c); c.KeepHeading, c.Sight = false, -1 },
	Random:       func(c *engine.Config) { everytick(c); c.KeepHeading, c.Window = false, 0 },
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
