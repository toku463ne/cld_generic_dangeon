package engine

import "fmt"

// Food.
//
// Food comes in kinds and pools (stage 9-7b-1, VISION "沸く食料"). A kind
// (FoodKind) is the label a body sees: the unit the species' tags attach
// to, what the client draws apart, and the subject of a memory row. A pool
// (FoodSpec, the "ID" of VISION) is a conservation law of its own: a share
// of FoodCap of one kind, with a return rate. Two pools may hold the same
// kind in different places at different rates; one species may eat several
// kinds. A world with no tables has one kind every species eats and one
// pool of the whole cap: the world before this stage.
//
// A species whose name is not among a kind's tags cannot eat it, and to
// its bodies the food is not there - not in sight, not remembered, not
// evidence, not an option (foodFor, own.go): the same shape as another's
// owned thing. No rule reads the kind otherwise.
//
// Each unit sits on one land tile, at most one per tile. Each pool's total
// is conserved: its cap of units exist, each either on the ground or
// vacant, and a vacancy comes back with chance Return per tick. A region
// does not make food of its own; it takes its RegionFood share of what
// comes back. That is what makes the contrast between regions a division
// of one total rather than new food. On a map with seasons
// (Map.SeasonFood) the shares are the season's; food on the ground does
// not move, so it follows the seasons late.
//
// A unit can also be held by a body (Carry): it is then neither on the
// ground nor vacant. A unit held by a body that dies is lost: it becomes a
// vacancy of its pool.
//
// A unit lasts FoodLife ticks from when it appears, on the ground or held
// (stage 4-4); then it decays and becomes a vacancy. Without it, food on land
// no body can reach any more would hold the cap there for ever.
//
// The ledger (FoodLedger) counts every unit that appears, is eaten or is
// lost, per pool and in all, so that a test can check each total tick by
// tick.

// FoodKind is one kind of food: what a body sees, and who may eat it.
type FoodKind struct {
	Name string
	// Tags names the species that may eat it (SpeciesSpec.Name; "human"
	// is the name of a world without a table).
	Tags []string
}

// FoodSpec is one pool of food (VISION's ID): a kind, its share of
// FoodCap, and its own return rate (zero: FoodReturn).
type FoodSpec struct {
	Name   string
	Kind   int
	Share  float64
	Return float64
	// Terrain is where its units lie (stage 9-7b-2c): land unless said.
	Terrain Terrain `json:",omitempty"`
	// Seed and SeedRate make the pool a plant (stage 9-7b-3, VISION
	// "植物", the user's choice of form A): every unit on the ground drops
	// a seed with chance SeedRate a tick, and the seed grows into a unit
	// on a free tile within Seed tiles of it, on the pool's terrain in a
	// region of the pool's shares - so the pool spreads from where it
	// grows, as fast as it is large. A seed that falls on no such tile is
	// lost, and no seed grows past the cap: the total stays conserved.
	// Start is the plant's seedbed, where the map says it grows: while
	// fewer than Start units stand, the pool also comes back by its
	// shares at its return rate, as any pool does (a plant grazed to a
	// few units does not die out with them); the first fill is Start
	// units too, the rest vacant, so that the plant can be seen to spread.
	// Seed zero for a pool that comes back by its shares alone.
	Seed     int     `json:",omitempty"`
	SeedRate float64 `json:",omitempty"`
	Start    int     `json:",omitempty"`
	// Kill makes the pool the meat of the hunt (stage 10-2a, hunt.go):
	// its units come only from prey struck down, so it never fills nor
	// returns and takes no share; its cap is FoodCap.
	Kill bool `json:",omitempty"`
}

// Food is one unit of food on a tile, of pool ID.
type Food struct {
	X, Y int
	ID   int `json:",omitempty"`
}

// PoolLedger is the running account of one pool.
type PoolLedger struct {
	Name     string
	Kind     int
	Cap      int   // its share of FoodCap, in units
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
	// Decayed counts the units that decayed (FoodLife), on the ground or
	// held.
	Decayed int64
}

// Balanced reports whether the pool's account closes.
func (l PoolLedger) Balanced() bool {
	return int64(l.OnGround+l.Held) == l.Appeared-l.Eaten-l.Lost-l.Decayed && l.OnGround+l.Held <= l.Cap && l.OnGround >= 0 && l.Held >= 0
}

