package main

import (
	"fmt"

	"github.com/toku463ne/cld_generic_dangeon/engine"
	"github.com/toku463ne/cld_generic_dangeon/variant"
)

// alleeBins are the population sizes the adults' ticks are read by.
var alleeBins = [...]int{0, 80, 110, 140, 170, 1 << 30}

type alleeTally struct {
	// By population bin, after tick 5000: adult ticks, those with a mate
	// in sight (an adult of the other sex, neither resting), and births.
	adultTicks, withMate, births [len(alleeBins) - 1]float64
	// Adults that died after tick 5000: how many, of age, and of those
	// who died of age the ones that never had a child, and the ticks since
	// each last had a mate in sight, summed.
	died, diedOld, oldChildless, oldSinceMate float64
}

func alleeBin(n int) int {
	for i := len(alleeBins) - 2; i >= 0; i-- {
		if n >= alleeBins[i] {
			return i
		}
	}
	return 0
}

// runAllee follows every adult: whether a mate is in sight, by how many
// bodies the world holds, and how the adults that die of age last fared.
func runAllee(cfg engine.Config, m engine.Map, ticks int) (alleeTally, error) {
	var t alleeTally
	w, err := engine.NewWorld(cfg, m)
	if err != nil {
		return t, err
	}
	lastMate := map[int64]int64{} // tick an adult last had a mate in sight
	hadChild := map[int64]bool{}
	prev := map[int64]engine.Body{}
	s := cfg.Sight
	for tick := 1; tick <= ticks; tick++ {
		w.Step()
		now := w.Tick()
		bs := w.Bodies()
		n := len(bs)
		bin := alleeBin(n)
		cur := map[int64]engine.Body{}
		// Who stands where, among the adults that can mate now.
		at := map[[2]int][]engine.Body{}
		for _, b := range bs {
			cur[b.ID] = b
			if b.Born == now && b.Parents[0] >= 0 {
				hadChild[b.Parents[0]], hadChild[b.Parents[1]] = true, true
				if tick > 5000 {
					t.births[bin]++
				}
			}
			if now >= b.Mature && !(b.Sex == engine.Female && now < b.Rested) {
				k := [2]int{int(b.X), int(b.Y)}
				at[k] = append(at[k], b)
			}
		}
		for _, b := range bs {
			if now < b.Mature {
				continue
			}
			x, y := int(b.X), int(b.Y)
			seen := false
			for dy := -s; dy <= s && !seen; dy++ {
				for dx := -s; dx <= s && !seen; dx++ {
					for _, o := range at[[2]int{x + dx, y + dy}] {
						if o.ID != b.ID && o.Sex != b.Sex {
							seen = true
							break
						}
					}
				}
			}
			if seen {
				lastMate[b.ID] = now
			}
			if tick > 5000 {
				t.adultTicks[bin]++
				if seen {
					t.withMate[bin]++
				}
			}
		}
		for id, b := range prev {
			if _, ok := cur[id]; ok {
				continue
			}
			if tick > 5000 && now >= b.Mature {
				t.died++
				if now-b.Born >= int64(cfg.Lifespan) && cfg.Lifespan > 0 {
					t.diedOld++
					if !hadChild[id] {
						t.oldChildless++
					}
					last, ok := lastMate[id]
					if !ok {
						last = b.Mature
					}
					t.oldSinceMate += float64(now - last)
				}
			}
			delete(lastMate, id)
			delete(hadChild, id)
		}
		prev = cur
	}
	return t, nil
}

// allee counts whether bodies grow rarer to each other as they thin: by
// how many bodies the world holds, how often an adult has a mate in sight
// and how many are born per adult, and how the adults that die of age
// last fared.
func allee(m engine.Map, name string, seeds int, seed0 int64, ticks int, vname string) {
	fmt.Printf("地図 %s (%dx%d)、%d シード、条件 %s、%d tick（tick 5000 より後）。個体数の帯ごとに、全シードの成人の tick を合わせて読む。相手＝視界の中の異性の成人（休んでいる母を除く）。\n\n", name, m.Width, m.Height, seeds, vname, ticks)
	var sum alleeTally
	for s := 0; s < seeds; s++ {
		cfg, err := variant.Config(vname, seed0+int64(s))
		if err != nil {
			fail(err)
		}
		t, err := runAllee(cfg, m, ticks)
		if err != nil {
			fail(err)
		}
		for i := range sum.adultTicks {
			sum.adultTicks[i] += t.adultTicks[i]
			sum.withMate[i] += t.withMate[i]
			sum.births[i] += t.births[i]
		}
		sum.died += t.died
		sum.diedOld += t.diedOld
		sum.oldChildless += t.oldChildless
		sum.oldSinceMate += t.oldSinceMate
	}
	fmt.Printf("| 個体数 | 成人の tick（全シード） | 相手が視界にいる割合 | 成人 1000 tick あたりの出生 |\n| --- | --- | --- | --- |\n")
	for i := range sum.adultTicks {
		hi := fmt.Sprint(alleeBins[i+1])
		if alleeBins[i+1] > 1<<20 {
			hi = ""
		}
		if sum.adultTicks[i] == 0 {
			fmt.Printf("| %d〜%s | 0 | — | — |\n", alleeBins[i], hi)
			continue
		}
		fmt.Printf("| %d〜%s | %.0f | %.4f | %.3f |\n", alleeBins[i], hi, sum.adultTicks[i], sum.withMate[i]/sum.adultTicks[i], 1000*sum.births[i]/sum.adultTicks[i])
	}
	if sum.diedOld > 0 {
		fmt.Printf("\n死んだ成人 %.0f 体のうち老いで死んだ割合 %.4f。老いで死んだ成人のうち、子を持たなかった割合 %.4f、最後に相手が視界にいてから死ぬまでの tick の平均 %.0f\n\n", sum.died, sum.diedOld/sum.died, sum.oldChildless/sum.diedOld, sum.oldSinceMate/sum.diedOld)
	} else {
		fmt.Printf("\n死んだ成人 %.0f 体（老いで死んだ身体は無い）\n\n", sum.died)
	}
}
