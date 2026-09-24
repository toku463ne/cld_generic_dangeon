// Command count runs the counts PLAN.md asks for before a rule is built. A
// count reads a world and changes nothing in it.
//
//	reach  (stage 1-1) how far the nearest food is from each land tile, in a
//	       world with food and no bodies, against how far a full body can
//	       walk before it starves.
//
//	underfoot (stage 1-2) in the stage 1-1 world, how often a body comes to
//	       stand on a tile with food. A body that always ate then would eat
//	       that often; it is set against the rate that keeps energy level.
//
//	split  (stage 1-2) in the stage 1-1 world, the share of decisions in
//	       which the truth table's valuation (engine.World.Value) puts the
//	       best option strictly above the second, per window and energy band.
//
//	meet   (stage 1-2) in the worlds of the random and base variants, the
//	       share of tiles a body enters that hold food, against what the
//	       truth table's third row reads: the food of the region over its land.
package main

import (
	"container/heap"
	"flag"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/toku463ne/cld_generic_dangeon/engine"
	"github.com/toku463ne/cld_generic_dangeon/variant"
	"github.com/toku463ne/cld_generic_dangeon/worldmap"
)

func main() {
	what := flag.String("what", "reach", "count to run: reach | underfoot | split | meet")
	mapName := flag.String("map", "", "map (required)")
	width := flag.Int("w", 0, "map width in tiles (required)")
	height := flag.Int("h", 0, "map height in tiles (required)")
	seeds := flag.Int("seeds", 0, "number of seeds (required)")
	seed0 := flag.Int64("seed0", 1, "first seed")
	ticks := flag.Int("ticks", 0, "ticks per run (underfoot, split and meet, required there)")
	every := flag.Int("every", 50, "split: read the world every this many ticks")
	flag.Parse()

	if *what != "reach" && *what != "underfoot" && *what != "split" && *what != "meet" {
		fail(fmt.Errorf("unknown count %q", *what))
	}
	if *seeds <= 0 || *width <= 0 || *height <= 0 || *mapName == "" {
		fail(fmt.Errorf("-map, -w, -h and -seeds are required"))
	}
	m, err := worldmap.Build(*mapName, *width, *height)
	if err != nil {
		fail(err)
	}
	if *what == "underfoot" || *what == "split" || *what == "meet" {
		if *ticks <= 0 {
			fail(fmt.Errorf("-ticks is required for %s", *what))
		}
		if *what == "meet" {
			meet(m, *mapName, *seeds, *seed0, *ticks)
			return
		}
		if *what == "split" {
			split(m, *mapName, *seeds, *seed0, *ticks, *every)
			return
		}
		underfoot(m, *mapName, *seeds, *seed0, *ticks)
		return
	}
	reach(m, *mapName, *seeds, *seed0)
}

// stage11 is the config of the stage 1-1 world the 1-2 counts read: the
// random control, before valuation chose anything.
func stage11() engine.Config {
	cfg, err := variant.Config(variant.Random, 1)
	if err != nil {
		fail(err)
	}
	return cfg
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "count:", err)
	os.Exit(2)
}

func reach(m engine.Map, name string, seeds int, seed0 int64) {
	cfg := engine.DefaultConfig()
	walk := cfg.EnergyMax / cfg.EnergyBurn * cfg.Speed
	var all []float64
	unreachable, land := 0, 0
	for s := 0; s < seeds; s++ {
		cfg.Seed = seed0 + int64(s)
		cfg.Bodies = 0
		w, err := engine.NewWorld(cfg, m)
		if err != nil {
			fail(err)
		}
		d := nearestFood(m, w.Foods())
		for i, t := range m.Terrain {
			if t != engine.TerrainLand {
				continue
			}
			land++
			if math.IsInf(d[i], 1) {
				unreachable++
				continue
			}
			all = append(all, d[i])
		}
	}
	sort.Float64s(all)
	q := func(p float64) float64 {
		if len(all) == 0 {
			return math.NaN()
		}
		return all[min(int(p*float64(len(all))), len(all)-1)]
	}
	fmt.Printf("| 地図 | 最寄りの食料までの距離 p50 / p90 / p99 / 最大（タイル） | 食料に届かない陸 | 満腹から餓死までに歩ける距離 | 到達比（歩ける距離 ÷ p99） |\n")
	fmt.Printf("| --- | --- | --- | --- | --- |\n")
	fmt.Printf("| %s (%dx%d, %d シード) | %.2f / %.2f / %.2f / %.2f | %.1f%% | %.1f | %.1f |\n",
		name, m.Width, m.Height, seeds, q(0.5), q(0.9), q(0.99), q(1), 100*float64(unreachable)/float64(land), walk, walk/q(0.99))
}

