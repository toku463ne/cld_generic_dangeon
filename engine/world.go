package engine

import (
	"encoding/binary"
	"fmt"
	"hash/fnv"
	"math"
	"math/rand"
	"sort"
)

// World is the whole simulation. It knows nothing about drawing, networking,
// map files, or who is playing.
type World struct {
	cfg Config
	m   Map

	// light is the Brightness of tick lightFor-1 (zero: none worked out).
	light    float64
	lightFor int64

	// rng is the single source of randomness of the simulation. Everything
	// random draws from it, so a given seed always gives the same run.
	rng   *rand.Rand
	draws *countingSource

	tick int64

	food   foodState
	bodies []Body
	nextID int64
	stats  Stats

	// grid lists the bodies on each tile (breed.go; with Breed or Collide),
	// and born the children
	// born this tick, who join the world at its end.
	grid [][]int32
	born []Body

	// pred is what valuing options needs and can be rebuilt from the rest,
	// so it is neither saved nor fingerprinted.
	pred predictor
	// valuation is scratch space for the decision of the body acting now.
	valuation Valuation
	ties      []int
	drawn     bool // the last decision drew among ties (LastTie); no rule reads it
	// places are the built places standing (stage 8-1), oldest first;
	// placeAt the place each tile belongs to (0: none), and nextPlace the
	// last place's ID.
	places  []Place
	placeAt []int64
	placeSp []int8 // per tile, the species of its place plus one (0: none)
	// shelterOwner is, per region, the species plus one that the map's
	// places to rest there belong to (ClosedShelters), 0 for everyone's.
	shelterOwner []int8
	nextPlace    int64
	// groundTents are the tents lying folded on tiles (stage 8-2a3).
	groundTents []GroundTent
	// coins are the coins issued and lying on the ground (stage 11-2a).
	coins coinState
	// shops are the map's shops as they stand (stage 11-2b).
	shops []shopState
	// shopAt marks, per tile, a shop's walls and doors (nil: no shops).
	shopAt []int8
	// parcels are the things on their way to the shops (stage 11-2c).
	parcels []Parcel
	// groundOrnaments are the ornaments lying on the ground (stage 11-4).
	groundOrnaments []GroundOrnament
	rates           rates // a learning body's estimates, for the decision under way

	// died is the tick each body died, for the provenance instrument
	// (provenance.go). No rule reads it; it is neither saved nor
	// fingerprinted.
	died map[int64]int64

	// chooser, when set, takes the decisions of the bodies it names
	// (chooser.go).
	chooser Chooser

	// meet, when set, is shown every meeting at which bodies pass evidence
	// (tell.go).
	onMeet func(a, b Body)

	// trace, when set, is shown every decision as it is made.
	trace func(Body, Valuation, Action)
	// prey says, per species, which species it preys on (hunt.go); nil
	// where none does. hunt holds, for the decision under way, the ticks
	// to the kill of each prey among the options.
	prey [][]bool
	hunt map[int64]int
	// threats are the hunters in sight of the body deciding now
	// (hunt.go), for its valuation.
	threats []*Body
	// asking lists the indices of the bodies with a request to mate open
	// (breed.go), kept through a tick.
	// far holds, for the decision under way, the ticks of walking to each
	// requester out of sight among the options (breed.go).
	far    map[int64]int
	asking []int
	// calling lists the bodies with a call to sell open, and toCaller the
	// ticks of walking to each caller out of sight for the decision under
	// way (stage 12-1).
	calling []int
	// signs are the signs standing (stage 12-2).
	signs    []Sign
	toCaller map[int64]int
}

// NewWorld builds a world on the given map: the food cap laid out by region
// share, then Config.Bodies bodies with drawn genomes. The map is copied,
// so the caller may reuse it.
func NewWorld(cfg Config, m Map) (*World, error) { return newWorld(cfg, m, nil) }

// NewWorldFrom builds a world like NewWorld, but its first bodies are one
// per genome given, in that order, in place of Config.Bodies drawn ones
// (genome.go); none given, none placed. This is how a world begins from
// bodies bred elsewhere.
func NewWorldFrom(cfg Config, m Map, genomes []Genome) (*World, error) {
	if genomes == nil {
		genomes = []Genome{}
	}
	return newWorld(cfg, m, genomes)
}

