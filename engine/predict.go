package engine

import (
	"math"
	"sort"
)

// Prediction and valuation.
//
// This is the one place a body's options are predicted and valued (NODE.md
// "予測の唯一の実装"). Stage 1-2 values them against a table built from the
// world's true rules; stage 1-6 will read the same rows from experience
// instead. decide takes the best option by these values, and a trace
// (SetTrace) is handed the very Valuation the choice was made from.
//
// The tables count energy in grains (Config.Grain), not ticks: a body's
// burn and speed fold into its chance of meeting food per grain, so bodies
// of every burn and speed share the tables (stage 7-3c). Ticks are turned
// into grains at the boundary (grains), with half a grain's rounding.
//
// The table holds outcomes only, never worth (NODE.md "記憶は結果だけを持つ"):
//
//	(tile with food, eat)                -> energy +FoodEnergy, up to EnergyMax
//	(land, wait or move)                 -> energy -EnergyBurn, position moved by Speed
//	(this region, keep moving)           -> stands on a tile with food, chance p per tile entered
//	(tile with food in sight, walk to it) -> stands on it after d/Speed ticks
//	(adult in sight, mate)               -> energy -its share, a child
//
// The third and fourth rows' outcome is the first row's subject, so they
// chain into "keep moving -> energy back, chance p per tile" and "walk to
// the food in sight -> energy back after d/Speed ticks". The risk of an
// option is computed from these each time: the chance of being dead at the
// end of the window if the body takes the option now and then follows the
// better of two continuations - keep moving through the region it is in,
// eating whenever it stands on food, or walk to one of the units it saw
// when it decided and eat it there, then keep moving through that unit's
// region. That another body may eat the unit first is not in the row: the
// table holds what the body's own actions lead to.

// TruthTable is the table of stage 1-2, filled from the world's true rules.
type TruthTable struct {
	// Meal is how many grains of energy one meal gives (FoodEnergy over
	// Grain), and Full how many a full body holds.
	Meal, Full int
	// Meet is, per region, the chance per grain burned that the config body
	// keeping on the move comes to stand on food: the food on the ground
	// over the land tiles, times the tiles entered per tick (Speed), times
	// the ticks a grain lasts it (Grain over EnergyBurn).
	Meet []float64
	// Reach is the most grains the fourth row can take to walk to a unit in
	// sight; survival tables keep the shorter windows the walk leaves, and
	// build any other row when it is first asked for.
	Reach int
	// Extra are more window lengths a survival table keeps (learn.go).
	Extra []int
	// Pre are window lengths built with the table, only so that they are
	// built in its first pass: unlike Extra, they change no reading.
	Pre []int
	// Over is how many ticks of energy past Full the table reads: food a
	// body holds (Carry, stage 4-1). Past Full a meal met on the ground
	// fits nowhere and is passed by.
	Over int
}

// TruthTable reads the true rules and the food on the ground now.
func (w *World) TruthTable() TruthTable {
	t := TruthTable{
		Meal:  w.units(w.cfg.FoodEnergy),
		Full:  w.units(w.cfg.EnergyMax),
		Meet:  make([]float64, len(w.m.RegionFood)),
		Reach: w.grains(w.reach(w.slowest()), w.cfg.EnergyBurn),
	}
	t.Over = w.cfg.Carry * t.Meal
	for r := range t.Meet {
		t.Meet[r] = w.meet(RegionID(r), w.cfg.Speed, w.cfg.EnergyBurn)
	}
	return t
}

// meet is the third row of the table for region r and a body of the given
// speed and burn, read from the food on its ground now.
func (w *World) meet(r RegionID, speed, burn float64) float64 {
	return w.meetAt(r, w.food.onGround[r], speed, burn)
}

// meetAt is meet with the given food on region r's ground: the chance per
// grain burned of coming to stand on food. It is the chance per tile times
// the tiles entered per tick, times the ticks a grain lasts, no more than
// 1: what a body burns per tile is what tells a fast or slow burner apart.
func (w *World) meetAt(r RegionID, food int, speed, burn float64) float64 {
	land := len(w.food.regionLand[r])
	if land == 0 {
		return 0
	}
	return math.Min(float64(food)/float64(land)*w.factor(speed, burn), 1)
}

// factor is how many tiles a body of the given speed and burn enters per
// grain of energy: what it multiplies a chance per tile by.
func (w *World) factor(speed, burn float64) float64 {
	if burn <= 0 {
		return 0
	}
	return math.Min(speed, 1) * w.cfg.Grain / burn
}

// factorLevel rounds a body's factor onto the grid of BeliefStep relative
// to the config body's, for the tables of the world's true rules: the
// config body is exactly level 0.
func (w *World) factorLevel(speed, burn float64) int {
	f, f0 := w.factor(speed, burn), w.factor(w.cfg.Speed, w.cfg.EnergyBurn)
	if f == f0 || f <= 0 || f0 <= 0 || w.cfg.BeliefStep <= 1 {
		return 0
	}
	return int(math.Round(math.Log(f/f0) / math.Log(w.cfg.BeliefStep)))
}

// factorOf is the factor of a level relative to the config body's.
func (w *World) factorOf(level int) float64 {
	if level == 0 || w.cfg.BeliefStep <= 1 {
		return 1
	}
	return math.Pow(w.cfg.BeliefStep, float64(level))
}

