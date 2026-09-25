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
//	sight  (stage 1-2p) in the stage 1-2 world, for each sight radius, how
//	       often a body has food in sight, how often food it had not seen
//	       comes into sight, and how far the nearest food in sight is.
//
//	forage (stage 1-2p) in the base world, what bodies do on food, per
//	       energy band; how often a hungry body has food in sight; how far
//	       a body gets from where it was a full body's life ago; and how
//	       often the least risk is shared, and the body's last move is
//	       among the options sharing it.
//
//	cycle  (stage 1-2q) in the base world, the rhythm of the food on the
//	       ground: its autocorrelation at a few lags, how many phases the
//	       bodies' energy modulo one meal takes, how deaths fall over the
//	       meal cycle, and the swing and population per window - for the
//	       world as it starts and for the same world with its starting
//	       energies spread out.
//
//	explore (stage 1-2q) each body's exploration over a window - untrodden
//	       tiles per tick and food units come into sight per tick - against
//	       how long it lives after the window, by rank correlation, in the
//	       blind, restless and base worlds.
//
//	shuttle (stage 1-2q) in the base world, bodies that walk one row or
//	       column back and forth: the share of deaths that end such a walk,
//	       against the share of the living on one and on an axis heading.
//
//	change (stage 1-2e) in the base world, how often a decision changes
//	       the body's action, and what observable change came before: food
//	       underfoot, food in sight, or the next step of the last action
//	       leaving the land or the region.
//
//	breed  (stage 1-3) in the base world, how often a body deciding has
//	       another body near, what paying the energy of a birth does to
//	       its risk of death, and how long bodies live.
//
//	mate   (stage 1-3) in the base world, the decisions that had a mate
//	       among the options: the energy of those that took it against
//	       those that did not, and mates carried out per birth.
//
//	turnover (stage 1-3) in the base world, what the dead died of and at
//	       what age, whether they had just paid for a birth, and whether
//	       deaths come steadily or in waves: generations passing, or a
//	       world that dies back and grows again.
//
//	invade (stage 1-4) half of the bodies born have one ability raised or
//	       lowered; they are set against the other half in the same world,
//	       by the share of children coming of age and children per adult.
//
//	meet   (stage 1-2) in the worlds of the random and base variants, the
//	       share of tiles a body enters that hold food, against what the
//	       truth table's third row reads: the food of the region over its land.
package main

import (
	"bytes"
	"container/heap"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"math/rand"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/toku463ne/cld_generic_dangeon/engine"
	"github.com/toku463ne/cld_generic_dangeon/variant"
	"github.com/toku463ne/cld_generic_dangeon/worldmap"
)