func newWorld(cfg Config, m Map, genomes []Genome) (*World, error) {
	if err := m.Validate(); err != nil {
		return nil, err
	}
	if cfg.AgeBand > 0 && cfg.Learn && (cfg.PassPath || !cfg.StableRows) {
		return nil, fmt.Errorf("age bands (AgeBand) do not carry evidence passed between bodies: turn PassPath off and StableRows on")
	}
	if cfg.AgeBand > 0 && (cfg.AgeEpoch <= 0 || len(cfg.AgeTrust) == 0) {
		return nil, fmt.Errorf("AgeBand needs AgeEpoch above zero and AgeTrust")
	}
	if cfg.AllotSight && (cfg.SightMost < 1 || cfg.Sight < 0 || cfg.Sight > cfg.SightMost) {
		return nil, fmt.Errorf("AllotSight needs 0 <= Sight <= SightMost and SightMost >= 1, got Sight %d SightMost %d", cfg.Sight, cfg.SightMost)
	}
	if cfg.AllotFertility && (cfg.FertMost < 1 || cfg.Fertility < 0 || cfg.Fertility > 1) {
		return nil, fmt.Errorf("AllotFertility needs FertMost >= 1 and 0 <= Fertility <= 1, got FertMost %d Fertility %v", cfg.FertMost, cfg.Fertility)
	}
	if cfg.Learn && !(cfg.Venture > 0) {
		return nil, fmt.Errorf("Venture must be above zero, got %v", cfg.Venture)
	}
	if err := validSpecies(cfg.Species); err != nil {
		return nil, err
	}
	if err := validFood(cfg.FoodKinds, cfg.Foods); err != nil {
		return nil, err
	}
	if err := validCoins(cfg); err != nil {
		return nil, err
	}
	if err := validOrnaments(cfg); err != nil {
		return nil, err
	}
	if cfg.Wade && !(cfg.WaterSpeed > 0 && cfg.WaterSpeed <= 1) {
		return nil, fmt.Errorf("WaterSpeed must be within (0, 1], got %v", cfg.WaterSpeed)
	}
	if cfg.Learn && !(cfg.Steady > 0) {
		return nil, fmt.Errorf("Steady must be above zero, got %v", cfg.Steady)
	}
	if cfg.AllotVenture && (cfg.VentureMost < 1 || !(cfg.VentureStep > 0)) {
		return nil, fmt.Errorf("AllotVenture needs VentureMost >= 1 and VentureStep above zero, got %d %v", cfg.VentureMost, cfg.VentureStep)
	}
	if cfg.Hunt && !(cfg.HuntHunger > 0 && cfg.HuntHunger <= 1) {
		return nil, fmt.Errorf("HuntHunger must be within (0, 1], got %v", cfg.HuntHunger)
	}
	if cfg.AllotFierce && (cfg.FierceMost < 1 || !(cfg.FierceStep > 0)) {
		return nil, fmt.Errorf("AllotFierce needs FierceMost >= 1 and FierceStep above zero, got %d %v", cfg.FierceMost, cfg.FierceStep)
	}
	if cfg.AllotCombat && (cfg.CombatMost < 1 || !(cfg.CombatStep > 0) || !(cfg.CombatTrade >= 0)) {
		return nil, fmt.Errorf("AllotCombat needs CombatMost >= 1, CombatStep above zero and CombatTrade at least zero, got %d %v %v", cfg.CombatMost, cfg.CombatStep, cfg.CombatTrade)
	}
	if cfg.AllotBudget && (cfg.BudgetLevels < 1 || !(cfg.BudgetStep > 0) || !(cfg.Budget > 0)) {
		return nil, fmt.Errorf("AllotBudget needs BudgetLevels >= 1 and BudgetStep and Budget above zero, got %d %v %v", cfg.BudgetLevels, cfg.BudgetStep, cfg.Budget)
	}
	if cfg.DayTicks > 0 && (cfg.NightTicks < 0 || cfg.NightTicks > cfg.DayTicks || cfg.NightSight > cfg.Sight || cfg.TwilightTicks < 0 || cfg.TwilightTicks > cfg.NightTicks || cfg.TwilightTicks > cfg.DayTicks-cfg.NightTicks) {
		return nil, fmt.Errorf("a night (NightTicks) must fit in a day (DayTicks), its twilights (TwilightTicks) in both, and see no further than the day (Sight)")
	}
	if cfg.DuskRead && (cfg.LightStages <= 0 || cfg.LightBand <= 0 || cfg.LightBands <= 0) {
		return nil, fmt.Errorf("DuskRead needs LightStages, LightBand and LightBands above zero")
	}
	if cfg.Sleep && cfg.ForceAt > 0 && (cfg.ForceAt <= cfg.WakeAt || cfg.ForceMax < cfg.ForceAt || cfg.ForceMax > 1) {
		return nil, fmt.Errorf("sleep must be forced (ForceAt) above WakeAt and by ForceMax, at most 1")
	}
	if cfg.Temperature && !(cfg.TempBurn > 0) {
		return nil, fmt.Errorf("Temperature needs TempBurn above zero")
	}
	if cfg.Window > 0 && cfg.Grain <= 0 {
		return nil, fmt.Errorf("Grain must be above zero")
	}
	w := &World{cfg: cfg, m: m.Clone()}
	for _, g := range genomes {
		if err := w.validGenome(g); err != nil {
			return nil, err
		}
	}
	w.rng, w.draws = newCountingRand(cfg.Seed)
	w.initShopTiles()
	w.initFood()
	w.initCoins()
	w.initShops()
	w.initPrey()
	w.initOwners()
	w.initPredict()
	w.fillFood()
	w.placeBodies(genomes)
	w.buildGrid()
	return w, nil
}

