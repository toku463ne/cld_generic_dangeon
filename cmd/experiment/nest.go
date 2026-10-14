package main

import (
	"fmt"
	"math"

	"github.com/toku463ne/cld_generic_dangeon/engine"
	"github.com/toku463ne/cld_generic_dangeon/variant"
)

// The nest instrument (stage 13-3): what the nests sent out, by species
// and half of the run, against the births of the second half - a nest
// that refills sends little once its species holds its ground, one that
// keeps sending is a stream of immigrants - and the levels of the bodies
// sent out, against the save they were drawn from.

// noteNest counts the bodies the nests sent out at tick t (the first
// bodies with no parents born then) and their levels.
func noteNest(r *result, w *engine.World, species int, late bool, t int64) {
	if r.nestEarly == nil {
		r.nestEarly, r.nestLate = make([]float64, species), make([]float64, species)
		r.genoNest = make([][]map[int]float64, species)
		for k := range r.genoNest {
			r.genoNest[k] = make([]map[int]float64, len(genoSlots))
			for s := range r.genoNest[k] {
				r.genoNest[k][s] = map[int]float64{}
			}
		}
	}
	for _, b := range w.Bodies() {
		if b.Born != t || b.Parents[0] >= 0 {
			continue
		}
		if late {
			r.nestLate[b.Species]++
		} else {
			r.nestEarly[b.Species]++
		}
		g := w.GenomeOf(b)
		for s, slot := range genoSlots {
			r.genoNest[b.Species][s][slot.of(g)]++
		}
	}
}

// quotaText is a nest quota as the table states it.
func quotaText(q int) string {
	if q <= 0 {
		return "なし"
	}
	return fmt.Sprintf("1 巣 %d 体", q)
}

// nestTables prints, for each variant whose world has nests, the bodies
// sent out by species and half, the births of the second half, and each
// slot's mean level among the bodies sent out beside the save's.
func nestTables(p func(string, ...any), o options, results map[string][]result, m engine.Map) {
	for _, v := range o.variants {
		rs := results[v]
		if len(rs) == 0 || rs[0].nestEarly == nil {
			continue
		}
		p("### 補助の表: 巣（%s）\n\n", v)
		p("巣から出た身体（1シードあたり）。前半は tick 0〜%d、後半はそれより後。後半の出生は全種族。巣が補充として働くなら、後半に巣から出る数は後半の出生より十分小さい。\n\n", o.ticks/2)
		p("| 種族 | 巣から出た（前半） | 巣から出た（後半） | 後半の出生 | 巣から出た（後半）÷ 後半の出生 |\n| --- | --- | --- | --- | --- |\n")
		for k, name := range rs[0].speciesNames {
			p("| %s | %s | %s | %s | %s |\n", name,
				fmtMeanSE(collect(rs, func(r result) float64 { return r.nestEarly[k] }), 2),
				fmtMeanSE(collect(rs, func(r result) float64 { return r.nestLate[k] }), 2),
				fmtMeanSE(collect(rs, func(r result) float64 { return r.birthsLate }), 2),
				fmtMeanSE(collectWhere(rs, func(r result) bool { return r.birthsLate > 0 }, func(r result) float64 { return r.nestLate[k] / r.birthsLate }), 4))
		}
		p("\n")
		p("巣ごとに出した身体（1シードあたり、地図の順）と、終わりに空だった（上限まで出した）シードの割合。上限: %s。\n\n", quotaText(rs[0].nestQuota))
		p("| 巣 | 種族 | 出した | 空だった |\n| --- | --- | --- | --- |\n")
		for i, n := range m.Nests {
			out := func(r result) float64 {
				if i < len(r.nestOut) {
					return float64(r.nestOut[i])
				}
				return 0
			}
			p("| %d（%d,%d） | %s | %s | %s |\n", i, n.X, n.Y, n.Species, fmtMeanSE(collect(rs, out), 2),
				fmtMeanSE(collect(rs, func(r result) float64 {
					if r.nestQuota > 0 && out(r) >= float64(r.nestQuota) {
						return 1
					}
					return 0
				}), 2))
		}
		p("\n")
		cfg, _ := variant.Config(v, o.seed0)
		stocks, _ := variant.Stocks(v, cfg, m)
		if len(stocks) == 0 {
			continue
		}
		save := make([][]map[int]float64, len(rs[0].speciesNames))
		for k := range save {
			save[k] = make([]map[int]float64, len(genoSlots))
			for s := range save[k] {
				save[k][s] = map[int]float64{}
			}
		}
		for _, st := range stocks {
			if st.Genome.Species < len(save) {
				for s, slot := range genoSlots {
					save[st.Genome.Species][s][slot.of(st.Genome)]++
				}
			}
		}
		p("巣から出た身体の遺伝子の段の平均（1シードあたり）と、巣が引くセーブ（訓練ステージの終わり）の平均。\n\n")
		p("| 種族 | 遺伝子 | 巣から出た | セーブ |\n| --- | --- | --- | --- |\n")
		for k, name := range rs[0].speciesNames {
			for s, slot := range genoSlots {
				if slot.kind {
					continue
				}
				p("| %s | %s | %s | %.3f |\n", name, slot.name,
					fmtMeanSE(collectWhere(rs, func(r result) bool { return !math.IsNaN(histMean(r.genoNest[k][s])) }, func(r result) float64 { return histMean(r.genoNest[k][s]) }), 3),
					histMean(save[k][s]))
			}
		}
		p("\n")
	}
}
