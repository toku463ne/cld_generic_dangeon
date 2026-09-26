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
	pop       []float64 // bodies at tick 0 and at each checkpoint
	series    []int32   // bodies at every tick from 0, up to the tick the seed stopped
	food      []float64 // food on the ground at the same ticks
	surviving []bool    // whether the seed was above the floor at the same ticks
	fellAt    int       // first tick at or below the floor, -1 if never
	starved   float64
	burned    float64
	actions   [engine.NumActionKinds]float64
	decided   float64 // decisions over actions (body-ticks)
	births    float64
	adultRate float64   // matured over matured and died young; NaN with neither
	regions   []float64 // regions with a body in them, at tick 0 and each checkpoint
	valley    float64   // fewest bodies at any tick after tick 0
	// displace is the mean straight-line distance, in tiles, a body gets
	// from where it was 1000 ticks before (over bodies alive at both ends,
	// every 1000 ticks after tick 5000); edge the mean share of the living
	// on land tiles next to the map's edge or water, sampled the same.
	displace, edge float64
	// back is the share of decisions after tick 5000 that took a move
	// straight back the way the body last moved.
	back float64
	// With Learn: where the evidence the living held came from, summed
	// over the checkpoints after tick 5000 (prov), and the ages at death
	// after tick 5000 (deathAges), for the median life to set it against.
	prov      map[string]*provSum
	provNames []string
	deathAges []float64
	// Evidence passed between bodies, by row (tell.go), per run.
	passedRegion, passedPath float64
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

	results := map[string][]result{}
	for _, v := range o.variants {
		for s := 0; s < o.seeds; s++ {
			cfg, _ := variant.Config(v, o.seed0+int64(s))
			start := time.Now()
			r, err := runOne(cfg, m, o.ticks, o.floor)
			if err != nil {
				return fmt.Errorf("%s seed %d: %w", v, cfg.Seed, err)
			}
			// Wall time goes to the log, not the report: it describes the
			// machine, not the world.
			fmt.Fprintf(log, "%s seed %d: %d ticks in %v\n", v, cfg.Seed, o.ticks, time.Since(start))
			results[v] = append(results[v], r)
		}
	}

	writeReport(out, o, m, command, results)
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
	record := func() {
		n := len(w.Bodies())
		r.pop = append(r.pop, float64(n))
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
	var dSum, dN, eSum, eN float64
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
					for _, p := range b.Parents {
						kids[p]++
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
			for len(r.pop) < ticks/every+1 {
				r.pop = append(r.pop, float64(len(w.Bodies())))
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
	r.back = backs / math.Max(traced, 1)
	if cfg.Learn {
		r.rows = memRows(w, m)
	}
	st := w.Stats()
	r.starved = float64(st.Deaths[engine.CauseStarved])
	r.burned = st.EnergyBurned
	total := 0.0
	for _, n := range st.Actions {
		total += float64(n)
	}
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
	r.passedRegion, r.passedPath = float64(st.PassedRegion), float64(st.PassedPath)
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
	p("### 3. 人口推移（%s・生存しているシードの平均）\n\n", base)
	p("| tick | 人口（全体） | 人間 | 獣 | 鳥 | 魚 | 生存シード数 |\n| --- | --- | --- | --- | --- | --- | --- |\n")
	for i := 0; i < rows; i++ {
		xs := collectWhere(rs, aliveAt(i), func(r result) float64 { return r.pop[i] })
		pop := fmtMeanSE(xs, 1)
		p("| %d | %s | %s | | | | %d / %d |\n", i*every, pop, pop, len(xs), len(rs))
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

	starved := collect(rs, func(r result) float64 { return r.starved })
	p("### 4. 出来事の回数（%s）\n\n", base)
	p("| 出来事 | 回数 |\n| --- | --- |\n")
	births := fmtMeanSE(collect(rs, func(r result) float64 { return r.births }), 2)
	p("| 交配 | %s |\n| 出生 | %s |\n", births, births)
	for _, e := range []string{"種族内の戦い", "種族間の戦い", "コインを拾った", "売買"} {
		p("| %s | 0 |\n", e)
	}
	p("| 体力消耗（合計） | %s |\n", fmtMeanSE(collect(rs, func(r result) float64 { return r.burned }), 1))
	p("| 死亡 | %s |\n\n", fmtMeanSE(starved, 2))

	p("### 5. 行動の選択割合（%s）\n\n", base)
	p("| 行動 | 割合 |\n| --- | --- |\n")
	for k, name := range []string{"待つ", "食べる", "移動", "交配"} {
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

	if baseCfg, _ := variant.Config(base, o.seed0); baseCfg.Allot {
		writeAllot(p, o, rs, every, rows)
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
		p("スキルはまだ無い。伝承されるのは学んだ行なので、行ごとに出す。伝承された回数は、身体が持っていた証拠を出会った身体に渡した回数（1シードあたり）、保有率は受け取った証拠を持つ生きている身体の割合（tick 5000 より後のチェックポイントの平均）。\n\n")
		p("| 行 | 伝承された回数 | 保有率 |\n| --- | --- | --- |\n")
		holders := func(match func(string) bool) []float64 {
			return collectWhere(rs, func(r result) bool { return len(r.provNames) > 0 }, func(r result) float64 {
				x, n := 0.0, 0.0
				for _, name := range r.provNames {
					if match(name) {
						ps := r.prov[name]
						x += ps.hearers / ps.n
						n++
					}
				}
				return x / math.Max(n, 1)
			})
		}
		p("| 地域の行（全地域） | %s | %s |\n", fmtMeanSE(collect(rs, func(r result) float64 { return r.passedRegion }), 1),
			fmtMeanSE(holders(func(n string) bool { return strings.HasPrefix(n, "region") }), 4))
		p("| 通ったタイルの行 | %s | %s |\n\n", fmtMeanSE(collect(rs, func(r result) float64 { return r.passedPath }), 1),
			fmtMeanSE(holders(func(n string) bool { return n == "path" }), 4))
	} else {
		p("| スキル | 伝承された回数 | 保有率 |\n| --- | --- | --- |\n\n")
	}

	p("### 8. 死因別死亡数（%s）\n\n", base)
	p("| 死因 | 回数 | 割合 |\n| --- | --- | --- |\n")
	if m, _ := meanSE(starved); m > 0 {
		p("| 餓死 | %s | 100%% |\n\n", fmtMeanSE(starved, 2))
	} else {
		p("| 餓死 | %s | — |\n\n", fmtMeanSE(starved, 2))
	}

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
	p("| 真後ろへの手（tick 5000 より後の決定のうち） | %s |\n\n", fmtMeanSE(collect(rs, func(r result) float64 { return r.back }), 4))

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
		{"餓死", 2, func(r result) float64 { return r.starved }},
		{"体力消耗（合計）", 2, func(r result) float64 { return r.burned }},
		{"出生", 2, func(r result) float64 { return r.births }},
		{"成人到達率", 4, func(r result) float64 { return r.adultRate }},
		{"1000 tick の変位（タイル）", 2, func(r result) float64 { return r.displace }},
		{"縁（地図の端か水の隣）にいる割合", 4, func(r result) float64 { return r.edge }},
		{"真後ろへの手（決定のうち）", 4, func(r result) float64 { return r.back }},
		{"決定の割合", 4, func(r result) float64 { return r.decided }},
	}
	for _, v := range o.variants[1:] {
		for _, mt := range metrics {
			b := collect(rs, mt.f)
			x := collect(results[v], mt.f)
			d := make([]float64, len(b))
			for i := range b {
				d[i] = x[i] - b[i]
			}
			p("| %s（%s） | %s | %s | %s |\n", mt.name, v, fmtMeanSE(b, mt.prec), fmtMeanSE(x, mt.prec), fmtMeanSE(d, mt.prec))
		}
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
	holders  float64 // share of the living with any evidence for it
	estimate float64 // mean estimate among them
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
		row := memRow{name: fmt.Sprintf("地域 %d の食料 / タイル", r), truth: food[r] / land[r]}
		for _, b := range bs {
			if r >= len(b.Memory.Regions) || b.Memory.Regions[r].N == 0 {
				continue
			}
			t := w.Fresh(b.Memory.Regions[r])
			row.holders++
			row.evidence += t.N
			row.estimate += (t.K + w.Config().RegionWeight*w.Belief(b).Land) / (t.N + w.Config().RegionWeight)
		}
		rows = append(rows, row)
	}
	path := memRow{name: "自分が通ったタイルの食料 / タイル（証拠はタイルに直した数）", truth: math.NaN()}
	mate := memRow{name: "交配の申し出が子になる割合", truth: math.NaN()}
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
		}
		if b.Memory.Asks > 0 {
			mate.holders++
			mate.evidence += b.Memory.Asks
			mate.estimate += bl.Child
		}
	}
	rows = append(rows, path, mate)
	for i := range rows {
		if rows[i].holders > 0 {
			rows[i].estimate /= rows[i].holders
			rows[i].evidence /= rows[i].holders
		}
		rows[i].holders /= n
	}
	return rows
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
	p("合成されていない記憶（学んだ行。終わりに生きている身体。合成と中間項はまだ無い）\n\n")
	p("| 行 | 持っている割合 | 推定の平均 | 証拠の平均 | 世界の値（終わり） |\n| --- | --- | --- | --- | --- |\n")
	if len(rs) == 0 || len(rs[0].rows) == 0 {
		p("\n")
		return
	}
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
