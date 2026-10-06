package main

import (
	"fmt"
	"math"
	"sort"

	"github.com/toku463ne/cld_generic_dangeon/engine"
	"github.com/toku463ne/cld_generic_dangeon/variant"
)

// jitter counts bodies that quiver on the spot (seen in the client after
// stage 9-4): awake, moving on most of the last jitterWindow ticks, yet
// staying within jitterBox tiles. For the decisions made while quivering
// it tallies what made the body decide, what it took, what plan the
// choice was read from, and what it saw; and prints the first quivering
// body's decisions as an example.
const (
	jitterWindow = 60
	jitterMoved  = 45
	jitterBox    = 2.0
	jitterAfter  = 5000
)

type jitterTally struct {
	awake, quivering float64 // body-ticks after jitterAfter
	bodies           map[int64]bool
	episodes         float64 // quivering spells begun
	// Decisions while quivering, by trigger, action kind, plan kind and
	// a few conditions.
	decisions                                 float64
	why                                       [engine.NumTriggers]float64
	kind                                      [engine.NumActionKinds]float64
	planSeen, planRecalled, planShelter, keep float64
	reversed, underfoot, sawFood, tied        float64
	resting, child, hungry                    float64
	// Energy over the most at the decision (bands of 0.1), whether the
	// body holds a unit, stands within a tile of the map's edge, and
	// the distance to the unit its plan walks to.
	band       [11]float64
	held, edge float64
	planDist   [4]float64 // 0, 1, 2, 3+ tiles
	// Tied decisions settled by keeping the heading.
	keptHeading float64
	// Decisions the body did not quiver at, for comparison: the same
	// shares.
	plain, plainSeen, plainKeep, plainTied float64
	example                                []string
	exampleID                              int64
	exampleFrom                            int64
}

type jitterRing struct {
	xs, ys [jitterWindow]float64
	n, at  int
	moved  int // moves within the ring
	on     bool
	last   int
}