// Step advances the world by one tick: vacant food comes back, then every
// body in turn follows its intent or decides (intent.go), acts and spends
// its energy, then the dead are removed and the children born this tick
// join.
func (w *World) Step() {
	w.tick++
	w.listAsking()
	w.listCalling()
	w.wearSigns()
	w.decayFood()
	w.returnFood()
	w.issueCoins()
	w.deliver()
	w.wearPlaces()
	w.wearGroundTents()
	for i := range w.bodies {
		b := &w.bodies[i]
		if b.Mature == w.tick && b.Mature > 0 {
			w.stats.Matured++
		}
		if w.cfg.ChildAbility < 1 {
			// Old age lowers the most a body can hold (stage 3-7).
			b.Energy = math.Min(b.Energy, w.maxOf(b))
		}
		if b.Asleep {
			w.stats.Actions[ActSleep]++
		} else {
			w.eatHeld(b)
			w.request(b)
			w.callSale(b)
			w.act(i, b, w.turn(b))
		}
		burn := w.burnOf(b)
		b.Energy -= burn
		w.feelBurn(b)
		w.feelWarm(b)
		w.noteHaunt(b)
		w.sleepTick(b)
		w.foldTent(b)
		w.pickTent(b)
		w.pickCoins(b)
		w.pickOrnaments(b)
		w.shopOrnaments(b)
		w.seeLight(b)
		w.stats.EnergyBurned += burn
	}
	w.removeDead()
	w.bodies = append(w.bodies, w.born...)
	w.born = w.born[:0]
	w.buildGrid()
	w.forget()
}

// removeDead drops the bodies that have run out of energy, keeping the order
// of the rest.
func (w *World) removeDead() {
	alive := w.bodies[:0]
	for _, b := range w.bodies {
		cause := CauseStarved
		old := w.cfg.Lifespan > 0 && w.tick-b.Born >= int64(w.cfg.Lifespan)
		if b.Slain {
			cause = CausePreyed
		} else if old && b.Energy > 0 {
			cause = CauseAged
		} else if w.cfg.Sleep && w.cfg.DebtBurn > 0 && b.Debt > w.cfg.DebtBurn/4 {
			cause = CauseSleepless
		}
		if b.Energy <= 0 || old {
			if b.Slain {
				w.dropMeat(&b)
			}
			if w.died == nil {
				w.died = map[int64]int64{}
			}
			w.died[b.ID] = w.tick
			w.loseHeld(&b)
			w.dropTent(&b)
			w.dropCoins(&b)
			w.dropOrnaments(&b)
			w.release(b.ID)
			w.stats.Deaths[cause]++
			if t := w.tileOf(b.X, b.Y); t >= 0 && w.m.Terrain[t] == TerrainWater && !w.spec(&b).Swims {
				w.stats.DiedOnWater++ // a wader, not a fish at home
			}
			w.stats.BudgetOut += b.Budget
			if w.tick < b.Mature {
				w.stats.DiedYoung++
			}
			continue
		}
		alive = append(alive, b)
	}
	w.bodies = alive
}

