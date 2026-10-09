// Command experiment runs variants of the world over a range of seeds and
// prints the report in the form MEASURE.md fixes.
//
// Every variant is a rewrite of engine.Config (package variant), never a
// branch in the code, so that all of them are in the same binary and can be
// paired seed by seed.
package main

import (
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/toku463ne/cld_generic_dangeon/engine"
	"github.com/toku463ne/cld_generic_dangeon/variant"
	"github.com/toku463ne/cld_generic_dangeon/worldmap"
)

// checkpoints is how many rows the population table has after tick 0.
const checkpoints = 20

type options struct {
	variants      []string
	seeds         int
	seed0         int64
	ticks         int
	mapName       string
	width, height int
	floor         int
	// jobs is how many runs go at once; brief prints only the conditions
	// and the paired differences.
	jobs  int
	brief bool
}

func main() {
	var o options
	var names string
	flag.StringVar(&names, "variants", variant.Base, "comma-separated variants; the first is the base")
	// Seeds and ticks are the values stage 1-5 fixed (PARAMETERS.md). The map size has no default:
	// it is part of choosing the map.
	flag.IntVar(&o.seeds, "seeds", 12, "number of seeds (fixed at stage 1-5, PARAMETERS.md)")
	flag.Int64Var(&o.seed0, "seed0", 1, "first seed")
	flag.IntVar(&o.ticks, "ticks", 40000, "ticks per run (fixed at stage 1-5, PARAMETERS.md)")
	flag.StringVar(&o.mapName, "map", "", "map: "+strings.Join(worldmap.Names, " | ")+" (required)")
	flag.IntVar(&o.width, "w", 0, "map width in tiles (required)")
	flag.IntVar(&o.height, "h", 0, "map height in tiles (required)")
	// The collapse floor is the value stage 1-5 fixed (PARAMETERS.md).
	flag.IntVar(&o.floor, "floor", 20, "collapse floor: a seed whose population falls to this or below stops (fixed at stage 1-5, PARAMETERS.md)")
	flag.IntVar(&o.jobs, "jobs", 3, "runs at once (each run is its own world, so the numbers do not depend on it)")
	flag.BoolVar(&o.brief, "brief", false, "print only the conditions and the paired differences (§2, §9)")
	flag.Parse()
	o.variants = strings.Split(names, ",")

	// The reproduction line names the package rather than the binary, whose
	// path under go run is a temporary directory.
	command := strings.Join(append([]string{"go run ./cmd/experiment"}, os.Args[1:]...), " ")
	if err := run(os.Stdout, os.Stderr, o, command); err != nil {
		fmt.Fprintln(os.Stderr, "experiment:", err)
		os.Exit(2)
	}
}

// result is what one run of one seed leaves behind.
//
// A seed stops early once its population falls to the collapse floor
// (MEASURE.md). The checkpoints after that are recorded as not surviving,
// and every per-tick figure is averaged over the seeds still surviving.
type result struct {
	pop []float64 // bodies at tick 0 and at each checkpoint
	// speciesPop is the same by species (stage 9-7a), speciesGone whether
	// each species had no body left at the end, and speciesBudget the
	// mean budget of each species' adults (sampled with the adults; NaN
	// where a species had none).
	speciesPop    [][]float64
	speciesNames  []string
	speciesGone   []float64
	speciesBudget []float64
	// speciesFood is the mean, over the samples of the adults, of the
	// food on the ground each species may eat (stage 9-7b-1).
	speciesFood []float64
	series      []int32   // bodies at every tick from 0, up to the tick the seed stopped
	food        []float64 // food on the ground at the same ticks
	surviving   []bool    // whether the seed was above the floor at the same ticks
	fellAt      int       // first tick at or below the floor, -1 if never
	starved     float64
	sleepless   float64
	asleep      float64 // body-ticks asleep over all body-ticks
	sheltered   float64 // sleeps begun on a place to rest over all sleeps begun
	// places built, sleeps begun on them after the building, and those by
	// their builders (stage 8-1)
	built, builtSleeps, returns float64
	// settlements begun, trips for food while building, sites given up
	// (stage 8-2b)
	begun, fetched, abandoned float64
	// tents made, nights one was pitched, tents worn out (stage 8-2a)
	tentsMade, tentNights, tentsWorn float64
	// tents left by the dead and taken up (stage 8-2a3)
	tentsDropped, tentsPicked float64
	aged                      float64 // deaths of age (Lifespan)
	burned                    float64
	actions                   [engine.NumActionKinds]float64
	decided                   float64 // decisions over actions (body-ticks)
	births                    float64
	// provisioned is the food that went straight to resting mothers.
	provisioned float64
	// requests are the requests to mate sent, and requestBirths the
	// births they brought about.
	requests, requestBirths float64
	adultRate               float64 // matured over matured and died young; NaN with neither
	// maxGen is the highest generation born by the end, topGenBirths the
	// bodies born into it (stage 9-1); genBirths the births by generation.
	maxGen, topGenBirths float64
	genBirths            []int64
	// sightLow and sightHigh are the mean shares of adults at sight 0 and
	// at SightMost (stage 9-3), sampled with the adults; zero without
	// AllotSight.
	sightLow, sightHigh float64
	// fertLow and fertHigh are the same for fertility level 0 and
	// FertMost (stage 9-4); matings the matings agreed on; fertMatings
	// and fertBirths the parents in them and in the births by level, and
	// fertSlope the births per mating at the top level less the bottom
	// (NaN where either level saw no mating).
	fertLow, fertHigh       float64
	matings                 float64
	fertMatings, fertBirths []int64
	fertSlope               float64
	// sizeLow and sizeHigh are the same for budget level 0 and
	// BudgetLevels-1 (stage 9-5b); meanBudget the adults' mean budget,
	// budgetDrift the second half's less the first half's; genBudget the
	// mean budget born into each generation.
	sizeLow, sizeHigh       float64
	meanBudget, budgetDrift float64
	genBudget               []float64
	// budgetCorr is the correlation between a child's budget and its
	// parents' mean budget over the children seen (NaN under 3 children or
	// with no variance on either side).
	budgetCorr float64
	regions    []float64 // regions with a body in them, at tick 0 and each checkpoint
	valley     float64   // fewest bodies at any tick after tick 0
	// displace is the mean straight-line distance, in tiles, a body gets
	// from where it was 1000 ticks before (over bodies alive at both ends,
	// every 1000 ticks after tick 5000); edge the mean share of the living
	// on land tiles next to the map's edge or water, sampled the same.
	displace, edge float64
	// untrod is the share of the steps after tick 5000 that took a body
	// onto a tile it had never stood on (NaN with no step); unknown the
	// mean share of adults standing in a region they hold less than
	// RegionWeight tiles of evidence of, sampled with the adults (stage
	// 9-6).
	untrod, unknown float64
	// boldLow and boldHigh are the mean shares of adults at exploratory
	// level 0 and VentureMost (stage 9-6a), sampled with the adults;
	// boldSteps and boldNew the steps and the steps onto new tiles by
	// level after tick 5000; boldSlope the untrodden-tile rate at the
	// top level less the bottom (NaN where either level took no step).
	boldLow, boldHigh  float64
	boldSteps, boldNew []int64
	boldSlope          float64
	// back is the share of decisions after tick 5000 that took a move
	// straight back the way the body last moved.
	back float64
	// adults is the share of the living that are adults, and minority the
	// share of the rarer sex among the adults (NaN without sexes), both
	// averaged over the checkpoints every 1000 ticks after tick 5000.
	adults, minority float64
	// female is the share of women among the adults (NaN without sexes).
	female float64
	// With Learn: where the evidence the living held came from, summed
	// over the checkpoints after tick 5000 (prov), and the ages at death
	// after tick 5000 (deathAges), for the median life to set it against.
	prov      map[string]*provSum
	provNames []string
	deathAges []float64
	// With Learn: what the living had learned at the end (rows), and, at
	// every checkpoint after tick 5000, how far off each age band's region
	// estimates were (learnAge).
	rows     []memRow
	learnAge [len(ageBands)]ageStat
	// With Allot: per share of the budget for speed, the bodies born
	// after the first quarter of the run that died before its end, and
	// the children they had; and the mean share of the living at tick 0
	// and each checkpoint.
	shares    []float64
	lives     map[float64]float64
	lifeKids  map[float64]float64
	shareLive []shareStats
	why       [engine.NumTriggers]float64 // share of the decisions by trigger
	regionOf  []float64                   // share of the bodies in each region, averaged over the second half of the run
	regionOK  bool                        // whether regionOf has any tick behind it
}

