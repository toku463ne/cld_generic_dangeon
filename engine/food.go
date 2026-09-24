package engine

// Food.
//
// Each unit sits on one land tile, at most one per tile. The total is
// conserved: FoodCap units exist, each either on the ground or vacant, and a
// vacancy comes back with chance FoodReturn per tick. A region does not make
// food of its own; it takes its RegionFood share of what comes back. That is
// what makes the contrast between regions a division of one total rather than
// new food.
//
// The ledger (FoodLedger) counts every unit that appears and every unit that
// is eaten, so that a test can check the total tick by tick.

// Food is one unit of food on a tile.
type Food struct {
	X, Y int
}

// FoodLedger is the running account of the food.
type FoodLedger struct {
	Cap      int   // FoodCap
	OnGround int   // units on the map now
	Appeared int64 // units that have appeared since the world was built
	Eaten    int64 // units that have been eaten
}

// Balanced reports whether the account closes: nothing appeared or vanished
// except by coming back and being eaten, and the map never held more than the
// cap.
func (l FoodLedger) Balanced() bool {
	return int64(l.OnGround) == l.Appeared-l.Eaten && l.OnGround <= l.Cap && l.OnGround >= 0
}

// foodState is the world's food. foodAt maps a tile to the index of the unit
// on it plus one, zero for none.
type foodState struct {
	foods  []Food
	foodAt []int32

	// owed is, per region, the fraction of a unit that region has been
	// promised by FoodReturn and not yet received. Returning food as whole
	// units from a running fraction draws nothing from the random source for
	// how many come back, only for where.
	owed []float64

	appeared, eaten int64

	// regionLand lists each region's land tiles, which is where its food can
	// appear.
	regionLand [][]int
	shareSum   float64

	// onGround is, per region, how many units are on the ground there. It
	// is all the truth table reads of the food (predict.go).
	onGround []int
}

func (w *World) initFood() {
	f := &w.food
	f.foodAt = make([]int32, len(w.m.Terrain))
	f.owed = make([]float64, len(w.m.RegionFood))
	f.regionLand = make([][]int, len(w.m.RegionFood))
	f.onGround = make([]int, len(w.m.RegionFood))
	for i, t := range w.m.Terrain {
		if t == TerrainLand {
			r := w.m.Region[i]
			f.regionLand[r] = append(f.regionLand[r], i)
		}
	}
	f.shareSum = 0
	for r, s := range w.m.RegionFood {
		if len(f.regionLand[r]) > 0 {
			f.shareSum += s
		}
	}
}

// fillFood puts the whole cap on the map, each unit in a region drawn by
// share. It is how a world starts.
func (w *World) fillFood() {
	for len(w.food.foods) < w.cfg.FoodCap {
		if !w.placeFood(w.drawRegion()) {
			return
		}
	}
}

// drawRegion picks a region with land, with chance proportional to its share.
func (w *World) drawRegion() RegionID {
	f := &w.food
	x := w.rng.Float64() * f.shareSum
	last := RegionID(0)
	for r, s := range w.m.RegionFood {
		if len(f.regionLand[r]) == 0 || s == 0 {
			continue
		}
		last = RegionID(r)
		if x < s {
			return last
		}
		x -= s
	}
	return last
}

// placeFood puts one unit on a free land tile of region r. A region whose
// tiles are all taken gives up after a few tries; the unit stays vacant and
// is owed again next tick.
func (w *World) placeFood(r RegionID) bool {
	f := &w.food
	land := f.regionLand[r]
	if len(land) == 0 {
		return false
	}
	const tries = 8
	for i := 0; i < tries; i++ {
		t := land[w.rng.Intn(len(land))]
		if f.foodAt[t] != 0 {
			continue
		}
		f.foods = append(f.foods, Food{X: t % w.m.Width, Y: t / w.m.Width})
		f.foodAt[t] = int32(len(f.foods))
		f.appeared++
		w.foodMoved(r, +1)
		return true
	}
	return false
}

// returnFood brings vacancies back. Each region is owed its share of
// FoodReturn times the vacancies at the start of the tick, and receives whole
// units as the debt passes one. Regions are served in ID order, so when the
// cap is reached mid-tick the lower IDs are the ones served; the debt of the
// others carries over.
func (w *World) returnFood() {
	f := &w.food
	vacant := w.cfg.FoodCap - len(f.foods)
	if vacant <= 0 || f.shareSum == 0 {
		return
	}
	for r, s := range w.m.RegionFood {
		if len(f.regionLand[r]) == 0 || s == 0 {
			continue
		}
		f.owed[r] += w.cfg.FoodReturn * float64(vacant) * s / f.shareSum
		for f.owed[r] >= 1 && len(f.foods) < w.cfg.FoodCap {
			if !w.placeFood(RegionID(r)) {
				break
			}
			f.owed[r]--
		}
	}
}

// foodOn returns the index of the unit on tile t, or -1.
func (w *World) foodOn(t int) int {
	return int(w.food.foodAt[t]) - 1
}

// eatFood removes unit i. The last unit moves into its slot, so indices are
// not stable across an eat.
func (w *World) eatFood(i int) {
	f := &w.food
	gone := f.foods[i]
	f.foodAt[gone.Y*w.m.Width+gone.X] = 0
	last := len(f.foods) - 1
	if i != last {
		moved := f.foods[last]
		f.foods[i] = moved
		f.foodAt[moved.Y*w.m.Width+moved.X] = int32(i + 1)
	}
	f.foods = f.foods[:last]
	f.eaten++
	w.foodMoved(w.m.RegionAt(gone.X, gone.Y), -1)
}

// foodMoved keeps the count of region r's food on the ground, and the
// prediction that reads it, up to date.
func (w *World) foodMoved(r RegionID, d int) {
	w.food.onGround[r] += d
	w.refreshRegion(r)
}

// FoodLedger returns the food account.
func (w *World) FoodLedger() FoodLedger {
	return FoodLedger{
		Cap:      w.cfg.FoodCap,
		OnGround: len(w.food.foods),
		Appeared: w.food.appeared,
		Eaten:    w.food.eaten,
	}
}

// Foods returns a copy of the food on the map.
func (w *World) Foods() []Food {
	return append([]Food(nil), w.food.foods...)
}