func main() {
	what := flag.String("what", "reach", "count to run: reach | underfoot | split | meet | sight | forage | cycle | explore | shuttle | change | breed | mate | turnover | invade")
	mapName := flag.String("map", "", "map (required)")
	width := flag.Int("w", 0, "map width in tiles (required)")
	height := flag.Int("h", 0, "map height in tiles (required)")
	seeds := flag.Int("seeds", 0, "number of seeds (required)")
	seed0 := flag.Int64("seed0", 1, "first seed")
	ticks := flag.Int("ticks", 0, "ticks per run (underfoot, split, meet, sight, forage, cycle and explore, required there)")
	every := flag.Int("every", 50, "split: read the world every this many ticks")
	flag.Parse()

	if *what != "reach" && *what != "underfoot" && *what != "split" && *what != "meet" && *what != "sight" && *what != "forage" && *what != "cycle" && *what != "explore" && *what != "shuttle" && *what != "change" && *what != "breed" && *what != "mate" && *what != "turnover" && *what != "invade" {
		fail(fmt.Errorf("unknown count %q", *what))
	}
	if *seeds <= 0 || *width <= 0 || *height <= 0 || *mapName == "" {
		fail(fmt.Errorf("-map, -w, -h and -seeds are required"))
	}
	m, err := worldmap.Build(*mapName, *width, *height)
	if err != nil {
		fail(err)
	}
	if *what == "underfoot" || *what == "split" || *what == "meet" || *what == "sight" || *what == "forage" || *what == "cycle" || *what == "explore" || *what == "shuttle" || *what == "change" || *what == "breed" || *what == "mate" || *what == "turnover" || *what == "invade" {
		if *ticks <= 0 {
			fail(fmt.Errorf("-ticks is required for %s", *what))
		}
		if *what == "invade" {
			invade(m, *mapName, *seeds, *seed0, *ticks)
			return
		}
		if *what == "turnover" {
			turnover(m, *mapName, *seeds, *seed0, *ticks)
			return
		}
		if *what == "mate" {
			mate(m, *mapName, *seeds, *seed0, *ticks)
			return
		}
		if *what == "breed" {
			breed(m, *mapName, *seeds, *seed0, *ticks)
			return
		}
		if *what == "change" {
			change(m, *mapName, *seeds, *seed0, *ticks)
			return
		}
		if *what == "shuttle" {
			shuttle(m, *mapName, *seeds, *seed0, *ticks)
			return
		}
		if *what == "explore" {
			explore(m, *mapName, *seeds, *seed0, *ticks)
			return
		}
		if *what == "cycle" {
			cycle(m, *mapName, *seeds, *seed0, *ticks)
			return
		}
		if *what == "forage" {
			forage(m, *mapName, *seeds, *seed0, *ticks)
			return
		}
		if *what == "sight" {
			sight(m, *mapName, *seeds, *seed0, *ticks)
			return
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

// sightRadii are the radii the sight count compares, in tiles around the
// tile a body stands on: radius r sees the (2r+1) x (2r+1) block of tiles
// centred on it. Radius 1 is the predecessor's "own cell and one ring".
var sightRadii = []int{0, 1, 2, 3, 5, 8}

// sightTally is one run's tally per radius over a span of ticks.
type sightTally struct {
	bodyTicks float64
	inSight   []float64 // body-ticks with food in sight
	newly     []float64 // units that came into sight, summed over bodies
	nearest   []float64 // sum over body-ticks with food in sight of the distance to the nearest
}

// countSight runs the base world (stage 1-2) and reads it between ticks,
// the state the next decisions start from. A unit comes into sight of a body
// when it is within the radius now and was not the tick before (it was
// farther, or not there). Food that comes back inside the radius counts too:
// it is food the body had not seen. Distance is from the body to the centre
// of the food's tile, in tiles, which is how far the body has to walk.
func countSight(cfg engine.Config, m engine.Map, from, to int) (sightTally, error) {
	w, err := engine.NewWorld(cfg, m)
	if err != nil {
		return sightTally{}, err
	}
	n := len(sightRadii)
	t := sightTally{inSight: make([]float64, n), newly: make([]float64, n), nearest: make([]float64, n)}
	far := sightRadii[n-1]
	// last[body][tile] is the block distance of a unit on that tile the
	// tick before, for units within the farthest radius.
	last := map[int64]map[int]int{}
	for tick := 0; tick < to && len(w.Bodies()) > 0; tick++ {
		foods := w.Foods()
		seen := map[int64]map[int]int{}
		for _, b := range w.Bodies() {
			bx, by := int(math.Floor(b.X)), int(math.Floor(b.Y))
			now := map[int]int{}
			near := make([]float64, n)
			for i := range near {
				near[i] = math.Inf(1)
			}
			for _, f := range foods {
				d := max(abs(f.X-bx), abs(f.Y-by))
				if d > far {
					continue
				}
				tile := f.Y*m.Width + f.X
				now[tile] = d
				dist := math.Hypot(float64(f.X)+0.5-b.X, float64(f.Y)+0.5-b.Y)
				before, had := last[b.ID][tile]
				for i, r := range sightRadii {
					if d > r {
						continue
					}
					near[i] = math.Min(near[i], dist)
					if tick >= from && (!had || before > r) {
						t.newly[i]++
					}
				}
			}
			seen[b.ID] = now
			if tick < from {
				continue
			}
			t.bodyTicks++
			for i := range sightRadii {
				if !math.IsInf(near[i], 1) {
					t.inSight[i]++
					t.nearest[i] += near[i]
				}
			}
		}
		last = seen
		w.Step()
	}
	return t, nil
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func sight(m engine.Map, name string, seeds int, seed0 int64, ticks int) {
	cfg, err := variant.Config(variant.Base, 1)
	if err != nil {
		fail(err)
	}
	balance := cfg.EnergyBurn / cfg.FoodEnergy
	full := energyTicks(cfg)
	fmt.Printf("釣り合いの頻度（`EnergyBurn` ÷ `FoodEnergy`）: %.3f%%\n\n", 100*balance)
	fmt.Printf("| 地図 | 区間（tick） | 半径 | 視界に食料がある割合 | 視界に食料が無い割合 | 新しく見えた頻度（/体・tick） | 新しく見えた ÷ 釣り合い | 一番近い食料までの距離（タイル） | 歩いて着くまで（tick） |\n")
	fmt.Printf("| --- | --- | --- | --- | --- | --- | --- | --- | --- |\n")
	// As in underfoot: the first span is before anyone can have starved,
	// the second the whole run.
	for _, span := range [][2]int{{0, min(full, ticks)}, {0, ticks}} {
		n := len(sightRadii)
		in, newly, near := make([][]float64, n), make([][]float64, n), make([][]float64, n)
		for s := 0; s < seeds; s++ {
			cfg.Seed = seed0 + int64(s)
			t, err := countSight(cfg, m, span[0], span[1])
			if err != nil {
				fail(err)
			}
			for i := range sightRadii {
				in[i] = append(in[i], t.inSight[i]/t.bodyTicks)
				newly[i] = append(newly[i], t.newly[i]/t.bodyTicks)
				if t.inSight[i] > 0 {
					near[i] = append(near[i], t.nearest[i]/t.inSight[i])
				}
			}
		}
		for i, r := range sightRadii {
			im, is := meanSE(in[i])
			nm, ns := meanSE(newly[i])
			dm, ds := meanSE(near[i])
			fmt.Printf("| %s (%dx%d, %d シード) | %d〜%d | %d | %.1f ± %.1f%% | %.1f%% | %.3f ± %.3f%% | %.2f | %.2f ± %.2f | %.1f |\n",
				name, m.Width, m.Height, seeds, span[0], span[1], r, 100*im, 100*is, 100*(1-im), 100*nm, 100*ns, nm/balance, dm, ds, dm/cfg.Speed)
		}
	}
}

// energyTicks is how many ticks a full body lasts.
func energyTicks(cfg engine.Config) int { return int(math.Ceil(cfg.EnergyMax/cfg.EnergyBurn - 1e-9)) }

// forageTally is one run's tally of the forage count.
type forageTally struct {
	onFood, ate, eatTied [bands]float64 // decisions on food, per energy band
	hungry, hungrySees   float64        // decisions below a third of full, and those with food in sight
	tied, lastAmong      float64        // decisions whose least risk is shared, and those where the body's last move is among them
	tiedUnseen           float64        // tied decisions with no food in sight
	decisions            float64
	spread, spreadN      float64 // distance from where the body was a life ago
}

// countForage runs the base world and reads it through the trace. A
// decision is on food when eating is offered; eating is tied when some other
// option carries the same least risk. The spread is taken once per body per
// tick, for bodies that were alive a full body's life ago.
func countForage(cfg engine.Config, m engine.Map, ticks int) (forageTally, error) {
	w, err := engine.NewWorld(cfg, m)
	if err != nil {
		return forageTally{}, err
	}
	var f forageTally
	life := energyTicks(cfg)
	lastMove := map[int64]int{}
	w.SetTrace(func(b engine.Body, v engine.Valuation, a engine.Action) {
		least := math.Inf(1)
		for _, r := range v.Risk[0] {
			least = math.Min(least, r)
		}
		same, among := 0, false
		last, moved := lastMove[b.ID]
		for j, o := range v.Options {
			if v.Risk[0][j] == least {
				same++
				among = among || (moved && o.Kind == engine.ActMove && o.Dir == last)
			}
		}
		if same > 1 {
			f.tied++
			if len(v.Seen) == 0 {
				f.tiedUnseen++
			}
			if among {
				f.lastAmong++
			}
		}
		if a.Kind == engine.ActMove {
			lastMove[b.ID] = a.Dir
		}
		f.decisions++
		band := min(int(b.Energy/cfg.EnergyMax*bands), bands-1)
		if b.Energy < cfg.EnergyMax/3 {
			f.hungry++
			if len(v.Seen) > 0 {
				f.hungrySees++
			}
		}
		for j, o := range v.Options {
			if o.Kind != engine.ActEat {
				continue
			}
			f.onFood[band]++
			if a.Kind == engine.ActEat {
				f.ate[band]++
			}
			least, same := true, 0
			for _, r := range v.Risk[0] {
				least = least && r >= v.Risk[0][j]
				if r == v.Risk[0][j] {
					same++
				}
			}
			if least && same > 1 {
				f.eatTied[band]++
			}
		}
	})
	past := map[int]map[int64][2]float64{}
	for t := 0; t < ticks && len(w.Bodies()) > 0; t++ {
		now := map[int64][2]float64{}
		for _, b := range w.Bodies() {
			now[b.ID] = [2]float64{b.X, b.Y}
			if p, ok := past[t-life][b.ID]; ok {
				f.spread += math.Hypot(b.X-p[0], b.Y-p[1])
				f.spreadN++
			}
		}
		past[t] = now
		delete(past, t-life)
		w.Step()
	}
	return f, nil
}

func forage(m engine.Map, name string, seeds int, seed0 int64, ticks int) {
	var ate, tied [bands][]float64
	var onFood [bands]float64
	var sees, spread, shared, unseen, among []float64
	for s := 0; s < seeds; s++ {
		cfg, err := variant.Config(variant.Base, seed0+int64(s))
		if err != nil {
			fail(err)
		}
		f, err := countForage(cfg, m, ticks)
		if err != nil {
			fail(err)
		}
		for k := range f.onFood {
			onFood[k] += f.onFood[k]
			if f.onFood[k] > 0 {
				ate[k] = append(ate[k], f.ate[k]/f.onFood[k])
				tied[k] = append(tied[k], f.eatTied[k]/f.onFood[k])
			}
		}
		if f.hungry > 0 {
			sees = append(sees, f.hungrySees/f.hungry)
		}
		if f.spreadN > 0 {
			spread = append(spread, f.spread/f.spreadN)
		}
		if f.decisions > 0 {
			shared = append(shared, f.tied/f.decisions)
		}
		if f.tied > 0 {
			unseen = append(unseen, f.tiedUnseen/f.tied)
			among = append(among, f.lastAmong/f.tied)
		}
	}
	cfg, _ := variant.Config(variant.Base, seed0)
	fmt.Printf("足元に食料がある決定（%s %dx%d、%d シード、%d tick）\n\n", name, m.Width, m.Height, seeds, ticks)
	fmt.Printf("| 体力の帯 | 決定の数 | 食べた割合 | 食べるが最善で同点 |\n| --- | --- | --- | --- |\n")
	for k := 0; k < bands; k++ {
		am, as := meanSE(ate[k])
		tm, ts := meanSE(tied[k])
		lo := cfg.EnergyMax / bands * float64(k)
		fmt.Printf("| %.0f〜%.0f | %.0f | %.1f ± %.1f%% | %.1f ± %.1f%% |\n", lo, lo+cfg.EnergyMax/bands, onFood[k], 100*am, 100*as, 100*tm, 100*ts)
	}
	sm, ss := meanSE(sees)
	dm, ds := meanSE(spread)
	fmt.Printf("\n体力が上限の 1/3 未満の決定で、視界に食料がある割合: %.1f ± %.1f%%\n", 100*sm, 100*ss)
	fmt.Printf("%d tick 前（満腹から餓死まで）にいた位置からの距離: %.2f ± %.2f タイル\n", energyTicks(cfg), dm, ds)
	tm, ts := meanSE(shared)
	um, us := meanSE(unseen)
	am, as := meanSE(among)
	fmt.Printf("最善の手が同点の決定: 全決定の %.1f ± %.1f%%。そのうち視界に食料が無い: %.1f ± %.1f%%、直前の移動が最善の手の中にある: %.1f ± %.1f%%\n", 100*tm, 100*ts, 100*um, 100*us, 100*am, 100*as)
}

// cycleLags are the lags the cycle count reads the food's autocorrelation
// at: half a meal, one meal, two, and the spacing of cmd/experiment's
// checkpoints and three of them.
var cycleLags = []int{150, 300, 600, 2000, 6000}

// cycleWindow is the width of the windows the swing is read over.
const cycleWindow = 5000

// cycleRun is one seed's series.
type cycleRun struct {
	food, pop []float64 // at the end of every tick
	phases    int       // distinct energies modulo one meal at the end, in tenths
}

// staggered rebuilds w with each body's starting energy drawn evenly from
// (1, EnergyMax], through a save and a load. It is not a rule of the world:
// it is the counterfactual start the count compares with, drawn from its
// own source so that the world's own draws are untouched.
func staggered(w *engine.World, seed int64) (*engine.World, error) {
	var buf bytes.Buffer
	if err := w.Save(&buf); err != nil {
		return nil, err
	}
	var snap map[string]any
	if err := json.Unmarshal(buf.Bytes(), &snap); err != nil {
		return nil, err
	}
	full := w.Config().EnergyMax
	r := rand.New(rand.NewSource(seed))
	for _, b := range snap["bodies"].([]any) {
		b.(map[string]any)["Energy"] = 1 + r.Float64()*(full-1)
	}
	out, err := json.Marshal(snap)
	if err != nil {
		return nil, err
	}
	return engine.Load(bytes.NewReader(out))
}

func runCycle(cfg engine.Config, m engine.Map, ticks int, stagger bool) (cycleRun, error) {
	w, err := engine.NewWorld(cfg, m)
	if err != nil {
		return cycleRun{}, err
	}
	if stagger {
		if w, err = staggered(w, cfg.Seed); err != nil {
			return cycleRun{}, err
		}
	}
	var c cycleRun
	for t := 0; t < ticks; t++ {
		w.Step()
		c.food = append(c.food, float64(w.FoodLedger().OnGround))
		c.pop = append(c.pop, float64(len(w.Bodies())))
	}
	phases := map[int64]bool{}
	for _, b := range w.Bodies() {
		phases[int64(math.Round(math.Mod(b.Energy, cfg.FoodEnergy)*10))] = true
	}
	c.phases = len(phases)
	return c, nil
}

func autocorr(xs []float64, lag int) float64 {
	n := len(xs) - lag
	if n < 2 {
		return math.NaN()
	}
	var ma, mb float64
	for i := 0; i < n; i++ {
		ma += xs[i]
		mb += xs[i+lag]
	}
	ma /= float64(n)
	mb /= float64(n)
	var sab, saa, sbb float64
	for i := 0; i < n; i++ {
		a, b := xs[i]-ma, xs[i+lag]-mb
		sab += a * b
		saa += a * a
		sbb += b * b
	}
	return sab / math.Sqrt(saa*sbb)
}

func spanOf(xs []float64) float64 {
	lo, hi := math.Inf(1), math.Inf(-1)
	for _, x := range xs {
		lo, hi = math.Min(lo, x), math.Max(hi, x)
	}
	return hi - lo
}

func cycle(m engine.Map, name string, seeds int, seed0 int64, ticks int) {
	cfg, err := variant.Config(variant.Base, seed0)
	if err != nil {
		fail(err)
	}
	meal := int(math.Round(cfg.FoodEnergy / cfg.EnergyBurn))
	// The first 10000 ticks are the fall from the start; the rhythm is read
	// after them.
	from := min(10000, ticks/2)
	fmt.Printf("地図 %s (%dx%d)、%d シード、%d tick。食事1回ぶんの tick: %d。周期は %d tick 以降で読む。\n\n", name, m.Width, m.Height, seeds, ticks, meal, from)
	for _, stagger := range []bool{false, true} {
		runs := make([]cycleRun, seeds)
		for s := range runs {
			c := cfg
			c.Seed = seed0 + int64(s)
			if runs[s], err = runCycle(c, m, ticks, stagger); err != nil {
				fail(err)
			}
		}
		label := "全員が満腹で同時に始まる（base）"
		if stagger {
			label = "始めの体力をばらした（1〜上限で一様）"
		}
		fmt.Printf("#### %s\n\n", label)

		fmt.Printf("地面の食料の自己相関（シードごと、平均 ± 標準誤差）\n\n| ずれ（tick） |")
		for _, l := range cycleLags {
			fmt.Printf(" %d |", l)
		}
		fmt.Printf("\n| --- |%s\n| 自己相関 |", strings.Repeat(" --- |", len(cycleLags)))
		for _, l := range cycleLags {
			var xs []float64
			for _, r := range runs {
				xs = append(xs, autocorr(r.food[from:], l))
			}
			m, se := meanSE(xs)
			fmt.Printf(" %.2f ± %.2f |", m, se)
		}
		fmt.Printf("\n\n")

		var phases []float64
		for _, r := range runs {
			if len(r.pop) > 0 && r.pop[len(r.pop)-1] > 0 {
				phases = append(phases, float64(r.phases))
			}
		}
		pm, _ := meanSE(phases)
		maxPhase := 0.0
		for _, x := range phases {
			maxPhase = math.Max(maxPhase, x)
		}
		fmt.Printf("最後の tick で、生きている身体の「体力 mod %.0f」（0.1 刻み）がとる値の数: 平均 %.1f、最大 %.0f（%d シード）\n\n", cfg.FoodEnergy, pm, maxPhase, len(phases))

		// Deaths over the meal cycle: the quarter of phases where the mean
		// food is lowest, against a quarter of the ticks.
		var share []float64
		for _, r := range runs {
			byPhase := make([]float64, meal)
			for t := from; t < ticks; t++ {
				byPhase[t%meal] += r.food[t]
			}
			order := make([]int, meal)
			for i := range order {
				order[i] = i
			}
			sort.Slice(order, func(a, b int) bool { return byPhase[order[a]] < byPhase[order[b]] })
			trough := map[int]bool{}
			for _, ph := range order[:meal/4] {
				trough[ph] = true
			}
			deaths, in := 0.0, 0.0
			for t := from + 1; t < ticks; t++ {
				d := r.pop[t-1] - r.pop[t]
				deaths += d
				if trough[t%meal] {
					in += d
				}
			}
			if deaths > 0 {
				share = append(share, in/deaths)
			}
		}
		sm, ss := meanSE(share)
		fmt.Printf("%d tick 以降の死亡のうち、食料が一番少ない 1/4 の位相で起きた割合: %.2f ± %.2f（一様なら 0.25、%d シード）\n\n", from, sm, ss, len(share))

		fmt.Printf("| 区間（tick） | 振れ幅: シード平均の食料 | 振れ幅: シードごと | 地面の食料（平均） | 人口（区間の終わり） |\n| --- | --- | --- | --- | --- |\n")
		for lo := 0; lo+cycleWindow <= ticks; lo += cycleWindow {
			hi := lo + cycleWindow
			mean := make([]float64, cycleWindow)
			var per, pop []float64
			food := 0.0
			for _, r := range runs {
				for t := lo; t < hi; t++ {
					mean[t-lo] += r.food[t] / float64(seeds)
					food += r.food[t] / float64(seeds*cycleWindow)
				}
				per = append(per, spanOf(r.food[lo:hi]))
				pop = append(pop, r.pop[hi-1])
			}
			pm, _ := meanSE(per)
			popM, popSE := meanSE(pop)
			fmt.Printf("| %d〜%d | %.1f | %.1f | %.1f | %.1f ± %.1f |\n", lo, hi, spanOf(mean), pm, food, popM, popSE)
		}
		fmt.Println()
	}
}

// exploreWindows are the windows exploration is read over: the first
// thousand ticks, when every body is alive, and a thousand ticks once the
// population has settled. The life counted is the life after the window.
var exploreWindows = [][2]int{{0, 1000}, {9000, 10000}}

// bodyTrack is what the explore count keeps of one body.
type bodyTrack struct {
	// slide* are per consecutive window of exploreSlide ticks from tick 0.
	slideUntrod, slideSighted, slideEnergy []float64
	visited                                map[int]bool
	tile                                   int
	seen                                   map[Food2]bool
	untrod                                 []float64 // per window: tiles entered for the first time
	sighted                                []float64 // per window: units come into sight
	energy                                 []float64 // per window: energy at the window's end
	died                                   int       // tick of death, -1 while alive
	inWindow                               []bool    // alive through the whole window
}

// exploreSlide is the width of the consecutive windows the hazard reading
// uses, and the horizon it asks about: does the body die within the next
// exploreSlide ticks after a window it lived through.
const exploreSlide = 1000

// Food2 is a food position as a map key.
type Food2 struct{ X, Y int }

// exploreRun runs one world to the end and returns every body's track.
// Tiles and sightings come from the trace, as the body decides: a tile is
// untrodden when the body has never decided on it before, and a unit comes
// into sight when it is in the valuation's Seen and was not in the body's
// previous decision's. A body's first decision counts neither.
func exploreRun(cfg engine.Config, m engine.Map, ticks int) (map[int64]*bodyTrack, error) {
	w, err := engine.NewWorld(cfg, m)
	if err != nil {
		return nil, err
	}
	tracks := map[int64]*bodyTrack{}
	nw := len(exploreWindows)
	win := -1
	w.SetTrace(func(b engine.Body, v engine.Valuation, _ engine.Action) {
		t := tracks[b.ID]
		tile := int(math.Floor(b.Y))*m.Width + int(math.Floor(b.X))
		if t == nil {
			ns := ticks / exploreSlide
			t = &bodyTrack{slideUntrod: make([]float64, ns), slideSighted: make([]float64, ns), slideEnergy: make([]float64, ns),
				visited: map[int]bool{tile: true}, tile: tile, seen: map[Food2]bool{},
				untrod: make([]float64, nw), sighted: make([]float64, nw), energy: make([]float64, nw),
				died: -1, inWindow: make([]bool, nw)}
			for _, f := range v.Seen {
				t.seen[Food2{f.X, f.Y}] = true
			}
			tracks[b.ID] = t
			return
		}
		slide := int(w.Tick()-1) / exploreSlide
		if tile != t.tile && !t.visited[tile] {
			t.visited[tile] = true
			if win >= 0 {
				t.untrod[win]++
			}
			if slide < len(t.slideUntrod) {
				t.slideUntrod[slide]++
			}
		}
		t.tile = tile
		now := map[Food2]bool{}
		for _, f := range v.Seen {
			k := Food2{f.X, f.Y}
			now[k] = true
			if !t.seen[k] {
				if win >= 0 {
					t.sighted[win]++
				}
				if slide < len(t.slideSighted) {
					t.slideSighted[slide]++
				}
			}
		}
		t.seen = now
	})
	// startAlive[i] is the bodies alive when window i opens; a body counts
	// in the window if it is among them and does not die before it closes.
	startAlive := make([]map[int64]bool, nw)
	alive := map[int64]bool{}
	for _, b := range w.Bodies() {
		alive[b.ID] = true
	}
	for tick := 0; tick < ticks && len(alive) > 0; tick++ {
		win = -1
		for i, wd := range exploreWindows {
			if tick >= wd[0] && tick < wd[1] {
				win = i
			}
			if tick == wd[0] {
				startAlive[i] = map[int64]bool{}
				for id := range alive {
					startAlive[i][id] = true
				}
			}
		}
		w.Step()
		now := map[int64]bool{}
		for _, b := range w.Bodies() {
			now[b.ID] = true
			if (tick+1)%exploreSlide == 0 && (tick+1)/exploreSlide-1 < ticks/exploreSlide {
				if t := tracks[b.ID]; t != nil {
					t.slideEnergy[(tick+1)/exploreSlide-1] = b.Energy
				}
			}
			for i, wd := range exploreWindows {
				if tick+1 == wd[1] {
					if t := tracks[b.ID]; t != nil {
						t.energy[i] = b.Energy
					}
				}
			}
		}
		for id := range alive {
			if !now[id] {
				if t := tracks[id]; t != nil {
					t.died = tick + 1
				}
			}
		}
		alive = now
	}
	for id, t := range tracks {
		for i, wd := range exploreWindows {
			t.inWindow[i] = startAlive[i][id] && (t.died < 0 || t.died > wd[1])
		}
	}
	return tracks, nil
}

// ranks returns the ranks of xs, ties sharing their average rank.
func ranks(xs []float64) []float64 {
	idx := make([]int, len(xs))
	for i := range idx {
		idx[i] = i
	}
	sort.Slice(idx, func(a, b int) bool { return xs[idx[a]] < xs[idx[b]] })
	r := make([]float64, len(xs))
	for i := 0; i < len(idx); {
		j := i
		for j+1 < len(idx) && xs[idx[j+1]] == xs[idx[i]] {
			j++
		}
		for k := i; k <= j; k++ {
			r[idx[k]] = float64(i+j)/2 + 1
		}
		i = j + 1
	}
	return r
}

// spearman is the rank correlation of xs and ys.
func spearman(xs, ys []float64) float64 {
	if len(xs) < 3 {
		return math.NaN()
	}
	return pearson(ranks(xs), ranks(ys))
}

func pearson(xs, ys []float64) float64 {
	n := float64(len(xs))
	var mx, my float64
	for i := range xs {
		mx += xs[i] / n
		my += ys[i] / n
	}
	var sxy, sxx, syy float64
	for i := range xs {
		sxy += (xs[i] - mx) * (ys[i] - my)
		sxx += (xs[i] - mx) * (xs[i] - mx)
		syy += (ys[i] - my) * (ys[i] - my)
	}
	if sxx == 0 || syy == 0 {
		return math.NaN()
	}
	return sxy / math.Sqrt(sxx*syy)
}

// auc is the chance that a body that died has a lower x than one that
// lived, ties counting half: 0.5 when x tells them apart no better than a
// coin, 1 when every body that died had less of x than every one that lived.
func auc(died, lived []float64) float64 {
	if len(died) == 0 || len(lived) == 0 {
		return math.NaN()
	}
	all := append(append([]float64(nil), died...), lived...)
	r := ranks(all)
	sum := 0.0
	for i := range died {
		sum += r[i]
	}
	nd, nl := float64(len(died)), float64(len(lived))
	// Mann-Whitney: the rank sum of the died, less its least value, over
	// the pairs; low x among the died pushes it towards 1.
	return 1 - (sum-nd*(nd+1)/2)/(nd*nl)
}

// hazard is one seed's reading of the consecutive windows: for every body
// alive through a window, its rates in the window and energy at its end,
// split by whether it died within the next window.
type hazard struct {
	died, lived [3][]float64 // untrodden rate, sighting rate, energy
}

// byID lists the tracks in the order of the bodies' IDs, so that what is
// read from them does not depend on the order of a map.
func byID(tracks map[int64]*bodyTrack) []*bodyTrack {
	ids := make([]int64, 0, len(tracks))
	for id := range tracks {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(a, b int) bool { return ids[a] < ids[b] })
	out := make([]*bodyTrack, len(ids))
	for i, id := range ids {
		out[i] = tracks[id]
	}
	return out
}

func hazardOf(tracks map[int64]*bodyTrack, ticks int) hazard {
	var h hazard
	span := float64(exploreSlide)
	for k := 0; (k+2)*exploreSlide <= ticks; k++ {
		start, end := k*exploreSlide, (k+1)*exploreSlide
		for _, t := range byID(tracks) {
			// Alive through the window: it has a track, so it was alive
			// at the start of the run, and it died after the window's end.
			if t.died >= 0 && t.died <= end {
				continue
			}
			_ = start
			xs := [3]float64{t.slideUntrod[k] / span, t.slideSighted[k] / span, t.slideEnergy[k]}
			dies := t.died >= 0 && t.died <= end+exploreSlide
			for j, x := range xs {
				if dies {
					h.died[j] = append(h.died[j], x)
				} else {
					h.lived[j] = append(h.lived[j], x)
				}
			}
		}
	}
	return h
}

func explore(m engine.Map, name string, seeds int, seed0 int64, ticks int) {
	fmt.Printf("地図 %s (%dx%d)、%d シード、%d tick。窓のあとの寿命は %d tick で打ち切る（生き残りは打ち切りの値で順位をつける）。\n\n", name, m.Width, m.Height, seeds, ticks, ticks)
	type perSeed struct {
		untrod, sighted, life, lifeCensored []float64 // per window, means over bodies
		n                                   []float64
		rhoU, rhoS, rhoE, rhoUS             []float64 // per window
		quint                               [][5][2]float64
	}
	variants := []string{variant.Blind, variant.Restless, variant.Base}
	nw := len(exploreWindows)
	for _, vn := range variants {
		var ps []perSeed
		var hz []hazard
		for s := 0; s < seeds; s++ {
			cfg, err := variant.Config(vn, seed0+int64(s))
			if err != nil {
				fail(err)
			}
			tracks, err := exploreRun(cfg, m, ticks)
			if err != nil {
				fail(err)
			}
			hz = append(hz, hazardOf(tracks, ticks))
			p := perSeed{untrod: make([]float64, nw), sighted: make([]float64, nw), life: make([]float64, nw),
				lifeCensored: make([]float64, nw), n: make([]float64, nw), rhoU: make([]float64, nw),
				rhoS: make([]float64, nw), rhoE: make([]float64, nw), rhoUS: make([]float64, nw), quint: make([][5][2]float64, nw)}
			for i, wd := range exploreWindows {
				span := float64(wd[1] - wd[0])
				var u, sg, e, life []float64
				censored := 0.0
				for _, t := range byID(tracks) {
					if !t.inWindow[i] {
						continue
					}
					l := float64(ticks - wd[1])
					if t.died >= 0 {
						l = float64(t.died - wd[1])
					} else {
						censored++
					}
					u = append(u, t.untrod[i]/span)
					sg = append(sg, t.sighted[i]/span)
					e = append(e, t.energy[i])
					life = append(life, l)
				}
				p.n[i] = float64(len(u))
				if len(u) == 0 {
					for _, f := range []*[]float64{&p.untrod, &p.sighted, &p.life, &p.lifeCensored, &p.rhoU, &p.rhoS, &p.rhoE, &p.rhoUS} {
						(*f)[i] = math.NaN()
					}
					continue
				}
				um, _ := meanSE(u)
				sm, _ := meanSE(sg)
				lm, _ := meanSE(life)
				p.untrod[i], p.sighted[i], p.life[i] = um, sm, lm
				p.lifeCensored[i] = censored / float64(len(u))
				p.rhoU[i] = spearman(u, life)
				p.rhoS[i] = spearman(sg, life)
				p.rhoE[i] = spearman(e, life)
				p.rhoUS[i] = spearman(u, sg)
				// Quintiles of the sighting rate: mean life and share dying.
				order := make([]int, len(sg))
				for k := range order {
					order[k] = k
				}
				sort.SliceStable(order, func(a, b int) bool { return sg[order[a]] < sg[order[b]] })
				for q := 0; q < 5; q++ {
					lo, hi := q*len(order)/5, (q+1)*len(order)/5
					for _, k := range order[lo:hi] {
						p.quint[i][q][0] += life[k] / float64(max(hi-lo, 1))
						if life[k] < float64(ticks-wd[1]) {
							p.quint[i][q][1] += 1 / float64(max(hi-lo, 1))
						}
					}
				}
			}
			ps = append(ps, p)
		}
		col := func(i int, f func(perSeed) []float64) string {
			var xs []float64
			for _, p := range ps {
				if x := f(p)[i]; !math.IsNaN(x) {
					xs = append(xs, x)
				}
			}
			if len(xs) == 0 {
				return "—"
			}
			m, se := meanSE(xs)
			if math.IsNaN(se) {
				return fmt.Sprintf("%.3f", m)
			}
			return fmt.Sprintf("%.3f ± %.3f", m, se)
		}
		fmt.Printf("#### %s\n\n", vn)
		fmt.Printf("| 窓（tick） | 窓を通して生きていた身体（1シードあたり） | 未踏タイル率（/tick） | 食料視認率（/tick） | 窓のあとの寿命（平均） | 打ち切り（生き残り）の割合 |\n| --- | --- | --- | --- | --- | --- |\n")
		for i, wd := range exploreWindows {
			fmt.Printf("| %d〜%d | %s | %s | %s | %s | %s |\n", wd[0], wd[1], col(i, func(p perSeed) []float64 { return p.n }),
				col(i, func(p perSeed) []float64 { return p.untrod }), col(i, func(p perSeed) []float64 { return p.sighted }),
				col(i, func(p perSeed) []float64 { return p.life }), col(i, func(p perSeed) []float64 { return p.lifeCensored }))
		}
		fmt.Printf("\n順位相関（シードごとに身体をまたいで取り、シードで平均 ± 標準誤差）\n\n| 窓（tick） | 未踏タイル率 × 寿命 | 食料視認率 × 寿命 | 窓の終わりの体力 × 寿命 | 未踏タイル率 × 食料視認率 |\n| --- | --- | --- | --- | --- |\n")
		for i, wd := range exploreWindows {
			fmt.Printf("| %d〜%d | %s | %s | %s | %s |\n", wd[0], wd[1], col(i, func(p perSeed) []float64 { return p.rhoU }),
				col(i, func(p perSeed) []float64 { return p.rhoS }), col(i, func(p perSeed) []float64 { return p.rhoE }),
				col(i, func(p perSeed) []float64 { return p.rhoUS }))
		}
		fmt.Printf("\n%d tick ごとの窓を通して生きていた身体が、次の %d tick で死ぬかどうか（全区間をシードごとにまとめ、シードで平均 ± 標準誤差）\n\n", exploreSlide, exploreSlide)
		fmt.Printf("| 指標 | 死んだ（1シードあたり） | 生き残った（1シードあたり） | AUC: 未踏タイル率 | AUC: 食料視認率 | AUC: 窓の終わりの体力 |\n| --- | --- | --- | --- | --- | --- |\n")
		var nd, nl []float64
		var a [3][]float64
		for _, h := range hz {
			nd = append(nd, float64(len(h.died[0])))
			nl = append(nl, float64(len(h.lived[0])))
			for j := range a {
				if x := auc(h.died[j], h.lived[j]); !math.IsNaN(x) {
					a[j] = append(a[j], x)
				}
			}
		}
		cell := func(xs []float64) string {
			if len(xs) == 0 {
				return "—"
			}
			m, se := meanSE(xs)
			return fmt.Sprintf("%.3f ± %.3f", m, se)
		}
		ndm, _ := meanSE(nd)
		nlm, _ := meanSE(nl)
		fmt.Printf("| 次の窓で死ぬか | %.1f | %.1f | %s | %s | %s |\n", ndm, nlm, cell(a[0]), cell(a[1]), cell(a[2]))
		fmt.Printf("\n食料視認率の五分位ごとの、窓のあとの寿命と、打ち切りまでに死んだ割合（シード平均）\n\n| 窓（tick） | 五分位 | 寿命 | 死んだ割合 |\n| --- | --- | --- | --- |\n")
		for i, wd := range exploreWindows {
			for q := 0; q < 5; q++ {
				var l, d []float64
				for _, p := range ps {
					if p.n[i] >= 5 {
						l = append(l, p.quint[i][q][0])
						d = append(d, p.quint[i][q][1])
					}
				}
				if len(l) == 0 {
					continue
				}
				lm, _ := meanSE(l)
				dm, _ := meanSE(d)
				fmt.Printf("| %d〜%d | %d（低→高） | %.0f | %.2f |\n", wd[0], wd[1], q+1, lm, dm)
			}
		}
		fmt.Println()
	}
}

// shuttleSpan is the path length the shuttle count reads: the ticks a full
// body takes to starve, so that a body that dies of hunger is read over the
// whole stretch in which it did not eat.
func shuttleSpan(cfg engine.Config) int { return energyTicks(cfg) }

// shuttleTrack is the last shuttleSpan positions of one body.
type shuttleTrack struct {
	xs, ys []float64
	n      int // positions recorded so far, up to the span
	next   int
}

func (t *shuttleTrack) push(x, y float64) {
	t.xs[t.next], t.ys[t.next] = x, y
	t.next = (t.next + 1) % len(t.xs)
	t.n = min(t.n+1, len(t.xs))
}

// onLine reports whether the whole recorded span stays within one column or
// one row: a spread under one tile along either axis. Only a full span
// counts.
func (t *shuttleTrack) onLine() bool {
	if t.n < len(t.xs) {
		return false
	}
	minX, maxX, minY, maxY := math.Inf(1), math.Inf(-1), math.Inf(1), math.Inf(-1)
	for i := range t.xs {
		minX, maxX = math.Min(minX, t.xs[i]), math.Max(maxX, t.xs[i])
		minY, maxY = math.Min(minY, t.ys[i]), math.Max(maxY, t.ys[i])
	}
	return maxX-minX < 1 || maxY-minY < 1
}

// shuttleChecks are the ticks at which the living bodies are read.
var shuttleChecks = []int{5000, 10000, 20000, 40000}

type shuttleRun struct {
	deaths, lineDeaths float64 // after shuttleFrom
	alive, lineAlive   [4]float64
	axisAlive          [4]float64
}

// shuttleFrom is where the deaths start to count: past the first two meal
// waves, where the starting bodies thin out wherever they stand.
const shuttleFrom = 5000

func runShuttle(cfg engine.Config, m engine.Map, ticks int) (shuttleRun, error) {
	var r shuttleRun
	w, err := engine.NewWorld(cfg, m)
	if err != nil {
		return r, err
	}
	span := shuttleSpan(cfg)
	tracks := map[int64]*shuttleTrack{}
	for tick := 1; tick <= ticks; tick++ {
		w.Step()
		now := map[int64]bool{}
		bodies := w.Bodies()
		for _, b := range bodies {
			now[b.ID] = true
			t := tracks[b.ID]
			if t == nil {
				t = &shuttleTrack{xs: make([]float64, span), ys: make([]float64, span)}
				tracks[b.ID] = t
			}
			t.push(b.X, b.Y)
		}
		for id, t := range tracks {
			if now[id] {
				continue
			}
			if tick > shuttleFrom {
				r.deaths++
				if t.onLine() {
					r.lineDeaths++
				}
			}
			delete(tracks, id)
		}
		for i, c := range shuttleChecks {
			if tick != c {
				continue
			}
			for _, b := range bodies {
				r.alive[i]++
				if tracks[b.ID].onLine() {
					r.lineAlive[i]++
				}
				if b.Heading >= 0 && b.Heading%2 == 0 {
					r.axisAlive[i]++
				}
			}
		}
		if len(bodies) == 0 {
			break
		}
	}
	return r, nil
}

// shuttle counts bodies that walk one row or column back and forth: a
// heading along an axis reflects into its own reverse, so a body that
// keeps it covers a strip three tiles wide and nothing else. It sets the
// share of deaths that end such a walk against the share of the living on
// one.
func shuttle(m engine.Map, name string, seeds int, seed0 int64, ticks int) {
	cfg0, _ := variant.Config(variant.Base, seed0)
	fmt.Printf("地図 %s (%dx%d)、%d シード、%d tick、base。直線の往復 ＝ 直前の %d tick の位置の広がりが、x か y のどちらかで 1 タイル未満。死亡は tick %d より後だけ数える。\n\n",
		name, m.Width, m.Height, seeds, ticks, shuttleSpan(cfg0), shuttleFrom)
	var runs []shuttleRun
	for s := 0; s < seeds; s++ {
		cfg, err := variant.Config(variant.Base, seed0+int64(s))
		if err != nil {
			fail(err)
		}
		r, err := runShuttle(cfg, m, ticks)
		if err != nil {
			fail(err)
		}
		runs = append(runs, r)
	}
	cell := func(f func(shuttleRun) (float64, bool)) string {
		var xs []float64
		for _, r := range runs {
			if x, ok := f(r); ok {
				xs = append(xs, x)
			}
		}
		if len(xs) == 0 {
			return "—"
		}
		mm, se := meanSE(xs)
		if math.IsNaN(se) {
			return fmt.Sprintf("%.3f", mm)
		}
		return fmt.Sprintf("%.3f ± %.3f", mm, se)
	}
	fmt.Printf("| 量 | 値（シード平均 ± 標準誤差） |\n| --- | --- |\n")
	fmt.Printf("| 死亡（1シードあたり） | %s |\n", cell(func(r shuttleRun) (float64, bool) { return r.deaths, true }))
	fmt.Printf("| 死亡のうち直線の往復で終わった割合 | %s |\n", cell(func(r shuttleRun) (float64, bool) { return r.lineDeaths / r.deaths, r.deaths > 0 }))
	fmt.Printf("\n| tick | 生きている身体 | 直線の往復をしている割合 | 向きが軸に沿っている割合 |\n| --- | --- | --- | --- |\n")
	for i, c := range shuttleChecks {
		if c > ticks {
			continue
		}
		fmt.Printf("| %d | %s | %s | %s |\n", c,
			cell(func(r shuttleRun) (float64, bool) { return r.alive[i], true }),
			cell(func(r shuttleRun) (float64, bool) { return r.lineAlive[i] / r.alive[i], r.alive[i] > 0 }),
			cell(func(r shuttleRun) (float64, bool) { return r.axisAlive[i] / r.alive[i], r.alive[i] > 0 }))
	}
	fmt.Println()
}

// changeKinds name what came before a decision, in the order a decision
// is filed: the first that applies.
var changeKinds = []string{
	"足元の食料が変わった",
	"見えている食料が変わった",
	"前の手の次の一歩が陸の外か別の地域",
	"どれでもない",
}

type changeTally struct {
	decisions     float64
	same          float64
	by, changedBy [4]float64
	// noneBy splits the changes with none of the kinds above: [0] still
	// walking to the same unit in sight (the path turned), [1] from moving
	// on through the region to walking to a unit in sight, [2] the rest.
	noneBy [3]float64
	runs   []float64 // ticks between changes of action, per body
}

// runChange reads every decision of the base world from the trace, after
// the first meal waves (tick 5000), and files it by what changed since the
// same body's previous decision.
func runChange(cfg engine.Config, m engine.Map, ticks int) (changeTally, error) {
	var t changeTally
	w, err := engine.NewWorld(cfg, m)
	if err != nil {
		return t, err
	}
	type last struct {
		act    engine.Action
		eat    bool
		seen   map[Food2]bool
		region engine.RegionID
		x, y   float64
		since  int
		target Food2
		walks  bool
		ok     bool
	}
	prev := map[int64]*last{}
	speed := cfg.Speed
	step := func(x, y float64, a engine.Action) (float64, float64) {
		ang := float64(a.Dir) * math.Pi / 4
		dx, dy := math.Round(math.Cos(ang)*1e9)/1e9, math.Round(math.Sin(ang)*1e9)/1e9
		return x + dx*speed, y + dy*speed
	}
	w.SetTrace(func(b engine.Body, v engine.Valuation, a engine.Action) {
		eat := false
		for _, o := range v.Options {
			if o.Kind == engine.ActEat {
				eat = true
			}
		}
		seen := map[Food2]bool{}
		for _, f := range v.Seen {
			seen[Food2{f.X, f.Y}] = true
		}
		walks, target := false, Food2{}
		if j := optionOf(v.Options, a); j >= 0 && j < len(v.Plan) && v.Plan[j] >= 0 {
			f := v.Seen[v.Plan[j]]
			walks, target = true, Food2{f.X, f.Y}
		}
		p := prev[b.ID]
		if p == nil {
			p = &last{}
			prev[b.ID] = p
		}
		if p.ok && w.Tick() > 5000 {
			kind := 3
			switch {
			case eat != p.eat:
				kind = 0
			case len(seen) != len(p.seen) || !sameSet(seen, p.seen):
				kind = 1
			case p.act.Kind == engine.ActMove:
				nx, ny := step(b.X, b.Y, p.act)
				tx, ty := int(math.Floor(nx)), int(math.Floor(ny))
				if !m.InBounds(tx, ty) || m.TerrainAt(tx, ty) != engine.TerrainLand ||
					m.RegionAt(tx, ty) != m.RegionAt(int(math.Floor(b.X)), int(math.Floor(b.Y))) {
					kind = 2
				}
			}
			t.decisions++
			t.by[kind]++
			if a == p.act {
				t.same++
				p.since++
			} else {
				t.changedBy[kind]++
				if kind == 3 {
					switch {
					case walks && p.walks && target == p.target:
						t.noneBy[0]++
					case walks && !p.walks:
						t.noneBy[1]++
					default:
						t.noneBy[2]++
					}
				}
				t.runs = append(t.runs, float64(p.since+1))
				p.since = 0
			}
		}
		p.act, p.eat, p.seen, p.ok = a, eat, seen, true
		p.walks, p.target = walks, target
	})
	for tick := 0; tick < ticks; tick++ {
		w.Step()
		if len(w.Bodies()) == 0 {
			break
		}
	}
	return t, nil
}

func optionOf(opts []engine.Action, a engine.Action) int {
	for i, o := range opts {
		if o == a {
			return i
		}
	}
	return -1
}

func sameSet(a, b map[Food2]bool) bool {
	for k := range a {
		if !b[k] {
			return false
		}
	}
	return true
}

// change counts, for stage 1-2e, how often a decision changes the body's
// action and what came before the ones that do: whether deciding only when
// something observable changes would take the same actions.
func change(m engine.Map, name string, seeds int, seed0 int64, ticks int) {
	fmt.Printf("地図 %s (%dx%d)、%d シード、%d tick、base。tick 5000 より後の決定を、同じ身体の前の決定からの変化で分ける（上から順に最初に当てはまるもの）。\n\n", name, m.Width, m.Height, seeds, ticks)
	var ts []changeTally
	for s := 0; s < seeds; s++ {
		cfg, err := variant.Config(variant.Base, seed0+int64(s))
		if err != nil {
			fail(err)
		}
		t, err := runChange(cfg, m, ticks)
		if err != nil {
			fail(err)
		}
		ts = append(ts, t)
	}
	cell := func(f func(changeTally) float64) string {
		var xs []float64
		for _, t := range ts {
			xs = append(xs, f(t))
		}
		mm, se := meanSE(xs)
		return fmt.Sprintf("%.4f ± %.4f", mm, se)
	}
	fmt.Printf("前の決定と同じ手を選んだ割合: %s\n\n", cell(func(t changeTally) float64 { return t.same / t.decisions }))
	fmt.Printf("| 直前に変わったこと | 決定に占める割合 | そのうち手が変わった割合 | 手が変わった決定に占める割合 |\n| --- | --- | --- | --- |\n")
	for k, n := range changeKinds {
		fmt.Printf("| %s | %s | %s | %s |\n", n,
			cell(func(t changeTally) float64 { return t.by[k] / t.decisions }),
			cell(func(t changeTally) float64 {
				if t.by[k] == 0 {
					return 0
				}
				return t.changedBy[k] / t.by[k]
			}),
			cell(func(t changeTally) float64 { return t.changedBy[k] / (t.decisions - t.same) }))
	}
	fmt.Printf("\n「どれでもない」で手が変わった決定の内訳\n\n| 内訳 | 割合 |\n| --- | --- |\n")
	for k, n := range []string{"同じ見えている食料へ歩き続けて、道が曲がった", "地域を歩き続ける手から、見えている食料へ歩く手に変わった", "その他"} {
		fmt.Printf("| %s | %s |\n", n, cell(func(t changeTally) float64 {
			if t.changedBy[3] == 0 {
				return 0
			}
			return t.noneBy[k] / t.changedBy[3]
		}))
	}
	var all []float64
	for _, t := range ts {
		all = append(all, t.runs...)
	}
	sort.Float64s(all)
	q := func(p float64) float64 { return all[int(p*float64(len(all)-1))] }
	fmt.Printf("\n同じ手が続いた長さ（tick、全シードの手の変わり目をまとめて）: 中央値 %.0f、90%% 点 %.0f、99%% 点 %.0f、最大 %.0f\n\n", q(0.5), q(0.9), q(0.99), all[len(all)-1])
}
