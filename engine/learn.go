package engine

import "math"

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
// first observed it (provenance.go). The rest the body observed itself.
type Tally struct {
	N, K  float64         `json:",omitempty"`
	Heard map[int64]Count `json:",omitempty"`
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
}

// Belief is what a body's memory makes of the world now, as the trace shows
// it: each estimate and the evidence behind it.
type Belief struct {
	Land, Here, Path, Child float64
	// HereN and PathN are the tiles behind Here (the region the body stands
	// in) and Path; Asks the mates behind Child.
	HereN, PathN, Asks float64
}

// landTally pools a body's regions.
func (mem *Memory) landTally() Tally {
	var t Tally
	for _, r := range mem.Regions {
		t.N += r.N
		t.K += r.K
	}
	return t
}

// land is a body's estimate of the chance a tile holds food, anywhere.
func (w *World) land(b *Body) float64 {
	return b.Memory.landTally().estimate(w.cfg.PriorFood, w.cfg.PriorWeight)
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
	return b.Memory.region(r).estimate(w.land(b), w.cfg.RegionWeight)
}

// pathRate is a body's estimate for a tile it walked lately, in region r.
func (w *World) pathRate(b *Body, r RegionID) float64 {
	return b.Memory.Path.estimate(w.regionRate(b, r), w.cfg.PathWeight)
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
	return Belief{
		Land: w.land(b), Here: w.regionRate(b, r), Path: w.pathRate(b, r), Child: w.childRate(b),
		HereN: b.Memory.region(r).N, PathN: b.Memory.Path.N, Asks: b.Memory.Asks,
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
	rs.region, rs.path = rs.region[:0], rs.path[:0]
	rs.tables, rs.built = rs.tables[:0], rs.built[:0]
	for r := 0; r < n; r++ {
		p := b.Memory.region(RegionID(r)).estimate(land, w.cfg.RegionWeight)
		rs.region = append(rs.region, p)
		rs.path = append(rs.path, b.Memory.Path.estimate(p, w.cfg.PathWeight))
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
				mem.Regions = append(mem.Regions, Tally{})
			}
			mem.Regions[r].N++
			mem.Regions[r].K += food
			if w.walked(b, t) {
				mem.Path.N++
				mem.Path.K += food
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