// units is how many grains of energy an amount is; a body with energy e
// starves when its last grain is burned.
func (w *World) units(e float64) int {
	return int(math.Ceil(e/w.cfg.Grain - 1e-9))
}

// grains is how many grains a body of the given burn spends in k ticks, to
// the nearest, at least one for any tick: how a walk or a window in ticks
// reads in the tables.
func (w *World) grains(k int, burn float64) int {
	if k <= 0 {
		return 0
	}
	return max(int(math.Round(float64(k)*burn/w.cfg.Grain)), 1)
}

// windowGrains is the window in grains: Window ticks of the config body's
// burn, the same energy for every body.
func (w *World) windowGrains() int {
	return w.grains(w.cfg.Window, w.cfg.EnergyBurn)
}

// reach bounds the ticks it takes a body of the given speed to walk onto a
// unit in sight from where any option can leave it: the unit is up to Sight
// tiles from the body's tile on either axis, and a move takes the body up to
// speed out of it, so each axis has at most Sight+speed to cover.
func (w *World) reach(speed float64) int {
	if w.cfg.Sight < 0 || speed <= 0 {
		return 0
	}
	gap := float64(w.cfg.Sight) + speed
	return walkTicks(gap, gap, speed)
}

// walkTicks is the fewest ticks it takes to cover dx and dy with moves of
// length speed in the eight directions. A diagonal move covers speed/sqrt2
// of each axis, so the path is the octile distance.
func walkTicks(dx, dy, speed float64) int {
	return int(math.Ceil(octile(dx, dy)/speed - 1e-9))
}

// energyTicks is how many ticks of burning an amount of energy lasts. A body
// with energy e starves at the end of tick ceil(e/burn). The small margin
// keeps 100/0.1 from rounding up to 1001.
func energyTicks(e, burn float64) int {
	if burn <= 0 {
		return math.MaxInt32
	}
	return int(math.Ceil(e/burn - 1e-9))
}

// Survival is, for one chance of meeting food per grain and a set of
// windows, the chance of being dead at the end of each window from every
// amount of energy (in grains, 0 to Full plus Over).
//
// It holds the chance of dying rather than of living on because the two
// differ in what float64 can tell apart: where food is plentiful both
// options' chances of living on round to exactly 1, and the difference
// between eating and not eating is lost; their chances of dying, 1e-9 and
// 1e-20 say, are kept. The quantity valued is the same.
type Survival struct {
	Windows []int
	// rows is shared by every copy of the table, so that a row built for
	// one copy is there for all, and a copy stays small.
	*rows
}

// rows is what a survival table holds and is built from.
type rows struct {
	// dead[l][n] is the chance of being dead l-1 grains on from n grains
	// of energy: the first grain of a window is the option itself. It is
	// kept for each window and for the shorter ones a walk of up to Reach
	// grains leaves of it, and any other length is built the first time it
	// is read (row); nil until then.
	dead [][]float64
	t    TruthTable
	meet float64
	// floor is the shortest window a walk in sight leaves (longest minus
	// Reach): below it only the Extra rows are read exactly, and any other
	// length is read at the next of them (deadAtLeast) - a rule of the
	// table, not of which rows happen to be built.
	floor int
}

// rowBand is how many rows below one asked for are built with it. Above
// it, every missing row up to the floor is built in the same pass: a walk
// nearby asks for its neighbours next, a pass of the recursion costs the
// same however many rows it keeps, and a table then needs at most a pass
// or two beyond the one that made it.
const rowBand = 4

// NewSurvival works the chances out grain by grain: in each grain the body
// meets food with chance meet, and eats it, or does not; then it burns the
// grain. It is exact for the table's rows; its cost is the longest window
// times Full.
func (t TruthTable) NewSurvival(meet float64, windows []int) Survival {
	longest := 0
	for _, w := range windows {
		longest = max(longest, w)
	}
	s := Survival{Windows: windows, rows: &rows{dead: make([][]float64, longest+1), t: t, meet: meet, floor: max(longest-1-t.Reach, 1)}}
	want := make([]bool, longest+1)
	for _, w := range windows {
		for l := max(w-1-t.Reach, 1); l <= w; l++ {
			want[l] = true
		}
	}
	for _, l := range append(t.Extra, t.Pre...) {
		if l >= 1 && l <= longest {
			want[l] = true
		}
	}
	s.build(want)
	return s
}

// build fills in the rows want marks, by one pass of the recursion up to
// the longest of them. A row is the same whenever it is built.
func (s Survival) build(want []bool) {
	longest := len(want) - 1
	// The table's numbers as locals: read through the shared rows in the
	// inner loop, they cost a checked access each under the race detector.
	meal, full, meet, dead := s.t.Meal, s.t.Full, s.meet, s.dead
	top := full + s.t.Over
	cur := make([]float64, top+1)
	next := make([]float64, top+1)
	cur[0] = 1
	keep := func(step int) {
		if step+1 <= longest && want[step+1] && dead[step+1] == nil {
			dead[step+1] = append([]float64(nil), cur...)
		}
	}
	keep(0)
	miss := 1 - meet
	for step := 1; step < longest; step++ {
		next[0] = 1
		// A body with more grains of energy than the steps so far cannot be
		// dead yet: those entries stay 0, as they were made, and are not
		// worked out. It halves the work and changes no number.
		for n := 1; n <= min(step, top); n++ {
			fed := max(min(n+meal, full), n) - 1
			next[n] = meet*cur[fed] + miss*cur[n-1]
		}
		cur, next = next, cur
		keep(step)
	}
}