// FoodLedger is the running account of the food: the sum of the pools,
// and each pool.
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
	// Decayed counts the units that decayed (FoodLife), on the ground or
	// held.
	Decayed int64
	// Pools is the account of each pool, in the order of Config.Foods.
	Pools []PoolLedger
}

// Balanced reports whether the account closes: in every pool nothing
// appeared or vanished except by coming back and being eaten, and the map
// never held more than the cap.
func (l FoodLedger) Balanced() bool {
	sum := 0
	for _, p := range l.Pools {
		if !p.Balanced() {
			return false
		}
		sum += p.Cap
	}
	return int64(l.OnGround+l.Held) == l.Appeared-l.Eaten-l.Lost-l.Decayed && l.OnGround+l.Held <= l.Cap && sum == l.Cap
}

// pool is the state of one pool of food.
type pool struct {
	name     string
	kind     int
	cap      int
	ret      float64
	terrain  Terrain
	seed     int
	seedRate float64
	start    int
	kill     bool
	// sown is the fraction of a seed a plant pool's units have dropped
	// and not yet grown; bed the fraction of a unit its seedbed is owed
	// (Start).
	sown, bed float64
	// owed is, per region, the fraction of a unit that region has been
	// promised by the return rate and not yet received. Returning food as
	// whole units from a running fraction draws nothing from the random
	// source for how many come back, only for where.
	owed []float64
	// onGround is, per region, how many units are on the ground there. It
	// is all the survival tables read of the food (predict.go).
	onGround []int
	// count is the units of the pool on the ground now, held the units
	// bodies hold; the rest count what has happened since the world was
	// built.
	count, held                                         int
	appeared, eaten, provisioned, picked, lost, decayed int64
}

// foodState is the world's food. foodAt maps a tile to the index of the unit
// on it plus one, zero for none.
type foodState struct {
	foods  []Food
	foodAt []int32
	// owner is each unit's owner, in the order of foods (noOwner: none;
	// own.go).
	owner []int64
	// born is the tick each unit of foods appeared, in the same order.
	born []int64

	pools []pool
	// eats says, per species and pool, whether the species may eat the
	// pool's kind.
	eats [][]bool

	// regionTiles lists, per terrain and region, the region's tiles of
	// that terrain: where a pool of that terrain's food can appear, and
	// what a body that may enter it walks over.
	regionTiles [NumTerrains][][]int

	scratch []int
}

// validFood reports whether the food tables can be used with the species.
func validFood(kinds []FoodKind, pools []FoodSpec) error {
	if (len(kinds) == 0) != (len(pools) == 0) {
		return fmt.Errorf("FoodKinds and Foods must both be set or both be empty, got %d kinds and %d pools", len(kinds), len(pools))
	}
	seen := map[string]bool{}
	for _, k := range kinds {
		if k.Name == "" || seen[k.Name] {
			return fmt.Errorf("food kinds' names must be set and distinct, got %q", k.Name)
		}
		seen[k.Name] = true
		if len(k.Tags) == 0 {
			return fmt.Errorf("food kind %s has no species tag", k.Name)
		}
	}
	seen = map[string]bool{}
	sum := 0.0
	for _, p := range pools {
		if p.Name == "" || seen[p.Name] {
			return fmt.Errorf("food pools' names must be set and distinct, got %q", p.Name)
		}
		seen[p.Name] = true
		if p.Kind < 0 || p.Kind >= len(kinds) {
			return fmt.Errorf("food pool %s is of kind %d, have %d kinds", p.Name, p.Kind, len(kinds))
		}
		if p.Terrain >= NumTerrains {
			return fmt.Errorf("food pool %s lies on unknown terrain %d", p.Name, p.Terrain)
		}
		if p.Seed < 0 || p.Start < 0 || !(p.SeedRate >= 0) {
			return fmt.Errorf("food pool %s has seed %d, seed rate %v and start %d", p.Name, p.Seed, p.SeedRate, p.Start)
		}
		if (p.Seed > 0) != (p.SeedRate > 0) {
			return fmt.Errorf("food pool %s needs both Seed and SeedRate, or neither, got %d and %v", p.Name, p.Seed, p.SeedRate)
		}
		if p.Seed > 0 && p.Start <= 0 {
			return fmt.Errorf("food pool %s is a plant and needs a seedbed (Start above zero)", p.Name)
		}
		if !(p.Share >= 0) || !(p.Return >= 0) {
			return fmt.Errorf("food pool %s has share %v and return %v", p.Name, p.Share, p.Return)
		}
		if p.Kill && (p.Share != 0 || p.Seed != 0) {
			return fmt.Errorf("kill pool %s takes no share and does not spread", p.Name)
		}
		if !p.Kill {
			sum += p.Share
		}
	}
	if len(pools) > 0 && !(sum > 0) {
		return fmt.Errorf("the food pools' shares sum to %v", sum)
	}
	return nil
}

