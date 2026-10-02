package main

import (
	"fmt"
	"math"
	"sort"

	"github.com/toku463ne/cld_generic_dangeon/engine"
	"github.com/toku463ne/cld_generic_dangeon/variant"
)

// mateRoomBins split the samples by how many bodies the world holds.
var mateRoomBins = [...]int{0, 110, 140, 1 << 30}

type mateRoomTally struct {
	// Samples (every 10 ticks after tick 5000) of bodies that could mate,
	// by population bin: how many, with a mate in sight, with only a
	// request in range, and with neither.
	could, inSight, request, none [2][3]float64
	// Of the samples with neither: a mate was seen within 200 / 1000 ticks
	// before (its last place remembered), and of those within 1000 ticks,
	// the mate is still within Sight+RequestRange of where it was seen.
	saw200, saw1000, stillNear [2][3]float64
	// Distances, in tiles, from the body to where it last saw a mate,
	// for samples with neither and a sighting within 1000 ticks.
	dist []float64
}

func mateRoomBin(n int) int {
	for i := len(mateRoomBins) - 2; i >= 0; i-- {
		if n >= mateRoomBins[i] {
			return i
		}
	}
	return 0
}

// runMateRoom samples every 10 ticks the bodies that could mate, what mate
// they have, and the last mate they saw.
func runMateRoom(cfg engine.Config, m engine.Map, ticks int) (mateRoomTally, error) {
	var t mateRoomTally
	w, err := engine.NewWorld(cfg, m)
	if err != nil {
		return t, err
	}
	type sighting struct {
		tick int64
		id   int64
		x, y float64
	}
	last := map[int64]sighting{}
	s := float64(cfg.Sight)
	cheb := func(ax, ay, bx, by float64) float64 {
		return math.Max(math.Abs(math.Floor(ax)-math.Floor(bx)), math.Abs(math.Floor(ay)-math.Floor(by)))
	}
	for tick := 1; tick <= ticks; tick++ {
		w.Step()
		now := w.Tick()
		bs := w.Bodies()
		byID := make(map[int64]engine.Body, len(bs))
		for _, b := range bs {
			byID[b.ID] = b
		}
		// Who could be a mate: fertile and not resting.
		var cand []engine.Body
		for _, b := range bs {
			if b.Sex != engine.NoSex && w.Fertile(b) && !w.Resting(b) {
				cand = append(cand, b)
			}
		}
		sample := tick > 5000 && tick%10 == 0
		bin := mateRoomBin(len(bs))
		for _, b := range bs {
			if b.Sex == engine.NoSex || !w.Fertile(b) {
				continue
			}
			seen, req := false, false
			for _, o := range cand {
				if o.Sex == b.Sex {
					continue
				}
				d := cheb(b.X, b.Y, o.X, o.Y)
				if d <= s {
					seen = true
					last[b.ID] = sighting{now, o.ID, o.X, o.Y}
				} else if o.Requested > now && d <= float64(cfg.RequestRange) {
					req = true
				}
			}
			if !sample || !w.CanMate(b) {
				continue
			}
			x := int(b.Sex) - 1
			t.could[x][bin]++
			switch {
			case seen:
				t.inSight[x][bin]++
			case req:
				t.request[x][bin]++
			default:
				t.none[x][bin]++
				sg, ok := last[b.ID]
				if !ok {
					break
				}
				gap := now - sg.tick
				if gap <= 200 {
					t.saw200[x][bin]++
				}
				if gap <= 1000 {
					t.saw1000[x][bin]++
					t.dist = append(t.dist, cheb(b.X, b.Y, sg.x, sg.y))
					if o, alive := byID[sg.id]; alive && cheb(o.X, o.Y, sg.x, sg.y) <= s+float64(cfg.RequestRange) {
						t.stillNear[x][bin]++
					}
				}
			}
		}
		if tick%1000 == 0 {
			for id := range last {
				if _, ok := byID[id]; !ok {
					delete(last, id)
				}
			}
		}
	}
	return t, nil
}

// mateRoom counts, before stage 5-4 lets bodies remember where they saw a
// mate, how often a body that could mate has no mate in sight and no
// request in range, and whether it saw one lately and that one is still
// near where it was seen - by how many bodies the world holds.
func mateRoom(m engine.Map, name string, seeds int, seed0 int64, ticks int, vname string) {
	var ts []mateRoomTally
	var dists []float64
	for s := 0; s < seeds; s++ {
		cfg, err := variant.Config(vname, seed0+int64(s))
		if err != nil {
			fail(err)
		}
		t, err := runMateRoom(cfg, m, ticks)
		if err != nil {
			fail(err)
		}
		ts = append(ts, t)
		dists = append(dists, t.dist...)
	}
	fmt.Printf("地図 %s (%dx%d)、%d シード、条件 %s、%d tick（tick 5000 より後、10 tick ごと）。交配できる身体＝成人で老人でなく、休んでおらず、自分の分を払える。相手＝交配できる（成人・老人でない・休んでいない）異性。\n\n", name, m.Width, m.Height, seeds, vname, ticks)
	ratio := func(a, b float64) float64 {
		if b == 0 {
			return math.NaN()
		}
		return a / b
	}
	cell := func(f func(mateRoomTally) float64, prec int) string {
		var xs []float64
		for _, t := range ts {
			if x := f(t); !math.IsNaN(x) {
				xs = append(xs, x)
			}
		}
		if len(xs) == 0 {
			return "—"
		}
		mm, se := meanSE(xs)
		return fmt.Sprintf("%.*f ± %.*f", prec, mm, prec, se)
	}
	labels := []string{"< 110", "110〜139", "≥ 140"}
	for x, sexName := range []string{"女", "男"} {
		x := x
		fmt.Printf("%s\n\n", sexName)
		fmt.Printf("| 個体数 | 標本（1シードあたり） | 相手が視界に | 要求だけが範囲に | どちらも無い | 　無いうち 200 tick 以内に相手を見た | 　無いうち 1000 tick 以内に見た | 　その相手がまだ見た場所の近く（視界＋要求の範囲） |\n| --- | --- | --- | --- | --- | --- | --- | --- |\n")
		for k, l := range labels {
			k := k
			fmt.Printf("| %s | %s | %s | %s | %s | %s | %s | %s |\n", l,
				cell(func(t mateRoomTally) float64 { return t.could[x][k] }, 0),
				cell(func(t mateRoomTally) float64 { return ratio(t.inSight[x][k], t.could[x][k]) }, 4),
				cell(func(t mateRoomTally) float64 { return ratio(t.request[x][k], t.could[x][k]) }, 4),
				cell(func(t mateRoomTally) float64 { return ratio(t.none[x][k], t.could[x][k]) }, 4),
				cell(func(t mateRoomTally) float64 { return ratio(t.saw200[x][k], t.none[x][k]) }, 4),
				cell(func(t mateRoomTally) float64 { return ratio(t.saw1000[x][k], t.none[x][k]) }, 4),
				cell(func(t mateRoomTally) float64 { return ratio(t.stillNear[x][k], t.saw1000[x][k]) }, 4))
		}
		fmt.Println()
	}
	sort.Float64s(dists)
	q := func(p float64) float64 {
		if len(dists) == 0 {
			return math.NaN()
		}
		return dists[int(p*float64(len(dists)-1))]
	}
	fmt.Printf("\n相手を最後に見た場所までの距離（どちらも無く 1000 tick 以内に見た標本、全シード、タイル）: 中央値 %.0f / 75%% %.0f / 95%% %.0f\n\n", q(0.5), q(0.75), q(0.95))
}
