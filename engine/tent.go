package engine

import "math"

// Tents (stage 8-2a, VISION "テント"; the user's order: tents, then
// settlements).
//
// A body may be born with the skill to make a tent (TentBorn). A tent is
// carried folded, in a place of its own (not the food a body holds), and
// pitched where its owner falls asleep: asleep in it, the owner burns as
// on a place to rest, no more for the cold. One body to a tile (Collide)
// keeps others out of it. It is folded again on waking and wears out
// TentLife ticks after it was made.
//
// Tents change only how a body sleeps, never whether: what a tent is worth
// beyond tonight is read at the sleep itself (the user's choice after
// 8-1b, where worth beyond the window read as energy now and turned the
// night's choice to sleep). A body falling asleep with a tent pitches it.
// One falling asleep without, that has the skill, makes one first if
// making and sleeping in it reads as less risk than sleeping as it is -
// the cost TentEnergy paid, the sleep at the warmth it learned on places
// to rest (Memory.Warm; its burn now if none), and as reserve the nights
// it expects to use the tent (Memory.TentUses, learned when a tent wears
// out, pulled toward ReusePrior by ReuseWeight) times one night's saving -
// and where the two tie, the draw may take either. A body with a tent
// reads its sleeping option at that warmth, as it is tonight's.

// Tent is the tent a body carries.
type Tent struct {
	// Has is whether it has one; Until the tick it wears out; Uses the
	// nights it was pitched; Pitched whether it is pitched now (its owner
	// asleep in it).
	Has     bool  `json:",omitempty"`
	Until   int64 `json:",omitempty"`
	Uses    int   `json:",omitempty"`
	Pitched bool  `json:",omitempty"`
}

// tentWarm is the burn body b reads itself sleeping at in a tent: the burn
// it learned on places to rest, over the burn it knows of its own body, or
// its burn now if it has learned none.
func (w *World) tentWarm(b *Body) float64 {
	now := w.planBurn(b)
	if b.Memory.Warm[1] == 0 {
		return now
	}
	return math.Min(now, w.ownBurn(b, true)*b.Memory.Warm[0]/b.Memory.Warm[1])
}

// tentCredit is the worth beyond tonight, in grains, of a tent body b
// would make now: the nights it expects to use one times a night's saving.
func (w *World) tentCredit(b *Body) int {
	n := b.Memory.TentUses[1] + w.cfg.ReuseWeight
	saving := w.nightSaving(b)
	if n <= 0 || saving <= 0 {
		return 0
	}
	nights := (b.Memory.TentUses[0] + w.cfg.ReusePrior*w.cfg.ReuseWeight) / n
	return w.units(nights * saving)
}

// nightSaving is the energy body b reads a night's sleep out of the cold
// to save: with NightSaving, its own burn times the cold it felt asleep
// outside less the warmth it felt on a place to rest or in a tent, over
// SleepTicks (nothing until it has felt both); without, the burn now less
// the warm burn, over SleepTicks (8-2a).
func (w *World) nightSaving(b *Body) float64 {
	if !w.cfg.NightSaving {
		return math.Max(0, w.planBurn(b)-w.tentWarm(b)) * float64(w.cfg.SleepTicks)
	}
	c, h := b.Memory.Cold, b.Memory.Warm
	if c[1] == 0 || h[1] == 0 {
		return 0
	}
	return math.Max(0, c[0]/c[1]-h[0]/h[1]) * w.ownBurn(b, true) * float64(w.cfg.SleepTicks)
}

// canMakeTent reports whether body b could make a tent now.
func (w *World) canMakeTent(b *Body) bool {
	return w.cfg.Tents && b.Skill[SkillTent] > 0 && !b.Tent.Has && b.Mature <= w.tick && b.Energy > w.cfg.TentEnergy
}

// howSleep is how body b, falling asleep (a is ActSleep), does it: with a
// tent it pitches it; without one, it makes one first where that reads as
// less risk than sleeping as it is, drawn between them on a tie.
func (w *World) howSleep(b *Body, a Action) {
	if a.Kind != ActSleep || !w.cfg.Tents {
		return
	}
	if !b.Tent.Has && w.canMakeTent(b) && w.cfg.Learn && w.cfg.Window > 0 {
		reserve := w.restReserve(b) + b.Held*w.pred.meal
		win := w.pred.windows[0]
		var rs rates
		w.ratesOf(b, &rs)
		plain := w.sleepRiskAt(b, &rs, win, reserve, w.planBurn(b), b.Energy)
		made := w.sleepRiskAt(b, &rs, win, reserve+w.tentCredit(b), w.tentWarm(b), b.Energy-w.cfg.TentEnergy)
		if made < plain || made == plain && w.rng.Intn(2) == 0 {
			b.Energy -= w.cfg.TentEnergy
			b.Tent = Tent{Has: true, Until: w.tick + int64(w.cfg.TentLife)}
			w.stats.TentsMade++
		}
	}
	if b.Tent.Has {
		b.Tent.Pitched = true
		b.Tent.Uses++
		w.stats.TentNights++
	}
}

// foldTent folds body b's tent when it is awake, and takes it from it when
// it wears out: the body learns the nights it used it.
func (w *World) foldTent(b *Body) {
	if !b.Tent.Has {
		return
	}
	if !b.Asleep {
		b.Tent.Pitched = false
	}
	if b.Tent.Until > w.tick {
		return
	}
	if w.cfg.Learn {
		b.Memory.TentUses[0] += float64(b.Tent.Uses)
		b.Memory.TentUses[1]++
	}
	b.Tent = Tent{}
	w.stats.TentsWorn++
}
