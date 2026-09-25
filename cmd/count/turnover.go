package main

import (
	"fmt"
	"math"
	"sort"

	"github.com/toku463ne/cld_generic_dangeon/engine"
	"github.com/toku463ne/cld_generic_dangeon/variant"
)

// deathKinds file a death, in order: a child (before coming of age); an
// adult that paid for a birth within the last full-to-starving span; an
// adult that had paid for births, but not that recently; an adult that
// never paid for one.
var deathKinds = []string{"成人前（子）", "成人・直前 1000 tick 以内に子の代金を払った", "成人・払ったのはそれより前", "成人・一度も払っていない"}

type turnoverTally struct {
	deaths     [4]float64
	ageAll     []float64 // age at death, every death after tick 5000
	ageAdult   []float64
	sincePaid  []float64  // ticks from the last payment to death, adults that paid
	payEnergy  []float64  // parent's energy at the start of the tick it paid
	children   []float64  // children per adult that died
	binDeaths  []float64  // deaths per 100-tick bin after tick 5000
	binPop     []float64  // population at the end of each bin
	olderShare [3]float64 // share of the living older than 1000/2000/4000, summed over samples
	samples    float64
}

// runTurnover reads, in the base world after tick 5000, every death: how
// old, whether a child, how recently it paid for a birth and how many
// children it had; how deaths fall over time; and the ages of the living.
func runTurnover(cfg engine.Config, m engine.Map, ticks int) (turnoverTally, error) {
	var t turnoverTally
	w, err := engine.NewWorld(cfg, m)
	if err != nil {
		return t, err
	}
	burnTicks := int64(math.Ceil(cfg.EnergyMax/cfg.EnergyBurn - 1e-9))
	lastPaid := map[int64]int64{}
	kids := map[int64]int{}
	binD := 0.0
	for tick := 1; tick <= ticks; tick++ {
		before := w.Bodies()
		prev := make(map[int64]engine.Body, len(before))
		for _, b := range before {
			prev[b.ID] = b
		}
		w.Step()
		now := int64(w.Tick())
		alive := map[int64]bool{}
		for _, b := range w.Bodies() {
			alive[b.ID] = true
			if _, old := prev[b.ID]; !old && b.Parents[0] >= 0 {
				for _, p := range b.Parents {
					lastPaid[p] = now
					kids[p]++
					if tick > 5000 {
						t.payEnergy = append(t.payEnergy, prev[p].Energy)
					}
				}
			}
		}
		for _, b := range before {
			if alive[b.ID] {
				continue
			}
			if tick > 5000 {
				age := float64(now - b.Born)
				t.ageAll = append(t.ageAll, age)
				paid, ok := lastPaid[b.ID]
				switch {
				case now < b.Mature:
					t.deaths[0]++
				case ok && now-paid <= burnTicks:
					t.deaths[1]++
				case ok:
					t.deaths[2]++
				default:
					t.deaths[3]++
				}
				if now >= b.Mature {
					t.ageAdult = append(t.ageAdult, age)
					t.children = append(t.children, float64(kids[b.ID]))
					if ok {
						t.sincePaid = append(t.sincePaid, float64(now-paid))
					}
				}
				binD++
			}
			delete(lastPaid, b.ID)
			delete(kids, b.ID)
		}
		if tick > 5000 && tick%100 == 0 {
			t.binDeaths = append(t.binDeaths, binD)
			t.binPop = append(t.binPop, float64(len(w.Bodies())))
			binD = 0
			bs := w.Bodies()
			for i, a := range []int64{1000, 2000, 4000} {
				n := 0
				for _, b := range bs {
					if now-b.Born > a {
						n++
					}
				}
				if len(bs) > 0 {
					t.olderShare[i] += float64(n) / float64(len(bs))
				}
			}
			t.samples++
		}
		if len(w.Bodies()) == 0 {
			break
		}
	}
	return t, nil
}

func quantiles(xs []float64, ps ...float64) string {
	if len(xs) == 0 {
		return "—"
	}
	ys := append([]float64(nil), xs...)
	sort.Float64s(ys)
	out := ""
	for i, p := range ps {
		if i > 0 {
			out += "・"
		}
		out += fmt.Sprintf("%.0f", ys[int(p*float64(len(ys)-1))])
	}
	return out
}

func meanVar(xs []float64) (float64, float64) {
	m, v := 0.0, 0.0
	for _, x := range xs {
		m += x
	}
	m /= float64(len(xs))
	for _, x := range xs {
		v += (x - m) * (x - m)
	}
	return m, v / float64(len(xs)-1)
}