// row is the row for a window of length l, built with its neighbours
// (rowBand either side) if it is not kept yet.
func (s Survival) row(l int) []float64 {
	if s.rows == nil {
		return nil
	}
	if d := s.dead[l]; d != nil {
		return d
	}
	hi := len(s.dead) - 1
	want := make([]bool, hi+1)
	if l < s.floor {
		// Below the floor (a window shortened by a foreseen sleep, stage
		// 7-3e): the rows a walk from there reads, and their neighbours,
		// not every row up to the floor.
		for i := max(l-s.t.Reach-rowBand, 1); i <= min(l+rowBand, hi); i++ {
			want[i] = s.dead[i] == nil
		}
		s.build(want)
		return s.dead[l]
	}
	for i := max(l-rowBand, 1); i <= hi; i++ {
		want[i] = s.dead[i] == nil && (i <= l || i < s.floor)
	}
	s.build(want)
	return s.dead[l]
}

// Dead returns the chance of being dead at the end of window i from n
// ticks of energy after the option's own tick.
func (s Survival) Dead(i, n int) float64 { return s.deadIn(s.Windows[i], n) }

// deadIn is Dead for a window of length l, which must be one the table
// keeps.
func (s Survival) deadIn(l, n int) float64 {
	if n <= 0 {
		return 1
	}
	if l < 1 {
		return 0
	}
	if l >= len(s.dead) {
		l = len(s.dead) - 1
	}
	d := s.row(l)
	return d[min(n, len(d)-1)]
}

// deadAtLeast is deadIn for a window of length l or, below the table's
// floor, the shortest of its Extra rows above l, else the floor: a walk
// longer than Reach (to a remembered unit, stage 5-2) is read as if it left
// more of the window, which reads it no safer than it is.
func (s Survival) deadAtLeast(l, n int) float64 {
	if n <= 0 {
		return 1
	}
	if l < 1 {
		return 0
	}
	if l < s.floor {
		next := s.floor
		for _, e := range s.t.Extra {
			if e >= l && e < next {
				next = e
			}
		}
		l = next
	}
	return s.deadIn(l, n)
}

// Alive is 1 - Dead, for reading; it loses what Dead keeps near 1.
func (s Survival) Alive(i, n int) float64 { return 1 - s.Dead(i, n) }

// Valuation is the options of one body and the risk of each in every
// window: the chance of being dead at the window's end. The lower the risk,
// the better the option; the worth of an option in the survival currency is
// the risk it takes away, and in the child currency the chance of a child
// it brings.
//
// Seen is the food the body saw when it decided, and Plan says, for each
// option in the first window, which continuation its risk was read from:
// the index into Seen of the unit it walks to, Arrive ticks after the
// option's own tick, or -1 for keeping on the move through the region.
type Valuation struct {
	// Why is what made the body decide (intent.go).
	Why     Trigger
	Options []Action
	Risk    [][]float64 // Risk[window][option]
	// Child is each option's chance of a child, the second currency
	// (breed.go): for a mate the chance it makes a child (1 read from the
	// truth table, learned with Learn), 0 otherwise. Score is what the
	// choice is made on, the first window's risk minus ChildWorth times
	// Child.
	Child []float64
	Score []float64
	Seen  []Food
	// Recalled are the remembered units read (stage 5-2), each there with
	// chance SpotP; a Plan past the end of Seen points into them.
	Recalled []Food
	SpotP    float64
	// Shelter is the place to rest the body may walk to and sleep at
	// (stage 6-1b), if any; a Plan past Recalled points to it.
	Shelter []Food
	// Spend is, for each option, the energy the plan its risk was read
	// from spends over Window ticks (stage 6-3a): what TieSpend breaks
	// ties by.
	Spend  []float64
	Plan   []int
	Arrive []int
	// Belief is what the body's memory made of the world (with Learn).
	Belief Belief
}

// unit is the food a Plan points to: in sight, or remembered (stage 5-2).
func (v *Valuation) unit(i int) Food {
	if i < len(v.Seen) {
		return v.Seen[i]
	}
	if i -= len(v.Seen); i < len(v.Recalled) {
		return v.Recalled[i]
	}
	return v.Shelter[i-len(v.Recalled)]
}

// Value predicts and values every option body b has now. survival is the
// table of chances per region, built by NewSurvival from the same
// TruthTable. The table is the config's (TruthTable), so b is valued as a
// body of the config's build, whatever its own; decide values a body of its
// own build against tables of its own.
func (w *World) Value(t TruthTable, survival []Survival, b Body) Valuation {
	var v Valuation
	b.Build = Build{}
	b.Energy = math.Min(b.Energy, w.cfg.EnergyMax)
	w.valueInto(&v, t.Meal, t.Full, survival[0].Windows, func(r RegionID) Survival { return survival[r] }, 1, nil, &b)
	return v
}