func (w *World) initFood() {
	f := &w.food
	f.foodAt = make([]int32, len(w.m.Terrain))
	for t := range f.regionTiles {
		f.regionTiles[t] = make([][]int, len(w.m.RegionFood))
	}
	for i, t := range w.m.Terrain {
		r := w.m.Region[i]
		f.regionTiles[t][r] = append(f.regionTiles[t][r], i)
	}
	kinds, specs := w.cfg.FoodKinds, w.cfg.Foods
	if len(specs) == 0 {
		// One kind every species eats, one pool of the whole cap.
		kinds = []FoodKind{{Name: "food"}}
		specs = []FoodSpec{{Name: "food", Share: 1}}
	}
	sum := 0.0
	for _, s := range specs {
		if !s.Kill {
			sum += s.Share
		}
	}
	// Caps are the shares of FoodCap, rounded so that they add up to it;
	// a kill pool stands outside the split with FoodCap as its cap.
	given, cum := 0, 0.0
	f.pools = make([]pool, len(specs))
	for i, s := range specs {
		cap := w.cfg.FoodCap
		if !s.Kill {
			cum += s.Share / sum
			cap = int(cum*float64(w.cfg.FoodCap)+0.5) - given
			given += cap
		}
		f.pools[i] = pool{name: s.Name, kind: s.Kind, cap: cap, ret: s.Return, terrain: s.Terrain, seed: s.Seed, seedRate: s.SeedRate, start: s.Start, kill: s.Kill,
			owed: make([]float64, len(w.m.RegionFood)), onGround: make([]int, len(w.m.RegionFood))}
		if f.pools[i].ret == 0 {
			f.pools[i].ret = w.cfg.FoodReturn
		}
	}
	f.eats = make([][]bool, w.SpeciesCount())
	for s := range f.eats {
		f.eats[s] = make([]bool, len(specs))
		for p, spec := range specs {
			f.eats[s][p] = len(w.cfg.Foods) == 0 || tagged(kinds[spec.Kind].Tags, w.SpeciesName(s))
		}
	}
}

// tagged reports whether name is among tags.
func tagged(tags []string, name string) bool {
	for _, t := range tags {
		if t == name {
			return true
		}
	}
	return false
}

// eats reports whether body b may eat the food of pool p.
func (w *World) eats(b *Body, p int) bool { return w.food.eats[b.Species][p] }

// shareSum is the sum of the shares, now, of the regions with tiles of
// terrain t.
func (w *World) shareSum(shares []float64, t Terrain) float64 {
	sum := 0.0
	for r, s := range shares {
		if len(w.food.regionTiles[t][r]) > 0 {
			sum += s
		}
	}
	return sum
}

// regionLand is region r's ground tiles (land and forest): what the
// config body, at home on every ground, walks.
func (w *World) regionLand(r RegionID) []int {
	if len(w.food.regionTiles[TerrainForest][r]) == 0 {
		return w.food.regionTiles[TerrainLand][r]
	}
	return append(append([]int(nil), w.food.regionTiles[TerrainLand][r]...), w.food.regionTiles[TerrainForest][r]...)
}

// areaOf is the tiles of region r a body of species s lives on: its own
// terrains, not water it could wade into (the tables read the ground it
// walks, not the ground it could drown on).
func (w *World) areaOf(s int, r RegionID) int {
	n := 0
	for t := range w.food.regionTiles {
		if w.specOf(s).enters(Terrain(t)) {
			n += len(w.food.regionTiles[t][r])
		}
	}
	return n
}

// poolShares is the regions' shares of pool p's food that comes back now:
// the map's for that pool (PoolPlaces), or the map's for all food.
func (w *World) poolShares(p int) []float64 {
	if w.cfg.PoolPlaces {
		return w.m.PoolShares(w.food.pools[p].name, w.tick)
	}
	return w.m.FoodShares(w.tick)
}