// nearestFood returns, for every tile, the walking distance over land to the
// nearest food (one tile straight, the square root of two diagonally), or
// +Inf where no food can be reached.
func nearestFood(m engine.Map, foods []engine.Food) []float64 {
	d := make([]float64, m.Width*m.Height)
	for i := range d {
		d[i] = math.Inf(1)
	}
	pq := &queue{}
	for _, f := range foods {
		i := f.Y*m.Width + f.X
		d[i] = 0
		heap.Push(pq, item{i, 0})
	}
	for pq.Len() > 0 {
		it := heap.Pop(pq).(item)
		if it.d > d[it.i] {
			continue
		}
		x, y := it.i%m.Width, it.i/m.Width
		for dy := -1; dy <= 1; dy++ {
			for dx := -1; dx <= 1; dx++ {
				if dx == 0 && dy == 0 {
					continue
				}
				nx, ny := x+dx, y+dy
				if !m.InBounds(nx, ny) || m.TerrainAt(nx, ny) != engine.TerrainLand {
					continue
				}
				step := 1.0
				if dx != 0 && dy != 0 {
					step = math.Sqrt2
				}
				j := ny*m.Width + nx
				if nd := it.d + step; nd < d[j] {
					d[j] = nd
					heap.Push(pq, item{j, nd})
				}
			}
		}
	}
	return d
}

type item struct {
	i int
	d float64
}

type queue []item

func (q queue) Len() int           { return len(q) }
func (q queue) Less(a, b int) bool { return q[a].d < q[b].d }
func (q queue) Swap(a, b int)      { q[a], q[b] = q[b], q[a] }
func (q *queue) Push(x any)        { *q = append(*q, x.(item)) }
func (q *queue) Pop() any          { old := *q; x := old[len(old)-1]; *q = old[:len(old)-1]; return x }

// footing is what a count remembers about one body between ticks.
type footing struct {
	tile    int
	hadFood bool
}

// meetings counts, over one run of the stage 1-1 world, the body-ticks and
// the times a body came to stand on food: it is on a tile with food now, and
// last tick it was on another tile or the tile had none. Standing on the same
// unit for several ticks is one meeting, since a body that always ate would
// have eaten it at the first. The world is read between ticks, the state the
// next decisions start from, except for the food that comes back at the start
// of the next tick.
type meetings struct {
	bodyTicks, met float64
	eaten          float64 // meals the random decider actually took
}

func countMeetings(cfg engine.Config, m engine.Map, from, to int) (meetings, error) {
	w, err := engine.NewWorld(cfg, m)
	if err != nil {
		return meetings{}, err
	}
	var r meetings
	last := map[int64]footing{}
	food := make([]bool, m.Width*m.Height)
	eatenAtFrom := int64(0)
	for t := 0; t < to; t++ {
		if t == from {
			eatenAtFrom = w.FoodLedger().Eaten
		}
		if t >= from {
			clear(food)
			for _, f := range w.Foods() {
				food[f.Y*m.Width+f.X] = true
			}
			for _, b := range w.Bodies() {
				tile := int(b.Y)*m.Width + int(b.X)
				prev, seen := last[b.ID]
				if food[tile] && (!seen || prev.tile != tile || !prev.hadFood) {
					r.met++
				}
				last[b.ID] = footing{tile, food[tile]}
				r.bodyTicks++
			}
		}
		w.Step()
	}
	r.eaten = float64(w.FoodLedger().Eaten - eatenAtFrom)
	return r, nil
}

func underfoot(m engine.Map, name string, seeds int, seed0 int64, ticks int) {
	cfg := stage11()
	balance := cfg.EnergyBurn / cfg.FoodEnergy
	full := int(cfg.EnergyMax / cfg.EnergyBurn)
	fmt.Printf("| 地図 | 区間（tick） | 足元に食料が来た頻度 | 実際に食べた頻度（無作為） | 釣り合いの頻度 | 足元 ÷ 釣り合い |\n")
	fmt.Printf("| --- | --- | --- | --- | --- | --- |\n")
	// The first window is before anyone can have starved: every body is
	// alive and the food has only begun to fall. The second is the whole run.
	for _, span := range [][2]int{{0, min(full, ticks)}, {0, ticks}} {
		var met, eaten []float64
		for s := 0; s < seeds; s++ {
			cfg.Seed = seed0 + int64(s)
			r, err := countMeetings(cfg, m, span[0], span[1])
			if err != nil {
				fail(err)
			}
			met = append(met, r.met/r.bodyTicks)
			eaten = append(eaten, r.eaten/r.bodyTicks)
		}
		mm, ms := meanSE(met)
		em, es := meanSE(eaten)
		fmt.Printf("| %s (%dx%d, %d シード) | %d〜%d | %.3f ± %.3f%% | %.3f ± %.3f%% | %.3f%% | %.2f |\n",
			name, m.Width, m.Height, seeds, span[0], span[1], 100*mm, 100*ms, 100*em, 100*es, 100*balance, mm/balance)
	}
}

