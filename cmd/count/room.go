package main

import (
	"fmt"
	"math"

	"github.com/toku463ne/cld_generic_dangeon/engine"
	"github.com/toku463ne/cld_generic_dangeon/variant"
)

// The path row as the room count reads it: the tiles a body left within
// roomRecent ticks, looked for along each heading for roomAhead tiles.
const (
	roomRecent = 200
	roomAhead  = 8
)

// roomChild is the chance a mate offer becomes a child, as the learn count
// measured it (0.11 to 0.12).
const roomChild = 0.115

type roomTally struct {
	decisions, read float64
	// Decisions whose best options change: by the path row, by the mate
	// row, by either.
	path, mate, either float64
	// Of the decisions the path row changes, how many had the body heading
	// back the way it came (the reverse of its last move) - and how many
	// decisions it took the reverse at all.
	pathBack, back float64
	// Mate choices, and those the mate row would turn down.
	mated, unmated float64
	// Decisions where every best option's risk had rounded to zero (no row
	// of this kind can reorder them).
	zero float64
}

// runRoom reads, in the base world after tick 5000, how many decisions the
// two rows stage 1-6 is to learn could reorder, keeping each body's path
// beside the world without feeding it back.
func runRoom(cfg engine.Config, m engine.Map, ticks int) (roomTally, error) {
	var t roomTally
	w, err := engine.NewWorld(cfg, m)
	if err != nil {
		return t, err
	}
	walked := map[int64]map[int]int64{}
	tileOf := map[int64]int{}
	cache := map[float64]engine.Survival{}
	w.SetTrace(func(b engine.Body, v engine.Valuation, a engine.Action) {
		if w.Tick() <= 5000 {
			return
		}
		t.decisions++
		tt := w.TruthTable()
		surv := make([]engine.Survival, len(tt.Meet))
		for r, q := range tt.Meet {
			s, ok := cache[q]
			if !ok {
				s = tt.NewSurvival(q, []int{cfg.Window})
				cache[q] = s
			}
			surv[r] = s
		}
		cv := w.Value(tt, surv, b)
		score := make([]float64, len(cv.Options))
		lo := math.Inf(1)
		for j := range cv.Options {
			score[j] = cv.Risk[0][j] - cfg.ChildWorth*cv.Child[j]
			lo = math.Min(lo, score[j])
		}
		taken := -1
		for j, o := range cv.Options {
			if o == a && score[j] == lo {
				taken = j
			}
		}
		if taken < 0 {
			return // the body's own build chose otherwise; not read
		}
		t.read++
		if a.Kind == engine.ActMove && b.Heading >= 0 && a.Dir == (b.Heading+4)%8 {
			t.back++
		}
		// Path row: among the best, keep-moving moves heading into tiles
		// walked lately lose, if the risk has not rounded to zero.
		path := false
		if lo > 0 {
			ahead := func(d int) float64 {
				dx := []int{1, 1, 0, -1, -1, -1, 0, 1}[d]
				dy := []int{0, 1, 1, 1, 0, -1, -1, -1}[d]
				x, y := int(b.X), int(b.Y)
				n := 0.0
				for k := 1; k <= roomAhead; k++ {
					x, y = x+dx, y+dy
					if !m.InBounds(x, y) {
						break
					}
					if when, ok := walked[b.ID][y*m.Width+x]; ok && w.Tick()-when <= roomRecent {
						n++
					}
				}
				return n
			}
			f := make([]float64, len(cv.Options))
			least := math.Inf(1)
			for j, o := range cv.Options {
				if score[j] != lo {
					continue
				}
				if o.Kind == engine.ActMove && cv.Plan[j] < 0 {
					f[j] = ahead(o.Dir)
				}
				least = math.Min(least, f[j])
			}
			path = f[taken] > least
		} else {
			t.zero++
		}
		// Mate row: a child with chance roomChild, not 1.
		mate := false
		if a.Kind == engine.ActMate {
			t.mated++
			other := math.Inf(1)
			for j, o := range cv.Options {
				if o.Kind != engine.ActMate {
					other = math.Min(other, cv.Risk[0][j])
				}
			}
			if cv.Risk[0][taken]-cfg.ChildWorth*roomChild > other {
				mate = true
				t.unmated++
			}
		}
		if path {
			t.path++
			if a.Kind == engine.ActMove && b.Heading >= 0 && a.Dir == (b.Heading+4)%8 {
				t.pathBack++
			}
		}
		if mate {
			t.mate++
		}
		if path || mate {
			t.either++
		}
	})
	for tick := 1; tick <= ticks; tick++ {
		w.Step()
		now := w.Tick()
		alive := map[int64]bool{}
		for _, b := range w.Bodies() {
			alive[b.ID] = true
			tile := int(b.Y)*m.Width + int(b.X)
			if old, ok := tileOf[b.ID]; ok && old != tile {
				if walked[b.ID] == nil {
					walked[b.ID] = map[int]int64{}
				}
				walked[b.ID][old] = now
			}
			tileOf[b.ID] = tile
		}
		for id := range tileOf {
			if !alive[id] {
				delete(tileOf, id)
				delete(walked, id)
			}
		}
		if len(w.Bodies()) == 0 {
			break
		}
	}
	return t, nil
}