// turnover separates, for stage 1-3, generations passing from a world that
// dies back and grows again: what the dead died of and at what age, and
// whether deaths come steadily or in waves.
func turnover(m engine.Map, name string, seeds int, seed0 int64, ticks int) {
	fmt.Printf("地図 %s (%dx%d)、%d シード、%d tick、base（1-3）。tick 5000 より後の死亡。\n\n", name, m.Width, m.Height, seeds, ticks)
	var ts []turnoverTally
	for s := 0; s < seeds; s++ {
		cfg, err := variant.Config(variant.Base, seed0+int64(s))
		if err != nil {
			fail(err)
		}
		t, err := runTurnover(cfg, m, ticks)
		if err != nil {
			fail(err)
		}
		ts = append(ts, t)
	}
	cell := func(f func(turnoverTally) float64, prec int) string {
		var xs []float64
		for _, t := range ts {
			xs = append(xs, f(t))
		}
		mm, se := meanSE(xs)
		return fmt.Sprintf("%.*f ± %.*f", prec, mm, prec, se)
	}
	all := func(f func(turnoverTally) []float64) []float64 {
		var xs []float64
		for _, t := range ts {
			xs = append(xs, f(t)...)
		}
		return xs
	}
	total := func(t turnoverTally) float64 { return t.deaths[0] + t.deaths[1] + t.deaths[2] + t.deaths[3] }
	fmt.Printf("死因は全て餓死（1-3 の世界に他の死因は無い）。餓死の内訳:\n\n| 死んだ身体 | 1シードあたり | 死亡に占める割合 |\n| --- | --- | --- |\n")
	for k, n := range deathKinds {
		fmt.Printf("| %s | %s | %s |\n", n, cell(func(t turnoverTally) float64 { return t.deaths[k] }, 1), cell(func(t turnoverTally) float64 { return t.deaths[k] / total(t) }, 4))
	}
	fmt.Printf("\n| 量 | 10%% 点・25%% 点・中央値・75%% 点・90%% 点・最大（全シード） |\n| --- | --- |\n")
	q := []float64{0.1, 0.25, 0.5, 0.75, 0.9, 1}
	fmt.Printf("| 死んだ年齢（全員） | %s |\n", quantiles(all(func(t turnoverTally) []float64 { return t.ageAll }), q...))
	fmt.Printf("| 死んだ年齢（成人） | %s |\n", quantiles(all(func(t turnoverTally) []float64 { return t.ageAdult }), q...))
	fmt.Printf("| 最後に子の代金を払ってから死ぬまでの tick（払ったことのある成人） | %s |\n", quantiles(all(func(t turnoverTally) []float64 { return t.sincePaid }), q...))
	fmt.Printf("| 子の代金を払った tick の始めの親の体力 | %s |\n", quantiles(all(func(t turnoverTally) []float64 { return t.payEnergy }), q...))
	fmt.Printf("| 死んだ成人 1 体あたりの子の数 | %s |\n", quantiles(all(func(t turnoverTally) []float64 { return t.children }), q...))
	fmt.Printf("\n死亡の時間的な偏り（100 tick ごとの死亡数）:\n\n| 量 | 値 |\n| --- | --- |\n")
	fmt.Printf("| 100 tick あたりの死亡の平均 | %s |\n", cell(func(t turnoverTally) float64 { m, _ := meanVar(t.binDeaths); return m }, 2))
	fmt.Printf("| 分散 ÷ 平均（1 ならでたらめに一様、大きいほど波） | %s |\n", cell(func(t turnoverTally) float64 { m, v := meanVar(t.binDeaths); return v / m }, 2))
	fmt.Printf("| 最大の区間の死亡 ÷ 平均 | %s |\n", cell(func(t turnoverTally) float64 {
		m, _ := meanVar(t.binDeaths)
		mx := 0.0
		for _, x := range t.binDeaths {
			mx = math.Max(mx, x)
		}
		return mx / m
	}, 2))
	fmt.Printf("| 人口の変動係数（100 tick ごと） | %s |\n", cell(func(t turnoverTally) float64 { m, v := meanVar(t.binPop); return math.Sqrt(v) / m }, 3))
	fmt.Printf("| 人口の最小 ÷ 平均 | %s |\n", cell(func(t turnoverTally) float64 {
		m, _ := meanVar(t.binPop)
		mn := math.Inf(1)
		for _, x := range t.binPop {
			mn = math.Min(mn, x)
		}
		return mn / m
	}, 3))
	fmt.Printf("\n生きている身体の年齢（100 tick ごとの平均）:\n\n| 年齢 | 割合 |\n| --- | --- |\n")
	for i, a := range []int{1000, 2000, 4000} {
		fmt.Printf("| %d tick より上 | %s |\n", a, cell(func(t turnoverTally) float64 { return t.olderShare[i] / t.samples }, 4))
	}
	fmt.Println()
}
