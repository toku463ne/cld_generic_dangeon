package engine

import (
	"math"
	"sort"
)

// Learning from experience (stage 1-6).
//
// With Learn, a body no longer reads the world's true rules about the world
// (TruthTable's region rows, and a mate always making a child). It reads its
// own memory, which holds outcomes only (NODE.md "記憶は結果だけを持つ") and
// dies with it:
//
//	(this region, step onto a tile)       -> food there, K times in N
//	(a tile I walked lately, step onto it) -> food there, K times in N
//	(an adult of my kind, ask to mate)     -> a child, K times in N asks
//
// A row's estimate is its own count pulled toward its parent's estimate by
// as many pseudo-observations as the row's weight: the path row toward
// its region, a region toward the land (all the body's regions pooled), the
// land toward the inborn expectation PriorFood; the mate row toward
// PriorChild. What a body is and perceives - what a meal gives, what a tick
// burns, how long a walk takes, what is in sight - it knows, and does not
// learn.
//
// Nothing here reads the food on the ground but through the tile a body
// steps onto.

// Tally counts a row's outcomes: K of N came out one way. N and K are all
// of it; Heard is the part received from other bodies, by the body that
// first observed it (provenance.go, tell.go), and HN and HK its sums. The
// rest, N-HN of it, the body observed itself.
//
// With EvidenceHalfLife, evidence weighs less the older it is: all of a
// tally is kept as of tick T, and halves every EvidenceHalfLife ticks
// after (age, Fresh).
type Tally struct {
	N, K   float64 `json:",omitempty"`
	Heard  []Heard `json:",omitempty"`
	HN, HK float64 `json:",omitempty"`
	T      int64   `json:",omitempty"`
	// S scales the Heard entries: each weighs S times what it holds (zero
	// stands for 1). Ageing scales S rather than every entry; passing
	// evidence brings the entries to scale (settle).
	S float64 `json:",omitempty"`
}

// Heard is evidence received from one observer: K of N came out one way. A
// tally keeps its entries in the order of their observers' IDs.
type Heard struct {
	ID   int64
	N, K float64
}

// heard returns the entry of observer id, or a zero one.
func (t Tally) heard(id int64) Heard {
	i := sort.Search(len(t.Heard), func(i int) bool { return t.Heard[i].ID >= id })
	if i < len(t.Heard) && t.Heard[i].ID == id {
		return t.Heard[i]
	}
	return Heard{ID: id}
}

// scale is what the Heard entries of t are multiplied by.
func (t Tally) scale() float64 {
	if t.S == 0 {
		return 1
	}
	return t.S
}

// estimate is the tally's rate pulled toward prior by weight observations.
func (t Tally) estimate(prior, weight float64) float64 {
	return (t.K + weight*prior) / (t.N + weight)
}

// Memory is what a body has learned.
type Memory struct {
	// Regions is indexed by RegionID; regions past its end have no evidence.
	Regions []Tally `json:",omitempty"`
	Path    Tally
	// Walked holds the tiles the body left within PathRecall ticks, and
	// when.
	Walked map[int]int64 `json:",omitempty"`
	// Asks and Kids: mates asked and children had.
	Asks, Kids float64 `json:",omitempty"`
	// Met are the bodies it has passed evidence with (tell.go).
	Met map[int64]bool `json:",omitempty"`
	// Ver counts the changes to the rows it can pass on; Told holds, for
	// each child, Ver when it last passed them to it (tell.go, with Kin).
	Ver  int64           `json:",omitempty"`
	Told map[int64]int64 `json:",omitempty"`
}

// Belief is what a body's memory makes of the world now, as the trace shows
// it: each estimate and the evidence behind it.
type Belief struct {
	Land, Here, Path, Child float64
	// PathRatio is Path over Here: how much of its region's food a tile
	// walked lately is believed to hold.
	PathRatio float64
	// HereN and PathN are the tiles behind Here (the region the body stands
	// in) and Path; Asks the mates behind Child.
	HereN, PathN, Asks float64
}

// decay is what evidence kept as of tick t weighs now.
func (w *World) decay(t int64) float64 {
	if w.cfg.EvidenceHalfLife <= 0 || t >= w.tick {
		return 1
	}
	return math.Exp2(-float64(w.tick-t) / w.cfg.EvidenceHalfLife)
}

// Fresh returns tally t's sums as they weigh now (its Heard entries are left
// as they were kept; age brings those to now as well).
func (w *World) Fresh(t Tally) Tally {
	f := w.decay(t.T)
	return Tally{N: f * t.N, K: f * t.K, HN: f * t.HN, HK: f * t.HK, T: w.tick}
}