// room counts, for stage 1-6, how many decisions the rows it is to learn
// could reorder: the tiles walked lately, read per heading, and the
// chance a mate offer becomes a child.
func room(m engine.Map, name string, seeds int, seed0 int64, ticks int) {
	fmt.Printf("地図 %s (%dx%d)、%d シード、%d tick、base（跳ね返しなし）。決定は tick 5000 より後。通ったタイルは %d tick 以内に離れたタイル、向きの先 %d タイルを見る。子になる割合は %.3f。\n\n", name, m.Width, m.Height, seeds, ticks, roomRecent, roomAhead, roomChild)
	var ts []roomTally
	for s := 0; s < seeds; s++ {
		cfg, err := variant.Config(variant.Base, seed0+int64(s))
		if err != nil {
			fail(err)
		}
		t, err := runRoom(cfg, m, ticks)
		if err != nil {
			fail(err)
		}
		ts = append(ts, t)
	}
	cell := func(f func(roomTally) float64) string {
		var xs []float64
		for _, t := range ts {
			xs = append(xs, f(t))
		}
		mm, se := meanSE(xs)
		return fmt.Sprintf("%.4f ± %.4f", mm, se)
	}
	fmt.Printf("| 量 | 値 |\n| --- | --- |\n")
	fmt.Printf("| 読めた決定の割合（設定の能力で読んだ最善に、身体の手が入っていた） | %s |\n", cell(func(t roomTally) float64 { return t.read / t.decisions }))
	fmt.Printf("| 最善の手の死ぬ確率が 0 に丸まっていた決定 | %s |\n", cell(func(t roomTally) float64 { return t.zero / t.read }))
	fmt.Printf("| 通ったタイルの行で最善から外れる決定 | %s |\n", cell(func(t roomTally) float64 { return t.path / t.read }))
	fmt.Printf("| 　そのうち真後ろ（直前の移動の逆）へ動いていた割合 | %s |\n", cell(func(t roomTally) float64 { return t.pathBack / t.path }))
	fmt.Printf("| 真後ろへ動いた決定（全体に対して） | %s |\n", cell(func(t roomTally) float64 { return t.back / t.read }))
	fmt.Printf("| 交配の行で最善から外れる決定 | %s |\n", cell(func(t roomTally) float64 { return t.mate / t.read }))
	fmt.Printf("| 　交配を選んだ決定のうち、見送りに変わる割合 | %s |\n", cell(func(t roomTally) float64 { return t.unmated / t.mated }))
	fmt.Printf("| どちらかで最善から外れる決定 | %s |\n\n", cell(func(t roomTally) float64 { return t.either / t.read }))
}
