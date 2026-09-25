package engine

import "math"

// Prediction and valuation.
//
// This is the one place a body's options are predicted and valued (NODE.md
// "予測の唯一の実装"). Stage 1-2 values them against a table built from the
// world's true rules; stage 1-6 will read the same rows from experience
// instead. decide takes the best option by these values, and a trace
// (SetTrace) is handed the very Valuation the choice was made from.
//
// The table holds outcomes only, never worth (NODE.md "記憶は結果だけを持つ"):
//
//	(tile with food, eat)                -> energy +FoodEnergy, up to EnergyMax
//	(land, wait or move)                 -> energy -EnergyBurn, position moved by Speed
//	(this region, keep moving)           -> stands on a tile with food, chance p per tile entered
//	(tile with food in sight, walk to it) -> stands on it after d/Speed ticks
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
	// Meal is how many ticks of energy one meal gives (FoodEnergy over
	// EnergyBurn), and Full how many a full body holds.
	Meal, Full int
	// Meet is, per region, the chance per tick that a body keeping on the
	// move comes to stand on food: the food on the ground over the land
	// tiles, times the tiles entered per tick (Speed).
	Meet []float64
	// Reach is the most ticks the fourth row can take to walk to a unit in
	// sight; survival tables keep the shorter windows the walk leaves.
	Reach int
}

// TruthTable reads the true rules and the food on the ground now.
func (w *World) TruthTable() TruthTable {
	t := TruthTable{
		Meal:  energyTicks(w.cfg.FoodEnergy, w.cfg.EnergyBurn),
		Full:  energyTicks(w.cfg.EnergyMax, w.cfg.EnergyBurn),
		Meet:  make([]float64, len(w.m.RegionFood)),
		Reach: w.reach(),
	}
	for r := range t.Meet {
		t.Meet[r] = w.meet(RegionID(r))
	}
	return t
}

// meet is the third row of the table for region r, read from the food on
// its ground now.
func (w *World) meet(r RegionID) float64 {
	land := len(w.food.regionLand[r])
	if land == 0 {
		return 0
	}
	return float64(w.food.onGround[r]) / float64(land) * math.Min(w.cfg.Speed, 1)
}

// reach bounds the ticks it takes to walk onto a unit in sight from where
// any option can leave the body: the unit is up to Sight tiles from the
// body's tile on either axis, and a move takes the body up to Speed out of
// it, so each axis has at most Sight+Speed to cover.
func (w *World) reach() int {
	if w.cfg.Sight < 0 || w.cfg.Speed <= 0 {
		return 0
	}
	gap := float64(w.cfg.Sight) + w.cfg.Speed
	return walkTicks(gap, gap, w.cfg.Speed)
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

// Survival is, for one chance of meeting food per tick and a set of windows,
// the chance of being dead at the end of each window from every amount of
// energy (in ticks of burning, 0 to Full).
//
// It holds the chance of dying rather than of living on because the two
// differ in what float64 can tell apart: where food is plentiful both
// options' chances of living on round to exactly 1, and the difference
// between eating and not eating is lost; their chances of dying, 1e-9 and
// 1e-20 say, are kept. The quantity valued is the same.
type Survival struct {
	Windows []int
	// dead[l][n] is the chance of being dead l-1 ticks on from n ticks of
	// energy: the first tick of a window is the option itself. It is kept
	// for each window and for the shorter ones a walk of up to Reach ticks
	// leaves of it; nil for every other length.
	dead [][]float64
}

// NewSurvival works the chances out tick by tick: in each tick the body
// meets food with chance meet, and eats it, or does not; then it burns one
// tick of energy. It is exact for the table's rows; its cost is the longest
// window times Full.
func (t TruthTable) NewSurvival(meet float64, windows []int) Survival {
	longest := 0
	for _, w := range windows {
		longest = max(longest, w)
	}
	s := Survival{Windows: windows, dead: make([][]float64, longest+1)}
	want := make([]bool, longest+1)
	for _, w := range windows {
		for l := max(w-1-t.Reach, 1); l <= w; l++ {
			want[l] = true
		}
	}
	cur := make([]float64, t.Full+1)
	next := make([]float64, t.Full+1)
	cur[0] = 1
	keep := func(step int) {
		if want[step+1] {
			s.dead[step+1] = append([]float64(nil), cur...)
		}
	}
	keep(0)
	for step := 1; step < longest; step++ {
		next[0] = 1
		for n := 1; n <= t.Full; n++ {
			fed := min(n+t.Meal, t.Full) - 1
			next[n] = meet*cur[fed] + (1-meet)*cur[n-1]
		}
		cur, next = next, cur
		keep(step)
	}
	return s
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
	d := s.dead[l]
	return d[min(n, len(d)-1)]
}

// Alive is 1 - Dead, for reading; it loses what Dead keeps near 1.
func (s Survival) Alive(i, n int) float64 { return 1 - s.Dead(i, n) }

// Valuation is the options of one body and the risk of each in every
// window: the chance of being dead at the window's end. The lower the risk,
// the better the option; the worth of an option in the survival currency is
// the risk it takes away.
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
	Seen    []Food
	Plan    []int
	Arrive  []int
}

