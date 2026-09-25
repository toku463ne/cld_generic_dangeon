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
	// Seeds and ticks are the provisional values stage 1-2 settled on
	// (PARAMETERS.md); stage 1-5 fixes them. The map size has no default:
	// it is part of choosing the map.
	flag.IntVar(&o.seeds, "seeds", 12, "number of seeds (provisional default, PARAMETERS.md)")
	flag.Int64Var(&o.seed0, "seed0", 1, "first seed")
	flag.IntVar(&o.ticks, "ticks", 40000, "ticks per run (provisional default, PARAMETERS.md)")
	flag.StringVar(&o.mapName, "map", "", "map: "+strings.Join(worldmap.Names, " | ")+" (required)")
	flag.IntVar(&o.width, "w", 0, "map width in tiles (required)")
	flag.IntVar(&o.height, "h", 0, "map height in tiles (required)")
	// The collapse floor is decided in stage 1-3 (PLAN.md). Until then it is
	// zero: a seed has collapsed when nobody is left.
	flag.IntVar(&o.floor, "floor", 0, "collapse floor: a seed whose population falls to this or below stops")
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
	adultRate float64                     // matured over matured and died young; NaN with neither
	regions   []float64                   // regions with a body in them, at tick 0 and each checkpoint
	valley    float64                     // fewest bodies at any tick after tick 0
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
	every := max(ticks/checkpoints, 1)
	half := ticks / 2
	sampled := 0.0
	for t := 1; t <= ticks; t++ {
		w.Step()
		if l := w.FoodLedger(); !l.Balanced() {
			return result{}, fmt.Errorf("tick %d: food ledger does not close: %+v", t, l)
		}
		r.series = append(r.series, int32(len(w.Bodies())))
		r.valley = math.Min(r.valley, float64(len(w.Bodies())))
		if t%every == 0 {
			record()
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
	p("| 種別 | 上位10 |\n| --- | --- |\n")
	p("| 合成されていない高頻度記憶 | |\n| 合成された高頻度記憶 | |\n| ノード平均が高得点の中間項 | |\n\n")

	p("### 7. スキル別伝承数\n\n")
	p("| スキル | 伝承された回数 | 保有率 |\n| --- | --- | --- |\n\n")

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