// Brightness is how light it is now, 0 to 1 (stage 7-1, O21). A day of
// DayTicks has its last NightTicks as night. The light changes along a
// trapezoid: 1 by day, 0 by night, and in between a straight ramp of
// TwilightTicks centred on each change, so the light over a day is the same
// as with no ramps. The clock starts half a twilight into the day: tick 0
// is the first of full light. No days: always 1.
func (w *World) Brightness() float64 {
	if w.lightFor != w.tick+1 {
		w.light, w.lightFor = w.brightnessAt(w.tick), w.tick+1
	}
	return w.light
}

func (w *World) brightnessAt(tick int64) float64 {
	d, n := w.cfg.DayTicks, w.cfg.NightTicks
	if d <= 0 || n <= 0 {
		return 1
	}
	x := float64((tick+int64(w.cfg.TwilightTicks/2))%int64(d)) + 0.5 // the middle of the tick
	dark := float64(d - n)                                           // the night starts here
	var into float64                                                 // signed ticks into the day (- into the night)
	if x < dark {
		into = math.Min(x, dark-x)
	} else {
		into = -math.Min(x-dark, float64(d)-x)
	}
	if w.cfg.TwilightTicks <= 0 {
		if into > 0 {
			return 1
		}
		return 0
	}
	return math.Max(0, math.Min(1, 0.5+into/float64(w.cfg.TwilightTicks)))
}

// BrightnessAt is Brightness at tick t, for counts.
func (w *World) BrightnessAt(t int64) float64 { return w.brightnessAt(t) }

// Night reports whether it is darker than half light now.
func (w *World) Night() bool { return w.Brightness() < 0.5 }

// Sight is the radius of the square a body can see in daylight; the tiles
// in it it sees now are those sees reports. The survival tables' reach
// keeps it.
func (w *World) Sight() int { return w.cfg.Sight }

// sightOf is how far body b sees, in tiles: its own drawn sight
// (AllotSight, stage 9-3), or the config's.
func (w *World) sightOf(b *Body) int {
	if b.Sight > 0 {
		return b.Sight - 1
	}
	return w.cfg.Sight
}

// BodySight is sightOf, for counts and clients.
func (w *World) BodySight(b Body) int { return w.sightOf(&b) }

// fertOf is body b's fertility level (AllotFertility, stage 9-4): its own
// drawn level, or the neutral one.
func (w *World) fertOf(b *Body) int {
	if b.Fert > 0 {
		return b.Fert - 1
	}
	return w.cfg.FertMost / 2
}

// fertChance is the chance a level of fertility gives a mating: Fertility
// at the neutral level, FertStep a level off it, held to 0..1. One without
// AllotFertility, whatever the level.
func (w *World) fertChance(level int) float64 {
	if !w.cfg.AllotFertility {
		return 1
	}
	return math.Min(math.Max(w.cfg.Fertility+w.cfg.FertStep*float64(level-w.cfg.FertMost/2), 0), 1)
}

// fertility is the chance body b's own fertility gives a mating of its.
func (w *World) fertility(b *Body) float64 { return w.fertChance(w.fertOf(b)) }

// BodyFertility is fertOf, for counts and clients.
func (w *World) BodyFertility(b Body) int { return w.fertOf(&b) }

// BodySize is sizeOf, the budget level, for counts and clients.
func (w *World) BodySize(b Body) int { return w.sizeOf(&b) }

// boldOf is body b's exploratory level (AllotVenture, stage 9-6a): its own
// drawn level, or the neutral one.
func (w *World) boldOf(b *Body) int {
	if b.Bold > 0 {
		return b.Bold - 1
	}
	return w.cfg.VentureMost / 2
}

// ventureOf is what body b expects of a region it has little evidence of,
// over the land: the config's Venture, scaled by VentureStep for every
// level of boldness off the neutral (AllotVenture).
func (w *World) ventureOf(b *Body) float64 {
	v := w.cfg.Venture
	if w.cfg.AllotVenture {
		if off := w.boldOf(b) - w.cfg.VentureMost/2; off != 0 {
			v *= math.Pow(w.cfg.VentureStep, float64(off))
		}
	}
	return v
}