// Value predicts and values every option body b has now. survival is the
// table of chances per region, built by NewSurvival from the same
// TruthTable.
func (w *World) Value(t TruthTable, survival []Survival, b Body) Valuation {
	var v Valuation
	w.valueInto(&v, t.Meal, t.Full, survival[0].Windows, func(r RegionID) Survival { return survival[r] }, b)
	return v
}

// valueInto is Value writing into v's slices, so that deciding every tick
// does not allocate. alive gives the survival table of a region.
func (w *World) valueInto(v *Valuation, meal, full int, windows []int, alive func(RegionID) Survival, b Body) {
	v.Options = w.possibleActions(v.Options[:0], &b)
	v.Seen = w.inSight(v.Seen[:0], &b)
	v.Plan, v.Arrive = v.Plan[:0], v.Arrive[:0]
	n := energyTicks(b.Energy, w.cfg.EnergyBurn)
	for len(v.Risk) < len(windows) {
		v.Risk = append(v.Risk, nil)
	}
	v.Risk = v.Risk[:len(windows)]
	for i := range v.Risk {
		v.Risk[i] = v.Risk[i][:0]
	}
	for _, a := range v.Options {
		// Where the option leaves the body, and with how much energy.
		x, y, after := b.X, b.Y, n-1
		eaten := -1
		switch a.Kind {
		case ActEat:
			after = min(n+meal, full) - 1
			eaten = w.m.index(int(math.Floor(b.X)), int(math.Floor(b.Y)))
		case ActMove:
			d := moveDirs[a.Dir]
			x, y = b.X+d[0]*w.cfg.Speed, b.Y+d[1]*w.cfg.Speed
		}
		region := w.m.RegionAt(int(math.Floor(x)), int(math.Floor(y)))
		for i, win := range windows {
			// Keep moving through the region.
			risk, plan, arrive := alive(region).deadIn(win, after), -1, 0
			// Or walk to a unit in sight: k ticks of walking, then a tick
			// of eating, leave win-2-k ticks of the window, which is a
			// window of win-1-k from the energy after the meal.
			for f, food := range v.Seen {
				if w.m.index(food.X, food.Y) == eaten {
					continue
				}
				k := walkTicks(gapTo(x, food.X), gapTo(y, food.Y), w.cfg.Speed)
				r := 1.0
				if after-k > 0 {
					r = alive(w.m.RegionAt(food.X, food.Y)).deadIn(win-1-k, min(after-k+meal, full)-1)
				}
				if r < risk {
					risk, plan, arrive = r, f, k
				}
			}
			v.Risk[i] = append(v.Risk[i], risk)
			if i == 0 {
				v.Plan = append(v.Plan, plan)
				v.Arrive = append(v.Arrive, arrive)
			}
		}
	}
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
	if w.cfg.Sight < 0 {
		return dst
	}
	bx, by := int(math.Floor(b.X)), int(math.Floor(b.Y))
	for y := by - w.cfg.Sight; y <= by+w.cfg.Sight; y++ {
		for x := bx - w.cfg.Sight; x <= bx+w.cfg.Sight; x++ {
			if w.m.InBounds(x, y) && w.foodOn(w.m.index(x, y)) >= 0 {
				dst = append(dst, Food{X: x, Y: y})
			}
		}
	}
	return dst
}