// fillFood puts every pool's whole cap on the map (or its Start), each
// unit in a region drawn by the pool's share. It is how a world starts.
func (w *World) fillFood() {
	for p := range w.food.pools {
		if w.food.pools[p].kill {
			continue // nothing but a kill lays it
		}
		want := w.food.pools[p].cap
		if s := w.cfg.Foods; p < len(s) && s[p].Start > 0 {
			want = min(s[p].Start, want)
		}
		for w.food.pools[p].count < want {
			if !w.placeFood(w.drawRegion(w.poolShares(p), w.food.pools[p].terrain), p) {
				break
			}
		}
	}
}

// drawRegion picks a region with tiles of terrain t, with chance
// proportional to its share.
func (w *World) drawRegion(shares []float64, t Terrain) RegionID {
	f := &w.food
	x := w.rng.Float64() * w.shareSum(shares, t)
	last := RegionID(0)
	for r, s := range shares {
		if len(f.regionTiles[t][r]) == 0 || s == 0 {
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

// placeFood puts one unit of pool p on a free tile of its terrain in
// region r. A region whose tiles are all taken gives up after a few tries;
// the unit stays vacant and is owed again next tick.
func (w *World) placeFood(r RegionID, p int) bool {
	f := &w.food
	land := f.regionTiles[f.pools[p].terrain][r]
	if len(land) == 0 {
		return false
	}
	const tries = 8
	for i := 0; i < tries; i++ {
		t := land[w.rng.Intn(len(land))]
		if f.foodAt[t] != 0 {
			continue
		}
		w.putFood(t, p)
		return true
	}
	return false
}

// putFood puts one unit of pool p on free tile t.
func (w *World) putFood(t, p int) {
	f := &w.food
	f.foods = append(f.foods, Food{X: t % w.m.Width, Y: t / w.m.Width, ID: p})
	f.born = append(f.born, w.tick)
	f.owner = append(f.owner, noOwner)
	f.foodAt[t] = int32(len(f.foods))
	f.pools[p].appeared++
	w.foodMoved(w.m.Region[t], p, +1)
}

// spreadFood grows pool p, a plant: its units on the ground drop
// seedRate seeds a tick between them (a running fraction, sown), and
// each seed grows into a unit on a tile within seed tiles of a unit drawn
// at random, if that tile is free, of the pool's terrain and in a region
// of the pool's shares - else the seed is lost. No seed grows past the
// cap. While fewer than start units stand, the pool also comes back by
// its shares at its return rate, as any pool does: the seedbed, where the
// map says it grows (VISION: it grows where it is told to, and spreads
// from there). Plants are not handed to resting mothers.
func (w *World) spreadFood(p int, vacant int) {
	f := &w.food
	pl := &f.pools[p]
	shares := w.poolShares(p)
	if pl.count < pl.start {
		if w.shareSum(shares, pl.terrain) > 0 {
			pl.bed += pl.ret * float64(vacant)
			for pl.bed >= 1 && pl.count < pl.start && pl.count+pl.held < pl.cap {
				if !w.placeFood(w.drawRegion(shares, pl.terrain), p) {
					break
				}
				pl.bed--
			}
		}
		if pl.count == 0 {
			return
		}
	}
	pl.sown += pl.seedRate * float64(pl.count)
	const tries = 8
	for pl.sown >= 1 {
		pl.sown--
		if pl.count+pl.held >= pl.cap {
			continue // a seed past the cap does not grow
		}
		// The unit that dropped it, by rejection among all units.
		u := -1
		for i := 0; i < tries && u < 0; i++ {
			if k := w.rng.Intn(len(f.foods)); f.foods[k].ID == p {
				u = k
			}
		}
		if u < 0 {
			continue
		}
		x := f.foods[u].X + w.rng.Intn(2*pl.seed+1) - pl.seed
		y := f.foods[u].Y + w.rng.Intn(2*pl.seed+1) - pl.seed
		if !w.m.InBounds(x, y) {
			continue
		}
		t := w.m.index(x, y)
		if f.foodAt[t] != 0 || w.m.Terrain[t] != pl.terrain || shares[w.m.Region[t]] == 0 {
			continue // the seed is lost
		}
		w.putFood(t, p)
	}
}

// returnFood brings vacancies back, pool by pool. Each region is owed its
// share of the pool's return rate times the pool's vacancies at the start
// of the tick, and receives whole units as the debt passes one. Regions
// are served in ID order, so when the cap is reached mid-tick the lower
// IDs are the ones served; the debt of the others carries over.
func (w *World) returnFood() {
	f := &w.food
	asked := false
	var resting [][]int // per region, the indices of the mothers resting there
	for p := range f.pools {
		pl := &f.pools[p]
		vacant := pl.cap - pl.count - pl.held
		if vacant <= 0 || pl.kill {
			continue
		}
		if pl.seed > 0 {
			w.spreadFood(p, vacant)
			continue
		}
		shares := w.poolShares(p)
		sum := w.shareSum(shares, pl.terrain)
		if sum == 0 {
			continue
		}
		if !asked {
			w.askProvision()
			asked = true
		}
		for r, s := range shares {
			if len(f.regionTiles[pl.terrain][r]) == 0 || s == 0 {
				continue
			}
			pl.owed[r] += pl.ret * float64(vacant) * s / sum
			for pl.owed[r] >= 1 && pl.count+pl.held < pl.cap {
				if resting == nil {
					resting = w.restingMothers()
				}
				if w.provision(RegionID(r), p, &resting[r]) {
					pl.owed[r]--
					continue
				}
				if !w.placeFood(RegionID(r), p) {
					break
				}
				pl.owed[r]--
			}
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

// provision gives a unit of pool p returning to region r to one of the
// mothers resting there (the indices in list) who may eat it, drawn at
// random, with chance ProvisionWith if the father of her last child stands
// in r now and ProvisionAlone if not. It reports whether it did: the unit
// then appeared and was eaten at once, and never lay on the ground.
func (w *World) provision(r RegionID, p int, list *[]int) bool {
	mothers := *list
	able := w.food.scratch[:0]
	for j, i := range mothers {
		if w.eats(&w.bodies[i], p) {
			able = append(able, j)
		}
	}
	w.food.scratch = able
	if len(able) == 0 {
		return false
	}
	k := able[w.rng.Intn(len(able))]
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
		prob := w.cfg.ProvisionAlone
		if w.partnerNear(m, r) {
			prob = w.cfg.ProvisionWith
		}
		if w.rng.Float64() >= prob {
			return false
		}
	}
	m.Energy = min(m.Energy+w.cfg.FoodEnergy, w.maxOf(m))
	w.food.pools[p].appeared++
	w.food.pools[p].eaten++
	w.food.pools[p].provisioned++
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
	p := w.food.foods[i].ID
	w.liftFood(i)
	w.food.pools[p].eaten++
}

// takeFood lifts unit i off the ground into body b's hold (Carry). The
// unit keeps the tick it appeared and its pool.
func (w *World) takeFood(i int, b *Body) {
	born, p := w.food.born[i], w.food.foods[i].ID
	w.liftFood(i)
	w.food.pools[p].held++
	w.food.pools[p].picked++
	b.Held++
	b.HeldBorn = append(b.HeldBorn, born)
	b.HeldPool = append(b.HeldPool, p)
}

// eatHeld has body b eat the units it holds while a whole meal fits; it
// spends no turn (Carry). A body eats what it holds as soon as nothing of it
// is lost, which is when its valuation reads it.
func (w *World) eatHeld(b *Body) {
	for b.Held > 0 && b.Energy <= w.maxOf(b)-w.cfg.FoodEnergy {
		p := w.dropHeld(b)
		b.Energy += w.cfg.FoodEnergy
		w.food.pools[p].held--
		w.food.pools[p].eaten++
		w.stats.EatenHeld++
	}
}

// dropHeld takes the oldest unit out of body b's hold and returns its
// pool: a body eats the oldest first.
func (w *World) dropHeld(b *Body) int {
	p := b.HeldPool[0]
	b.Held--
	b.HeldBorn = dropOldest(b.HeldBorn)
	b.HeldPool = append(b.HeldPool[:0], b.HeldPool[1:]...)
	return p
}

// loseHeld drops what a dead body held out of the world: the units become
// vacancies of their pools (Carry).
func (w *World) loseHeld(b *Body) {
	for _, p := range b.HeldPool {
		w.food.pools[p].held--
		w.food.pools[p].lost++
	}
	b.Held = 0
	b.HeldBorn, b.HeldPool = nil, nil
}

// dropOldest removes the first (oldest) of the ticks a body's held units
// appeared.
func dropOldest(born []int64) []int64 {
	if len(born) == 0 {
		return born
	}
	return append(born[:0], born[1:]...)
}

// decayFood turns the units that have lasted FoodLife ticks into vacancies:
// those on the ground, then those held. On a place to rest a ground unit
// lasts ShelterFoodLife ticks instead (stage 8-2c). It draws nothing from
// the random source.
func (w *World) decayFood() {
	if w.cfg.FoodLife <= 0 {
		return
	}
	f := &w.food
	end := w.tick - int64(w.cfg.FoodLife)
	endKept := end
	if w.cfg.ShelterFoodLife > 0 {
		endKept = w.tick - int64(w.cfg.ShelterFoodLife)
	}
	for i := 0; i < len(f.foods); {
		e := end
		if endKept != end && w.m.ShelterAt(f.foods[i].X, f.foods[i].Y) {
			e = endKept
		}
		if f.born[i] <= e {
			p := f.foods[i].ID
			w.liftFood(i) // the last unit moves into slot i: look at it next
			f.pools[p].decayed++
			continue
		}
		i++
	}
	for i := range w.bodies {
		b := &w.bodies[i]
		for len(b.HeldBorn) > 0 && b.HeldBorn[0] <= end {
			p := w.dropHeld(b)
			f.pools[p].held--
			f.pools[p].decayed++
		}
	}
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
		f.born[i] = f.born[last]
		f.owner[i] = f.owner[last]
		f.foodAt[moved.Y*w.m.Width+moved.X] = int32(i + 1)
	}
	f.foods = f.foods[:last]
	f.born = f.born[:last]
	f.owner = f.owner[:last]
	w.foodMoved(w.m.RegionAt(gone.X, gone.Y), gone.ID, -1)
}

// foodMoved keeps the count of pool p's food on region r's ground, and
// the prediction that reads it, up to date.
func (w *World) foodMoved(r RegionID, p int, d int) {
	w.food.pools[p].onGround[r] += d
	w.food.pools[p].count += d
	w.refreshRegion(r)
}

// groundFood is the units on region r's ground that body b may eat: the
// food as its survival tables read it.
func (w *World) groundFood(b *Body, r RegionID) int {
	n := 0
	for p := range w.food.pools {
		if w.eats(b, p) {
			n += w.food.pools[p].onGround[r]
		}
	}
	return n
}

// allGroundFood is the units on region r's ground, of every pool.
func (w *World) allGroundFood(r RegionID) int {
	n := 0
	for p := range w.food.pools {
		n += w.food.pools[p].onGround[r]
	}
	return n
}

// PoolGround returns the units of the named pool on the ground now, and
// how many regions hold one; ok is false for no such pool.
func (w *World) PoolGround(name string) (units, regions int, ok bool) {
	for _, p := range w.food.pools {
		if p.name != name {
			continue
		}
		for _, n := range p.onGround {
			if n > 0 {
				regions++
			}
		}
		return p.count, regions, true
	}
	return 0, 0, false
}

// GroundFoodOf returns the units on the ground now that species s may
// eat, over the whole map.
func (w *World) GroundFoodOf(s int) int {
	n := 0
	for p := range w.food.pools {
		if w.food.eats[s][p] {
			n += w.food.pools[p].count
		}
	}
	return n
}

// Home reports whether region r is home to species s: whether some food
// it may eat comes back there now, on terrain it may stand on. The first
// bodies of a species start in its home regions (placeBodies).
func (w *World) Home(s int, r RegionID) bool {
	for p := range w.food.pools {
		if w.food.pools[p].kill {
			continue // meat comes back nowhere
		}
		if w.food.eats[s][p] && w.poolShares(p)[r] > 0 && w.canStand(s, w.food.pools[p].terrain) && len(w.food.regionTiles[w.food.pools[p].terrain][r]) > 0 {
			return true
		}
	}
	return false
}

// KindOf returns the kind of food unit f, for drawing.
func (w *World) KindOf(f Food) int { return w.food.pools[f.ID].kind }

// FoodLedger returns the food account.
func (w *World) FoodLedger() FoodLedger {
	var l FoodLedger
	for _, p := range w.food.pools {
		pl := PoolLedger{Name: p.name, Kind: p.kind, Cap: p.cap, OnGround: p.count, Appeared: p.appeared, Eaten: p.eaten,
			Provisioned: p.provisioned, Held: p.held, Picked: p.picked, Lost: p.lost, Decayed: p.decayed}
		l.Pools = append(l.Pools, pl)
		l.Cap += pl.Cap
		l.OnGround += pl.OnGround
		l.Appeared += pl.Appeared
		l.Eaten += pl.Eaten
		l.Provisioned += pl.Provisioned
		l.Held += pl.Held
		l.Picked += pl.Picked
		l.Lost += pl.Lost
		l.Decayed += pl.Decayed
	}
	return l
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