// age brings tally t to now: its sums, and the scale of its entries.
func (w *World) age(t *Tally) {
	f := w.decay(t.T)
	t.T = w.tick
	if f == 1 {
		return
	}
	t.N, t.K, t.HN, t.HK = f*t.N, f*t.K, f*t.HN, f*t.HK
	if len(t.Heard) > 0 {
		t.S = t.scale() * f
	}
}

// settle brings the entries of an aged tally to scale, letting go of those
// worth less than a hundredth of a tile, and sums again.
func (t *Tally) settle() {
	s := t.scale()
	ownN, ownK := t.N-t.HN, t.K-t.HK
	kept := t.Heard[:0]
	for _, h := range t.Heard {
		h.N, h.K = s*h.N, s*h.K
		if h.N >= 0.01 {
			kept = append(kept, h)
		}
	}
	t.Heard, t.S = kept, 0
	t.resum(ownN, ownK)
}

// resum sets the heard sums from the entries (in the order of their
// observers, so that fractions always sum alike), and the totals from them
// and the own part.
func (t *Tally) resum(ownN, ownK float64) {
	t.HN, t.HK = 0, 0
	for _, h := range t.Heard {
		t.HN += h.N
		t.HK += h.K
	}
	t.N, t.K = ownN+t.HN, ownK+t.HK
}

// landTally pools a body's regions, as they weigh now.
func (w *World) landTally(mem *Memory) Tally {
	var t Tally
	for _, r := range mem.Regions {
		r = w.Fresh(r)
		t.N += r.N
		t.K += r.K
	}
	return t
}

// land is a body's estimate of the chance a tile holds food, anywhere.
func (w *World) land(b *Body) float64 {
	return w.landTally(&b.Memory).estimate(w.cfg.PriorFood, w.cfg.PriorWeight)
}

// region is the tally of region r.
func (mem *Memory) region(r RegionID) Tally {
	if int(r) < len(mem.Regions) {
		return mem.Regions[r]
	}
	return Tally{}
}

// regionRate is a body's estimate for region r.
func (w *World) regionRate(b *Body, r RegionID) float64 {
	return w.Fresh(b.Memory.region(r)).estimate(w.land(b), w.cfg.RegionWeight)
}

// pathRate is a body's estimate for a tile it walked lately, in region r.
func (w *World) pathRate(b *Body, r RegionID) float64 {
	return w.pathFrom(w.pathTally(b), w.regionRate(b, r))
}

// pathTally is body b's path row as it weighs now. With StableRows and
// without AgePath it does not age (stage 2-2): that tiles walked lately
// hold less than their region was taken for a fact that does not change.
func (w *World) pathTally(b *Body) Tally {
	if !w.pathAges() {
		return b.Memory.Path
	}
	return w.Fresh(b.Memory.Path)
}

// pathAges reports whether the path row ages: always without StableRows,
// with it only by AgePath.
func (w *World) pathAges() bool { return !w.cfg.StableRows || w.cfg.AgePath }

// PathTally is body b's path row as it weighs now, for reports.
func (w *World) PathTally(b Body) Tally { return w.pathTally(&b) }

// agePath ages a path row, where it ages.
func (w *World) agePath(t *Tally) {
	if w.pathAges() {
		w.age(t)
	}
}

// pathFrom is the path's estimate from its tally, for a region estimated
// at p. With StableRows the tally counts food seen on tiles walked lately
// (K) against what the region was believed to hold there (N), so the
// estimate is that ratio - one, a tile like any other, with no evidence,
// worth PathWeight tiles - times p. Without, the tally counts food in
// tiles seen, pulled toward p.
func (w *World) pathFrom(t Tally, p float64) float64 {
	if !w.cfg.StableRows {
		return t.estimate(p, w.cfg.PathWeight)
	}
	weight := w.cfg.PathWeight * p
	if t.N+weight <= 0 {
		return p
	}
	return (t.K + weight) / (t.N + weight) * p
}

// childRate is a body's estimate of the chance a mate becomes a child.
func (w *World) childRate(b *Body) float64 {
	return Tally{N: b.Memory.Asks, K: b.Memory.Kids}.estimate(w.cfg.PriorChild, w.cfg.ChildWeight)
}

// Belief is what body b's memory makes of the world now (with Learn), as
// the trace shows it.
func (w *World) Belief(b Body) Belief { return w.belief(&b) }

// belief reads a body's memory for the trace.
func (w *World) belief(b *Body) Belief {
	r := w.m.RegionAt(int(math.Floor(b.X)), int(math.Floor(b.Y)))
	here, path := w.regionRate(b, r), w.pathRate(b, r)
	ratio := 1.0
	if here > 0 {
		ratio = path / here
	}
	return Belief{
		Land: w.land(b), Here: here, Path: path, PathRatio: ratio, Child: w.childRate(b),
		HereN: w.Fresh(b.Memory.region(r)).N, PathN: w.pathTally(b).N, Asks: b.Memory.Asks,
	}
}

