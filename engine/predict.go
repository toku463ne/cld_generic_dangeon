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
//	(tile with food, eat)      -> energy +FoodEnergy, up to EnergyMax
//	(land, wait or move)       -> energy -EnergyBurn, position moved by Speed
//	(this region, keep moving) -> stands on a tile with food, chance p per tile entered
//
// The third row's outcome is the first row's subject, so the two chain into
// "keep moving -> energy back, chance p per tile". The worth of an option is
// computed from these each time: the chance of being alive at the end of the
// window if the body takes the option now and then keeps moving through the
// region it is in, eating whenever it stands on food.

// TruthTable is the table of stage 1-2, filled from the world's true rules.
type TruthTable struct {
	// Meal is how many ticks of energy one meal gives (FoodEnergy over
	// EnergyBurn), and Full how many a full body holds.
	Meal, Full int
	// Meet is, per region, the chance per tick that a body keeping on the
	// move comes to stand on food: the food on the ground over the land
	// tiles, times the tiles entered per tick (Speed).
	Meet []float64
}

// TruthTable reads the true rules and the food on the ground now.
func (w *World) TruthTable() TruthTable {
	t := TruthTable{
		Meal: energyTicks(w.cfg.FoodEnergy, w.cfg.EnergyBurn),
		Full: energyTicks(w.cfg.EnergyMax, w.cfg.EnergyBurn),
		Meet: make([]float64, len(w.m.RegionFood)),
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
	// dead[i][n] is the chance of being dead Windows[i]-1 ticks on from n
	// ticks of energy: the first tick of a window is the option itself.
	dead [][]float64
}

// NewSurvival works the chances out tick by tick: in each tick the body
// meets food with chance meet, and eats it, or does not; then it burns one
// tick of energy. It is exact for the table's rows; its cost is the longest
// window times Full.
func (t TruthTable) NewSurvival(meet float64, windows []int) Survival {
	s := Survival{Windows: windows, dead: make([][]float64, len(windows))}
	longest := 0
	for _, w := range windows {
		longest = max(longest, w)
	}
	cur := make([]float64, t.Full+1)
	next := make([]float64, t.Full+1)
	cur[0] = 1
	keep := func(step int) {
		for i, w := range windows {
			if w-1 == step {
				s.dead[i] = append([]float64(nil), cur...)
			}
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
func (s Survival) Dead(i, n int) float64 {
	if n <= 0 {
		return 1
	}
	return s.dead[i][min(n, len(s.dead[i])-1)]
}

// Alive is 1 - Dead, for reading; it loses what Dead keeps near 1.
func (s Survival) Alive(i, n int) float64 { return 1 - s.Dead(i, n) }

// Valuation is the options of one body and the risk of each in every
// window: the chance of being dead at the window's end. The lower the risk,
// the better the option; the worth of an option in the survival currency is
// the risk it takes away.
type Valuation struct {
	Options []Action
	Risk    [][]float64 // Risk[window][option]
}

// Value predicts and values every option body b has now. survival is the
// table of chances per region, built by NewSurvival from the same
// TruthTable.
func (w *World) Value(t TruthTable, survival []Survival, b Body) Valuation {
	var v Valuation
	w.valueInto(&v, t.Meal, t.Full, len(survival[0].Windows), func(r RegionID) Survival { return survival[r] }, b)
	return v
}

// valueInto is Value writing into v's slices, so that deciding every tick
// does not allocate. alive gives the survival table of a region.
func (w *World) valueInto(v *Valuation, meal, full, windows int, alive func(RegionID) Survival, b Body) {
	v.Options = w.possibleActions(v.Options[:0], &b)
	n := energyTicks(b.Energy, w.cfg.EnergyBurn)
	here := w.m.RegionAt(int(math.Floor(b.X)), int(math.Floor(b.Y)))
	for len(v.Risk) < windows {
		v.Risk = append(v.Risk, nil)
	}
	v.Risk = v.Risk[:windows]
	for i := range v.Risk {
		v.Risk[i] = v.Risk[i][:0]
	}
	for _, a := range v.Options {
		region, after := here, n-1
		switch a.Kind {
		case ActEat:
			after = min(n+meal, full) - 1
		case ActMove:
			d := moveDirs[a.Dir]
			region = w.m.RegionAt(int(math.Floor(b.X+d[0]*w.cfg.Speed)), int(math.Floor(b.Y+d[1]*w.cfg.Speed)))
		}
		s := alive(region)
		for i := range v.Risk {
			v.Risk[i] = append(v.Risk[i], s.Dead(i, after))
		}
	}
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
			t := TruthTable{Meal: p.meal, Full: p.full}
			s = t.NewSurvival(w.meet(r), p.windows)
			p.cache[r][n] = s
		}
		p.surv[r], p.fresh[r] = s, true
	}
	return p.surv[r]
}

// decide values body b's options and takes the one of least risk, drawing
// at random among those of equal risk. With no window every option carries
// the same risk, so the draw is over all of them: the stage 1-1 control.
func (w *World) decide(b *Body) Action {
	v := &w.valuation
	if w.cfg.Window > 0 {
		w.valueInto(v, w.pred.meal, w.pred.full, 1, w.survival, *b)
	} else {
		v.Options = w.possibleActions(v.Options[:0], b)
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
	a := v.Options[w.ties[w.rng.Intn(len(w.ties))]]
	if w.trace != nil {
		w.trace(*b, *v, a)
	}
	return a
}

// SetTrace sets a function shown every decision: the body before it acts,
// the valuation the choice was made from, and the action taken. The
// valuation's slices are reused by the next decision, so copy what is kept.
// nil turns the trace off. The trace draws nothing and changes nothing.
func (w *World) SetTrace(f func(b Body, v Valuation, a Action)) { w.trace = f }