// valueInto is Value writing into v's slices, so that deciding every tick
// does not allocate. meal and full are in grains, and alive
// gives the survival table of a region for b's build. child is the chance
// a mate makes a child; path reads keeping on the move with the path row
// (learn.go).
func (w *World) valueInto(v *Valuation, meal, full int, windows []int, alive func(RegionID) Survival, child float64, rs *rates, b *Body) {
	path := rs != nil
	speed, burn := w.speedOf(b), w.readBurn(b)
	// A resting mother's reserve is energy she has not got yet (stage 4-0),
	// and the food a body holds is energy it eats as soon as a whole meal
	// fits (stage 4-1): both add to what it can last, but not to what a
	// meal can fill.
	reserve := w.restReserve(b) + b.Held*meal
	most := full + reserve
	// A walk of k ticks costs ku grains at this body's burn.
	grainsOf := func(k int) int { return w.grains(k, burn) }
	// top is what a meal can fill up to after the option under way: a unit
	// picked up raises it by a meal. room is whether a unit walked to and
	// found full can still be held.
	top, room := most, b.Held < w.cfg.Carry
	v.Options = w.possibleActions(v.Options[:0], b)
	v.Seen = w.inSight(v.Seen[:0], b)
	v.Recalled = w.recalled(v.Recalled[:0], b)
	if len(v.Recalled) > 0 {
		v.SpotP = w.spotRate(b)
	}
	// A place to rest to walk to and sleep at (stage 6-1b): the sleep
	// there burns warm, and the body wakes to its burn as from any sleep.
	v.Shelter = v.Shelter[:0]
	var warm, woken float64
	if rs != nil {
		if f, burn, ok := w.shelterAhead(b); ok {
			v.Shelter, warm, woken = append(v.Shelter, f), burn, w.wokenBurn(b)
		}
	}
	// What a plan spends over Window ticks (stage 6-3a): searching burns
	// as now; a sleep SleepTicks at its burn and the rest of the window at
	// the burn woken to.
	spendNow := float64(w.cfg.Window) * w.planBurn(b)
	sleepSpend := func(k int, asleep, after float64) float64 {
		s := min(w.cfg.SleepTicks, max(w.cfg.Window-k, 0))
		return float64(k)*w.planBurn(b) + float64(s)*asleep + float64(max(w.cfg.Window-k-s, 0))*after
	}
	v.Spend = v.Spend[:0]
	v.Plan, v.Arrive = v.Plan[:0], v.Arrive[:0]
	v.Child = v.Child[:0]
	n := w.units(b.Energy) + reserve
	for len(v.Risk) < len(windows) {
		v.Risk = append(v.Risk, nil)
	}
	v.Risk = v.Risk[:len(windows)]
	for i := range v.Risk {
		v.Risk[i] = v.Risk[i][:0]
	}
	// read is the risk of an option that leaves the body at (x, y) with
	// after ticks of energy, a unit eaten on tile eaten (or -1), in window
	// win: the better of keeping on the move and walking to a unit in
	// sight, and which it was.
	read := func(a Action, x, y float64, after, eaten, win int) (float64, int, int) {
		// A sleep the body will be made to take within the window (stage
		// 7-3e) is taken off its end for every way of staying awake; the
		// walk to a place to rest and sleep there is that sleep, taken now.
		whole, wholeWin := after, win
		if rs != nil {
			after, win = after-rs.lump, win-rs.lump
		}
		region := w.m.RegionAt(int(math.Floor(x)), int(math.Floor(y)))
		// Keep moving through the region.
		s := alive(region)
		risk, plan, arrive := s.deadIn(win, after), -1, 0
		if path {
			// A move keeps on its own heading. An option that stays keeps
			// on afterwards as the best of the moves open now would, each
			// read from where that move leaves the body - so that staying
			// reads no better than moving the best way at once (read from
			// here instead, it did by a step, and a body waited tick after
			// tick for a start it never made).
			if a.Kind == ActMove {
				risk = w.keepReading(b, rs, a.Dir, x, y, win, after, meal, top, s, risk)
			} else {
				best := math.Inf(1)
				for _, o := range v.Options {
					if o.Kind != ActMove {
						continue
					}
					d := moveDirs[o.Dir]
					mx, my := b.X+d[0]*speed, b.Y+d[1]*speed
					ms := alive(w.m.RegionAt(int(math.Floor(mx)), int(math.Floor(my))))
					best = math.Min(best, w.keepReading(b, rs, o.Dir, mx, my, win, after, meal, top, ms, ms.deadIn(win, after)))
				}
				if !math.IsInf(best, 1) {
					risk = best
				}
			}
		}
		// Or walk to a unit in sight: k ticks of walking, then a tick
		// of eating, leave win-2-k ticks of the window, which is a
		// window of win-1-k from the energy after the meal.
		for f, food := range v.Seen {
			if w.m.index(food.X, food.Y) == eaten {
				continue
			}
			k := walkTicks(gapTo(x, food.X), gapTo(y, food.Y), speed)
			ku := grainsOf(k)
			r := 1.0
			if after-ku > 0 {
				// It eats the unit, or holds it where a meal does not
				// fit and it has room (Carry, stage 4-1).
				fill := top
				if room {
					fill += meal
				}
				r = alive(w.m.RegionAt(food.X, food.Y)).deadIn(win-1-ku, min(after-ku+meal, fill)-1)
			}
			if r < risk {
				risk, plan, arrive = r, f, k
			}
		}
		// Or walk to a remembered unit (stage 5-2): there with chance
		// SpotP, eaten as one in sight; gone, the body keeps on the move
		// from there.
		for f, food := range v.Recalled {
			k := walkTicks(gapTo(x, food.X), gapTo(y, food.Y), speed)
			ku := grainsOf(k)
			r := 1.0
			if after-ku > 0 {
				fill := top
				if room {
					fill += meal
				}
				at := alive(w.m.RegionAt(food.X, food.Y))
				r = v.SpotP*at.deadAtLeast(win-1-ku, min(after-ku+meal, fill)-1) + (1-v.SpotP)*at.deadAtLeast(win-ku, after-ku)
			}
			if r < risk {
				risk, plan, arrive = r, len(v.Seen)+f, k
			}
		}
		// Or walk to a place to rest and sleep there (stage 6-1b): k
		// ticks of walking, SleepTicks asleep at the warm burn, then on
		// the move from there at the burn woken to.
		for f, place := range v.Shelter {
			r, k := w.shelterRisk(b, rs, place, x, y, whole, wholeWin, full, warm, woken)
			// With TieSpend, a risk no worse that spends less is taken
			// (stage 6-3a).
			if r < risk || w.cfg.TieSpend && r == risk && sleepSpend(k, warm, w.wokenBurn(b)) < spendNow {
				risk, plan, arrive = r, len(v.Seen)+len(v.Recalled)+f, k
			}
		}
		return risk, plan, arrive
	}
	for _, a := range v.Options {
		// Where the option leaves the body, and with how much energy.
		x, y, after := b.X, b.Y, n-1
		eaten := -1
		top, room = most, b.Held < w.cfg.Carry
		switch a.Kind {
		case ActEat:
			after = min(n+meal, most) - 1
			eaten = w.m.index(int(math.Floor(b.X)), int(math.Floor(b.Y)))
		case ActPick:
			// Held, the unit is a meal it eats once one fits (stage 4-1).
			after = n + meal - 1
			eaten = w.m.index(int(math.Floor(b.X)), int(math.Floor(b.Y)))
			top, room = most+meal, b.Held+1 < w.cfg.Carry
		case ActMove:
			d := moveDirs[a.Dir]
			x, y = b.X+d[0]*speed, b.Y+d[1]*speed
		case ActMate:
			// Read as agreed: the share paid, and a child - with the
			// chance child, and otherwise a tick spent as a wait. A
			// requester out of sight is reached first, k ticks of walking
			// (Requests).
			after = w.units(b.Energy-w.birthShare(b)) + reserve - 1 - grainsOf(w.farTicks(a.Mate))
		}
		c := 0.0
		if a.Kind == ActMate {
			c = child
		}
		v.Child = append(v.Child, c)
		for i, win := range windows {
			if a.Kind == ActSleep {
				v.Risk[i] = append(v.Risk[i], w.sleepRisk(b, rs, win, reserve))
				if i == 0 {
					v.Plan = append(v.Plan, -1)
					v.Arrive = append(v.Arrive, 0)
					v.Spend = append(v.Spend, sleepSpend(0, w.planBurn(b), w.wokenBurn(b)))
				}
				continue
			}
			risk, plan, arrive := read(a, x, y, after, eaten, win)
			if a.Kind == ActMate && c < 1 {
				unpaid, _, _ := read(a, x, y, n-1, eaten, win)
				risk = c*risk + (1-c)*unpaid
			}
			v.Risk[i] = append(v.Risk[i], risk)
			if i == 0 {
				v.Plan = append(v.Plan, plan)
				v.Arrive = append(v.Arrive, arrive)
				sp := spendNow
				if s := len(v.Seen) + len(v.Recalled); plan >= s && plan < s+len(v.Shelter) {
					sp = sleepSpend(arrive, warm, woken)
				}
				v.Spend = append(v.Spend, sp)
			}
		}
	}
}