// rates are a body's estimates for one decision, per region, and the
// survival tables they read (built when first asked for).
type rates struct {
	region, path []float64
	tables       []Survival
	built        []bool
	// rays hold, per heading, what the path reading found along it from a
	// start tile: the decision reads each heading from the same tile for
	// every option.
	rays [8]ray
}

// ray is what lies along one heading from start: the chance of meeting no
// food at the region's rate and at the rates read tile by tile, and whether
// any tile of the path is among them.
type ray struct {
	start               int
	noneRegion, noneAll float64
	walked              bool
}

// ratesOf reads body b's memory once for a decision.
func (w *World) ratesOf(b *Body, rs *rates) {
	n := len(w.m.RegionFood)
	land := w.land(b)
	path := w.pathTally(b)
	rs.region, rs.path = rs.region[:0], rs.path[:0]
	rs.tables, rs.built = rs.tables[:0], rs.built[:0]
	for r := 0; r < n; r++ {
		p := w.Fresh(b.Memory.region(RegionID(r))).estimate(land, w.cfg.RegionWeight)
		rs.region = append(rs.region, p)
		rs.path = append(rs.path, w.pathFrom(path, p))
		rs.tables = append(rs.tables, Survival{})
		rs.built = append(rs.built, false)
	}
	for d := range rs.rays {
		rs.rays[d].start = -1
	}
}

// table returns region r's survival table for the rates and a build.
func (w *World) rateTable(rs *rates, r RegionID, meal, full int, speed float64) Survival {
	if !rs.built[r] {
		rs.tables[r], rs.built[r] = w.learnedTable(rs.region[r], meal, full, speed), true
	}
	return rs.tables[r]
}