func run(out, log io.Writer, o options, command string) error {
	if o.seeds <= 0 || o.ticks <= 0 || o.width <= 0 || o.height <= 0 || o.mapName == "" {
		return fmt.Errorf("-seeds, -ticks, -map, -w and -h are required")
	}
	for _, v := range o.variants {
		if _, err := variant.Config(v, 0); err != nil {
			return err
		}
	}
	m, err := worldmap.Build(o.mapName, o.width, o.height)
	if err != nil {
		return err
	}

	// Every run is a world of its own (NewWorld copies the map), so runs
	// go o.jobs at once and land in their seed's place: the report does not
	// depend on how many ran together.
	results := map[string][]result{}
	for _, v := range o.variants {
		results[v] = make([]result, o.seeds)
	}
	type job struct {
		v string
		s int
	}
	jobs := make(chan job)
	errs := make(chan error, len(o.variants)*o.seeds)
	var mu sync.Mutex
	var wg sync.WaitGroup
	for k := 0; k < max(o.jobs, 1); k++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				cfg, _ := variant.Config(j.v, o.seed0+int64(j.s))
				start := time.Now()
				r, err := runOne(cfg, m, o.ticks, o.floor)
				if err != nil {
					errs <- fmt.Errorf("%s seed %d: %w", j.v, cfg.Seed, err)
					continue
				}
				mu.Lock()
				results[j.v][j.s] = r
				// Wall time goes to the log, not the report: it describes
				// the machine, not the world.
				fmt.Fprintf(log, "%s seed %d: %d ticks in %v\n", j.v, cfg.Seed, o.ticks, time.Since(start))
				mu.Unlock()
			}
		}()
	}
	for _, v := range o.variants {
		for s := 0; s < o.seeds; s++ {
			jobs <- job{v, s}
		}
	}
	close(jobs)
	wg.Wait()
	close(errs)
	if err := <-errs; err != nil {
		return err
	}

	if !o.brief {
		writeReport(out, o, m, command, results)
		return nil
	}
	// Brief: the conditions (with the replay commands - MEASURE.md "UI
	// での再生" wants them in every report) and the paired differences only.
	var full strings.Builder
	writeReport(&full, o, m, command, results)
	rep := full.String()
	if i := strings.Index(rep, "### 3. "); i > 0 {
		fmt.Fprint(out, rep[:i])
	}
	if i := strings.Index(rep, "### 9. 対の差"); i >= 0 {
		fmt.Fprint(out, rep[i:])
	}
	return nil
}

func runOne(cfg engine.Config, m engine.Map, ticks, floor int) (result, error) {
	w, err := engine.NewWorld(cfg, m)
	if err != nil {
		return result{}, err
	}
	r := result{regionOf: make([]float64, len(m.RegionFood)), fellAt: -1}
	occupied := func() float64 {
		in := make([]bool, len(m.RegionFood))
		k := 0.0
		for _, b := range w.Bodies() {
			if g := m.RegionAt(int(b.X), int(b.Y)); !in[g] {
				in[g] = true
				k++
			}
		}
		return k
	}
	species := w.SpeciesCount()
	r.speciesPop = make([][]float64, species)
	for k := 0; k < species; k++ {
		r.speciesNames = append(r.speciesNames, w.SpeciesName(k))
	}
	record := func() {
		n := len(w.Bodies())
		r.pop = append(r.pop, float64(n))
		var by []float64 = make([]float64, species)
		for _, b := range w.Bodies() {
			by[b.Species]++
		}
		for k := range by {
			r.speciesPop[k] = append(r.speciesPop[k], by[k])
		}
		r.regions = append(r.regions, occupied())
		r.food = append(r.food, float64(w.FoodLedger().OnGround))
		r.surviving = append(r.surviving, n > floor)
	}
	record()
	r.series = append(r.series, int32(len(w.Bodies())))
	r.valley = math.Inf(1)
	r.lives, r.lifeKids = map[float64]float64{}, map[float64]float64{}
	meanShare := func() shareStats { return shareStatsOf(w.Bodies(), cfg.Speed) }
	if cfg.Allot {
		r.shareLive = append(r.shareLive, meanShare())
	}
	known := map[int64]engine.Body{}
	kids := map[int64]int{}
	edgeTile := edgeTiles(m)
	bornAt := map[int64]int64{}
	for _, b := range w.Bodies() {
		bornAt[b.ID] = b.Born
	}
	var traced, backs float64
	w.SetTrace(func(b engine.Body, _ engine.Valuation, a engine.Action) {
		if w.Tick() <= 5000 {
			return
		}
		traced++
		if a.Kind == engine.ActMove && b.Heading >= 0 && a.Dir == (b.Heading+4)%8 {
			backs++
		}
	})
	var was map[int64][2]float64
	var dSum, dN, eSum, eN, aSum, aN, mSum, mN, fSum float64
	var sLow, sHigh, sN float64
	var fLow, fHigh, fN float64
	var zLow, zHigh, zN, bSum1, bN1, bSum2, bN2 float64
	var uSum, uN float64
	var vLow, vHigh, vN float64
	spBudget, spN := make([]float64, species), make([]float64, species)
	spFood, spFoodN := make([]float64, species), 0.0
	var st5 engine.Stats           // the counts as of tick 5000
	budgets := map[int64]float64{} // every body seen, for its children
	var cN, cX, cY, cXX, cYY, cXY float64
	for _, b := range w.Bodies() {
		known[b.ID] = b
	}
	every := max(ticks/checkpoints, 1)
	half := ticks / 2
	sampled := 0.0
	for t := 1; t <= ticks; t++ {
		w.Step()
		if l := w.FoodLedger(); !l.Balanced() {
			return result{}, fmt.Errorf("tick %d: food ledger does not close: %+v", t, l)
		}
		if l := w.BudgetLedger(); !l.Balanced() {
			return result{}, fmt.Errorf("tick %d: budget ledger does not close: %+v", t, l)
		}
		if t == 5000 {
			st5 = w.Stats()
		}
		r.series = append(r.series, int32(len(w.Bodies())))
		r.valley = math.Min(r.valley, float64(len(w.Bodies())))
		if cfg.Learn {
			// Ages at death, for the median life.
			alive := make(map[int64]bool, len(bornAt))
			for _, b := range w.Bodies() {
				alive[b.ID] = true
				if _, ok := bornAt[b.ID]; !ok {
					bornAt[b.ID] = b.Born
				}
			}
			for id, born := range bornAt {
				if !alive[id] {
					if t > 5000 {
						r.deathAges = append(r.deathAges, float64(int64(t)-born))
					}
					delete(bornAt, id)
				}
			}
		}
		if cfg.Allot {
			// Lifetime births by share: credit the parents of the newborn,
			// and close the lives of the dead.
			alive := map[int64]bool{}
			for _, b := range w.Bodies() {
				alive[b.ID] = true
				if _, ok := known[b.ID]; !ok {
					known[b.ID] = b
					budgets[b.ID] = b.Budget
					for _, p := range b.Parents {
						kids[p]++
					}
					if pa, ok := budgets[b.Parents[0]]; ok {
						if pb, ok := budgets[b.Parents[1]]; ok {
							x, y := (pa+pb)/2, b.Budget
							cN++
							cX, cY, cXX, cYY, cXY = cX+x, cY+y, cXX+x*x, cYY+y*y, cXY+x*y
						}
					}
				}
			}
			for id, b := range known {
				if alive[id] {
					continue
				}
				if b.Born > int64(ticks/4) {
					if _, ok := r.lives[b.Share]; !ok {
						r.shares = append(r.shares, b.Share)
					}
					r.lives[b.Share]++
					r.lifeKids[b.Share] += float64(kids[id])
				}
				delete(known, id)
				delete(kids, id)
			}
			if t%every == 0 {
				r.shareLive = append(r.shareLive, meanShare())
			}
		}
		if t%every == 0 {
			record()
		}
		if cfg.Learn && t > 5000 && t%every == 0 {
			readAges(w, m, &r.learnAge)
			readProvenance(w, &r)
		}
		if t > 5000 && t%1000 == 0 {
			now := map[int64][2]float64{}
			bs := w.Bodies()
			on := 0.0
			var bySex [3]float64
			low, high := 0.0, 0.0
			fertLow, fertHigh := 0.0, 0.0
			sizeLow, sizeHigh, budget := 0.0, 0.0, 0.0
			unk := 0.0
			boldLow, boldHigh := 0.0, 0.0
			for k := 0; k < species; k++ {
				spFood[k] += float64(w.GroundFoodOf(k))
			}
			spFoodN++
			for _, b := range bs {
				if w.Tick() >= b.Mature {
					bySex[b.Sex]++
					budget += b.Budget
					spBudget[b.Species] += b.Budget
					spN[b.Species]++
					switch w.BodyBold(b) {
					case 0:
						boldLow++
					case cfg.VentureMost:
						boldHigh++
					}
					if w.RegionEvidence(b, m.RegionAt(int(b.X), int(b.Y))) < cfg.RegionWeight {
						unk++
					}
					switch w.BodySize(b) {
					case 0:
						sizeLow++
					case cfg.BudgetLevels - 1:
						sizeHigh++
					}
					switch w.BodySight(b) {
					case 0:
						low++
					case cfg.SightMost:
						high++
					}
					switch w.BodyFertility(b) {
					case 0:
						fertLow++
					case cfg.FertMost:
						fertHigh++
					}
				}
			}
			if len(bs) > 0 {
				adults := bySex[0] + bySex[1] + bySex[2]
				aSum += adults / float64(len(bs))
				aN++
				if adults > 0 {
					uSum += unk / adults
					uN++
				}
				if cfg.AllotSight && adults > 0 {
					sLow += low / adults
					sHigh += high / adults
					sN++
				}
				if cfg.AllotFertility && adults > 0 {
					fLow += fertLow / adults
					fHigh += fertHigh / adults
					fN++
				}
				if cfg.AllotVenture && adults > 0 {
					vLow += boldLow / adults
					vHigh += boldHigh / adults
					vN++
				}
				if cfg.AllotBudget && adults > 0 {
					zLow += sizeLow / adults
					zHigh += sizeHigh / adults
					zN++
					if t <= ticks/2 {
						bSum1 += budget / adults
						bN1++
					} else {
						bSum2 += budget / adults
						bN2++
					}
				}
				if f, m := bySex[engine.Female], bySex[engine.Male]; f+m > 0 {
					mSum += math.Min(f, m) / (f + m)
					fSum += f / (f + m)
					mN++
				}
			}
			for _, b := range bs {
				now[b.ID] = [2]float64{b.X, b.Y}
				if p, ok := was[b.ID]; ok {
					dSum += math.Hypot(b.X-p[0], b.Y-p[1])
					dN++
				}
				if edgeTile[int(b.Y)*m.Width+int(b.X)] {
					on++
				}
			}
			if len(bs) > 0 {
				eSum += on / float64(len(bs))
				eN++
			}
			was = now
		}
		if len(w.Bodies()) <= floor {
			// Early termination: this seed stops, the run goes on for
			// the others. The remaining checkpoints are not surviving.
			r.fellAt = t
			bySpecies := make([]float64, species)
			for _, b := range w.Bodies() {
				bySpecies[b.Species]++
			}
			for len(r.pop) < ticks/every+1 {
				r.pop = append(r.pop, float64(len(w.Bodies())))
				for k := range r.speciesPop {
					// The count at the fall, as pop is: not the last
					// checkpoint's.
					r.speciesPop[k] = append(r.speciesPop[k], bySpecies[k])
				}
				r.regions = append(r.regions, math.NaN())
				r.food = append(r.food, math.NaN())
				r.surviving = append(r.surviving, false)
			}
			break
		}
		if t > half {
			bodies := w.Bodies()
			if len(bodies) > 0 {
				for _, b := range bodies {
					r.regionOf[m.RegionAt(int(b.X), int(b.Y))] += 1 / float64(len(bodies))
				}
				sampled++
			}
		}
	}
	if sampled > 0 {
		r.regionOK = true
		for i := range r.regionOf {
			r.regionOf[i] /= sampled
		}
	}
	r.displace, r.edge = dSum/math.Max(dN, 1), eSum/math.Max(eN, 1)
	r.unknown = uSum / math.Max(uN, 1)
	r.back = backs / math.Max(traced, 1)
	r.adults = aSum / math.Max(aN, 1)
	r.sightLow, r.sightHigh = sLow/math.Max(sN, 1), sHigh/math.Max(sN, 1)
	r.fertLow, r.fertHigh = fLow/math.Max(fN, 1), fHigh/math.Max(fN, 1)
	r.sizeLow, r.sizeHigh = zLow/math.Max(zN, 1), zHigh/math.Max(zN, 1)
	r.boldLow, r.boldHigh = vLow/math.Max(vN, 1), vHigh/math.Max(vN, 1)
	r.speciesGone, r.speciesBudget = make([]float64, species), make([]float64, species)
	r.speciesFood = make([]float64, species)
	for k := 0; k < species; k++ {
		if r.speciesPop[k][len(r.speciesPop[k])-1] == 0 {
			r.speciesGone[k] = 1
		}
		r.speciesFood[k] = math.NaN()
		if spFoodN > 0 {
			r.speciesFood[k] = spFood[k] / spFoodN
		}
		r.speciesBudget[k] = math.NaN()
		if spN[k] > 0 {
			r.speciesBudget[k] = spBudget[k] / spN[k]
		}
	}
	r.budgetCorr = math.NaN()
	if cN >= 3 {
		vx, vy := cXX-cX*cX/cN, cYY-cY*cY/cN
		if vx > 0 && vy > 0 {
			r.budgetCorr = (cXY - cX*cY/cN) / math.Sqrt(vx*vy)
		}
	}
	r.meanBudget, r.budgetDrift = math.NaN(), math.NaN()
	if bN1+bN2 > 0 {
		r.meanBudget = (bSum1 + bSum2) / (bN1 + bN2)
	}
	if bN1 > 0 && bN2 > 0 {
		r.budgetDrift = bSum2/bN2 - bSum1/bN1 // a collapsed seed has no second half
	}
	r.minority, r.female = math.NaN(), math.NaN()
	if mN > 0 {
		r.minority, r.female = mSum/mN, fSum/mN
	}
	st := w.Stats()
	if cfg.Learn {
		r.rows = memRows(w, m)
	}
	r.untrod = math.NaN()
	if steps := st.Steps - st5.Steps; steps > 0 {
		r.untrod = float64(st.NewSteps-st5.NewSteps) / float64(steps)
	}
	r.boldSlope = math.NaN()
	if cfg.AllotVenture {
		n := cfg.VentureMost + 1
		r.boldSteps, r.boldNew = make([]int64, n), make([]int64, n)
		for k := 0; k < n; k++ {
			if k < len(st.BoldSteps) {
				r.boldSteps[k] = st.BoldSteps[k]
				r.boldNew[k] = st.BoldNewSteps[k]
			}
			if k < len(st5.BoldSteps) {
				r.boldSteps[k] -= st5.BoldSteps[k]
				r.boldNew[k] -= st5.BoldNewSteps[k]
			}
		}
		if top := cfg.VentureMost; r.boldSteps[0] > 0 && r.boldSteps[top] > 0 {
			r.boldSlope = boldRate(r, top) - boldRate(r, 0)
		}
	}
	r.starved = float64(st.Deaths[engine.CauseStarved])
	r.aged = float64(st.Deaths[engine.CauseAged])
	r.sleepless = float64(st.Deaths[engine.CauseSleepless])
	r.burned = st.EnergyBurned
	total := 0.0
	for _, n := range st.Actions {
		total += float64(n)
	}
	r.asleep = float64(st.Actions[engine.ActSleep]) / math.Max(total, 1)
	r.sheltered = float64(st.SleptSheltered) / math.Max(float64(st.Slept), 1)
	r.built, r.builtSleeps, r.returns = float64(st.Built), float64(st.BuiltSleeps), float64(st.Returns)
	if n := len(st.GenBirths); n > 0 {
		r.maxGen = float64(n - 1)
		r.topGenBirths = float64(st.GenBirths[n-1])
		r.genBirths = append([]int64(nil), st.GenBirths...)
		r.genBudget = make([]float64, n)
		for g := range r.genBudget {
			if g < len(st.GenBudget) && st.GenBirths[g] > 0 {
				r.genBudget[g] = st.GenBudget[g] / float64(st.GenBirths[g])
			}
		}
	}
	r.begun, r.fetched, r.abandoned = float64(st.Begun), float64(st.Fetched), float64(st.Abandoned)
	r.tentsMade, r.tentNights, r.tentsWorn = float64(st.TentsMade), float64(st.TentNights), float64(st.TentsWorn)
	r.tentsDropped, r.tentsPicked = float64(st.TentsDropped), float64(st.TentsPicked)
	for k, n := range st.Actions {
		if total > 0 {
			r.actions[k] = float64(n) / total
		}
	}
	decisions := 0.0
	for _, n := range st.Decisions {
		decisions += float64(n)
	}
	for k, n := range st.Decisions {
		if decisions > 0 {
			r.why[k] = float64(n) / decisions
		}
	}
	if total > 0 {
		r.decided = decisions / total
	}
	r.births = float64(st.Births)
	r.matings = float64(st.Matings)
	r.fertMatings, r.fertBirths = append([]int64(nil), st.FertMatings...), append([]int64(nil), st.FertBirths...)
	r.fertSlope = math.NaN()
	if top := cfg.FertMost; cfg.AllotFertility && top < len(r.fertMatings) && r.fertMatings[0] > 0 && r.fertMatings[top] > 0 {
		r.fertSlope = fertRate(r, top) - fertRate(r, 0)
	}
	r.provisioned = float64(w.FoodLedger().Provisioned)
	r.requests, r.requestBirths = float64(st.Requests), float64(st.RequestBirths)
	for i := range r.rows {
		c := rowCount(st, r.rows[i].key)
		r.rows[i].learned, r.rows[i].passed = float64(c.Learned), float64(c.Passed)
	}
	r.adultRate = math.NaN()
	if n := st.Matured + st.DiedYoung; n > 0 {
		r.adultRate = float64(st.Matured) / float64(n)
	}
	return r, nil
}