// sleepRisk is the risk in a window of win grains of body b falling asleep
// now (stage 7-3b), read from what it knows of its own body: it sleeps
// SleepTicks (nothing it can foresee wakes it), eating nothing and burning
// as now; it wakes with the debt it repaid off its
// burn, and its burn rising as the debt then builds at WakeAt; and it
// keeps on the move through the region it is in for the rest of the
// window. reserve is what it holds and any rest left (valueInto).
func (w *World) sleepRisk(b *Body, rs *rates, win, reserve int) float64 {
	return w.sleepRiskAt(b, rs, win, reserve, w.planBurn(b), b.Energy)
}

// sleepRiskAt is sleepRisk for a sleep at burn now, from energy.
func (w *World) sleepRiskAt(b *Body, rs *rates, win, reserve int, now, energy float64) float64 {
	ticks := w.cfg.SleepTicks
	left := energy - now*float64(ticks)
	if left <= 0 {
		return 1
	}
	ts := w.grains(ticks, now)
	if ts >= win {
		return 0
	}
	burn := w.wokenBurn(b)
	full := w.units(w.maxOf(b))
	r := w.m.RegionAt(int(math.Floor(b.X)), int(math.Floor(b.Y)))
	var s Survival
	if rs != nil {
		s = w.learnedTable(rs.region[r]*rs.slept*w.factor(w.speedOf(b), burn), full)
	} else {
		s = w.survivalFor(r, full, w.factorLevel(w.speedOf(b), burn))
	}
	return s.deadAtLeast(win-ts, w.units(left)+reserve-1)
}