// steadyOf is how much more body b believes the rows of places it knows
// than their parents (Steady).
func (w *World) steadyOf(b *Body) float64 { return w.cfg.Steady }

// regionWeight and pathWeight are the weights pulling body b's region rows
// toward the land and its path row toward its region, as it believes them.
func (w *World) regionWeight(b *Body) float64 { return w.cfg.RegionWeight / w.steadyOf(b) }
func (w *World) pathWeight(b *Body) float64   { return w.cfg.PathWeight / w.steadyOf(b) }

// attackOf and defenseOf are body b's combat levels (AllotCombat, stage
// 10-1): its own drawn level, or the neutral one. attackPower and
// defensePower are what they buy: CombatStep to the level off the
// neutral, exactly one at it.
func (w *World) attackOf(b *Body) int {
	if b.Attack > 0 {
		return b.Attack - 1
	}
	return w.cfg.CombatMost / 2
}

func (w *World) defenseOf(b *Body) int {
	if b.Defense > 0 {
		return b.Defense - 1
	}
	return w.cfg.CombatMost / 2
}

func (w *World) attackPower(b *Body) float64 {
	return math.Pow(w.cfg.CombatStep, float64(w.attackOf(b)-w.cfg.CombatMost/2))
}

func (w *World) defensePower(b *Body) float64 {
	return math.Pow(w.cfg.CombatStep, float64(w.defenseOf(b)-w.cfg.CombatMost/2))
}

// fierceOf is body b's aggressive level (AllotFierce, stage 10-3): its
// own drawn level, or the neutral one. fierceness is what it does to
// the loss a body reads of a fight: divides it by FierceStep per level
// off the neutral (VISION: an aggressive node reads its odds higher).
func (w *World) fierceOf(b *Body) int {
	if b.Fierce > 0 {
		return b.Fierce - 1
	}
	return w.cfg.FierceMost / 2
}

func (w *World) fierceness(b *Body) float64 {
	if !w.cfg.AllotFierce {
		return 1
	}
	return math.Pow(w.cfg.FierceStep, float64(w.fierceOf(b)-w.cfg.FierceMost/2))
}

// BodyFierce is fierceOf, for counts and clients.
func (w *World) BodyFierce(b Body) int { return w.fierceOf(&b) }

// BodyAttack and BodyDefense are attackOf and defenseOf, for counts and
// clients.
func (w *World) BodyAttack(b Body) int  { return w.attackOf(&b) }
func (w *World) BodyDefense(b Body) int { return w.defenseOf(&b) }

// BodyBold is boldOf, for counts and clients.
func (w *World) BodyBold(b Body) int { return w.boldOf(&b) }

// maxSight is the largest sight any body can have, for sizing bounds.
func (w *World) maxSight() int {
	if w.cfg.AllotSight && w.cfg.SightMost > w.cfg.Sight {
		return w.cfg.SightMost
	}
	return w.cfg.Sight
}

// sees reports whether body b sees tile (x, y) now: within NightSight of
// its tile always, within Sight when the light is above the tile's
// threshold for b, a number in [0, 1) fixed by the body and the tile. The
// share of the square it sees follows the light, and at one light the same
// tiles stay seen, so the light alone does not keep a body re-deciding.
// Nothing is drawn from the random source.
func (w *World) sees(b *Body, x, y int) bool {
	t := w.tileOf(b.X, b.Y)
	return w.seesFrom(b, t%w.m.Width, t/w.m.Width, x, y)
}

// seesFrom is sees with body b standing on tile (tx, ty).
func (w *World) seesFrom(b *Body, tx, ty, x, y int) bool {
	d := max(abs(x-tx), abs(y-ty))
	if d > w.sightOf(b) {
		return false
	}
	if d <= w.cfg.NightSight {
		return true
	}
	light := w.Brightness()
	if light >= 1 || light <= 0 {
		return light >= 1
	}
	h := uint64(b.ID)*0x9e3779b97f4a7c15 ^ uint64(y*w.m.Width+x)
	h ^= h >> 30
	h *= 0xbf58476d1ce4e5b9
	h ^= h >> 27
	h *= 0x94d049bb133111eb
	h ^= h >> 31
	return float64(h>>11)/(1<<53) < light
}

