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

// Sight0 and Sight2 are the sight sweep before stage 9-3 (sight as a
// budget slot): every body sees only its own tile, or two tiles out.
const (
	Sight0 = "sight0"
	Sight2 = "sight2"
)

// Single is stage 9-6a: one species, the config's table cut to its first
// row (no beasts, and no draw of a species), and one kind of food in one
// pool.
const Single = "single"

// Shared is stage 9-7a: two species, one kind of food in one pool that
// both eat.
const Shared = "shared"

// Twin is stage 9-7a with the beast no different from a human but its
// tag: the control that shows what the budget's shift does.
const Twin = "twin"

// Open is stage 9-7b-2a: a settlement admits bodies of every species.
const Open = "open"

// Fish is stage 9-7b-2c: a third species that lives in the water and
// eats a pool of food that comes back there, each species' pool 300
// (the cap three times the default, as Roomy doubles it for two). Wade
// is Fish with water costing the land species nothing extra, and Dry is
// Fish with water a wall to them, as before the stage.
const (
	Fish = "fish"
	Wade = "wade"
	Dry  = "dry"
)

// Planted is stage 9-7b-3: on Roomy's cap, a third of the food is a plant
// both land species eat, whose units each drop a seed with chance
// PlantSeedRate a tick that grows beside them, starting from a few units;
// Planted4 and Planted10 seed four and ten times as often;
// Strewn has the same plant come back by the map's shares instead (no
// spreading).
const (
	Planted   = "planted"
	Planted4  = "planted4"
	Planted10 = "planted10"
	Strewn    = "strewn"
)

// PlantSeedRate is the seeding chance per unit per tick of Planted. (At
// 0.002, 0.005 and 0.01 the plant never left its seedbed: a colony of
// twenty seeds a unit every 5 to 25 ticks and is grazed faster.)
const PlantSeedRate = 0.005

// Skilled is stage 9-7b-3: every species is born with skills as humans
// are; Shallow is stage 9-7b-3 too: bodies block each other in water as
// on land. Both are the controls of stage 9-7b-4 on Roomy's cap.
const (
	Skilled = "skilled"
	Shallow = "shallow"
)

// Roomy gives the two species, each in its own regions, the food one
// species had alone: twice the cap, so each pool is 300 (stage 9-7b-2;
// on 150 each, the world settles under the collapse floor however it is
// cut). Roomytogether lays the pools over each other on that cap (stage
// 9-7b-1's ample), and Roomyopen opens the settlements on it.
const (
	Roomy         = "roomy"
	Roomytogether = "roomytogether"
	Roomyopen     = "roomyopen"
)

// Together is stage 9-7b-1: every pool of food comes back by the map's
// region shares, so the species' pools lie over each other instead of
// each in its own regions.
const Together = "together"

// Halffed is the count before stage 9-7b-1: one species (Single) on half
// the food cap, from half the first bodies, which is what one of two
// species has once the food is split into a pool per species. (Half the
// cap under all the first bodies is an overshoot that collapses every
// seed on flat and constrained alike, and measures nothing else.)
const Halffed = "halffed"

// Steady and Fickle are the sweep before stage 9-6b (the conservative
// trait): every body's own evidence of a place outweighs the prior one
// and a half times sooner, or later (the weights toward the parents
// divided by 1.5, or multiplied).
const (
	Steady = "steady"
	Fickle = "fickle"
)

// Unventured is stage 9-5b: the exploratory trait is not in the genome,
// and every body expects of a region it has little evidence of what the
// land has shown it.
const Unventured = "unventured"

// Bold and Timid are the sweep before stage 9-6a, on the unventured world:
// every body expects a region it has little evidence of to hold one and a
// half times what the land has shown it, or two thirds of it.
const (
	Bold  = "bold"
	Timid = "timid"
)

// Exact builds a survival table for every exact most (FullStep 0): the
// worlds before the tables' cap went on a grid (2026-10-07), as stage
// 9-5b was measured.
const Exact = "exact"

// Unsized is stage 9-5a: the size of the budget is not in the genome, so
// every body has the config's budget and burn.
const Unsized = "unsized"

// Unpriced is stage 9-5b without its price: the size of the budget is
// inherited and a bigger body is simply more body, burning no more than
// a small one. The control that shows what the price holds back.
const Unpriced = "unpriced"

// Unmixed is stage 9-4: a child takes every slot of its budget from the
// one parent drawn.
const Unmixed = "unmixed"

// Unfertile is stage 9-3b: fertility is not in the budget, and every
// mating agreed on makes a child.
const Unfertile = "unfertile"

