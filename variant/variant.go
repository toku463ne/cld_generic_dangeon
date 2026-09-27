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

// Unasked is stage 3-7: no body broadcasts a request to mate.
const Unasked = "unasked"

// Unaged is stage 3-6: abilities do not change with age.
const Unaged = "unaged"

// Slowgrow is stage 3-5b: a child comes of age at 1000 ticks.
const Slowgrow = "slowgrow"

// Short is stage 3-5: bodies die of age at 3000 ticks.
const Short = "short"

// Ageless is stage 3-4: no body dies of age.
const Ageless = "ageless"

// Busy is stage 3-3d: a resting mother burns as any body does.
const Busy = "busy"

// Doubled is stage 3-3c: a mother asks for what the birth costs and what
// she burns while resting, and mothers come to outnumber fathers.
const Doubled = "doubled"

// Lean is stage 3-3b: a mother asks for enough to make up the birth, not
// what she burns while she rests.
const Lean = "lean"

// Perunit is stage 3-3: each unit coming back goes to a resting mother by
// chance, so a mother's share thins as mothers multiply.
const Perunit = "perunit"

// Unfed is stage 3-2: resting mothers get none of the food that comes back.
const Unfed = "unfed"

// Even is stage 3-1: each parent pays half a birth.
const Even = "even"

// Sexless is stage 2-4: any two adults can mate.
const Sexless = "sexless"

// Aged is stage 2-3: the path row passes from parent to child, ageing
// from when each piece was observed.
const Aged = "aged"

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

// Mature750 and Mature400 are the base world with children coming of age
// at 750 and 400 ticks instead of MatureAge: a sweep of how long a body
// cannot mate.
const (
	Mature750 = "mature750"
	Mature400 = "mature400"
)

// Food2 and Food4 are the base world with food coming back twice and four
// times as fast (FoodReturn): a sweep of how rich the map is, the rules
// left as they are.
const (
	Food2 = "food2"
	Food4 = "food4"
)

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
func unasked(c *engine.Config)   { c.Requests = false }
func unaged(c *engine.Config)    { unasked(c); c.ChildAbility = 1 }
func slowgrow(c *engine.Config)  { unaged(c); c.MatureAge = 1000 }
func short(c *engine.Config)     { slowgrow(c); c.Lifespan = 3000 }
func ageless(c *engine.Config)   { short(c); c.Lifespan = 0 }
func busy(c *engine.Config)      { ageless(c); c.RestBurn = 1 }
func doubled(c *engine.Config)   { busy(c); c.ProvisionEachWith, c.ProvisionEachAlone = 0.0075, 0.00375 }
func lean(c *engine.Config)      { busy(c); c.ProvisionEachWith, c.ProvisionEachAlone = 0.00375, 0.001875 }
func perunit(c *engine.Config)   { lean(c); c.ProvisionEach = false }
func unfed(c *engine.Config)     { perunit(c); c.Provision = false }
func even(c *engine.Config)      { unfed(c); c.FemaleBears = false }
func sexless(c *engine.Config)   { even(c); c.Sexes = false }
func aged(c *engine.Config)      { sexless(c); c.PassPath = true }
func kin(c *engine.Config)       { aged(c); c.AgePath = false }
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
	Mature750:    func(c *engine.Config) { c.MatureAge = 750 },
	Mature400:    func(c *engine.Config) { c.MatureAge = 400 },
	Food2:        func(c *engine.Config) { c.FoodReturn *= 2 },
	Food4:        func(c *engine.Config) { c.FoodReturn *= 4 },
	Tight:        func(c *engine.Config) { c.AllotCurve /= 2 },
	Unasked:      unasked,
	Unaged:       unaged,
	Slowgrow:     slowgrow,
	Short:        short,
	Ageless:      ageless,
	Busy:         busy,
	Doubled:      doubled,
	Lean:         lean,
	Perunit:      perunit,
	Unfed:        unfed,
	Even:         even,
	Sexless:      sexless,
	Aged:         aged,
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