// wokenBurn is the burn body b reads itself waking to from a sleep of
// SleepTicks (sleepRisk): its burn now with the debt the sleep repays
// taken off, rising as the debt then builds at the pressure the sleep
// leaves.
func (w *World) wokenBurn(b *Body) float64 {
	ticks := w.cfg.SleepTicks
	base := w.planBurn(b) / w.debtBurn(b)
	debt := math.Max(0, b.Debt-w.cfg.DebtRepay*float64(ticks))
	after := base * (1 + debt/w.cfg.DebtBurn)
	// Woken, the pressure is what a sleep of ticks leaves of it now.
	rise := base * b.Pressure * math.Exp(-float64(ticks)/w.cfg.PressureAsleep) / w.cfg.DebtBurn
	return w.aheadBurn(after, rise)
}

// gapTo is how far p has to go along one axis to be on tile t.
func gapTo(p float64, t int) float64 {
	switch {
	case p < float64(t):
		return float64(t) - p
	case p >= float64(t+1):
		// Onto the tile means below t+1, not at it.
		return p - float64(t+1) + 1e-9
	}
	return 0
}

// inSight appends the food on the tiles within Sight of the tile body b
// stands on, row by row. A body sees the tile it stands on and the square
// of tiles around it; the food is all a body perceives.
func (w *World) inSight(dst []Food, b *Body) []Food {
	sight := w.Sight()
	if sight < 0 {
		return dst
	}
	bx, by := int(math.Floor(b.X)), int(math.Floor(b.Y))
	for y := by - sight; y <= by+sight; y++ {
		for x := bx - sight; x <= bx+sight; x++ {
			if w.m.InBounds(x, y) && w.foodFor(b, w.m.index(x, y)) >= 0 && w.seesFrom(b, bx, by, x, y) {
				dst = append(dst, Food{X: x, Y: y})
			}
		}
	}
	return dst
}

// predictor is what deciding needs to value options against the world's own
// window: the table's two fixed rows, and one survival table per region for
// its food on the ground now. A survival table depends on nothing but the
// region's food count and the body's build, so each is built once per count
// and build and kept; the config's build has a fast path (surv, fresh). All
// of it can be rebuilt from the rest of the world, and a table is the same
// whenever it is built: when tables are built, kept or dropped changes no
// decision, which is what lets the cache forget and a loaded world warm up.
type predictor struct {
	meal, full int
	windows    []int
	surv       []*table
	fresh      []bool
	cache      []map[survKey]*table // per region
	// learned are the tables of bodies that learn (learn.go), by a
	// believed chance per tile (its level, in survKey.food) and build.
	learned map[survKey]*table
	// pending are the tables a saved world was using, to be built by Warm
	// before they are asked for.
	pending []TableKey
}

// table is a survival table and the last tick it was read.
type table struct {
	s    Survival
	used int64
}

// survKey tells survival tables apart: the food (a region's count on its
// ground, or a believed chance per grain's level), the most grains the
// body holds, and for the tables of the true rules the level of the body's
// factor relative to the config body's (0: the config body). A body's burn
// and speed are in the chance per grain, not the key.
type survKey struct {
	food, full, factor int
}

// TableKey names one survival table, for a saved world to say which tables
// it was using (Warm).
type TableKey struct {
	Region, Food, Full, Factor int
}

const (
	// forgetEvery is how often, in ticks, the tables not read for
	// forgetAfter ticks are dropped. Without it the cache keeps every food
	// count a region ever had - some 2600 tables and 200 MB after 20000
	// ticks, most of them from the full map of the first ticks.
	forgetEvery = 1000
	forgetAfter = 10000
	// warmAfter is how lately a table must have been read for a saved
	// world to name it for warming.
	warmAfter = 1000
)

// initPredict sets the predictor up for the world's window. With no window
// there is nothing to predict.
func (w *World) initPredict() {
	w.pred = predictor{}
	if w.cfg.Window <= 0 {
		return
	}
	n := len(w.m.RegionFood)
	w.pred = predictor{
		meal:    w.units(w.cfg.FoodEnergy),
		full:    w.units(w.cfg.EnergyMax),
		windows: []int{w.windowGrains()},
		surv:    make([]*table, n),
		fresh:   make([]bool, n),
		cache:   make([]map[survKey]*table, n),
		learned: map[survKey]*table{},
	}
	for r := range w.pred.cache {
		w.pred.cache[r] = map[survKey]*table{}
	}
}

// refreshRegion marks region r's survival table stale after its food
// changed. It is rebuilt, or found in the cache, when next asked for.
func (w *World) refreshRegion(r RegionID) {
	if w.pred.fresh != nil {
		w.pred.fresh[r] = false
	}
}

// survival returns region r's survival table for its food on the ground now
// and the config's build.
func (w *World) survival(r RegionID) Survival {
	p := &w.pred
	if !p.fresh[r] {
		p.surv[r], p.fresh[r] = w.tableFor(r, p.full, 0), true
	}
	t := p.surv[r]
	t.used = w.tick
	return t.s
}

// survivalFor returns region r's survival table for its food on the ground
// now, a body of the given most grains, and a factor level.
func (w *World) survivalFor(r RegionID, full, factor int) Survival {
	t := w.tableFor(r, full, factor)
	t.used = w.tick
	return t.s
}