// Evenfert is stage 9-4 with no selection: every body's fertility is the
// neutral chance (FertStep 0) and costs nothing (FertTrade 0), so the
// dice and the mating paid for are in, and nothing to select on.
const Evenfert = "evenfert"

// Unbedded is stage 9-3b less its second rule: a builder weighs building
// against sleeping as it stands only, not against a known bed.
const Unbedded = "unbedded"

// Hopeful is stage 9-3: a builder's expected returns are never corrected
// in time (learned only at a wear-out no builder lives to see), and the
// comparison ignores known beds.
const Hopeful = "hopeful"

// Unsighted is stage 8-2c: sight is not in the budget, every body seeing
// the config's Sight.
const Unsighted = "unsighted"

// Unkept is stage 8-2b3: food on a place to rest lasts no longer than
// anywhere.
const Unkept = "unkept"

// Unsettled is stage 8-2a3: no body builds a settlement.
const Unsettled = "unsettled"

// Buried is stage 8-2a2: a tent is lost with its owner.
const Buried = "buried"

// Atdusk is stage 8-2a: a night's saving in a tent is read at the burn of
// falling asleep.
const Atdusk = "atdusk"

// Unbuilt is stage 6-5a: no tents and no building.
const Unbuilt = "unbuilt"

// Untaught is stage 7-3e: parents do not pass where places to rest are.
const Untaught = "untaught"

// Sleepblind is stage 7-3d: the valuation does not foresee a forced sleep.
const Sleepblind = "sleepblind"

// Unforced is stage 6-1b: sleep is never forced.
const Unforced = "unforced"

// Shelterblind is stage 6-1a: bodies do not know places to rest.
const Shelterblind = "shelterblind"

// Shelterless is stage 7-2b: no place to rest is warm.
const Shelterless = "shelterless"

// Duskblind is stage 7-4: bodies do not learn when dark and light come.
const Duskblind = "duskblind"

// Templess is stage 7-3b: the temperature does not touch the burn.
const Templess = "templess"

// Sleepless is stage 7-3a: no sleep.
const Sleepless = "sleepless"

// Burnblind is stage 7-2a: the valuation reads the burn as it is now, not
// how it is rising.
const Burnblind = "burnblind"

// Sightblind is stage 7-1b: the valuation does not read sight when keeping
// on the move.
const Sightblind = "sightblind"

// Daylong is stage 4-4: no night.
const Daylong = "daylong"

// Unspoiled is stage 5-2 (and the lock count): food never decays.
const Unspoiled = "unspoiled"

// Unrecalled is stage 5-1: a body knows only the food in sight.
const Unrecalled = "unrecalled"

// Halflife is stage 4-1: evidence ages by a continuous half-life, and the
// mate row does not age.
const Halflife = "halflife"

// Unheld is stage 4-0: no body holds food.
const Unheld = "unheld"

// Restblind is stage 3-8: a resting mother reads her whole window at her
// resting burn.
const Restblind = "restblind"

// Oldmating is stage M-3: the old mate on.
const Oldmating = "oldmating"

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

// Unprovided is the base world with no food going straight to resting
// mothers (Provision off): stage 4-3 asks whether mothers who carry food
// manage without it.
const Unprovided = "unprovided"

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
func shared(c *engine.Config) { c.FoodKinds, c.Foods = nil, nil }

// planted adds a plant: a kind both land species eat, a pool of a third
// of twice the cap that spreads from twenty units.
func planted(c *engine.Config) {
	c.FoodKinds = append(append([]engine.FoodKind(nil), c.FoodKinds...), engine.FoodKind{Name: "plants", Tags: []string{"human", "beast"}})
	c.Foods = append([]engine.FoodSpec(nil), c.Foods...)
	for i := range c.Foods {
		c.Foods[i].Share = 1.0 / 3
	}
	c.Foods = append(c.Foods, engine.FoodSpec{Name: "plants", Kind: len(c.FoodKinds) - 1, Share: 1.0 / 3, Seed: 1, SeedRate: PlantSeedRate, Start: 20})
	c.FoodCap *= 2
}