// replayPoint is the run of one variant that the client replays, and the
// tick it starts drawing from (MEASURE.md "UI での再生").
type replayPoint struct {
	ok     bool  // false when every seed collapsed
	seed   int64 // the representative seed
	steady int   // the steady-state tick
}

// chooseReplay picks the representative seed and its steady-state tick.
//
// Only seeds that never collapsed are candidates: a collapsed seed has no
// average over the experiment to be close to. The experiment's average and
// standard deviation are those of the population over every tick of every
// candidate. The representative seed is the one whose own average over the
// run is closest to it (the lower seed on a tie); its steady-state tick is
// the first tick its population is within one standard deviation of it.
func chooseReplay(rs []result, seed0 int64) replayPoint {
	var sum, sumSq, n float64
	for _, r := range rs {
		if r.fellAt >= 0 {
			continue
		}
		for _, x := range r.series {
			sum += float64(x)
			sumSq += float64(x) * float64(x)
			n++
		}
	}
	if n == 0 {
		return replayPoint{}
	}
	mean := sum / n
	sd := math.Sqrt(math.Max(sumSq/n-mean*mean, 0))

	best, bestGap := -1, math.Inf(1)
	for i, r := range rs {
		if r.fellAt >= 0 {
			continue
		}
		own := 0.0
		for _, x := range r.series {
			own += float64(x)
		}
		own /= float64(len(r.series))
		if gap := math.Abs(own - mean); gap < bestGap {
			best, bestGap = i, gap
		}
	}
	p := replayPoint{ok: true, seed: seed0 + int64(best)}
	for t, x := range rs[best].series {
		if math.Abs(float64(x)-mean) <= sd {
			p.steady = t
			break
		}
	}
	return p
}