// stepped records what body b learns stepping from tile from onto tile to:
// what comes into view. Each tile within Sight of the new tile and not of
// the old is one tile of evidence of its region - food on it or not - and,
// if the body left it within PathRecall ticks, of its path too. The tile
// left joins the path.
//
// What comes into view is the evidence because nothing the body chose
// selected it. The tiles a body steps onto are not: counting them, a walk
// to food in sight made regions read two or three times as rich as they
// were, and leaving the walks out made them read three times as poor (food
// ahead is seen, and walked to, before it is stepped on).
func (w *World) stepped(b *Body, from, to int) {
	if !w.cfg.Learn || from == to || to < 0 || w.cfg.Sight < 0 {
		return
	}
	mem := &b.Memory
	if mem.Walked == nil {
		mem.Walked = map[int]int64{}
	}
	s := w.cfg.Sight
	tx, ty := to%w.m.Width, to/w.m.Width
	fx, fy := -1<<20, -1<<20
	if from >= 0 {
		fx, fy = from%w.m.Width, from/w.m.Width
	}
	for y := ty - s; y <= ty+s; y++ {
		for x := tx - s; x <= tx+s; x++ {
			if !w.m.InBounds(x, y) || w.m.TerrainAt(x, y) != TerrainLand {
				continue
			}
			if abs(x-fx) <= s && abs(y-fy) <= s {
				continue // in view before the step
			}
			t := w.m.index(x, y)
			food := 0.0
			if w.foodOn(t) >= 0 {
				food = 1
			}
			r := w.m.Region[t]
			for int(r) >= len(mem.Regions) {
				mem.Regions = append(mem.Regions, Tally{T: w.tick})
			}
			w.age(&mem.Regions[r])
			mem.Regions[r].N++
			mem.Regions[r].K += food
			w.regionRow(r).Learned++
			if !w.cfg.StableRows {
				mem.Ver++ // the regions' rows pass too
			}
			if w.walked(b, t) {
				w.stats.PathRow.Learned++
				w.agePath(&mem.Path)
				if w.cfg.StableRows {
					// Against what the region was believed to hold.
					mem.Path.N += w.regionRate(b, r)
				} else {
					mem.Path.N++
				}
				mem.Path.K += food
				if w.cfg.PassPath {
					mem.Ver++
				}
			}
		}
	}
	if from >= 0 {
		mem.Walked[from] = w.tick
	}
	// Forget the tiles left too long ago, now and then.
	if len(mem.Walked) > 2*w.cfg.PathRecall {
		for tile, when := range mem.Walked {
			if w.tick-when > int64(w.cfg.PathRecall) {
				delete(mem.Walked, tile)
			}
		}
	}
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// walked reports whether body b left tile t within PathRecall ticks.
func (w *World) walked(b *Body, t int) bool {
	when, ok := b.Memory.Walked[t]
	return ok && w.tick-when <= int64(w.cfg.PathRecall)
}

// beliefLevel rounds a chance per tile onto the grid survival tables are
// built for: powers of BeliefStep. Zero has a level of its own.
func (w *World) beliefLevel(p float64) int {
	if p <= 0 {
		return math.MinInt32
	}
	return int(math.Round(math.Log(p) / math.Log(w.cfg.BeliefStep)))
}

// levelRate is the chance per tile of a level.
func (w *World) levelRate(level int) float64 {
	if level == math.MinInt32 {
		return 0
	}
	return math.Min(math.Pow(w.cfg.BeliefStep, float64(level)), 1)
}

// learnedTable returns the survival table for a chance per tile p (rounded
// to its level) and a build of the given meal, full and speed, keeping the
// rows the path reading needs.
func (w *World) learnedTable(p float64, meal, full int, speed float64) Survival {
	level := w.beliefLevel(p)
	k := survKey{food: level, meal: meal, full: full, speed: speed}
	t, ok := w.pred.learned[k]
	if !ok {
		ahead := w.pathTicks(speed)
		tt := TruthTable{Meal: meal, Full: full, Reach: w.reach(speed)}
		win := w.cfg.Window
		tt.Extra = []int{win - ahead, win - ahead/2 - 1}
		t = &table{s: tt.NewSurvival(w.levelRate(level)*math.Min(speed, 1), w.pred.windows), used: w.tick}
		w.pred.learned[k] = t
	}
	t.used = w.tick
	return t.s
}

// pathTicks is how many ticks a body of the given speed takes to cover the
// PathAhead tiles the path reading looks along.
func (w *World) pathTicks(speed float64) int {
	return int(math.Ceil(float64(w.cfg.PathAhead)/speed - 1e-9))
}

// keepReading returns, for body b and a move in direction dir that leaves
// it at (x, y) with after ticks of energy, the risk of keeping on the move
// read with the path row: base is the region reading. The next PathAhead
// tiles along dir are each met with the path rate if the body walked them
// lately and the region rate otherwise; the reading changes the region's
// only by how far those tiles differ from it (so a heading with none of
// its path ahead is read exactly as the region reads it): the chance of
// meeting none in the first stretch, then the region from there, against
// meeting one about half way and eating it.
func (w *World) keepReading(b *Body, rs *rates, dir int, x, y float64, win, after, meal, full int, s Survival, base float64) float64 {
	if w.cfg.PathAhead <= 0 {
		return base
	}
	r := w.m.RegionAt(int(math.Floor(x)), int(math.Floor(y)))
	p, q := rs.region[r], rs.path[r]
	tx, ty := int(math.Floor(x)), int(math.Floor(y))
	ry := &rs.rays[dir]
	if start := w.m.index(tx, ty); ry.start != start {
		d := moveDirs[dir]
		sx, sy := int(math.Round(d[0])), int(math.Round(d[1]))
		*ry = ray{start: start, noneRegion: 1, noneAll: 1}
		for i := 1; i <= w.cfg.PathAhead; i++ {
			ax, ay := tx+sx*i, ty+sy*i
			if !w.m.InBounds(ax, ay) || w.m.TerrainAt(ax, ay) != TerrainLand {
				break
			}
			rate := p
			if w.walked(b, w.m.index(ax, ay)) {
				rate, ry.walked = q, true
			}
			ry.noneRegion *= 1 - p
			ry.noneAll *= 1 - rate
		}
	}
	if !ry.walked {
		return base
	}
	noneRegion, noneHere := ry.noneRegion, ry.noneAll
	ahead := w.pathTicks(w.speedOf(b))
	half := ahead / 2
	dead := func(l, n int) float64 {
		if n <= 0 {
			return 1
		}
		return s.deadIn(l, n)
	}
	none := dead(win-ahead, after-ahead)
	met := dead(win-half-1, min(after-half+meal, full)-1)
	read := func(pNone float64) float64 { return pNone*none + (1-pNone)*met }
	return math.Min(math.Max(base+read(noneHere)-read(noneRegion), 0), 1)
}

// ValueWith values body b's options as a learning body would, but with the
// given estimates of food per tile - per region, and of the tiles it walked
// lately, per region - in place of its own; the chance a mate makes a child
// stays its own. It is for counts asking what a body would do knowing
// otherwise; it draws nothing and changes nothing (the survival tables it
// builds are the ones any body's estimate could ask for).
func (w *World) ValueWith(b Body, region, path []float64) Valuation {
	var v Valuation
	if w.cfg.Window <= 0 {
		return v
	}
	burn, speed := w.burnOf(&b), w.speedOf(&b)
	meal, full := energyTicks(w.cfg.FoodEnergy, burn), energyTicks(w.maxOf(&b), burn)
	var rs rates
	w.ratesOf(&b, &rs)
	copy(rs.region, region)
	copy(rs.path, path)
	alive := func(r RegionID) Survival { return w.rateTable(&rs, r, meal, full, speed) }
	w.valueInto(&v, meal, full, w.pred.windows, alive, w.childRate(&b), &rs, &b)
	return v
}