// fish adds the fish to the species table, with their kind and pool of
// food in the water, every species' pool 300.
func fish(c *engine.Config) {
	c.Species = append([]engine.SpeciesSpec(nil), c.Species...)
	for i := range c.Species {
		c.Species[i].Share = 1.0 / 3
	}
	c.Species = append(c.Species, engine.SpeciesSpec{Name: "fish", Share: 1.0 / 3, Terrains: []engine.Terrain{engine.TerrainWater}, Swims: true, NoSkills: true})
	c.FoodKinds = append(append([]engine.FoodKind(nil), c.FoodKinds...), engine.FoodKind{Name: "fish food", Tags: []string{"fish"}})
	c.Foods = append([]engine.FoodSpec(nil), c.Foods...)
	for i := range c.Foods {
		c.Foods[i].Share = 1.0 / 3
	}
	c.Foods = append(c.Foods, engine.FoodSpec{Name: "fish food", Kind: len(c.FoodKinds) - 1, Share: 1.0 / 3, Terrain: engine.TerrainWater})
	c.FoodCap *= 3
}
func single(c *engine.Config)       { shared(c); c.Species = c.Species[:1] }
func unventured(c *engine.Config)   { single(c); c.AllotVenture = false }
func unsized(c *engine.Config)      { unventured(c); c.AllotBudget = false }
func unmixed(c *engine.Config)      { unsized(c); c.AllotMix = false }
func unfertile(c *engine.Config)    { unmixed(c); c.AllotFertility = false }
func unbedded(c *engine.Config)     { unfertile(c); c.SeekBed = false }
func hopeful(c *engine.Config)      { unbedded(c); c.ReuseNights = false }
func unsighted(c *engine.Config)    { hopeful(c); c.AllotSight = false }
func unkept(c *engine.Config)       { unsighted(c); c.ShelterFoodLife = 0 }
func unsettled(c *engine.Config)    { unkept(c); c.Building = false }
func buried(c *engine.Config)       { unsettled(c); c.DropTents = false }
func atdusk(c *engine.Config)       { buried(c); c.NightSaving = false }
func unbuilt(c *engine.Config)      { unkept(c); c.Tents, c.Building = false, false }
func untaught(c *engine.Config)     { unbuilt(c); c.PassShelters = false }
func sleepblind(c *engine.Config)   { untaught(c); c.SleepAhead = false }
func unforced(c *engine.Config)     { sleepblind(c); c.ForceAt = 0 }
func shelterblind(c *engine.Config) { unforced(c); c.ShelterRead = false }
func shelterless(c *engine.Config)  { shelterblind(c); c.Shelter = false }
func duskblind(c *engine.Config)    { shelterless(c); c.DuskRead = false }
func templess(c *engine.Config)     { duskblind(c); c.Temperature = false }
func sleepless(c *engine.Config)    { templess(c); c.Sleep = false }
func burnblind(c *engine.Config)    { sleepless(c); c.BurnRead = false }
func sightblind(c *engine.Config)   { burnblind(c); c.SightRead = false }
func daylong(c *engine.Config)      { sightblind(c); c.DayTicks = 0 }
func unspoiled(c *engine.Config)    { daylong(c); c.FoodLife = 0 }
func unrecalled(c *engine.Config)   { unspoiled(c); c.SpotRead = 0 }
func halflife(c *engine.Config)     { unrecalled(c); c.AgeBand = 0 }
func unheld(c *engine.Config)       { halflife(c); c.Carry = 0 }
func restblind(c *engine.Config)    { unheld(c); c.RestAhead = false }
func oldmating(c *engine.Config)    { restblind(c); c.OldBarren = false }
func unasked(c *engine.Config)      { oldmating(c); c.Requests = false }
func unaged(c *engine.Config)       { unasked(c); c.ChildAbility = 1 }
func slowgrow(c *engine.Config)     { unaged(c); c.MatureAge = 1000 }
func short(c *engine.Config)        { slowgrow(c); c.Lifespan = 3000 }
func ageless(c *engine.Config)      { short(c); c.Lifespan = 0 }
func busy(c *engine.Config)         { ageless(c); c.RestBurn = 1 }
func doubled(c *engine.Config)      { busy(c); c.ProvisionEachWith, c.ProvisionEachAlone = 0.0075, 0.00375 }
func lean(c *engine.Config)         { busy(c); c.ProvisionEachWith, c.ProvisionEachAlone = 0.00375, 0.001875 }
func perunit(c *engine.Config)      { lean(c); c.ProvisionEach = false }
func unfed(c *engine.Config)        { perunit(c); c.Provision = false }
func even(c *engine.Config)         { unfed(c); c.FemaleBears = false }
func sexless(c *engine.Config)      { even(c); c.Sexes = false }
func aged(c *engine.Config)         { sexless(c); c.PassPath = true }
func kin(c *engine.Config)          { aged(c); c.AgePath = false }
func near(c *engine.Config)         { kin(c); c.Kin = false }
func mixed(c *engine.Config)        { near(c); c.StableRows = false }
func undecayed(c *engine.Config)    { mixed(c); c.EvidenceHalfLife = 0 }
func untold(c *engine.Config)       { undecayed(c); c.Tell = false }
func truth(c *engine.Config)        { untold(c); c.Learn = false }
func bounced(c *engine.Config)      { truth(c); c.Bounce = true }
func drawn(c *engine.Config)        { bounced(c); c.AllotInherit = false }
func overlap(c *engine.Config)      { drawn(c); c.Collide = false }
func fixed(c *engine.Config)        { overlap(c); c.Allot = false }
func nobreed(c *engine.Config)      { fixed(c); c.Breed = false }
func everytick(c *engine.Config)    { nobreed(c); c.Recheck = 0 }

