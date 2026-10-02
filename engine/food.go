package engine

// Food.
//
// Each unit sits on one land tile, at most one per tile. The total is
// conserved: FoodCap units exist, each either on the ground or vacant, and a
// vacancy comes back with chance FoodReturn per tick. A region does not make
// food of its own; it takes its RegionFood share of what comes back. That is
// what makes the contrast between regions a division of one total rather than
// new food. On a map with seasons (Map.SeasonFood) the shares are the
// season's; food on the ground does not move, so it follows the seasons late.
//
// A unit can also be held by a body (Carry): it is then neither on the
// ground nor vacant. A unit held by a body that dies is lost: it becomes a
// vacancy.
//
// The ledger (FoodLedger) counts every unit that appears, is eaten or is
// lost, so that a test can check the total tick by tick.

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
	// Provisioned counts the units that went straight to a resting mother
	// (Provision): each appeared and was eaten in the same tick.
	Provisioned int64
	// Held is the units bodies hold now (Carry), and Picked and Lost the
	// units picked up, and held by bodies that died, since the world was
	// built.
	Held         int
	Picked, Lost int64
}

// Balanced reports whether the account closes: nothing appeared or vanished
// except by coming back and being eaten, and the map never held more than the
// cap.
func (l FoodLedger) Balanced() bool {
	return int64(l.OnGround+l.Held) == l.Appeared-l.Eaten-l.Lost && l.OnGround+l.Held <= l.Cap && l.OnGround >= 0 && l.Held >= 0
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

	appeared, eaten, provisioned int64
	// held is the units bodies hold now; picked and lost count units
	// picked up and units held by bodies that died (Carry).
	held         int
	picked, lost int64

	// regionLand lists each region's land tiles, which is where its food can
	// appear.
	regionLand [][]int

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
}