func jitter(m engine.Map, mapName string, seeds int, seed0 int64, ticks int, vname string) {
	fmt.Printf("# jitter: bodies quivering on the spot (%s, %s, %d seeds, %d ticks)\n\n", mapName, vname, seeds, ticks)
	fmt.Printf("A body quivers when awake, moved on %d or more of the last %d ticks, and stayed within %.0f tiles both ways; counted after tick %d.\n\n", jitterMoved, jitterWindow, jitterBox, jitterAfter)
	var all []jitterTally
	for s := 0; s < seeds; s++ {
		cfg, err := variant.Config(vname, seed0+int64(s))
		if err != nil {
			fail(err)
		}
		t, err := runJitter(cfg, m, ticks)
		if err != nil {
			fail(err)
		}
		all = append(all, t)
	}
	var sum jitterTally
	bodies := 0.0
	for _, t := range all {
		sum.awake += t.awake
		sum.quivering += t.quivering
		sum.episodes += t.episodes
		bodies += float64(len(t.bodies))
		sum.decisions += t.decisions
		for k := range t.why {
			sum.why[k] += t.why[k]
		}
		for k := range t.kind {
			sum.kind[k] += t.kind[k]
		}
		sum.planSeen += t.planSeen
		sum.planRecalled += t.planRecalled
		sum.planShelter += t.planShelter
		sum.keep += t.keep
		sum.reversed += t.reversed
		sum.underfoot += t.underfoot
		sum.sawFood += t.sawFood
		sum.tied += t.tied
		sum.resting += t.resting
		sum.child += t.child
		sum.hungry += t.hungry
		sum.plain += t.plain
		sum.plainSeen += t.plainSeen
		sum.plainKeep += t.plainKeep
		sum.plainTied += t.plainTied
		sum.held += t.held
		sum.edge += t.edge
		sum.keptHeading += t.keptHeading
		for k := range t.band {
			sum.band[k] += t.band[k]
		}
		for k := range t.planDist {
			sum.planDist[k] += t.planDist[k]
		}
	}
	n := float64(seeds)
	fmt.Printf("| 指標 | 値（1シードあたり） |\n| --- | --- |\n")
	fmt.Printf("| 起きている身体 tick | %.0f |\n", sum.awake/n)
	fmt.Printf("| 振動している身体 tick | %.0f（%.2f%%） |\n", sum.quivering/n, 100*sum.quivering/math.Max(sum.awake, 1))
	fmt.Printf("| 振動した身体の数・振動の回数 | %.1f・%.1f（1回あたり %.0f tick） |\n", bodies/n, sum.episodes/n, sum.quivering/math.Max(sum.episodes, 1))
	fmt.Printf("| 振動中の決定 | %.0f（%.1f tick に 1 回） |\n\n", sum.decisions/n, sum.quivering/math.Max(sum.decisions, 1))
	d := math.Max(sum.decisions, 1)
	fmt.Printf("振動中の決定のきっかけ:\n\n| きっかけ | 割合 |\n| --- | --- |\n")
	for k, name := range engine.TriggerNames {
		if sum.why[k] > 0 {
			fmt.Printf("| %s | %.3f |\n", name, sum.why[k]/d)
		}
	}
	fmt.Printf("\n振動中の決定の中身（割合。括弧は振動していない決定）:\n\n| 項目 | 振動中 | それ以外 |\n| --- | --- | --- |\n")
	p := math.Max(sum.plain, 1)
	kinds := []string{"wait", "eat", "move", "mate", "pick", "sleep", "build"}
	for k, name := range kinds {
		if k < len(sum.kind) {
			fmt.Printf("| 手: %s | %.3f | — |\n", name, sum.kind[k]/d)
		}
	}
	fmt.Printf("| 移動が直前の向きの逆（差 3 以上） | %.3f | — |\n", sum.reversed/d)
	fmt.Printf("| 計画: 見えている食料へ歩く | %.3f | %.3f |\n", sum.planSeen/d, sum.plainSeen/p)
	fmt.Printf("| 計画: 覚えている食料へ歩く | %.3f | — |\n", sum.planRecalled/d)
	fmt.Printf("| 計画: 休める場所へ歩く | %.3f | — |\n", sum.planShelter/d)
	fmt.Printf("| 計画: 移動を続ける | %.3f | %.3f |\n", sum.keep/d, sum.plainKeep/p)
	fmt.Printf("| 足元に食料 | %.3f | — |\n", sum.underfoot/d)
	fmt.Printf("| 視界に食料 | %.3f | — |\n", sum.sawFood/d)
	fmt.Printf("| 最善が同点 | %.3f | %.3f |\n", sum.tied/d, sum.plainTied/p)
	fmt.Printf("| 休んでいる母 | %.3f | — |\n", sum.resting/d)
	fmt.Printf("| 交配の手が候補にある | %.3f | — |\n", sum.child/d)
	fmt.Printf("| 体力 < 0.5 | %.3f | — |\n", sum.hungry/d)
	fmt.Printf("| 持っている | %.3f | — |\n", sum.held/d)
	fmt.Printf("| 地図の縁から 1 タイル以内 | %.3f | — |\n", sum.edge/d)
	fmt.Printf("| 同点を向きの維持で決めた | %.3f | — |\n", sum.keptHeading/d)
	fmt.Printf("| 計画の食料までの距離 0 / 1 / 2 / 3+ | %.3f / %.3f / %.3f / %.3f | — |\n", sum.planDist[0]/d, sum.planDist[1]/d, sum.planDist[2]/d, sum.planDist[3]/d)
	fmt.Printf("| 体力の帯（0.0〜1.0、0.1 刻み） | ")
	for k := range sum.band {
		fmt.Printf("%.2f ", sum.band[k]/d)
	}
	fmt.Printf("| — |\n\n")
	for _, t := range all {
		if len(t.example) > 0 {
			fmt.Printf("例: 身体 %d の決定、振動が始まってから 150 tick（* は振動中。tick, x, y, 体力/上限, 持ち物, きっかけ, 手, 計画, 計画の食料, 最善の点数, 次点との差, 同点の幅, 視界の食料）:\n\n```\n", t.exampleID)
			for _, l := range t.example {
				fmt.Println(l)
			}
			fmt.Printf("```\n\n")
			break
		}
	}
}

