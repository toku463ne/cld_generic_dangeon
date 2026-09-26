package main

import (
	"fmt"

	"github.com/toku463ne/cld_generic_dangeon/engine"
	"github.com/toku463ne/cld_generic_dangeon/variant"
)

// pseudoSex gives a body a sex from its ID alone, drawing nothing from the
// world's random source, so that a world without sexes can be read as if it
// had them.
func pseudoSex(id int64) int {
	x := uint64(id) + 0x9e3779b97f4a7c15 // the SplitMix64 finalizer
	x = (x ^ (x >> 30)) * 0xbf58476d1ce4e5b9
	x = (x ^ (x >> 27)) * 0x94d049bb133111eb
	return int((x ^ (x >> 31)) >> 63)
}

type sexTally struct {
	// Decisions after tick 5000 with a mate among the options, by how many
	// adults they named: 1, 2, 3 or more; and those whose mates would all
	// be of the body's own pseudo-sex.
	mateDecisions float64
	byCount       [3]float64
	noneOpposite  float64
	// Births after tick 5000, and those whose parents share a pseudo-sex.
	births, sameSex float64
}

// runSex reads, in a world without sexes, what sexes would take away: the
// mates each mating decision could name, and the births, of bodies given a
// pseudo-sex from their IDs.
func runSex(cfg engine.Config, m engine.Map, ticks int) (sexTally, error) {
	var t sexTally
	w, err := engine.NewWorld(cfg, m)
	if err != nil {
		return t, err
	}
	w.SetTrace(func(b engine.Body, v engine.Valuation, a engine.Action) {
		if w.Tick() <= 5000 {
			return
		}
		n, opposite := 0, 0
		for _, o := range v.Options {
			if o.Kind != engine.ActMate {
				continue
			}
			n++
			if pseudoSex(o.Mate) != pseudoSex(b.ID) {
				opposite++
			}
		}
		if n == 0 {
			return
		}
		t.mateDecisions++
		t.byCount[min(n, 3)-1]++
		if opposite == 0 {
			t.noneOpposite++
		}
	})
	seen := map[int64]bool{}
	for _, b := range w.Bodies() {
		seen[b.ID] = true
	}
	for tick := 1; tick <= ticks; tick++ {
		w.Step()
		for _, b := range w.Bodies() {
			if seen[b.ID] {
				continue
			}
			seen[b.ID] = true
			if tick <= 5000 {
				continue
			}
			t.births++
			if pseudoSex(b.Parents[0]) == pseudoSex(b.Parents[1]) {
				t.sameSex++
			}
		}
	}
	return t, nil
}

// sex counts, before stage 3-1, what mating only with the other sex would
// take away in today's world.
func sex(m engine.Map, name string, seeds int, seed0 int64, ticks int, vname string) {
	fmt.Printf("地図 %s (%dx%d)、%d シード、条件 %s、%d tick（tick 5000 より後）。身体に ID から決めた仮の性別を割り当てて読む（乱数は引かない）。\n\n", name, m.Width, m.Height, seeds, vname, ticks)
	var ts []sexTally
	for s := 0; s < seeds; s++ {
		cfg, err := variant.Config(vname, seed0+int64(s))
		if err != nil {
			fail(err)
		}
		t, err := runSex(cfg, m, ticks)
		if err != nil {
			fail(err)
		}
		ts = append(ts, t)
	}
	cell := func(f func(sexTally) float64, prec int) string {
		var xs []float64
		for _, t := range ts {
			xs = append(xs, f(t))
		}
		mm, se := meanSE(xs)
		return fmt.Sprintf("%.*f ± %.*f", prec, mm, prec, se)
	}
	fmt.Printf("| 量 | 値 |\n| --- | --- |\n")
	fmt.Printf("| 交配の手が候補にある決定（1シードあたり） | %s |\n", cell(func(t sexTally) float64 { return t.mateDecisions }, 0))
	for k, label := range []string{"1体", "2体", "3体以上"} {
		fmt.Printf("| そのうち名指しできる成人が%s | %s |\n", label, cell(func(t sexTally) float64 { return t.byCount[k] / t.mateDecisions }, 4))
	}
	fmt.Printf("| そのうち異性の候補が 0 になる | %s |\n", cell(func(t sexTally) float64 { return t.noneOpposite / t.mateDecisions }, 4))
	fmt.Printf("| 出生（1シードあたり） | %s |\n", cell(func(t sexTally) float64 { return t.births }, 0))
	fmt.Printf("| そのうち両親が同じ仮の性別 | %s |\n\n", cell(func(t sexTally) float64 { return t.sameSex / t.births }, 4))
}