// tableFor finds or builds region r's table for its food now, a most and a
// factor level.
func (w *World) tableFor(r RegionID, full, factor int) *table {
	return w.tableAt(r, survKey{food: w.food.onGround[r], full: full, factor: factor})
}

// tableAt finds or builds region r's table for key k.
func (w *World) tableAt(r RegionID, k survKey) *table {
	p := &w.pred
	t, ok := p.cache[r][k]
	if !ok {
		tt := TruthTable{Meal: p.meal, Full: k.full, Reach: w.grains(w.reach(w.slowest()), w.cfg.EnergyBurn), Over: w.cfg.Carry * p.meal}
		meet := math.Min(w.meetAt(r, k.food, w.cfg.Speed, w.cfg.EnergyBurn)*w.factorOf(k.factor), 1)
		t = &table{s: tt.NewSurvival(meet, p.windows), used: w.tick}
		p.cache[r][k] = t
	}
	return t
}

// forget drops the tables not read for forgetAfter ticks, every
// forgetEvery ticks.
func (w *World) forget() {
	p := &w.pred
	if p.cache == nil || w.tick%forgetEvery != 0 {
		return
	}
	for r := range p.cache {
		for k, t := range p.cache[r] {
			if w.tick-t.used > forgetAfter {
				delete(p.cache[r], k)
			}
		}
		p.fresh[r] = false
	}
	for k, t := range p.learned {
		if w.tick-t.used > forgetAfter {
			delete(p.learned, k)
		}
	}
}

// Tables names the survival tables the world has read in the last
// warmAfter ticks. A saved world carries them so that a loaded one can
// build them ahead (Warm); a table read less lately is built when it is
// next read.
func (w *World) Tables() []TableKey {
	var ks []TableKey
	for r, c := range w.pred.cache {
		for k, t := range c {
			if w.tick-t.used > warmAfter {
				continue
			}
			ks = append(ks, TableKey{Region: r, Food: k.food, Full: k.full, Factor: k.factor})
		}
	}
	for k, t := range w.pred.learned {
		if w.tick-t.used <= warmAfter {
			ks = append(ks, TableKey{Region: -1, Food: k.food, Full: k.full})
		}
	}
	sort.Slice(ks, func(i, j int) bool {
		a, b := ks[i], ks[j]
		switch {
		case a.Region != b.Region:
			return a.Region < b.Region
		case a.Food != b.Food:
			return a.Food < b.Food
		case a.Full != b.Full:
			return a.Full < b.Full
		}
		return a.Factor < b.Factor
	})
	return ks
}

// Warm builds up to n of the tables a loaded world was using when it was
// saved and returns how many are still to build. A world that is not warmed
// builds each table the first time it is read, and runs the same; warming
// moves that work to before play - a loading screen - so that the first
// ticks run as fast as those of the world that was saved. n <= 0 builds
// them all.
func (w *World) Warm(n int) int {
	p := &w.pred
	for i := 0; len(p.pending) > 0 && (n <= 0 || i < n); i++ {
		k := p.pending[0]
		p.pending = p.pending[1:]
		switch {
		case k.Region == -1 && p.learned != nil:
			w.learnedTable(w.levelRate(k.Food), k.Full)
		case k.Region >= 0 && k.Region < len(p.cache):
			w.tableAt(RegionID(k.Region), survKey{food: k.Food, full: k.Full, factor: k.Factor})
		}
	}
	return len(p.pending)
}

