package engine

import "math"

// Foreseeing a forced sleep (stage 7-3e).
//
// A body knows its own pressure, so it knows in how many ticks its sleep
// will be forced (ForceAt). A sleep that falls within the window is a
// stretch of it in which it eats nothing; read without it, every waking
// option looks as if the body could search through it, and sleeping now
// never looks better than staying awake until made to sleep. With
// SleepAhead the waking options read that sleep taken off the end of the
// window, and every option reads keeping on the move with the mean sight
// of the hours it would be awake: from the times it learned dark and light
// come (stage 7-2b), sleeping through the night leaves bright hours awake,
// sleeping through the day dark ones.

// sleepFore is what a decision of body b reads of its coming sleep: the
// grains taken off waking options, and the factors on the chance per tile
// of meeting food for waking options and for sleeping now. Off, or with
// no forced sleep in the window, it is 0, 1, 1.
func (w *World) sleepFore(b *Body) (lump int, awake, slept float64) {
	if !w.cfg.SleepAhead || !w.cfg.Sleep || w.cfg.ForceAt <= 0 || b.Asleep || b.Pressure >= w.cfg.ForceAt || w.cfg.Window <= 0 {
		return 0, 1, 1
	}
	due := w.ticksToForce(b)
	if due >= w.cfg.Window {
		return 0, 1, 1
	}
	// The config body's sleep, whatever this body's burn: a table keeps a
	// row for every window length it is read at, and each such row is a
	// pass of its recursion, so the windows a foreseen sleep leaves must be
	// few (a burn of its own made them many: 1.8 GB of rows).
	lump = w.lumpGrains()
	sched, ok := w.lightAhead(b)
	if !ok {
		return lump, 1, 1
	}
	sight := w.sightOf(b)
	now := w.sightGainFor(sight, w.Brightness())
	return lump, sched.mean(due, w, sight) / now, sched.mean(0, w, sight) / now
}

// ticksToForce is how many ticks of waking take body b's pressure to
// ForceAt.
func (w *World) ticksToForce(b *Body) int {
	// Each tick awake leaves 1-1/PressureAwake of what is short of 1.
	t := math.Log((1-w.cfg.ForceAt)/(1-b.Pressure)) / math.Log(1-1/w.cfg.PressureAwake)
	return int(math.Ceil(t - 1e-9))
}

// lightSched is the light a body expects over the window: full light or
// dark from the start, switching at ticks s1 and s2.
type lightSched struct {
	lit    bool
	s1, s2 int
}

// lightAhead is body b's expected light, from the rows it learned
// (DuskRead); ok is false where it has not learned them.
func (w *World) lightAhead(b *Body) (lightSched, bool) {
	toDark, toLight, ok := w.duskAhead(b)
	if !ok {
		return lightSched{}, false
	}
	lit := b.LightSeen >= 0.5
	first, second := toDark, toLight
	if !lit {
		first, second = toLight, toDark
	}
	s1 := int(math.Round(first))
	return lightSched{lit: lit, s1: s1, s2: max(int(math.Round(second)), s1)}, true
}

// mean is the mean sight gain over the window's ticks but the SleepTicks
// from sleep on.
func (l lightSched) mean(sleep int, w *World, sight int) float64 {
	win := w.cfg.Window
	lit, dark := w.sightGainFor(sight, 1), w.sightGainFor(sight, 0)
	// over sums the gain over ticks [a, b).
	over := func(a, b int) float64 {
		sum := 0.0
		edges := []int{0, l.s1, l.s2, math.MaxInt32}
		on := l.lit
		for i := 0; i+1 < len(edges); i++ {
			lo, hi := max(a, edges[i]), min(b, edges[i+1])
			if hi > lo {
				g := dark
				if on {
					g = lit
				}
				sum += g * float64(hi-lo)
			}
			on = !on
		}
		return sum
	}
	end := min(sleep+w.cfg.SleepTicks, win)
	awake := win - (end - sleep)
	if awake <= 0 {
		return w.sightGainFor(sight, w.Brightness())
	}
	return (over(0, win) - over(sleep, end)) / float64(awake)
}

// lumpGrains is the grains a foreseen sleep is taken off the window: the
// config body's SleepTicks.
func (w *World) lumpGrains() int { return w.grains(w.cfg.SleepTicks, w.cfg.EnergyBurn) }

// aheadRows are the window lengths a foreseen sleep leaves (SleepAhead):
// the windows a walk or the path reading reads from there, built with a
// table in its first pass rather than one pass each.
func (w *World) aheadRows(win, reach int) []int {
	if !w.cfg.SleepAhead || !w.cfg.Sleep || w.cfg.ForceAt <= 0 {
		return nil
	}
	var ls []int
	add := func(lo, hi int) {
		for l := max(lo, 1); l <= hi; l++ {
			ls = append(ls, l)
		}
	}
	top := win - w.lumpGrains()
	add(top-reach-1, top)
	ahead := w.grains(w.pathTicks(w.cfg.Speed), w.cfg.EnergyBurn)
	add(top-ahead, top-ahead)
	add(top-ahead/2-1, top-ahead/2-1)
	return ls
}