// replayCommand is the line that opens the replay in the client. The
// variant is named only when it is not the base, which is the client's
// default.
func replayCommand(o options, v string, p replayPoint) string {
	c := fmt.Sprintf("go run ./cmd/client -map %s -w %d -h %d -seed %d -from-tick %d", o.mapName, o.width, o.height, p.seed, p.steady)
	if v != variant.Base {
		c += " -variant " + v
	}
	return c
}

// commit names the binary's source: the short hash, with -dirty when tracked
// files have uncommitted changes, or "(none)" outside a repository with
// commits.
func commit() string {
	hash, err := exec.Command("git", "rev-parse", "--short", "HEAD").Output()
	if err != nil {
		return "(none)"
	}
	c := strings.TrimSpace(string(hash))
	status, err := exec.Command("git", "status", "--porcelain", "--untracked-files=no").Output()
	if err != nil || len(strings.TrimSpace(string(status))) > 0 {
		c += "-dirty"
	}
	return c
}

// meanSE returns the mean and its standard error.
func meanSE(xs []float64) (float64, float64) {
	n := float64(len(xs))
	if n == 0 {
		return math.NaN(), math.NaN()
	}
	sum := 0.0
	for _, x := range xs {
		sum += x
	}
	mean := sum / n
	if n < 2 {
		return mean, math.NaN()
	}
	ss := 0.0
	for _, x := range xs {
		ss += (x - mean) * (x - mean)
	}
	return mean, math.Sqrt(ss/(n-1)) / math.Sqrt(n)
}

// fmtMeanSE prints "mean ± SE"; with fewer than two values there is no
// standard error to print.
func fmtMeanSE(xs []float64, prec int) string {
	m, se := meanSE(xs)
	if math.IsNaN(se) {
		return fmt.Sprintf("%.*f ± —", prec, m)
	}
	return fmt.Sprintf("%.*f ± %.*f", prec, m, prec, se)
}

// fertRate is the births per mating agreed on at fertility level k in
// result r, which must have seen a mating at it.
func fertRate(r result, k int) float64 {
	return float64(r.fertBirths[k]) / float64(r.fertMatings[k])
}

// speciesAt reads f for the species named, by its row in the result's
// world; NaN where the result's world has no species of that name (a
// world of one species is all human).
func speciesAt(name string, f func(result, int) float64) func(result) float64 {
	return func(r result) float64 {
		for k, n := range r.speciesNames {
			if n == name {
				return f(r, k)
			}
		}
		return math.NaN()
	}
}

// boldRate is the untrodden-tile rate of exploratory level k in result r.
func boldRate(r result, k int) float64 {
	return float64(r.boldNew[k]) / float64(r.boldSteps[k])
}

// collectWhere is collect over the results for which keep is true.
func collectWhere(rs []result, keep func(result) bool, f func(result) float64) []float64 {
	var xs []float64
	for _, r := range rs {
		if keep(r) {
			xs = append(xs, f(r))
		}
	}
	return xs
}

func collect(rs []result, f func(result) float64) []float64 {
	xs := make([]float64, len(rs))
	for i, r := range rs {
		xs[i] = f(r)
	}
	return xs
}

// regionShares returns each region's share of the land and of the food.
func regionShares(m engine.Map) (area, food []float64) {
	area = make([]float64, len(m.RegionFood))
	food = make([]float64, len(m.RegionFood))
	land := 0.0
	for i, t := range m.Terrain {
		if t == engine.TerrainLand {
			area[m.Region[i]]++
			land++
		}
	}
	sum := 0.0
	for r, s := range m.RegionFood {
		if area[r] > 0 {
			sum += s
		}
	}
	for r := range area {
		if area[r] > 0 {
			food[r] = m.RegionFood[r] / sum
		}
		area[r] /= land
	}
	return area, food
}