// predictor is what deciding needs to value options against the world's own
// window: the table's two fixed rows, and one survival table per region for
// its food on the ground now. A survival table depends on nothing but the
// region's food count, so each is built once per count and kept. All of it
// can be rebuilt from the rest of the world.
type predictor struct {
	meal, full int
	windows    []int
	surv       []Survival
	fresh      []bool
	cache      []map[int]Survival // per region, by food on the ground
}

// initPredict sets the predictor up for the world's window. With no window
// there is nothing to predict.
func (w *World) initPredict() {
	w.pred = predictor{}
	if w.cfg.Window <= 0 {
		return
	}
	n := len(w.m.RegionFood)
	w.pred = predictor{
		meal:    energyTicks(w.cfg.FoodEnergy, w.cfg.EnergyBurn),
		full:    energyTicks(w.cfg.EnergyMax, w.cfg.EnergyBurn),
		windows: []int{w.cfg.Window},
		surv:    make([]Survival, n),
		fresh:   make([]bool, n),
		cache:   make([]map[int]Survival, n),
	}
	for r := range w.pred.cache {
		w.pred.cache[r] = map[int]Survival{}
	}
}

// refreshRegion marks region r's survival table stale after its food
// changed. It is rebuilt, or found in the cache, when next asked for.
func (w *World) refreshRegion(r RegionID) {
	if w.pred.fresh != nil {
		w.pred.fresh[r] = false
	}
}

// survival returns region r's survival table for its food on the ground now.
func (w *World) survival(r RegionID) Survival {
	p := &w.pred
	if !p.fresh[r] {
		n := w.food.onGround[r]
		s, ok := p.cache[r][n]
		if !ok {
			t := TruthTable{Meal: p.meal, Full: p.full, Reach: w.reach()}
			s = t.NewSurvival(w.meet(r), p.windows)
			p.cache[r][n] = s
		}
		p.surv[r], p.fresh[r] = s, true
	}
	return p.surv[r]
}

// decide values body b's options and takes the one of least risk. Among
// options of equal risk it keeps the body's heading, reflected off whatever
// makes that way worse (bounce), if KeepHeading is set and moving that way
// is one of them, and draws at random otherwise. With no window every option carries
// the same risk, so the draw is over all of them: the stage 1-1 control.
func (w *World) decide(b *Body) Action {
	v := &w.valuation
	if w.cfg.Window > 0 {
		w.valueInto(v, w.pred.meal, w.pred.full, w.pred.windows, w.survival, *b)
	} else {
		v.Options = w.possibleActions(v.Options[:0], b)
		v.Seen, v.Plan, v.Arrive = v.Seen[:0], v.Plan[:0], v.Arrive[:0]
		if cap(v.Risk) == 0 {
			v.Risk = make([][]float64, 1)
		}
		v.Risk = v.Risk[:1]
		v.Risk[0] = v.Risk[0][:0]
		for range v.Options {
			v.Risk[0] = append(v.Risk[0], 0)
		}
	}
	best := math.Inf(1)
	w.ties = w.ties[:0]
	for j, x := range v.Risk[0] {
		switch {
		case x < best:
			best = x
			w.ties = append(w.ties[:0], j)
		case x == best:
			w.ties = append(w.ties, j)
		}
	}
	var a Action
	if w.cfg.KeepHeading && b.Heading >= 0 {
		best := func(d int) bool { return w.tied(Action{Kind: ActMove, Dir: d}) }
		h := bounce(b.Heading, best)
		if w.cfg.TurnOffReverse && h == (b.Heading+4)%8 {
			h = w.turnOff(b.Heading, h, best)
		}
		a = Action{Kind: ActMove, Dir: h}
	}
	if !w.cfg.KeepHeading || b.Heading < 0 || !w.tied(a) {
		a = v.Options[w.ties[w.rng.Intn(len(w.ties))]]
	}
	if w.trace != nil {
		w.trace(*b, *v, a)
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