// rewrites maps a variant name to the rewrite of the default config it
// stands for.
var rewrites = map[string]func(*engine.Config){
	Base:       func(*engine.Config) {},
	Mature750:  func(c *engine.Config) { c.MatureAge = 750 },
	Mature400:  func(c *engine.Config) { c.MatureAge = 400 },
	Unprovided: func(c *engine.Config) { c.Provision = false },
	Food2:      func(c *engine.Config) { c.FoodReturn *= 2 },
	Food4:      func(c *engine.Config) { c.FoodReturn *= 4 },
	Tight:      func(c *engine.Config) { c.AllotCurve /= 2 },
	Exact:      func(c *engine.Config) { unventured(c); c.FullStep = 0 },
	Shared:     shared,
	Together:   func(c *engine.Config) { c.PoolPlaces = false },
	Open:       func(c *engine.Config) { c.ClosedPlaces = false },
	Roomy:      func(c *engine.Config) { c.FoodCap *= 2 },
	Skilled: func(c *engine.Config) {
		c.FoodCap *= 2
		c.Species = append([]engine.SpeciesSpec(nil), c.Species...)
		for i := range c.Species {
			c.Species[i].NoSkills = false
		}
	},
	Shallow:   func(c *engine.Config) { fish(c); c.DeepWater = false },
	Fish:      fish,
	Planted:   planted,
	Planted4:  func(c *engine.Config) { planted(c); c.Foods[len(c.Foods)-1].SeedRate = 4 * PlantSeedRate },
	Planted10: func(c *engine.Config) { planted(c); c.Foods[len(c.Foods)-1].SeedRate = 10 * PlantSeedRate },
	Strewn: func(c *engine.Config) {
		planted(c)
		c.Foods[len(c.Foods)-1].Seed, c.Foods[len(c.Foods)-1].SeedRate, c.Foods[len(c.Foods)-1].Start = 0, 0, 0
	},
	Wade: func(c *engine.Config) { fish(c); c.WaterBurn = 1 },
	Dry:  func(c *engine.Config) { fish(c); c.Wade = false },
	Roomytogether: func(c *engine.Config) {
		c.FoodCap *= 2
		c.PoolPlaces = false
	},
	Roomyopen: func(c *engine.Config) {
		c.FoodCap *= 2
		c.ClosedPlaces = false
	},
	Single:  single,
	Halffed: func(c *engine.Config) { single(c); c.FoodCap /= 2; c.Bodies /= 2 },
	Twin: func(c *engine.Config) {
		c.Species = append([]engine.SpeciesSpec(nil), c.Species...)
		for i := range c.Species {
			c.Species[i].Size = 0
		}
	},
	Unventured:   unventured,
	Steady:       func(c *engine.Config) { c.Steady = 1.5 },
	Fickle:       func(c *engine.Config) { c.Steady = 1 / 1.5 },
	Bold:         func(c *engine.Config) { unventured(c); c.Venture = 1.5 },
	Timid:        func(c *engine.Config) { unventured(c); c.Venture = 1 / 1.5 },
	Unsized:      unsized,
	Unpriced:     func(c *engine.Config) { c.BudgetBurn = false },
	Unmixed:      unmixed,
	Unfertile:    unfertile,
	Evenfert:     func(c *engine.Config) { c.FertStep, c.FertTrade = 0, 0 },
	Unbedded:     unbedded,
	Hopeful:      hopeful,
	Unsighted:    unsighted,
	Sight0:       func(c *engine.Config) { unsighted(c); c.Sight = 0 },
	Sight2:       func(c *engine.Config) { unsighted(c); c.Sight = 2 },
	Unkept:       unkept,
	Unsettled:    unsettled,
	Buried:       buried,
	Atdusk:       atdusk,
	Unbuilt:      unbuilt,
	Untaught:     untaught,
	Sleepblind:   sleepblind,
	Unforced:     unforced,
	Shelterblind: shelterblind,
	Shelterless:  shelterless,
	Duskblind:    duskblind,
	Templess:     templess,
	Sleepless:    sleepless,
	Burnblind:    burnblind,
	Sightblind:   sightblind,
	Daylong:      daylong,
	Unspoiled:    unspoiled,
	Unrecalled:   unrecalled,
	Halflife:     halflife,
	Unheld:       unheld,
	Restblind:    restblind,
	Oldmating:    oldmating,
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