// writeReport prints sections 2 to 9 of MEASURE.md with their fixed columns,
// each figure the mean over seeds ± its standard error. Section 1 is written
// by whoever reads the numbers. The base variant fills the single-variant
// tables.
func writeReport(out io.Writer, o options, m engine.Map, command string, results map[string][]result) {
	p := func(format string, a ...any) { fmt.Fprintf(out, format, a...) }
	base := o.variants[0]
	rs := results[base]

	p("### 2. 条件\n\n")
	p("| 項目 | 値 |\n| --- | --- |\n")
	p("| コミット | %s |\n", commit())
	p("| マップ | %s (%dx%d) |\n", o.mapName, o.width, o.height)
	p("| tick 数 | %d |\n", o.ticks)
	p("| シード数 | %d（最初のシード %d） |\n", o.seeds, o.seed0)
	p("| 比較対象（base） | %s |\n", base)
	replays := make([]replayPoint, len(o.variants))
	var cells []string
	for i, v := range o.variants {
		replays[i] = chooseReplay(results[v], o.seed0)
		cell := "なし（全シードが崩壊）"
		if replays[i].ok {
			cell = fmt.Sprintf("シード %d・tick %d", replays[i].seed, replays[i].steady)
		}
		if len(o.variants) > 1 {
			cell = v + ": " + cell
		}
		cells = append(cells, cell)
	}
	p("| 代表シード・定常化 tick | %s |\n", strings.Join(cells, " / "))
	p("| 詳細レポート | |\n\n")
	p("**再現方法**:\n\n```\n%s\n```\n\n", command)
	p("**UI での再生**:\n\n")
	var lines []string
	for i, v := range o.variants {
		if replays[i].ok {
			lines = append(lines, replayCommand(o, v, replays[i]))
		}
	}
	if len(lines) > 0 {
		p("```\n%s\n```\n\n", strings.Join(lines, "\n"))
	} else {
		p("再生できるシードが無い（崩壊したシードは代表シードの候補から外す）。\n\n")
	}

	every := max(o.ticks/checkpoints, 1)
	// rows is how many checkpoints have any surviving seed; the rows after
	// every seed has fallen are left out.
	rows := 0
	for i := range rs[0].pop {
		for _, r := range rs {
			if r.surviving[i] {
				rows = i + 1
				break
			}
		}
	}
	aliveAt := func(i int) func(result) bool { return func(r result) bool { return r.surviving[i] } }
	for _, v := range o.variants {
		rs := results[v]
		p("### 3. 人口推移（%s・生存しているシードの平均）\n\n", v)
		p("| tick | 人口（全体） | 人間 | 獣 | 鳥 | 魚 | 生存シード数 |\n| --- | --- | --- | --- | --- | --- | --- |\n")
		popCfg, _ := variant.Config(v, o.seed0)
		speciesCol := func(name string, i int) string {
			for k, s := range popCfg.Species {
				if s.Name == name {
					return fmtMeanSE(collectWhere(rs, aliveAt(i), func(r result) float64 { return r.speciesPop[k][i] }), 1)
				}
			}
			if name == "human" && len(popCfg.Species) <= 1 {
				return fmtMeanSE(collectWhere(rs, aliveAt(i), func(r result) float64 { return r.pop[i] }), 1)
			}
			return ""
		}
		vrows := 0
		for i := range rs[0].pop {
			for _, r := range rs {
				if r.surviving[i] {
					vrows = i + 1
					break
				}
			}
		}
		for i := 0; i < vrows; i++ {
			xs := collectWhere(rs, aliveAt(i), func(r result) float64 { return r.pop[i] })
			p("| %d | %s | %s | %s | %s | %s | %d / %d |\n", i*every, fmtMeanSE(xs, 1), speciesCol("human", i), speciesCol("beast", i), speciesCol("bird", i), speciesCol("fish", i), len(xs), len(rs))
		}
		if v != base {
			p("\n")
		}
	}
	if rows < len(rs[0].pop) {
		p("\n%d tick 以降は全シードが崩壊の下限（%d）を割ったので省略。\n", rows*every, o.floor)
	}
	fell := collectWhere(rs, func(r result) bool { return r.fellAt >= 0 }, func(r result) float64 { return float64(r.fellAt) })
	p("\n崩壊したシード: %d / %d（下限 %d）。", len(fell), len(rs), o.floor)
	if len(fell) > 0 {
		p("初めて下限を割った tick: %s\n\n", fmtMeanSE(fell, 0))
	} else {
		p("\n\n")
	}

	p("### 4. 出来事の回数（%s）\n\n", base)
	p("| 出来事 | 回数 |\n| --- | --- |\n")
	births := fmtMeanSE(collect(rs, func(r result) float64 { return r.births }), 2)
	p("| 交配 | %s |\n| 出生 | %s |\n", fmtMeanSE(collect(rs, func(r result) float64 { return r.matings }), 2), births)
	for _, e := range []string{"種族内の戦い", "種族間の戦い", "コインを拾った", "売買"} {
		p("| %s | 0 |\n", e)
	}
	p("| 休んでいる母に配られた食料 | %s |\n", fmtMeanSE(collect(rs, func(r result) float64 { return r.provisioned }), 2))
	p("| 交配要求 | %s |\n| 交配要求で成り立った出生 | %s |\n", fmtMeanSE(collect(rs, func(r result) float64 { return r.requests }), 2), fmtMeanSE(collect(rs, func(r result) float64 { return r.requestBirths }), 2))
	p("| 体力消耗（合計） | %s |\n", fmtMeanSE(collect(rs, func(r result) float64 { return r.burned }), 1))
	p("| 死亡 | %s |\n\n", fmtMeanSE(collect(rs, func(r result) float64 { return r.starved + r.aged + r.sleepless }), 2))

	p("### 5. 行動の選択割合（%s）\n\n", base)
	p("| 行動 | 割合 |\n| --- | --- |\n")
	for k, name := range []string{"待つ", "食べる", "移動", "交配", "拾う"} {
		p("| %s | %s%% |\n", name, fmtMeanSE(collect(rs, func(r result) float64 { return 100 * r.actions[k] }), 2))
	}
	p("\n")

	adult := collectWhere(rs, func(r result) bool { return !math.IsNaN(r.adultRate) }, func(r result) float64 { return r.adultRate })
	p("### 補助の表: 成人到達率（%s）\n\n", base)
	if len(adult) > 0 {
		p("成人到達率: %s（%d / %d シード。成人した子 ÷（成人した子 ＋ 成人前に死んだ子））\n\n", fmtMeanSE(adult, 4), len(adult), len(rs))
	} else {
		p("成人到達率: —（成人した子も成人前に死んだ子もいない）\n\n")
	}

	p("### 補助の表: 世代ごとの出生（%s、上位3世代）\n\n", base)
	p("| 世代 | 出生（1シードあたり） |\n| --- | --- |\n")
	top := 0
	for _, r := range rs {
		if n := len(r.genBirths); n > top {
			top = n
		}
	}
	for g := top - 1; g >= 0 && g >= top-3; g-- {
		p("| %d | %s |\n", g, fmtMeanSE(collect(rs, func(r result) float64 {
			if g < len(r.genBirths) {
				return float64(r.genBirths[g])
			}
			return 0
		}), 2))
	}
	p("\n")

	// Mean budget by generation, for every condition that has the budget
	// in the genome (the base usually has not).
	for _, v := range o.variants {
		if vCfg, _ := variant.Config(v, o.seed0); !vCfg.AllotBudget {
			continue
		}
		vs := results[v]
		p("### 補助の表: 世代ごとの平均予算（%s、10 世代ごと）\n\n", v)
		p("その世代に生まれた身体の予算の平均（1シードあたり。その世代の出生が無いシードは除く）。世代とともに上がり続ければ、予算に価格が足りない。\n\n")
		p("| 世代 | 平均予算 |\n| --- | --- |\n")
		vTop := 0
		for _, r := range vs {
			vTop = max(vTop, len(r.genBirths))
		}
		for g := 0; g < vTop; g += 10 {
			p("| %d | %s |\n", g, fmtMeanSE(collectWhere(vs, func(r result) bool { return g < len(r.genBirths) && r.genBirths[g] > 0 }, func(r result) float64 { return r.genBudget[g] }), 4))
		}
		p("\n")
	}
	if baseCfg, _ := variant.Config(base, o.seed0); baseCfg.Allot {
		writeAllot(p, o, rs, every, rows)
	}
	if baseCfg, _ := variant.Config(base, o.seed0); baseCfg.AllotFertility {
		p("### 補助の表: 繁殖能力の段ごとの交配と出生（%s）\n\n", base)
		p("成立した交配と出生に親として入った身体を段ごとに数えたもの（1回の交配で両親それぞれ 1）。出生率はシードごとの出生 ÷ 交配。\n\n")
		p("| 段 | 交配（1シードあたり） | 出生（1シードあたり） | 交配1回あたりの出生 |\n| --- | --- | --- | --- |\n")
		for k := 0; k <= baseCfg.FertMost; k++ {
			at := func(xs []int64) float64 {
				if k < len(xs) {
					return float64(xs[k])
				}
				return 0
			}
			p("| %d | %s | %s | %s |\n", k,
				fmtMeanSE(collect(rs, func(r result) float64 { return at(r.fertMatings) }), 2),
				fmtMeanSE(collect(rs, func(r result) float64 { return at(r.fertBirths) }), 2),
				fmtMeanSE(collectWhere(rs, func(r result) bool { return k < len(r.fertMatings) && r.fertMatings[k] > 0 }, func(r result) float64 { return fertRate(r, k) }), 4))
		}
		p("\n")
	}

	p("### 補助の表: 占有地域数と個体数の谷（%s）\n\n", base)
	p("| tick | 占有地域数 | 生存シード数 |\n| --- | --- | --- |\n")
	for i := 0; i < rows; i++ {
		xs := collectWhere(rs, aliveAt(i), func(r result) float64 { return r.regions[i] })
		p("| %d | %s | %d / %d |\n", i*every, fmtMeanSE(xs, 2), len(xs), len(rs))
	}
	valleys := collectWhere(rs, func(r result) bool { return r.fellAt < 0 }, func(r result) float64 { return r.valley })
	if len(valleys) > 0 {
		lo := valleys[0]
		for _, x := range valleys {
			lo = math.Min(lo, x)
		}
		p("\n崩壊しなかったシードの個体数の谷（tick 1 以降の最小）: 平均 %s、最小 %.0f（%d シード）\n\n", fmtMeanSE(valleys, 1), lo, len(valleys))
	} else {
		p("\n崩壊しなかったシードが無い。\n\n")
	}

	p("### 補助の表: 判断のきっかけ（%s）\n\n", base)
	p("決定の割合: %s\n\n", fmtMeanSE(collect(rs, func(r result) float64 { return r.decided }), 4))
	p("| きっかけ | 決定に占める割合 |\n| --- | --- |\n")
	for k, name := range engine.TriggerNames {
		p("| %s | %s |\n", name, fmtMeanSE(collect(rs, func(r result) float64 { return r.why[k] }), 4))
	}
	p("\n")

	p("### 6. 記憶\n\n")
	if baseCfg, _ := variant.Config(base, o.seed0); baseCfg.Learn {
		writeMemory(p, rs)
	} else {
		p("| 種別 | 上位10 |\n| --- | --- |\n")
		p("| 合成されていない高頻度記憶 | |\n| 合成された高頻度記憶 | |\n| ノード平均が高得点の中間項 | |\n\n")
	}

	writeProvenance(p, rs)

	p("### 7. スキル別伝承数\n\n")
	if baseCfg, _ := variant.Config(base, o.seed0); baseCfg.Learn && baseCfg.Tell {
		p("スキルはまだ無い。学んだ行（記憶）が伝達された回数は §6 の「伝達された回数の上位10」。\n\n")
	}
	p("| スキル | 伝承された回数 | 保有率 |\n| --- | --- | --- |\n\n")

	p("### 8. 死因別死亡数（%s）\n\n", base)
	p("| 死因 | 回数 | 割合 |\n| --- | --- | --- |\n")
	for _, c := range []struct {
		name string
		f    func(result) float64
	}{
		{"餓死", func(r result) float64 { return r.starved }},
		{"老い", func(r result) float64 { return r.aged }},
		{"睡眠不足", func(r result) float64 { return r.sleepless }},
	} {
		share := collectWhere(rs, func(r result) bool { return r.starved+r.aged+r.sleepless > 0 }, func(r result) float64 { return c.f(r) / (r.starved + r.aged + r.sleepless) })
		sh := "—"
		if len(share) > 0 {
			sh = fmtMeanSE(share, 4)
		}
		p("| %s | %s | %s |\n", c.name, fmtMeanSE(collect(rs, c.f), 2), sh)
	}
	p("\n")

	p("### 補助の表: 地面の食料（%s）\n\n", base)
	p("| tick | 地面の食料 | 上限に対する割合 | 生存シード数 |\n| --- | --- | --- | --- |\n")
	baseCfg, _ := variant.Config(base, o.seed0)
	foodCap := float64(baseCfg.FoodCap)
	for i := 0; i < rows; i++ {
		xs := collectWhere(rs, aliveAt(i), func(r result) float64 { return r.food[i] })
		mean, _ := meanSE(xs)
		p("| %d | %s | %.2f | %d / %d |\n", i*every, fmtMeanSE(xs, 1), mean/foodCap, len(xs), len(rs))
	}
	p("\n")

	area, food := regionShares(m)
	p("### 補助の表: 地域別人口比（%s・後半の平均）\n\n", base)
	p("| 地域 | 面積比 | 食料比 | 人口比 | シード数 |\n| --- | --- | --- | --- | --- |\n")
	regionOK := func(r result) bool { return r.regionOK }
	for reg := range area {
		xs := collectWhere(rs, regionOK, func(r result) float64 { return r.regionOf[reg] })
		pop := "—"
		if len(xs) > 0 {
			pop = fmtMeanSE(xs, 3)
		}
		p("| %d | %.3f | %.3f | %s | %d / %d |\n", reg, area[reg], food[reg], pop, len(xs), len(rs))
	}
	p("\n")

	p("### 補助の表: 動き方（%s）\n\n", base)
	p("| 量 | 値 |\n| --- | --- |\n")
	p("| 1000 tick の変位（タイル、tick 5000 より後） | %s |\n", fmtMeanSE(collect(rs, func(r result) float64 { return r.displace }), 2))
	p("| 縁（地図の端か水の隣のタイル）にいる割合 | %s（縁の面積比 %.4f） |\n", fmtMeanSE(collect(rs, func(r result) float64 { return r.edge }), 4), edgeShare(m))
	p("| 未踏タイル率（tick 5000 より後） | %s |\n", fmtMeanSE(collect(rs, func(r result) float64 { return r.untrod }), 4))
	p("| 知らない地域にいる成人の割合 | %s |\n", fmtMeanSE(collect(rs, func(r result) float64 { return r.unknown }), 4))
	p("| 真後ろへの手（tick 5000 より後の決定のうち） | %s |\n\n", fmtMeanSE(collect(rs, func(r result) float64 { return r.back }), 4))

	p("### 補助の表: 成人と性（%s、tick 5000 より後、1000 tick ごと）\n\n", base)
	p("| 量 | 値 |\n| --- | --- |\n")
	p("| 生きている身体のうち成人の割合 | %s |\n", fmtMeanSE(collect(rs, func(r result) float64 { return r.adults }), 4))
	if xs := collectWhere(rs, func(r result) bool { return !math.IsNaN(r.minority) }, func(r result) float64 { return r.minority }); len(xs) > 0 {
		p("| 成人のうち少ない側の性の割合 | %s |\n", fmtMeanSE(xs, 4))
		p("| 成人のうち女の割合 | %s |\n\n", fmtMeanSE(collectWhere(rs, func(r result) bool { return !math.IsNaN(r.female) }, func(r result) float64 { return r.female }), 4))
	} else {
		p("| 成人のうち少ない側の性の割合 | —（性別なし） |\n\n")
	}

	p("### 9. 対の差\n\n")
	if len(o.variants) < 2 {
		p("（比較対象なし）\n")
		return
	}
	p("| 指標 | %s | %s | 差 |\n| --- | --- | --- | --- |\n", base, "本件")
	metrics := []struct {
		name string
		prec int
		f    func(result) float64
	}{
		{"人口（最終）", 2, func(r result) float64 { return r.pop[len(r.pop)-1] }},
		{"人間の個体数（最終）", 2, speciesAt("human", func(r result, k int) float64 { return r.speciesPop[k][len(r.speciesPop[k])-1] })},
		{"獣の個体数（最終）", 2, speciesAt("beast", func(r result, k int) float64 { return r.speciesPop[k][len(r.speciesPop[k])-1] })},
		{"人間が絶滅したシードの割合", 4, speciesAt("human", func(r result, k int) float64 { return r.speciesGone[k] })},
		{"獣が絶滅したシードの割合", 4, speciesAt("beast", func(r result, k int) float64 { return r.speciesGone[k] })},
		{"人間の成人の平均予算", 4, speciesAt("human", func(r result, k int) float64 { return r.speciesBudget[k] })},
		{"獣の成人の平均予算", 4, speciesAt("beast", func(r result, k int) float64 { return r.speciesBudget[k] })},
		{"人間の食料（地面）", 1, speciesAt("human", func(r result, k int) float64 { return r.speciesFood[k] })},
		{"獣の食料（地面）", 1, speciesAt("beast", func(r result, k int) float64 { return r.speciesFood[k] })},
		{"崩壊したシードの割合", 4, func(r result) float64 {
			if r.fellAt > 0 {
				return 1
			}
			return 0
		}},
		{"餓死", 2, func(r result) float64 { return r.starved }},
		{"老いによる死", 2, func(r result) float64 { return r.aged }},
		{"睡眠不足による死", 2, func(r result) float64 { return r.sleepless }},
		{"眠っている割合", 4, func(r result) float64 { return r.asleep }},
		{"休める場所で始まった眠りの割合", 4, func(r result) float64 { return r.sheltered }},
		{"始めた定住場所", 2, func(r result) float64 { return r.begun }},
		{"建てた場所", 2, func(r result) float64 { return r.built }},
		{"建てている途中で食べに行った", 2, func(r result) float64 { return r.fetched }},
		{"あきらめた定住場所", 2, func(r result) float64 { return r.abandoned }},
		{"建てた場所で始まった眠り", 2, func(r result) float64 { return r.builtSleeps }},
		{"建てた本人が戻って眠った", 2, func(r result) float64 { return r.returns }},
		{"作ったテント", 2, func(r result) float64 { return r.tentsMade }},
		{"テントで眠った夜", 2, func(r result) float64 { return r.tentNights }},
		{"使い切ったテント", 2, func(r result) float64 { return r.tentsWorn }},
		{"死んで残ったテント", 2, func(r result) float64 { return r.tentsDropped }},
		{"拾われたテント", 2, func(r result) float64 { return r.tentsPicked }},
		{"体力消耗（合計）", 2, func(r result) float64 { return r.burned }},
		{"出生", 2, func(r result) float64 { return r.births }},
		{"成人到達率", 4, func(r result) float64 { return r.adultRate }},
		{"最大世代数", 2, func(r result) float64 { return r.maxGen }},
		{"最大世代数の出生数", 2, func(r result) float64 { return r.topGenBirths }},
		{"1000 tick の変位（タイル）", 2, func(r result) float64 { return r.displace }},
		{"縁（地図の端か水の隣）にいる割合", 4, func(r result) float64 { return r.edge }},
		{"未踏タイル率", 4, func(r result) float64 { return r.untrod }},
		{"知らない地域にいる成人の割合", 4, func(r result) float64 { return r.unknown }},
		{"開拓0の成人の割合", 4, func(r result) float64 { return r.boldLow }},
		{"開拓が最大の成人の割合", 4, func(r result) float64 { return r.boldHigh }},
		{"開拓の最高段と最低段の未踏タイル率の差", 4, func(r result) float64 { return r.boldSlope }},
		{"真後ろへの手（決定のうち）", 4, func(r result) float64 { return r.back }},
		{"決定の割合", 4, func(r result) float64 { return r.decided }},
		{"成人の割合", 4, func(r result) float64 { return r.adults }},
		{"視野0の成人の割合", 4, func(r result) float64 { return r.sightLow }},
		{"視野が最大の成人の割合", 4, func(r result) float64 { return r.sightHigh }},
		{"交配（成立）", 2, func(r result) float64 { return r.matings }},
		{"繁殖能力0の成人の割合", 4, func(r result) float64 { return r.fertLow }},
		{"繁殖能力が最大の成人の割合", 4, func(r result) float64 { return r.fertHigh }},
		{"繁殖能力の最高段と最低段の出生率の差", 4, func(r result) float64 { return r.fertSlope }},
		{"予算が最小の段の成人の割合", 4, func(r result) float64 { return r.sizeLow }},
		{"予算が最大の段の成人の割合", 4, func(r result) float64 { return r.sizeHigh }},
		{"成人の平均予算", 4, func(r result) float64 { return r.meanBudget }},
		{"予算のドリフト（後半 − 前半）", 4, func(r result) float64 { return r.budgetDrift }},
		{"子の予算と親の予算の相関", 4, func(r result) float64 { return r.budgetCorr }},
		{"休んでいる母に配られた食料", 2, func(r result) float64 { return r.provisioned }},
		{"成人のうち女の割合", 4, func(r result) float64 { return r.female }},
		{"交配要求で成り立った出生", 2, func(r result) float64 { return r.requestBirths }},
	}
	// A seed with no value for a metric (NaN: a collapsed seed has no second
	// half to read, a condition without the slot has no level) is left out
	// of that column, and out of the paired difference when either side
	// has none.
	known := func(xs []float64) []float64 {
		var ks []float64
		for _, x := range xs {
			if !math.IsNaN(x) {
				ks = append(ks, x)
			}
		}
		return ks
	}
	for _, v := range o.variants[1:] {
		for _, mt := range metrics {
			b := collect(rs, mt.f)
			x := collect(results[v], mt.f)
			var d []float64
			for i := range b {
				if !math.IsNaN(b[i]) && !math.IsNaN(x[i]) {
					d = append(d, x[i]-b[i])
				}
			}
			p("| %s（%s） | %s | %s | %s |\n", mt.name, v, fmtMeanSE(known(b), mt.prec), fmtMeanSE(known(x), mt.prec), fmtMeanSE(d, mt.prec))
		}
	}

	// After §9 so that the brief report keeps it (brief prints from §9 on):
	// for every condition that has the slot, since the base often does not.
	for _, v := range o.variants {
		vCfg, _ := variant.Config(v, o.seed0)
		if !vCfg.AllotVenture {
			continue
		}
		vs := results[v]
		p("### 補助の表: 開拓の段ごとの未踏タイル率（%s）\n\n", v)
		p("tick 5000 より後の、別のタイルへの歩みを歩いた身体の開拓の段ごとに数えたもの。未踏タイル率はシードごとの、一度も立ったことのないタイルへの歩み ÷ 歩み。\n\n")
		p("| 段 | 歩み（1シードあたり） | 未踏タイル率 |\n| --- | --- | --- |\n")
		for k := 0; k <= vCfg.VentureMost; k++ {
			p("| %d | %s | %s |\n", k,
				fmtMeanSE(collect(vs, func(r result) float64 { return float64(r.boldSteps[k]) }), 2),
				fmtMeanSE(collectWhere(vs, func(r result) bool { return k < len(r.boldSteps) && r.boldSteps[k] > 0 }, func(r result) float64 { return boldRate(r, k) }), 4))
		}
		p("\n")
	}
}

