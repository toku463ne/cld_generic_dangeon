// Command experiment runs variants of the world over a range of seeds and
// prints the report in the form MEASURE.md fixes.
//
// Every variant is a rewrite of engine.Config, never a branch in the code, so
// that all of them are in the same binary and can be paired seed by seed.
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
	"github.com/toku463ne/cld_generic_dangeon/worldmap"
)

// variants maps a variant name to the rewrite of the default config it
// stands for.
var variants = map[string]func(*engine.Config){
	"base": func(*engine.Config) {},
}

// checkpoints is how many rows the population table has after tick 0.
const checkpoints = 20

type options struct {
	variants      []string
	seeds         int
	seed0         int64
	ticks         int
	mapName       string
	width, height int
}

func main() {
	var o options
	var names string
	flag.StringVar(&names, "variants", "base", "comma-separated variants; the first is the base")
	// Seeds, ticks and map size have no defaults yet: PLAN.md decides them
	// from measurements, and a made-up default would be quietly used.
	flag.IntVar(&o.seeds, "seeds", 0, "number of seeds (required)")
	flag.Int64Var(&o.seed0, "seed0", 1, "first seed")
	flag.IntVar(&o.ticks, "ticks", 0, "ticks per run (required)")
	flag.StringVar(&o.mapName, "map", "", "map: "+strings.Join(worldmap.Names, " | ")+" (required)")
	flag.IntVar(&o.width, "w", 0, "map width in tiles (required)")
	flag.IntVar(&o.height, "h", 0, "map height in tiles (required)")
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
type result struct {
	pop      []float64 // bodies at tick 0 and at each checkpoint
	food     []float64 // food on the ground at the same ticks
	starved  float64
	burned   float64
	actions  [engine.NumActionKinds]float64
	regionOf []float64 // share of the bodies in each region, averaged over the second half of the run
}

func run(out, log io.Writer, o options, command string) error {
	if o.seeds <= 0 || o.ticks <= 0 || o.width <= 0 || o.height <= 0 || o.mapName == "" {
		return fmt.Errorf("-seeds, -ticks, -map, -w and -h are required")
	}
	for _, v := range o.variants {
		if _, ok := variants[v]; !ok {
			return fmt.Errorf("unknown variant %q (known: %s)", v, strings.Join(knownVariants(), ", "))
		}
	}
	m, err := worldmap.Build(o.mapName, o.width, o.height)
	if err != nil {
		return err
	}

	results := map[string][]result{}
	for _, v := range o.variants {
		for s := 0; s < o.seeds; s++ {
			cfg := engine.DefaultConfig()
			cfg.Seed = o.seed0 + int64(s)
			variants[v](&cfg)
			start := time.Now()
			r, err := runOne(cfg, m, o.ticks)
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

func runOne(cfg engine.Config, m engine.Map, ticks int) (result, error) {
	w, err := engine.NewWorld(cfg, m)
	if err != nil {
		return result{}, err
	}
	r := result{regionOf: make([]float64, len(m.RegionFood))}
	r.pop = append(r.pop, float64(len(w.Bodies())))
	r.food = append(r.food, float64(w.FoodLedger().OnGround))
	every := max(ticks/checkpoints, 1)
	half := ticks / 2
	sampled := 0.0
	for t := 1; t <= ticks; t++ {
		w.Step()
		if l := w.FoodLedger(); !l.Balanced() {
			return result{}, fmt.Errorf("tick %d: food ledger does not close: %+v", t, l)
		}
		if t%every == 0 {
			r.pop = append(r.pop, float64(len(w.Bodies())))
			r.food = append(r.food, float64(w.FoodLedger().OnGround))
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
	return r, nil
}

func knownVariants() []string {
	names := make([]string, 0, len(variants))
	for n := range variants {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
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

func fmtMeanSE(xs []float64, prec int) string {
	m, se := meanSE(xs)
	return fmt.Sprintf("%.*f ± %.*f", prec, m, prec, se)
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
	p("| 詳細レポート | |\n\n")
	p("**再現方法**:\n\n```\n%s\n```\n\n", command)

	every := max(o.ticks/checkpoints, 1)
	p("### 3. 人口推移（%s）\n\n", base)
	p("| tick | 人口（全体） | 人間 | 獣 | 鳥 | 魚 |\n| --- | --- | --- | --- | --- | --- |\n")
	for i := range rs[0].pop {
		pop := fmtMeanSE(collect(rs, func(r result) float64 { return r.pop[i] }), 1)
		p("| %d | %s | %s | | | |\n", i*every, pop, pop)
	}
	p("\n")

	starved := collect(rs, func(r result) float64 { return r.starved })
	p("### 4. 出来事の回数（%s）\n\n", base)
	p("| 出来事 | 回数 |\n| --- | --- |\n")
	for _, e := range []string{"交配", "出生", "種族内の戦い", "種族間の戦い", "コインを拾った", "売買"} {
		p("| %s | 0 |\n", e)
	}
	p("| 体力消耗（合計） | %s |\n", fmtMeanSE(collect(rs, func(r result) float64 { return r.burned }), 1))
	p("| 死亡 | %s |\n\n", fmtMeanSE(starved, 2))

	p("### 5. 行動の選択割合（%s）\n\n", base)
	p("| 行動 | 割合 |\n| --- | --- |\n")
	for k, name := range []string{"待つ", "食べる", "移動"} {
		p("| %s | %s%% |\n", name, fmtMeanSE(collect(rs, func(r result) float64 { return 100 * r.actions[k] }), 2))
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
	p("| tick | 地面の食料 | 上限に対する割合 |\n| --- | --- | --- |\n")
	baseCfg := engine.DefaultConfig()
	variants[base](&baseCfg)
	foodCap := float64(baseCfg.FoodCap)
	for i := range rs[0].food {
		xs := collect(rs, func(r result) float64 { return r.food[i] })
		mean, _ := meanSE(xs)
		p("| %d | %s | %.2f |\n", i*every, fmtMeanSE(xs, 1), mean/foodCap)
	}
	p("\n")

	area, food := regionShares(m)
	p("### 補助の表: 地域別人口比（%s・後半の平均）\n\n", base)
	p("| 地域 | 面積比 | 食料比 | 人口比 |\n| --- | --- | --- | --- |\n")
	for reg := range area {
		pop := fmtMeanSE(collect(rs, func(r result) float64 { return r.regionOf[reg] }), 3)
		p("| %d | %.3f | %.3f | %s |\n", reg, area[reg], food[reg], pop)
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
		f    func(result) float64
	}{
		{"人口（最終）", func(r result) float64 { return r.pop[len(r.pop)-1] }},
		{"餓死", func(r result) float64 { return r.starved }},
		{"体力消耗（合計）", func(r result) float64 { return r.burned }},
	}
	for _, v := range o.variants[1:] {
		for _, mt := range metrics {
			b := collect(rs, mt.f)
			x := collect(results[v], mt.f)
			d := make([]float64, len(b))
			for i := range b {
				d[i] = x[i] - b[i]
			}
			p("| %s（%s） | %s | %s | %s |\n", mt.name, v, fmtMeanSE(b, 2), fmtMeanSE(x, 2), fmtMeanSE(d, 2))
		}
	}
}