func meanSE(xs []float64) (float64, float64) {
	n := float64(len(xs))
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

// splitWindows are the windows the split is counted over, in ticks. They run
// from well inside one meal (300 ticks of energy) to twice the time a full
// body takes to starve.
var splitWindows = []int{50, 100, 200, 300, 500, 1000, 2000}

// bands is how many energy bands the split is broken into, each a fifth of a
// full body.
const bands = 5

// tally counts decisions, and those in which the options differ.
type tally struct{ n, split float64 }

func (t tally) share() string {
	if t.n == 0 {
		return "—"
	}
	return fmt.Sprintf("%.1f%%", 100*t.split/t.n)
}

// splitCount is one run's tallies: over every decision, and over the
// decisions with food underfoot, where the split asked is whether eating is
// strictly best. Each is per window, overall and per energy band.
type splitCount struct {
	all, underfoot       [][bands + 1]tally // [window][band]; the last band is all of them
	values               float64
	valueTime, tableTime time.Duration
}

func countSplit(cfg engine.Config, m engine.Map, ticks, every int) (splitCount, error) {
	w, err := engine.NewWorld(cfg, m)
	if err != nil {
		return splitCount{}, err
	}
	c := splitCount{
		all:       make([][bands + 1]tally, len(splitWindows)),
		underfoot: make([][bands + 1]tally, len(splitWindows)),
	}
	for t := 0; t < ticks && len(w.Bodies()) > 0; t++ {
		if t%every == 0 {
			start := time.Now()
			tab := w.TruthTable()
			surv := make([]engine.Survival, len(tab.Meet))
			for r, q := range tab.Meet {
				surv[r] = tab.NewSurvival(q, splitWindows)
			}
			c.tableTime += time.Since(start)
			for _, b := range w.Bodies() {
				start := time.Now()
				v := w.Value(tab, surv, b)
				c.valueTime += time.Since(start)
				c.values++
				band := min(int(b.Energy/cfg.EnergyMax*bands), bands-1)
				eat := -1
				for j, a := range v.Options {
					if a.Kind == engine.ActEat {
						eat = j
					}
				}
				for i := range splitWindows {
					best, second := math.Inf(1), math.Inf(1)
					for _, x := range v.Risk[i] {
						if x < best {
							best, second = x, best
						} else if x < second {
							second = x
						}
					}
					split := 0.0
					if best < second {
						split = 1
					}
					for _, k := range []int{band, bands} {
						c.all[i][k].n++
						c.all[i][k].split += split
					}
					if eat < 0 {
						continue
					}
					eatBest := 1.0
					for j, x := range v.Risk[i] {
						if j != eat && x <= v.Risk[i][eat] {
							eatBest = 0
						}
					}
					for _, k := range []int{band, bands} {
						c.underfoot[i][k].n++
						c.underfoot[i][k].split += eatBest
					}
				}
			}
		}
		w.Step()
	}
	return c, nil
}

func split(m engine.Map, name string, seeds int, seed0 int64, ticks, every int) {
	cfg := stage11()
	all := make([][bands + 1]tally, len(splitWindows))
	under := make([][bands + 1]tally, len(splitWindows))
	var values float64
	var valueTime, tableTime time.Duration
	for s := 0; s < seeds; s++ {
		cfg.Seed = seed0 + int64(s)
		c, err := countSplit(cfg, m, ticks, every)
		if err != nil {
			fail(err)
		}
		for i := range splitWindows {
			for k := range all[i] {
				all[i][k].n += c.all[i][k].n
				all[i][k].split += c.all[i][k].split
				under[i][k].n += c.underfoot[i][k].n
				under[i][k].split += c.underfoot[i][k].split
			}
		}
		values += c.values
		valueTime += c.valueTime
		tableTime += c.tableTime
	}
	header := func(title string) {
		fmt.Printf("\n%s（%s %dx%d、%d シード、%d tick ごと）\n\n", title, name, m.Width, m.Height, seeds, every)
		cols := []string{"窓（tick）", "全体"}
		for k := 0; k < bands; k++ {
			cols = append(cols, fmt.Sprintf("体力 %d〜%d", 100*k/bands, 100*(k+1)/bands))
		}
		fmt.Printf("| %s |\n|%s\n", strings.Join(cols, " | "), strings.Repeat(" --- |", len(cols)))
	}
	row := func(t [bands + 1]tally, w int) {
		cells := []string{fmt.Sprint(w), t[bands].share()}
		for k := 0; k < bands; k++ {
			cells = append(cells, t[k].share())
		}
		fmt.Printf("| %s |\n", strings.Join(cells, " | "))
	}
	header("最善の手と2番目の手の差がゼロでない決定の割合")
	for i, w := range splitWindows {
		row(all[i], w)
	}
	header("足元に食料がある決定のうち、食べるが単独で最善の割合")
	for i, w := range splitWindows {
		row(under[i], w)
	}
	counts := []string{}
	for k := 0; k < bands; k++ {
		counts = append(counts, fmt.Sprintf("%.0f", all[0][k].n))
	}
	fmt.Printf("\n決定の数: %.0f（体力の帯ごと %s）、足元に食料: %.0f\n", all[0][bands].n, strings.Join(counts, " / "), under[0][bands].n)
	fmt.Printf("値付け1回（%d 窓）: %v、表の作成（全地域・全窓）1回: %v\n",
		len(splitWindows), valueTime/time.Duration(max(values, 1)), tableTime/time.Duration(max(float64(ticks/every*seeds), 1)))
}

// entries is one run's tally of the tiles bodies entered: how many, how many
// held food when entered, and the sum over them of the food density the
// truth table's third row reads for the region entered.
type entries struct {
	entered, withFood, density float64
}

// countEntries runs one world to the given tick, or until nobody is left,
// and tallies every decision made on a tile other than the one the same
// body decided on last. The tile holds food exactly when eating is offered.
// It reads the world through the trace, which changes nothing.
func countEntries(cfg engine.Config, m engine.Map, ticks int) (entries, error) {
	w, err := engine.NewWorld(cfg, m)
	if err != nil {
		return entries{}, err
	}
	var e entries
	last := map[int64]int{}
	speed := math.Min(cfg.Speed, 1)
	w.SetTrace(func(b engine.Body, v engine.Valuation, _ engine.Action) {
		tx, ty := int(math.Floor(b.X)), int(math.Floor(b.Y))
		tile := ty*m.Width + tx
		prev, seen := last[b.ID]
		last[b.ID] = tile
		if !seen || prev == tile {
			return
		}
		e.entered++
		for _, a := range v.Options {
			if a.Kind == engine.ActEat {
				e.withFood++
			}
		}
		e.density += w.TruthTable().Meet[m.RegionAt(tx, ty)] / speed
	})
	for t := 0; t < ticks && len(w.Bodies()) > 0; t++ {
		w.Step()
	}
	return e, nil
}

func meet(m engine.Map, name string, seeds int, seed0 int64, ticks int) {
	fmt.Printf("| 地図 | 条件 | 入ったタイル（1シードあたり） | 食料があった割合 | 表の読み（地域の食料 ÷ 陸） | 実際 ÷ 表 |\n")
	fmt.Printf("| --- | --- | --- | --- | --- | --- |\n")
	for _, v := range []string{variant.Random, variant.Base} {
		var entered, had, read, ratio []float64
		for s := 0; s < seeds; s++ {
			cfg, err := variant.Config(v, seed0+int64(s))
			if err != nil {
				fail(err)
			}
			e, err := countEntries(cfg, m, ticks)
			if err != nil {
				fail(err)
			}
			entered = append(entered, e.entered)
			had = append(had, e.withFood/e.entered)
			read = append(read, e.density/e.entered)
			ratio = append(ratio, e.withFood/e.density)
		}
		em, es := meanSE(entered)
		hm, hs := meanSE(had)
		rm, rs := meanSE(read)
		qm, qs := meanSE(ratio)
		fmt.Printf("| %s (%dx%d, %d シード, %d tick) | %s | %.0f ± %.0f | %.2f ± %.2f%% | %.2f ± %.2f%% | %.2f ± %.2f |\n",
			name, m.Width, m.Height, seeds, ticks, v, em, es, 100*hm, 100*hs, 100*rm, 100*rs, qm, qs)
	}
}