// writeAllot prints the selection gradient on the share of the budget for
// speed: lifetime births per body by share, the paired difference between
// the bodies leaning to speed and to the most energy, and the mean share of
// the living over the run.
func writeAllot(p func(string, ...any), o options, rs []result, every, rows int) {
	base := o.variants[0]
	p("### 補助の表: 配分と生涯出生数（%s）\n\n", base)
	p("tick %d より後に生まれ、終わりまでに死んだ身体。生涯出生数は子の数（成人前に死んだ身体は 0 として入る）。\n\n", o.ticks/4)
	set := map[float64]bool{}
	for _, r := range rs {
		for _, x := range r.shares {
			set[x] = true
		}
	}
	var shares []float64
	for x := range set {
		shares = append(shares, x)
	}
	sort.Float64s(shares)
	p("| 速さへの配分 | 生涯出生数 | 身体の数（1シードあたり） |\n| --- | --- | --- |\n")
	for _, x := range shares {
		ok := func(r result) bool { return r.lives[x] > 0 }
		kids := collectWhere(rs, ok, func(r result) float64 { return r.lifeKids[x] / r.lives[x] })
		n := collect(rs, func(r result) float64 { return r.lives[x] })
		p("| %.3f | %s | %s |\n", x, fmtMeanSE(kids, 3), fmtMeanSE(n, 1))
	}
	side := func(r result, hi bool) float64 {
		k, n := 0.0, 0.0
		for _, x := range r.shares {
			if (hi && x > 0.5+1e-9) || (!hi && x < 0.5-1e-9) {
				k += r.lifeKids[x]
				n += r.lives[x]
			}
		}
		return k / n
	}
	d := collect(rs, func(r result) float64 { return side(r, true) - side(r, false) })
	p("\n速さの側（配分 > 0.5）− 体力の上限の側（< 0.5）の生涯出生数（同じシードの対の差）: %s\n\n", fmtMeanSE(d, 3))
	p("| tick | 配分の平均 | 配分の標準偏差 | 最遅の速さ | 中央値の速さ | 最速の速さ | 最速 ÷ 中央値 |\n| --- | --- | --- | --- | --- | --- | --- |\n")
	ok := func(i int) func(result) bool {
		return func(r result) bool { return i < len(r.shareLive) && !math.IsNaN(r.shareLive[i].mean) }
	}
	for i := 0; i < rows; i++ {
		get := func(f func(shareStats) float64) []float64 {
			return collectWhere(rs, ok(i), func(r result) float64 { return f(r.shareLive[i]) })
		}
		p("| %d | %s | %s | %s | %s | %s | %s |\n", i*every,
			fmtMeanSE(get(func(s shareStats) float64 { return s.mean }), 4),
			fmtMeanSE(get(func(s shareStats) float64 { return s.sd }), 4),
			fmtMeanSE(get(func(s shareStats) float64 { return s.slowest }), 4),
			fmtMeanSE(get(func(s shareStats) float64 { return s.median }), 4),
			fmtMeanSE(get(func(s shareStats) float64 { return s.fastest }), 4),
			fmtMeanSE(get(func(s shareStats) float64 { return s.fastest / s.median }), 3))
	}
	// Whether the share has settled: the mean over the last quarter less
	// the mean over the quarter before, seed by seed.
	q := rows / 4
	if q > 0 {
		d := collect(rs, func(r result) float64 {
			avg := func(a, b int) float64 {
				x, n := 0.0, 0.0
				for i := a; i < b && i < len(r.shareLive); i++ {
					if !math.IsNaN(r.shareLive[i].mean) {
						x += r.shareLive[i].mean
						n++
					}
				}
				return x / n
			}
			return avg(rows-q, rows) - avg(rows-2*q, rows-q)
		})
		p("\n配分の平均の、最後の 4 分の 1 − その前の 4 分の 1（同じシードの対の差）: %s\n", fmtMeanSE(d, 4))
	}
	last := collectWhere(rs, ok(rows-1), func(r result) float64 { return r.shareLive[rows-1].median })
	if m, _ := meanSE(last); m > 0 {
		p("\n画面横断時間（地図の幅 %d タイル、1 フレーム 1 tick、60 フレーム/秒）: 終わりの中央値の身体 %.1f 秒\n", o.width, float64(o.width)/m/60)
	}
	p("\n")
}

