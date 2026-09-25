package main

import (
	"fmt"
	"math/rand"

	"github.com/toku463ne/cld_generic_dangeon/engine"
	"github.com/toku463ne/cld_generic_dangeon/variant"
)

// invaders are the builds the invade count sets half of the bodies to, as
// factors on the config's values: each candidate once for the better and
// once for the worse. A burn below 1 is the better fuel.
var invaders = []struct {
	name      string
	speed     float64
	max, burn float64
}{
	{"速さ ×1.25", 1.25, 1, 1},
	{"速さ ×0.8", 0.8, 1, 1},
	{"(a) 体力の上限 ×1.25", 1, 1.25, 1},
	{"(a) 体力の上限 ×0.8", 1, 0.8, 1},
	{"(b) 消耗 ×0.8（燃費が良い）", 1, 1, 0.8},
	{"(b) 消耗 ×1.25（燃費が悪い）", 1, 1, 1.25},
	{"(c) 上限 ×1.25・消耗 ×0.8", 1, 1.25, 0.8},
	{"(c) 上限 ×0.8・消耗 ×1.25", 1, 0.8, 1.25},
}

// invadeTally is, per group (0 the config's build, 1 set apart), the
// children born after tick 5000 that came of age and that died young, and
// the adults that died after tick 5000 and the children they had.
type invadeTally struct {
	matured, young [2]float64
	adults, kids   [2]float64
	// Bodies born after tick 5000 that died before the end, and the
	// children they had in their lives: lifetime births per body born,
	// children that died young counting as none.
	lives, lifeKids [2]float64
}

// runInvade sets each body, from tick 0 and as it is born, to the build
// with chance one half, drawn from a source of its own so that the world's
// draws are its own; and counts, by group, what came of the children and
// how many children the dead adults had.
func runInvade(cfg engine.Config, m engine.Map, ticks int, build engine.Build) (invadeTally, error) {
	var t invadeTally
	w, err := engine.NewWorld(cfg, m)
	if err != nil {
		return t, err
	}
	pick := rand.New(rand.NewSource(cfg.Seed*7919 + 17))
	group := map[int64]int{}
	kids := map[int64]int{}
	assign := func(b engine.Body) {
		g := pick.Intn(2)
		group[b.ID] = g
		if g == 1 {
			w.SetBuild(b.ID, build)
		}
	}
	for _, b := range w.Bodies() {
		assign(b)
	}
	for tick := 1; tick <= ticks; tick++ {
		before := w.Bodies()
		w.Step()
		now := w.Tick()
		alive := map[int64]bool{}
		for _, b := range w.Bodies() {
			alive[b.ID] = true
			if _, ok := group[b.ID]; !ok {
				assign(b)
				for _, p := range b.Parents {
					kids[p]++
				}
			}
			if b.Mature == now && b.Born > 5000 {
				t.matured[group[b.ID]]++
			}
		}
		for _, b := range before {
			if alive[b.ID] {
				continue
			}
			g := group[b.ID]
			if b.Born > 5000 {
				t.lives[g]++
				t.lifeKids[g] += float64(kids[b.ID])
			}
			switch {
			case now < b.Mature && b.Born > 5000:
				t.young[g]++
			case now >= b.Mature && tick > 5000:
				t.adults[g]++
				t.kids[g] += float64(kids[b.ID])
			}
			delete(group, b.ID)
			delete(kids, b.ID)
		}
		if len(w.Bodies()) == 0 {
			break
		}
	}
	return t, nil
}

// invade counts, for stage 1-4, the selection gradient on each candidate
// ability: half of the bodies born have it raised or lowered, and they are
// set against the other half in the same world.
func invade(m engine.Map, name string, seeds int, seed0 int64, ticks int) {
	fmt.Printf("地図 %s (%dx%d)、%d シード、%d tick、base（1-3）。生まれた身体の半分（と最初の身体の半分）の能力を振り、残り半分と同じシードの中で比べる。成人到達率は tick 5000 より後に生まれた子、子の数は tick 5000 より後に死んだ成人。\n\n", name, m.Width, m.Height, seeds, ticks)
	fmt.Printf("| 振った能力 | 生涯出生数: 振った − 振らない | 成人到達率: 振った − 振らない | 死んだ成人1体あたりの子の数: 振った − 振らない | 振らない身体の生涯出生数・成人到達率・子の数 |\n| --- | --- | --- | --- | --- |\n")
	for _, inv := range invaders {
		var dLife, life0, dRate, dKids, rate0, kids0 []float64
		for s := 0; s < seeds; s++ {
			cfg, err := variant.Config(variant.Base, seed0+int64(s))
			if err != nil {
				fail(err)
			}
			b := engine.Build{Speed: cfg.Speed * inv.speed, EnergyMax: cfg.EnergyMax * inv.max, EnergyBurn: cfg.EnergyBurn * inv.burn}
			t, err := runInvade(cfg, m, ticks, b)
			if err != nil {
				fail(err)
			}
			r := func(g int) float64 { return t.matured[g] / (t.matured[g] + t.young[g]) }
			k := func(g int) float64 { return t.kids[g] / t.adults[g] }
			l := func(g int) float64 { return t.lifeKids[g] / t.lives[g] }
			dLife = append(dLife, l(1)-l(0))
			life0 = append(life0, l(0))
			dRate = append(dRate, r(1)-r(0))
			dKids = append(dKids, k(1)-k(0))
			rate0 = append(rate0, r(0))
			kids0 = append(kids0, k(0))
		}
		c := func(xs []float64, prec int) string {
			mm, se := meanSE(xs)
			mark := ""
			if abs := mm; abs < 0 {
				abs = -abs
				if abs > 2*se {
					mark = " *"
				}
			} else if mm > 2*se {
				mark = " *"
			}
			return fmt.Sprintf("%+.*f ± %.*f%s", prec, mm, prec, se, mark)
		}
		m0, _ := meanSE(rate0)
		k0, _ := meanSE(kids0)
		l0, _ := meanSE(life0)
		fmt.Printf("| %s | %s | %s | %s | %.2f・%.3f・%.2f |\n", inv.name, c(dLife, 3), c(dRate, 4), c(dKids, 3), l0, m0, k0)
	}
	fmt.Printf("\n`*` は差が標準誤差の2倍を超えるもの。生涯出生数は tick 5000 より後に生まれて終わりまでに死んだ身体の子の数の平均（成人前に死んだ子は 0 として入る。終わりに生きている身体は入らない）。\n\n")
}