// Tick returns how many ticks the world has run.
func (w *World) Tick() int64 { return w.tick }

// Draws returns how many numbers have been taken from the random source.
func (w *World) Draws() uint64 { return w.draws.draws }

// Config returns the rules the world runs under.
func (w *World) Config() Config { return w.cfg }

// Map returns a copy of the world's map.
func (w *World) Map() Map { return w.m.Clone() }

// Fingerprint hashes everything the world's state consists of. Two worlds
// with the same fingerprint are, as far as any rule can tell, the same world;
// the behaviour fingerprint test pins its value for a fixed seed and run
// length, and it changes only when a rule is changed on purpose.
func (w *World) Fingerprint() uint64 {
	h := fnv.New64a()
	var buf [8]byte
	put := func(v uint64) {
		binary.LittleEndian.PutUint64(buf[:], v)
		h.Write(buf[:])
	}
	putF := func(v float64) { put(math.Float64bits(v)) }
	put(uint64(w.cfg.Seed))
	put(uint64(w.tick))
	put(w.draws.draws)
	put(uint64(w.m.Width))
	put(uint64(w.m.Height))
	for _, t := range w.m.Terrain {
		h.Write([]byte{byte(t)})
	}
	for _, r := range w.m.Region {
		binary.LittleEndian.PutUint16(buf[:2], uint16(r))
		h.Write(buf[:2])
	}
	// A unit's pool is state only where there is more than one.
	pools := len(w.food.pools) > 1
	for _, f := range w.food.foods {
		put(uint64(f.X))
		put(uint64(f.Y))
		if pools {
			put(uint64(f.ID))
		}
	}
	for _, p := range w.food.pools {
		for _, o := range p.owed {
			putF(o)
		}
		if p.seed > 0 {
			putF(p.sown)
			putF(p.bed)
		}
		put(uint64(p.appeared))
		put(uint64(p.eaten))
	}
	for _, b := range w.bodies {
		put(uint64(b.ID))
		putF(b.X)
		putF(b.Y)
		putF(b.Energy)
		put(uint64(b.Born))
		// A build is state only where it is not the config's.
		if b.Build != (Build{}) {
			putF(b.Build.Speed)
			putF(b.Build.EnergyMax)
			putF(b.Build.EnergyBurn)
		}
		// The heading is state only where a rule reads it.
		if w.cfg.KeepHeading {
			put(uint64(int64(b.Heading)))
		}
		// So is the intent, where bodies follow one between decisions.
		if w.cfg.Recheck > 0 {
			put(uint64(b.Intent.Kind))
			put(uint64(b.Intent.Dir))
			put(uint64(int64(b.Goal)))
			put(uint64(b.Decided))
			if b.Under {
				put(1)
			} else {
				put(0)
			}
			put(b.Saw)
			if b.Struck {
				put(1)
			}
			if b.Called {
				put(2)
			}
		}
		// And the age of coming of age, where bodies breed.
		if w.cfg.Allot && w.cfg.AllotInherit {
			put(uint64(b.Level))
		}
		// The drawn sight is state only where a body has one.
		if b.Sight != 0 {
			put(uint64(b.Sight))
		}
		if b.Size != 0 {
			put(uint64(b.Size))
		}
		if b.Bold != 0 {
			put(uint64(b.Bold))
		}
		if b.Attack != 0 || b.Defense != 0 {
			put(uint64(b.Attack))
			put(uint64(b.Defense))
		}
		if b.Fierce != 0 {
			put(uint64(b.Fierce))
		}
		if b.Species != 0 {
			put(uint64(b.Species))
		}
		// And what it has learned, where bodies learn.
		if w.cfg.Learn {
			mem := &b.Memory
			tally := func(t Tally) {
				putF(t.N)
				putF(t.K)
				if w.cfg.EvidenceHalfLife > 0 && w.cfg.AgeBand <= 0 {
					put(uint64(t.T))
					putF(t.S)
				}
				for _, c := range t.Chunks {
					put(uint64(c.T))
					putF(c.N)
					putF(c.K)
				}
				if !w.cfg.Tell {
					return
				}
				for _, h := range t.Heard {
					put(uint64(h.ID))
					putF(h.N)
					putF(h.K)
				}
			}
			for _, t := range mem.Regions {
				tally(t)
			}
			tally(mem.Path)
			if w.cfg.Tell {
				met := make([]int64, 0, len(mem.Met))
				for id := range mem.Met {
					met = append(met, id)
				}
				sort.Slice(met, func(i, j int) bool { return met[i] < met[j] })
				for _, id := range met {
					put(uint64(id))
				}
			}
			putF(mem.Asks)
			putF(mem.Mated)
			if w.cfg.AgeBand > 0 {
				tally(mem.Mate)
			}
			if w.cfg.SpotRead > 0 {
				tally(mem.Spot)
				spots := make([]int, 0, len(mem.Spots))
				for tl := range mem.Spots {
					spots = append(spots, tl)
				}
				sort.Ints(spots)
				for _, tl := range spots {
					put(uint64(tl))
					put(uint64(mem.Spots[tl]))
				}
			}
			tiles := make([]int, 0, len(mem.Walked))
			for tl, when := range mem.Walked {
				if w.tick-when <= int64(w.cfg.PathRecall) {
					tiles = append(tiles, tl)
				}
			}
			sort.Ints(tiles)
			for _, tl := range tiles {
				put(uint64(tl))
				put(uint64(mem.Walked[tl]))
			}
		}
		if w.cfg.Breed {
			put(uint64(b.Mature))
			put(b.Mates)
			put(uint64(b.Parents[0]))
			put(uint64(b.Parents[1]))
			if w.cfg.Sexes {
				put(uint64(b.Sex))
			}
			if w.bears() {
				put(uint64(b.Rested))
			}
			if w.bears() && w.cfg.Provision {
				put(uint64(b.Partner))
			}
			if w.bears() && w.cfg.Provision && w.cfg.ProvisionEach {
				put(uint64(b.Asks))
			}
			if w.cfg.Requests {
				put(uint64(b.Requested))
			}
			if b.Held > 0 {
				put(uint64(b.Held))
			}
		}
	}
	put(uint64(w.nextID))
	put(uint64(w.stats.Deaths[CauseStarved]))
	if w.cfg.Lifespan > 0 {
		put(uint64(w.stats.Deaths[CauseAged]))
	}
	if w.cfg.Breed {
		put(uint64(w.stats.Births))
	}
	// The coins, where they are issued (stage 11-2a).
	if w.cfg.Coins {
		put(uint64(w.coins.issued))
		putF(w.coins.due)
		for _, g := range w.GroundCoins() {
			put(uint64(g.Tile))
			put(uint64(g.N))
		}
		for _, b := range w.bodies {
			put(uint64(b.Coins))
		}
	}
	// The shops and what bodies know of them (stage 11-2b).
	for _, s := range w.shops {
		for _, g := range s.goods {
			put(uint64(g.stock))
			for _, l := range g.lives {
				put(uint64(l))
			}
		}
		put(uint64(s.fund))
	}
	for _, pc := range w.parcels {
		put(uint64(pc.Shop)<<32 | uint64(pc.Good))
		put(uint64(pc.Seller))
		put(uint64(pc.Arrive))
		put(uint64(pc.Born))
	}
	// The tastes and ornaments (stage 11-4).
	if w.ornaments() {
		for _, b := range w.bodies {
			put(uint64(b.Taste))
			for _, k := range b.Ornaments {
				put(uint64(k))
			}
		}
		for _, g := range w.groundOrnaments {
			put(uint64(g.Tile)<<16 | uint64(g.Kind))
		}
	}
	for _, s := range w.signs {
		put(uint64(s.Tile))
		put(uint64(s.Seller))
		put(uint64(s.Item)<<32 | uint64(s.Price))
		put(uint64(s.Until))
	}
	if w.cfg.SaleCalls && w.cfg.Coins {
		for _, b := range w.bodies {
			put(uint64(len(b.Memory.Signs)))
			if b.Selling.Until > 0 {
				put(uint64(b.Selling.Until))
				put(uint64(b.Selling.Item)<<32 | uint64(b.Selling.Price))
			}
		}
	}
	if len(w.shops) > 0 {
		for _, b := range w.bodies {
			put(uint64(len(b.Memory.Shops)))
			putF(b.Memory.Buys)
			putF(b.Memory.Bought)
			putF(b.Memory.Sends)
			putF(b.Memory.Paid)
		}
	}
	return h.Sum64()
}