// decide values body b's options and takes the one of least risk. Among
// options of equal risk it keeps the body's heading, reflected off whatever
// makes that way worse (bounce), if KeepHeading is set and moving that way
// is one of them, and draws at random otherwise. With no window every option carries
// the same risk, so the draw is over all of them: the stage 1-1 control.
func (w *World) decide(b *Body) Action {
	v := &w.valuation
	switch {
	case w.cfg.Window > 0 && w.cfg.Learn:
		full, factor := w.units(w.maxOf(b)), w.factor(w.speedOf(b), w.readBurn(b))
		rs := &w.rates
		w.ratesOf(b, rs)
		alive := func(r RegionID) Survival { return w.rateTable(rs, r, full, factor) }
		w.valueInto(v, w.pred.meal, full, w.pred.windows, alive, w.childRate(b), rs, b)
		v.Belief = w.belief(b)
	case w.cfg.Window > 0 && b.Build == (Build{}) && w.readBurn(b) == w.cfg.EnergyBurn && w.ageFactor(b) == 1:
		w.valueInto(v, w.pred.meal, w.pred.full, w.pred.windows, w.survival, 1, nil, b)
	case w.cfg.Window > 0:
		full, level := w.units(w.maxOf(b)), w.factorLevel(w.speedOf(b), w.readBurn(b))
		w.valueInto(v, w.pred.meal, full, w.pred.windows, func(r RegionID) Survival { return w.survivalFor(r, full, level) }, 1, nil, b)
	default:
		v.Options = w.possibleActions(v.Options[:0], b)
		v.Seen, v.Plan, v.Arrive, v.Child = v.Seen[:0], v.Plan[:0], v.Arrive[:0], v.Child[:0]
		v.Recalled = v.Recalled[:0]
		if cap(v.Risk) == 0 {
			v.Risk = make([][]float64, 1)
		}
		v.Risk = v.Risk[:1]
		v.Risk[0] = v.Risk[0][:0]
		for range v.Options {
			v.Risk[0] = append(v.Risk[0], 0)
		}
	}
	v.Score = v.Score[:0]
	for j, x := range v.Risk[0] {
		c := 0.0
		if j < len(v.Child) {
			c = v.Child[j]
		}
		v.Score = append(v.Score, x-w.cfg.ChildWorth*c)
	}
	best := math.Inf(1)
	w.ties = w.ties[:0]
	for j, x := range v.Score {
		switch {
		case x < best:
			best = x
			w.ties = append(w.ties[:0], j)
		case x == best:
			w.ties = append(w.ties, j)
		}
	}
	if w.cfg.TieSpend && len(w.ties) > 1 && len(v.Spend) == len(v.Score) {
		// Of the options of least risk, those that spend least (stage
		// 6-3a).
		lo := math.Inf(1)
		for _, j := range w.ties {
			lo = math.Min(lo, v.Spend[j])
		}
		kept := w.ties[:0]
		for _, j := range w.ties {
			if v.Spend[j] <= lo*(1+1e-9) {
				kept = append(kept, j)
			}
		}
		w.ties = kept
	}
	var a Action
	draw := !w.cfg.KeepHeading || b.Heading < 0
	if !draw {
		best := func(d int) bool { return w.tied(Action{Kind: ActMove, Dir: d}) }
		h := b.Heading
		switch {
		case best(h):
		case !w.cfg.Bounce:
			draw = true
		default:
			h = bounce(h, best)
			if w.cfg.TurnOffReverse && h == (b.Heading+4)%8 {
				if w.cfg.DrawOffReverse {
					draw = true
				} else {
					h = w.turnOff(b.Heading, h, best)
				}
			}
		}
		a = Action{Kind: ActMove, Dir: h}
		draw = draw || !w.tied(a)
	}
	if draw {
		a = v.Options[w.ties[w.rng.Intn(len(w.ties))]]
	}
	return a
}

// bounce returns heading h reflected off whatever makes moving that way
// worse than the best: an axis whose step alone is not among the options
// of least risk is turned back, and both are where only the diagonal is out.
// best says whether the move in a direction is one of those options. A move
// off the land is never an option, so the edge of the map and water are met
// the same way as the edge of a poorer region: as a ball meets a wall. A
// heading drawn afresh at such an edge would too often run along it, and
// bodies would circle edges they had eaten bare.
func bounce(h int, best func(dir int) bool) int {
	if h < 0 || best(h) {
		return h
	}
	d := moveDirs[h]
	flipX := d[0] != 0 && !best(axisDir(d[0], 0, 4))
	flipY := d[1] != 0 && !best(axisDir(d[1], 2, 6))
	switch {
	case flipX && flipY, !flipX && !flipY:
		return (h + 4) % 8
	case flipX:
		return (12 - h) % 8
	default:
		return (8 - h) % 8
	}
}

// turnOff turns a bounce that would send a body straight back, heading h
// reversed into back, to one of the two directions 45 degrees either side
// of back: away from the wall, but not along the line the body came in on.
// A heading along an axis reflects into its own reverse, and a body that
// kept it would walk one row or column for good, seeing a strip three tiles
// wide. It takes whichever of the two is among the options of least risk,
// draws between them if both are, and keeps back if neither is.
func (w *World) turnOff(h, back int, best func(dir int) bool) int {
	l, r := (h+3)%8, (h+5)%8
	switch bl, br := best(l), best(r); {
	case bl && br:
		if w.rng.Intn(2) == 0 {
			return l
		}
		return r
	case bl:
		return l
	case br:
		return r
	}
	return back
}

// axisDir is the straight move along one axis in the sign of c: pos when c
// is positive, neg when negative.
func axisDir(c float64, pos, neg int) int {
	if c > 0 {
		return pos
	}
	return neg
}

// tied reports whether a is one of the options of least risk of the
// decision under way.
func (w *World) tied(a Action) bool {
	for _, j := range w.ties {
		if w.valuation.Options[j] == a {
			return true
		}
	}
	return false
}

// SetTrace sets a function shown every decision: the body before it acts,
// the valuation the choice was made from, and the action taken. The
// valuation's slices are reused by the next decision, so copy what is kept.
// nil turns the trace off. The trace draws nothing and changes nothing.
func (w *World) SetTrace(f func(b Body, v Valuation, a Action)) { w.trace = f }

// slowest is the config's speed at the age it is least (ChildAbility), for
// tables read by bodies of any age.
func (w *World) slowest() float64 {
	if low := w.cfg.ChildAbility; low > 0 && low < 1 {
		return w.cfg.Speed * low
	}
	return w.cfg.Speed
}

// RowStats counts the survival tables held, their kept rows and cells, for
// measuring. It reads and changes nothing.
func (w *World) RowStats() (tables, rows, cells int) {
	count := func(s Survival) {
		tables++
		for _, d := range s.dead {
			if d != nil {
				rows++
				cells += len(d)
			}
		}
	}
	for _, c := range w.pred.cache {
		for _, t := range c {
			count(t.s)
		}
	}
	for _, t := range w.pred.learned {
		count(t.s)
	}
	return
}