// shareStats describes the shares of the living at one tick: their mean and
// standard deviation, and the slowest, median and fastest speed.
type shareStats struct {
	mean, sd                 float64
	slowest, median, fastest float64
}

func shareStatsOf(bs []engine.Body, speed float64) shareStats {
	if len(bs) == 0 {
		return shareStats{math.NaN(), math.NaN(), math.NaN(), math.NaN(), math.NaN()}
	}
	var st shareStats
	speeds := make([]float64, len(bs))
	for i, b := range bs {
		st.mean += b.Share
		speeds[i] = b.Build.Speed
		if speeds[i] == 0 {
			speeds[i] = speed
		}
	}
	st.mean /= float64(len(bs))
	for _, b := range bs {
		st.sd += (b.Share - st.mean) * (b.Share - st.mean)
	}
	st.sd = math.Sqrt(st.sd / float64(len(bs)))
	sort.Float64s(speeds)
	st.slowest, st.median, st.fastest = speeds[0], speeds[len(speeds)/2], speeds[len(speeds)-1]
	return st
}

// edgeTiles marks the land tiles with the map's edge or water among the
// eight around them.
func edgeTiles(m engine.Map) []bool {
	e := make([]bool, m.Width*m.Height)
	for y := 0; y < m.Height; y++ {
		for x := 0; x < m.Width; x++ {
			if m.TerrainAt(x, y) != engine.TerrainLand {
				continue
			}
			for dy := -1; dy <= 1; dy++ {
				for dx := -1; dx <= 1; dx++ {
					if !m.InBounds(x+dx, y+dy) || m.TerrainAt(x+dx, y+dy) != engine.TerrainLand {
						e[y*m.Width+x] = true
					}
				}
			}
		}
	}
	return e
}

// edgeShare is the share of the land that edgeTiles marks.
func edgeShare(m engine.Map) float64 {
	e := edgeTiles(m)
	n, land := 0.0, 0.0
	for i, t := range m.Terrain {
		if t == engine.TerrainLand {
			land++
			if e[i] {
				n++
			}
		}
	}
	return n / land
}

// memRow is one learned row as the living held it at the end of a run.
type memRow struct {
	name     string
	key      string  // the name Provenance and rowCount know it by
	holders  float64 // share of the living with any evidence for it
	estimate float64 // mean estimate among them
	spread   float64 // standard deviation of the estimate among them
	learned  float64 // observations the bodies added to it themselves, per run
	passed   float64 // times a body gave another its evidence for it, per run
	evidence float64 // mean evidence among them (tiles or asks)
	truth    float64 // what the world had (NaN where it is not one number)
}

// memRows reads the rows the living had learned: each region's food per
// tile, the food of their own path, the chance a mate makes a child.
func memRows(w *engine.World, m engine.Map) []memRow {
	bs := w.Bodies()
	n := float64(len(bs))
	if n == 0 {
		return nil
	}
	land := make([]float64, len(m.RegionFood))
	for i, t := range m.Terrain {
		if t == engine.TerrainLand {
			land[m.Region[i]]++
		}
	}
	food := make([]float64, len(m.RegionFood))
	for _, f := range w.Foods() {
		food[m.RegionAt(f.X, f.Y)]++
	}
	var rows []memRow
	for r := range m.RegionFood {
		row := memRow{name: fmt.Sprintf("地域 %d の食料 / タイル", r), key: fmt.Sprintf("region %d", r), truth: food[r] / land[r]}
		for _, b := range bs {
			if r >= len(b.Memory.Regions) || b.Memory.Regions[r].N == 0 {
				continue
			}
			t := w.Fresh(b.Memory.Regions[r])
			row.holders++
			row.evidence += t.N
			e := (t.K + w.Config().RegionWeight*w.Belief(b).Land) / (t.N + w.Config().RegionWeight)
			row.estimate += e
			row.spread += e * e
		}
		rows = append(rows, row)
	}
	path := memRow{name: "自分が通ったタイルの食料 / タイル（証拠はタイルに直した数）", key: "path", truth: math.NaN()}
	mate := memRow{name: "交配の申し出が成立する割合", key: "mate", truth: math.NaN()}
	for _, b := range bs {
		bl := w.Belief(b)
		if pt := w.PathTally(b); pt.N > 0 {
			path.holders++
			n := pt.N
			if w.Config().StableRows && bl.Here > 0 {
				n /= bl.Here // counted as food expected, a fraction of a tile each
			}
			path.evidence += n
			path.estimate += bl.Path
			path.spread += bl.Path * bl.Path
		}
		if b.Memory.Asks > 0 {
			mate.holders++
			mate.evidence += b.Memory.Asks
			mate.estimate += bl.Child
			mate.spread += bl.Child * bl.Child
		}
	}
	rows = append(rows, path, mate)
	for i := range rows {
		if h := rows[i].holders; h > 0 {
			rows[i].estimate /= h
			rows[i].evidence /= h
			rows[i].spread = math.Sqrt(math.Max(rows[i].spread/h-rows[i].estimate*rows[i].estimate, 0))
		}
		rows[i].holders /= n
	}
	return rows
}

// rowCount returns the counts of the learned row named key (memRow.key).
func rowCount(st engine.Stats, key string) engine.RowCount {
	switch key {
	case "path":
		return st.PathRow
	case "mate":
		return st.MateRow
	}
	var r int
	if n, _ := fmt.Sscanf(key, "region %d", &r); n == 1 && r < len(st.RegionRows) {
		return st.RegionRows[r]
	}
	return engine.RowCount{}
}

// ageBands are the age bands, in ticks, the learning delay is read in.
var ageBands = [...][2]int64{{0, 250}, {250, 500}, {500, 1000}, {1000, 2000}, {2000, 1 << 40}}

// ageStat sums, over the bodies of one age band read at checkpoints, how
// far their estimate of their own region was from the region's food per
// tile then (relative error), and the tiles behind it.
type ageStat struct{ err, tiles, n float64 }