func runJitter(cfg engine.Config, m engine.Map, ticks int) (jitterTally, error) {
	t := jitterTally{bodies: map[int64]bool{}}
	w, err := engine.NewWorld(cfg, m)
	if err != nil {
		return t, err
	}
	rings := map[int64]*jitterRing{}
	quiver := map[int64]bool{}
	w.SetTrace(func(b engine.Body, v engine.Valuation, a engine.Action) {
		if w.Tick() <= jitterAfter || len(v.Score) == 0 {
			return
		}
		best, next, bj := math.Inf(1), math.Inf(1), -1
		for j, s := range v.Score {
			if s < best {
				best, bj = s, j
			}
		}
		width := 0
		for _, s := range v.Score {
			if s == best {
				width++
			} else if s < next {
				next = s
			}
		}
		plan := -1
		if bj >= 0 && bj < len(v.Plan) {
			plan = v.Plan[bj]
		}
		seen, recalled := len(v.Seen), len(v.Recalled)
		planName := "keep"
		switch {
		case plan < 0:
		case plan < seen:
			planName = "seen"
		case plan < seen+recalled:
			planName = "recalled"
		default:
			planName = "shelter"
		}
		if b.ID == t.exampleID && w.Tick() < t.exampleFrom+150 {
			gap := math.NaN()
			if !math.IsInf(next, 1) {
				gap = next - best
			}
			act := fmt.Sprintf("%v", a.Kind)
			if a.Kind == engine.ActMove {
				act = fmt.Sprintf("move%d", a.Dir)
			}
			target := ""
			if plan >= 0 {
				u := v.Seen
				i := plan
				switch planName {
				case "recalled":
					u, i = v.Recalled, plan-seen
				case "shelter":
					u, i = v.Shelter, plan-seen-recalled
				}
				if i < len(u) {
					target = fmt.Sprintf("(%d,%d)", u[i].X, u[i].Y)
				}
			}
			q := " "
			if quiver[b.ID] {
				q = "*"
			}
			t.example = append(t.example, fmt.Sprintf("%6d%s %6.2f %6.2f e%.2f h%d %-9s %-7s %-8s %-8s %.6f %.1e tie%d seen%d", w.Tick(), q, b.X, b.Y, b.Energy/w.MaxEnergy(b), b.Held, engine.TriggerNames[v.Why], act, planName, target, best, gap, width, seen))
		}
		if !quiver[b.ID] {
			t.plain++
			if planName == "seen" {
				t.plainSeen++
			}
			if planName == "keep" {
				t.plainKeep++
			}
			if width > 1 {
				t.plainTied++
			}
			return
		}
		t.decisions++
		t.why[v.Why]++
		t.kind[a.Kind]++
		switch planName {
		case "seen":
			t.planSeen++
		case "recalled":
			t.planRecalled++
		case "shelter":
			t.planShelter++
		default:
			t.keep++
		}
		if a.Kind == engine.ActMove && b.Heading >= 0 {
			if d := (a.Dir - b.Heading + 8) % 8; d >= 3 && d <= 5 {
				t.reversed++
			}
		}
		tx, ty := int(math.Floor(b.X)), int(math.Floor(b.Y))
		if w.FoodOn(ty*m.Width + tx) {
			t.underfoot++
		}
		if seen > 0 {
			t.sawFood++
		}
		if width > 1 {
			t.tied++
		}
		if b.Sex == engine.Female && w.Tick() < b.Rested {
			t.resting++
		}
		for _, o := range v.Options {
			if o.Kind == engine.ActMate {
				t.child++
				break
			}
		}
		if b.Energy < 0.5*w.MaxEnergy(b) {
			t.hungry++
		}
		e := b.Energy / w.MaxEnergy(b)
		t.band[int(math.Min(math.Max(e*10, 0), 10))]++
		if b.Held > 0 {
			t.held++
		}
		if tx <= 0 || ty <= 0 || tx >= m.Width-1 || ty >= m.Height-1 {
			t.edge++
		}
		if plan >= 0 {
			u := v.Seen
			i := plan
			switch planName {
			case "recalled":
				u, i = v.Recalled, plan-seen
			case "shelter":
				u, i = v.Shelter, plan-seen-recalled
			}
			if i < len(u) {
				dd := int(math.Max(math.Abs(float64(u[i].X-tx)), math.Abs(float64(u[i].Y-ty))))
				t.planDist[min(dd, 3)]++
			}
		}
		if width > 1 && a.Kind == engine.ActMove && a.Dir == b.Heading {
			t.keptHeading++
		}
	})
	for i := 0; i < ticks; i++ {
		w.Step()
		if w.Tick() <= jitterAfter {
			continue
		}
		for _, b := range w.Bodies() {
			r := rings[b.ID]
			if r == nil {
				r = &jitterRing{}
				rings[b.ID] = r
			}
			if r.n > 0 {
				px, py := r.xs[(r.at+jitterWindow-1)%jitterWindow], r.ys[(r.at+jitterWindow-1)%jitterWindow]
				if px != b.X || py != b.Y {
					r.moved++
				}
			}
			r.xs[r.at], r.ys[r.at] = b.X, b.Y
			r.at = (r.at + 1) % jitterWindow
			if r.n < jitterWindow {
				r.n++
			} else {
				// Drop the oldest move from the count: recount is cheap.
				r.moved = 0
				for k := 1; k < jitterWindow; k++ {
					i0, i1 := (r.at+k-1)%jitterWindow, (r.at+k)%jitterWindow
					if r.xs[i0] != r.xs[i1] || r.ys[i0] != r.ys[i1] {
						r.moved++
					}
				}
			}
			if b.Asleep {
				quiver[b.ID] = false
				continue
			}
			t.awake++
			q := false
			if r.n == jitterWindow && r.moved >= jitterMoved {
				minX, maxX, minY, maxY := math.Inf(1), math.Inf(-1), math.Inf(1), math.Inf(-1)
				for k := 0; k < jitterWindow; k++ {
					minX, maxX = math.Min(minX, r.xs[k]), math.Max(maxX, r.xs[k])
					minY, maxY = math.Min(minY, r.ys[k]), math.Max(maxY, r.ys[k])
				}
				q = maxX-minX <= jitterBox && maxY-minY <= jitterBox
			}
			if q {
				t.quivering++
				t.bodies[b.ID] = true
				if !quiver[b.ID] {
					t.episodes++
					r.last = 0
				}
				r.last++
				if r.last == 30 && t.exampleID == 0 && b.Energy >= 0.5*w.MaxEnergy(b) {
					t.exampleID, t.exampleFrom = b.ID, w.Tick()
				}
			}
			quiver[b.ID] = q
		}
	}
	_ = sort.Ints
	return t, nil
}