// shareSum is the sum of the shares, now, of the regions with land.
func (w *World) shareSum(shares []float64) float64 {
	sum := 0.0
	for r, s := range shares {
		if len(w.food.regionLand[r]) > 0 {
			sum += s
		}
	}
	return sum
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
	shares := w.m.FoodShares(w.tick)
	x := w.rng.Float64() * w.shareSum(shares)
	last := RegionID(0)
	for r, s := range shares {
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
	vacant := w.cfg.FoodCap - len(f.foods) - f.held
	shares := w.m.FoodShares(w.tick)
	sum := w.shareSum(shares)
	if vacant <= 0 || sum == 0 {
		return
	}
	w.askProvision()
	var resting [][]int // per region, the indices of the mothers resting there
	for r, s := range shares {
		if len(f.regionLand[r]) == 0 || s == 0 {
			continue
		}
		f.owed[r] += w.cfg.FoodReturn * float64(vacant) * s / sum
		for f.owed[r] >= 1 && len(f.foods)+f.held < w.cfg.FoodCap {
			if resting == nil {
				resting = w.restingMothers()
			}
			if w.provision(RegionID(r), &resting[r]) {
				f.owed[r]--
				continue
			}
			if !w.placeFood(RegionID(r)) {
				break
			}
			f.owed[r]--
		}
	}
}

// restingMothers lists, per region, the indices of the mothers resting
// there (with ProvisionEach, those who have asked), in the order of the
// bodies; nothing without Provision.
func (w *World) restingMothers() [][]int {
	out := make([][]int, len(w.m.RegionFood))
	if !w.cfg.Provision || !w.bears() {
		return out
	}
	for i := range w.bodies {
		b := &w.bodies[i]
		if w.resting(b) && (!w.cfg.ProvisionEach || b.Asks > 0) {
			r := w.m.Region[w.tileOf(b.X, b.Y)]
			out[r] = append(out[r], i)
		}
	}
	return out
}

// provision gives a unit returning to region r to one of the mothers
// resting there (the indices in mothers), drawn at random, with chance
// ProvisionWith if the father of her last child stands in r now and
// ProvisionAlone if not. It reports whether it did: the unit then appeared
// and was eaten at once, and never lay on the ground.
func (w *World) provision(r RegionID, list *[]int) bool {
	mothers := *list
	if len(mothers) == 0 {
		return false
	}
	k := w.rng.Intn(len(mothers))
	m := &w.bodies[mothers[k]]
	if w.cfg.ProvisionEach {
		// She asked: the unit is hers. Once she has no asks left she
		// leaves the list, so the next unit goes to another who asked.
		m.Asks--
		if m.Asks == 0 {
			mothers[k] = mothers[len(mothers)-1]
			*list = mothers[:len(mothers)-1]
		}
	} else {
		p := w.cfg.ProvisionAlone
		if w.partnerNear(m, r) {
			p = w.cfg.ProvisionWith
		}
		if w.rng.Float64() >= p {
			return false
		}
	}
	m.Energy = min(m.Energy+w.cfg.FoodEnergy, w.maxOf(m))
	w.food.appeared++
	w.food.eaten++
	w.food.provisioned++
	w.provisioned(r)
	return true
}

// FoodOn reports whether tile t holds food now. It reads and changes
// nothing.
func (w *World) FoodOn(t int) bool { return w.foodOn(t) >= 0 }

// foodOn returns the index of the unit on tile t, or -1.
func (w *World) foodOn(t int) int {
	return int(w.food.foodAt[t]) - 1
}

// eatFood removes unit i, eaten. The last unit moves into its slot, so
// indices are not stable across an eat.
func (w *World) eatFood(i int) {
	w.liftFood(i)
	w.food.eaten++
}

// takeFood lifts unit i off the ground into a body's hold (Carry).
func (w *World) takeFood(i int) {
	w.liftFood(i)
	w.food.held++
	w.food.picked++
}

// eatHeld has body b eat the units it holds while a whole meal fits; it
// spends no turn (Carry). A body eats what it holds as soon as nothing of it
// is lost, which is when its valuation reads it.
func (w *World) eatHeld(b *Body) {
	for b.Held > 0 && b.Energy <= w.maxOf(b)-w.cfg.FoodEnergy {
		b.Held--
		b.Energy += w.cfg.FoodEnergy
		w.food.held--
		w.food.eaten++
		w.stats.EatenHeld++
	}
}

// loseHeld drops what a dead body held out of the world: the units become
// vacancies (Carry).
func (w *World) loseHeld(b *Body) {
	w.food.held -= b.Held
	w.food.lost += int64(b.Held)
	b.Held = 0
}

// liftFood takes unit i off the ground. The last unit moves into its slot.
func (w *World) liftFood(i int) {
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

		Provisioned: w.food.provisioned,
		Held:        w.food.held,
		Picked:      w.food.picked,
		Lost:        w.food.lost,
	}
}

// Foods returns a copy of the food on the map.
func (w *World) Foods() []Food {
	return append([]Food(nil), w.food.foods...)
}

// partnerNear reports whether the father of mother m's last child stands in
// region r now.
func (w *World) partnerNear(m *Body, r RegionID) bool {
	for i := range w.bodies {
		if o := &w.bodies[i]; o.ID == m.Partner {
			return w.m.Region[w.tileOf(o.X, o.Y)] == r
		}
	}
	return false
}

// askProvision has every resting mother ask for a unit (ProvisionEach):
// with ProvisionEachWith if her partner is in her region, ProvisionEachAlone
// if not, holding at most two asks. A mother not resting holds none.
func (w *World) askProvision() {
	if !w.cfg.Provision || !w.cfg.ProvisionEach || !w.bears() {
		return
	}
	for i := range w.bodies {
		m := &w.bodies[i]
		if !w.resting(m) {
			m.Asks = 0
			continue
		}
		r := w.m.Region[w.tileOf(m.X, m.Y)]
		p := w.cfg.ProvisionEachAlone
		if w.partnerNear(m, r) {
			p = w.cfg.ProvisionEachWith
		}
		if w.rng.Float64() < p && m.Asks < 2 {
			m.Asks++
		}
	}
}
