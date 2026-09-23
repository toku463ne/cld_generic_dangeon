package engine

import "math"

// Prediction and valuation.
//
// This is the one place a body's options are predicted and valued (NODE.md
// "予測の唯一の実装"). Stage 1-2 values them against a table built from the
// world's true rules; stage 1-6 will read the same rows from experience
// instead. Nothing here is wired into decide yet: the world runs as in 1-1,
// and the counts of PLAN.md read these values from outside.
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
	for _, f := range w.food.foods {
		t.Meet[w.m.RegionAt(f.X, f.Y)]++
	}
	for r, land := range w.food.regionLand {
		if len(land) > 0 {
			t.Meet[r] = t.Meet[r] / float64(len(land)) * math.Min(w.cfg.Speed, 1)
		}
	}
	return t
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
// the chance of being alive at the end of each window from every amount of
// energy (in ticks of burning, 0 to Full).
type Survival struct {
	Windows []int
	// alive[i][n] is the chance of being alive Windows[i]-1 ticks on from n
	// ticks of energy: the first tick of a window is the option itself.
	alive [][]float64
}

// NewSurvival works the chances out tick by tick: in each tick the body
// meets food with chance meet, and eats it, or does not; then it burns one
// tick of energy. It is exact for the table's rows; its cost is the longest
// window times Full.
func (t TruthTable) NewSurvival(meet float64, windows []int) Survival {
	s := Survival{Windows: windows, alive: make([][]float64, len(windows))}
	longest := 0
	for _, w := range windows {
		longest = max(longest, w)
	}
	cur := make([]float64, t.Full+1)
	next := make([]float64, t.Full+1)
	for n := 1; n <= t.Full; n++ {
		cur[n] = 1
	}
	keep := func(step int) {
		for i, w := range windows {
			if w-1 == step {
				s.alive[i] = append([]float64(nil), cur...)
			}
		}
	}
	keep(0)
	for step := 1; step < longest; step++ {
		next[0] = 0
		for n := 1; n <= t.Full; n++ {
			fed := min(n+t.Meal, t.Full) - 1
			next[n] = meet*cur[fed] + (1-meet)*cur[n-1]
		}
		cur, next = next, cur
		keep(step)
	}
	return s
}

// Alive returns the chance of being alive at the end of window i from n
// ticks of energy after the option's own tick.
func (s Survival) Alive(i, n int) float64 {
	if n <= 0 {
		return 0
	}
	return s.alive[i][min(n, len(s.alive[i])-1)]
}

// Valuation is the options of one body and the worth of each in every
// window: the chance of being alive at the window's end.
type Valuation struct {
	Options []Action
	Worth   [][]float64 // Worth[window][option]
}

// Value predicts and values every option body b has now. survival is the
// table of chances per region, built once for the tick by NewSurvival from
// the same TruthTable.
func (w *World) Value(t TruthTable, survival []Survival, b Body) Valuation {
	v := Valuation{Options: w.possibleActions(nil, &b)}
	n := energyTicks(b.Energy, w.cfg.EnergyBurn)
	here := w.m.RegionAt(int(math.Floor(b.X)), int(math.Floor(b.Y)))
	windows := survival[here].Windows
	v.Worth = make([][]float64, len(windows))
	for i := range windows {
		v.Worth[i] = make([]float64, len(v.Options))
		for j, a := range v.Options {
			region, after := here, n-1
			switch a.Kind {
			case ActEat:
				after = min(n+t.Meal, t.Full) - 1
			case ActMove:
				d := moveDirs[a.Dir]
				region = w.m.RegionAt(int(math.Floor(b.X+d[0]*w.cfg.Speed)), int(math.Floor(b.Y+d[1]*w.cfg.Speed)))
			}
			v.Worth[i][j] = survival[region].Alive(i, after)
		}
	}
	return v
}
