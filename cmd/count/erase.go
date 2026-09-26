package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/rand"

	"github.com/toku463ne/cld_generic_dangeon/engine"
	"github.com/toku463ne/cld_generic_dangeon/variant"
)

// eraseBands are the age bands, in ticks at the erasure, the fates are read
// in.
var eraseBands = [...][2]int64{{0, 250}, {250, 1000}, {1000, 1 << 40}}

// eraseGroup sums the fates of one group of bodies: how many there were,
// how many died within the window in the world left alone (kept) and in
// the world where half had their path evidence erased (erased), and their
// path evidence before the erasure.
type eraseGroup struct{ n, diedKept, diedErased, evidence float64 }

type eraseTally struct {
	// erased and spared are the bodies whose evidence was erased and the
	// rest, by age band; the last entry is every band.
	erased, spared [len(eraseBands) + 1]eraseGroup
}

// withPathErased rebuilds w, through a save and a load, with the path
// evidence of the bodies in ids gone: back to the prior, as if the body had
// never seen or heard of a tile it walked. It is not a rule of the world:
// it is the counterfactual the count compares with.
func withPathErased(w *engine.World, ids map[int64]bool) (*engine.World, error) {
	var buf bytes.Buffer
	if err := w.Save(&buf); err != nil {
		return nil, err
	}
	var snap map[string]any
	if err := json.Unmarshal(buf.Bytes(), &snap); err != nil {
		return nil, err
	}
	for _, b := range snap["bodies"].([]any) {
		body := b.(map[string]any)
		if ids[int64(body["ID"].(float64))] {
			body["Memory"].(map[string]any)["Path"] = map[string]any{}
		}
	}
	out, err := json.Marshal(snap)
	if err != nil {
		return nil, err
	}
	return engine.Load(bytes.NewReader(out))
}

// runErase runs the world to tick at, draws half the living (from a source
// of its own, so the world's draws are untouched), and runs on for window
// ticks twice from there: as it was, and with that half's path evidence
// erased. The same bodies are compared across the two worlds.
func runErase(cfg engine.Config, m engine.Map, at, window int) (eraseTally, error) {
	var t eraseTally
	w, err := engine.NewWorld(cfg, m)
	if err != nil {
		return t, err
	}
	for i := 0; i < at; i++ {
		w.Step()
	}
	living := w.Bodies()
	r := rand.New(rand.NewSource(cfg.Seed))
	ids := map[int64]bool{}
	for _, i := range r.Perm(len(living))[:len(living)/2] {
		ids[living[i].ID] = true
	}
	kept, err := withPathErased(w, nil)
	if err != nil {
		return t, err
	}
	erased, err := withPathErased(w, ids)
	if err != nil {
		return t, err
	}
	for i := 0; i < window; i++ {
		kept.Step()
		erased.Step()
	}
	alive := func(w *engine.World) map[int64]bool {
		a := map[int64]bool{}
		for _, b := range w.Bodies() {
			a[b.ID] = true
		}
		return a
	}
	ak, ae := alive(kept), alive(erased)
	now := w.Tick()
	for _, b := range living {
		g := &t.spared
		if ids[b.ID] {
			g = &t.erased
		}
		for k, band := range eraseBands {
			if age := now - b.Born; age < band[0] || age >= band[1] {
				continue
			}
			for _, e := range []*eraseGroup{&g[k], &g[len(eraseBands)]} {
				e.n++
				e.evidence += b.Memory.Path.N
				if !ak[b.ID] {
					e.diedKept++
				}
				if !ae[b.ID] {
					e.diedErased++
				}
			}
		}
	}
	return t, nil
}

// erase counts, for stage 2-2, whether thick path evidence helps a body
// live: the share of bodies that die within one full-to-starving span with
// their path evidence and without it, the same bodies in both.
func erase(m engine.Map, name string, seeds int, seed0 int64, ticks int, vname string) {
	cfg0, err := variant.Config(vname, seed0)
	if err != nil {
		fail(err)
	}
	window := energyTicks(cfg0)
	fmt.Printf("地図 %s (%dx%d)、%d シード、条件 %s。tick %d で生きている身体の半数（無作為）の通ったタイルの証拠を消し（事前の ×1 に戻す）、%d tick（満腹から餓死まで）のうちに死んだ割合を、消さなかった世界の同じ身体と対にして比べる。\n\n", name, m.Width, m.Height, seeds, vname, ticks, window)
	var ts []eraseTally
	for s := 0; s < seeds; s++ {
		cfg, err := variant.Config(vname, seed0+int64(s))
		if err != nil {
			fail(err)
		}
		t, err := runErase(cfg, m, ticks, window)
		if err != nil {
			fail(err)
		}
		ts = append(ts, t)
	}
	cell := func(f func(eraseTally) (float64, bool), prec int) string {
		var xs []float64
		for _, t := range ts {
			if x, ok := f(t); ok {
				xs = append(xs, x)
			}
		}
		if len(xs) == 0 {
			return "—"
		}
		mm, se := meanSE(xs)
		return fmt.Sprintf("%.*f ± %.*f", prec, mm, prec, se)
	}
	fmt.Printf("| 群 | 年齢（tick %d の時点） | 身体の数 | 消す前の証拠（タイル） | 死んだ割合（消さない世界） | 死んだ割合（消した世界） | 差（消した − 消さない） |\n| --- | --- | --- | --- | --- | --- | --- |\n", ticks)
	for _, grp := range []struct {
		name string
		get  func(eraseTally) *[len(eraseBands) + 1]eraseGroup
	}{
		{"消した半数", func(t eraseTally) *[len(eraseBands) + 1]eraseGroup { return &t.erased }},
		{"消さなかった半数", func(t eraseTally) *[len(eraseBands) + 1]eraseGroup { return &t.spared }},
	} {
		for k := 0; k <= len(eraseBands); k++ {
			label := "全て"
			if k < len(eraseBands) {
				label = fmt.Sprintf("%d〜", eraseBands[k][0])
				if eraseBands[k][1] < 1<<30 {
					label += fmt.Sprint(eraseBands[k][1])
				}
			}
			g := func(t eraseTally) eraseGroup { return grp.get(t)[k] }
			has := func(t eraseTally) bool { return g(t).n > 0 }
			fmt.Printf("| %s | %s | %s | %s | %s | %s | %s |\n", grp.name, label,
				cell(func(t eraseTally) (float64, bool) { return g(t).n, true }, 1),
				cell(func(t eraseTally) (float64, bool) { return g(t).evidence / g(t).n, has(t) }, 1),
				cell(func(t eraseTally) (float64, bool) { return g(t).diedKept / g(t).n, has(t) }, 4),
				cell(func(t eraseTally) (float64, bool) { return g(t).diedErased / g(t).n, has(t) }, 4),
				cell(func(t eraseTally) (float64, bool) { return (g(t).diedErased - g(t).diedKept) / g(t).n, has(t) }, 4))
		}
	}
	fmt.Println()
}