func readAges(w *engine.World, m engine.Map, st *[len(ageBands)]ageStat) {
	land := make([]float64, len(m.RegionFood))
	for i, t := range m.Terrain {
		if t == engine.TerrainLand {
			land[m.Region[i]]++
		}
	}
	food := make([]float64, len(m.RegionFood))
	for _, f := range w.Foods() {
		food[m.RegionAt(f.X, f.Y)]++
	}
	now := w.Tick()
	for _, b := range w.Bodies() {
		r := m.RegionAt(int(b.X), int(b.Y))
		truth := food[r] / land[r]
		if truth <= 0 {
			continue
		}
		bl := w.Belief(b)
		for i, band := range ageBands {
			if age := now - b.Born; age >= band[0] && age < band[1] {
				st[i].err += math.Abs(bl.Here-truth) / truth
				st[i].tiles += bl.HereN
				st[i].n++
			}
		}
	}
}

// writeMemory prints section 6 for a world that learns: the rows the living
// held at the end, and how far off the region estimates were by age.
func writeMemory(p func(string, ...any), rs []result) {
	if len(rs) == 0 || len(rs[0].rows) == 0 {
		p("学んだ行が無い。\n\n")
		return
	}
	p("世の中の記憶。合成されていない記憶（学んだ行）だけで、合成と中間項はまだ無い。回数は1シードあたりの実験全体の合計、推定は終わりに生きている身体の平均と、身体どうしのばらつき（標準偏差）。\n\n")
	writeTop(p, rs, "新規で学習された回数の上位10", "学習された回数", "持っている割合",
		func(m memRow) float64 { return m.learned },
		func(r result, m memRow) (float64, bool) { return m.holders, true })
	writeTop(p, rs, "伝達された回数の上位10", "伝達された回数", "受け取った証拠を持つ割合",
		func(m memRow) float64 { return m.passed },
		func(r result, m memRow) (float64, bool) {
			ps := r.prov[m.key]
			if ps == nil || ps.n == 0 {
				return 0, false
			}
			return ps.hearers / ps.n, true
		})
	p("#### 補助の表: 推定と世界の値\n\n")
	p("| 行 | 持っている割合 | 推定の平均 | 証拠の平均 | 世界の値（終わり） |\n| --- | --- | --- | --- | --- |\n")
	for i := range rs[0].rows {
		get := func(f func(memRow) float64) []float64 {
			return collectWhere(rs, func(r result) bool { return i < len(r.rows) }, func(r result) float64 { return f(r.rows[i]) })
		}
		truth := "—"
		if t := get(func(m memRow) float64 { return m.truth }); len(t) > 0 && !math.IsNaN(t[0]) {
			truth = fmtMeanSE(t, 4)
		}
		p("| %s | %s | %s | %s | %s |\n", rs[0].rows[i].name,
			fmtMeanSE(get(func(m memRow) float64 { return m.holders }), 3),
			fmtMeanSE(get(func(m memRow) float64 { return m.estimate }), 4),
			fmtMeanSE(get(func(m memRow) float64 { return m.evidence }), 1),
			truth)
	}
	p("\n自力で学ぶ遅れ: 年齢ごとの、今いる地域の食料 / タイルの推定の相対誤差（tick 5000 より後のチェックポイント）\n\n")
	p("| 年齢 | 相対誤差 | 今いる地域で入ったタイル |\n| --- | --- | --- |\n")
	for i, band := range ageBands {
		ok := func(r result) bool { return r.learnAge[i].n > 0 }
		errs := collectWhere(rs, ok, func(r result) float64 { return r.learnAge[i].err / r.learnAge[i].n })
		tiles := collectWhere(rs, ok, func(r result) float64 { return r.learnAge[i].tiles / r.learnAge[i].n })
		hi := fmt.Sprint(band[1])
		if band[1] > 1<<30 {
			hi = ""
		}
		p("| %d〜%s | %s | %s |\n", band[0], hi, fmtMeanSE(errs, 3), fmtMeanSE(tiles, 1))
	}
	p("\n")
}

// writeTop prints the learned rows ranked by count, the mean over seeds,
// highest first, leaving out those never counted: with the share of the
// living that share gives and their estimate.
func writeTop(p func(string, ...any), rs []result, title, countName, shareName string,
	count func(memRow) float64, share func(result, memRow) (float64, bool)) {
	get := func(i int, f func(result, memRow) (float64, bool)) []float64 {
		var xs []float64
		for _, r := range rs {
			if i < len(r.rows) {
				if x, ok := f(r, r.rows[i]); ok {
					xs = append(xs, x)
				}
			}
		}
		return xs
	}
	counts := func(i int) []float64 {
		return get(i, func(_ result, m memRow) (float64, bool) { return count(m), true })
	}
	var order []int
	for i := range rs[0].rows {
		if m, _ := meanSE(counts(i)); m > 0 {
			order = append(order, i)
		}
	}
	sort.SliceStable(order, func(a, b int) bool {
		ma, _ := meanSE(counts(order[a]))
		mb, _ := meanSE(counts(order[b]))
		return ma > mb
	})
	if len(order) > 10 {
		order = order[:10]
	}
	p("#### %s\n\n", title)
	if len(order) == 0 {
		p("無い。\n\n")
		return
	}
	p("| 順位 | 記憶 | %s | %s | 推定の平均 | 推定のばらつき（身体どうし） |\n| --- | --- | --- | --- | --- | --- |\n", countName, shareName)
	for k, i := range order {
		sh := "—"
		if xs := get(i, share); len(xs) > 0 {
			sh = fmtMeanSE(xs, 3)
		}
		p("| %d | %s | %s | %s | %s | %s |\n", k+1, rs[0].rows[i].name, fmtMeanSE(counts(i), 1), sh,
			fmtMeanSE(get(i, func(_ result, m memRow) (float64, bool) { return m.estimate, m.holders > 0 }), 4),
			fmtMeanSE(get(i, func(_ result, m memRow) (float64, bool) { return m.spread, m.holders > 0 }), 4))
	}
	p("\n")
}

// provSum sums one row's provenance over checkpoints.
type provSum struct {
	held, heard, orphan, hearers, n float64
	ages                            map[float64]float64
}

func readProvenance(w *engine.World, r *result) {
	if r.prov == nil {
		r.prov = map[string]*provSum{}
	}
	for _, rp := range w.Provenance() {
		ps := r.prov[rp.Name]
		if ps == nil {
			ps = &provSum{ages: map[float64]float64{}}
			r.prov[rp.Name] = ps
			r.provNames = append(r.provNames, rp.Name)
		}
		ps.held += rp.Held
		ps.heard += rp.Heard
		ps.orphan += rp.Orphan
		ps.hearers += rp.Hearers
		ps.n++
		for _, a := range rp.Ages {
			ps.ages[a[0]] += a[1]
		}
	}
}

// weightedQuantile is the q-quantile of values weighted by amounts.
func weightedQuantile(m map[float64]float64, q float64) float64 {
	if len(m) == 0 {
		return math.NaN()
	}
	keys := make([]float64, 0, len(m))
	total := 0.0
	for k, v := range m {
		keys = append(keys, k)
		total += v
	}
	sort.Float64s(keys)
	acc := 0.0
	for _, k := range keys {
		acc += m[k]
		if acc >= q*total {
			return k
		}
	}
	return keys[len(keys)-1]
}

// writeProvenance prints where the evidence the living held came from: the
// provenance instrument of stage 2-0.
func writeProvenance(p func(string, ...any), rs []result) {
	if len(rs) == 0 || len(rs[0].provNames) == 0 {
		return
	}
	var lives []float64
	for _, r := range rs {
		lives = append(lives, r.deathAges...)
	}
	sort.Float64s(lives)
	median := math.NaN()
	if len(lives) > 0 {
		median = lives[len(lives)/2]
	}
	p("### 補助の表: 証拠の寿命（計器。tick 5000 より後のチェックポイントの平均）\n\n")
	p("死んだ身体の死んだ年齢の中央値（tick 5000 より後、全シード）: %.0f tick\n\n", median)
	p("| 行 | 又聞きの割合 | 孤児の証拠の割合 | 孤児の証拠の年齢（中央値・最大） | 又聞きの保有者の割合 |\n| --- | --- | --- | --- | --- |\n")
	for _, name := range rs[0].provNames {
		get := func(f func(*provSum) float64) []float64 {
			return collectWhere(rs, func(r result) bool { return r.prov[name] != nil }, func(r result) float64 { return f(r.prov[name]) })
		}
		ratio := func(num func(*provSum) float64) []float64 {
			return get(func(ps *provSum) float64 {
				if ps.held == 0 {
					return 0
				}
				return num(ps) / ps.held
			})
		}
		ages := map[float64]float64{}
		for _, r := range rs {
			if ps := r.prov[name]; ps != nil {
				for k, v := range ps.ages {
					ages[k] += v
				}
			}
		}
		age := "—"
		if len(ages) > 0 {
			hi := 0.0
			for k := range ages {
				hi = math.Max(hi, k)
			}
			age = fmt.Sprintf("%.0f・%.0f", weightedQuantile(ages, 0.5), hi)
		}
		p("| %s | %s | %s | %s | %s |\n", name,
			fmtMeanSE(ratio(func(ps *provSum) float64 { return ps.heard }), 4),
			fmtMeanSE(ratio(func(ps *provSum) float64 { return ps.orphan }), 4),
			age,
			fmtMeanSE(get(func(ps *provSum) float64 { return ps.hearers / ps.n }), 4))
	}
	p("\n")
}
