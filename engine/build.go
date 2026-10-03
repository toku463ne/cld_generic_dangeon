package engine

import "math"

// Builds.
//
// A body's build is its own speed, most energy and energy burned per tick.
// Zero in a field stands for the config's value, so a world whose bodies
// all have the zero build is the world the config describes, fingerprint
// for fingerprint. With Allot, every body is born with a share of its budget
// for speed, and its build is what the share buys (allot); besides that,
// only an experiment sets one (SetBuild).
//
// Every rule and the valuation read a body's own build: a faster body walks
// faster and predicts with its own speed.

// Build is a body's own abilities; zero fields are the config's.
type Build struct {
	Speed      float64 `json:",omitempty"`
	EnergyMax  float64 `json:",omitempty"`
	EnergyBurn float64 `json:",omitempty"`
}

func (w *World) speedOf(b *Body) float64 {
	speed := w.cfg.Speed
	if b.Build.Speed > 0 {
		speed = b.Build.Speed
	}
	return speed * w.ageFactor(b)
}

func (w *World) maxOf(b *Body) float64 {
	most := w.cfg.EnergyMax
	if b.Build.EnergyMax > 0 {
		most = b.Build.EnergyMax
	}
	return most * w.ageFactor(b)
}

// MaxEnergy is the most energy body b can hold now, its build's scaled by
// its age. It reads and changes nothing.
func (w *World) MaxEnergy(b Body) float64 { return w.maxOf(&b) }

// ageFactor scales a body's speed and most energy by its age (stage 3-7):
// from ChildAbility at birth up to 1 when it comes of age, 1 through
// adulthood, and down from OldAge to ChildAbility at Lifespan - a baby and
// an old body alike. The first bodies, adults from the start, skip the
// rise. The factor moves in steps of AgeStep, so that bodies share survival
// tables. With ChildAbility 1 it is 1. The sleep debt's factor (debtAbility)
// is on it too, no lower than ChildAbility.
func (w *World) ageFactor(b *Body) float64 {
	low := w.cfg.ChildAbility
	if low >= 1 || low <= 0 {
		return w.debtAbility(b)
	}
	age := float64(w.tick - b.Born)
	f := 1.0
	switch {
	case w.tick < b.Mature && b.Mature > b.Born:
		f = low + (1-low)*age/float64(b.Mature-b.Born)
	case w.cfg.OldAge > 0 && w.cfg.Lifespan > w.cfg.OldAge && age > float64(w.cfg.OldAge):
		f = 1 - (1-low)*(age-float64(w.cfg.OldAge))/float64(w.cfg.Lifespan-w.cfg.OldAge)
	}
	if step := w.cfg.AgeStep; step > 0 {
		f = math.Floor(f/step) * step
	}
	return math.Max(math.Min(f, 1)*w.debtAbility(b), low)
}

func (w *World) burnOf(b *Body) float64 {
	return w.ownBurn(b, false) * w.tempBurn(b)
}

// ownBurn is the burn body b knows of its own body: its build's, a resting
// mother's RestBurn of it (stage 3-4) unless ahead and RestAhead (she
// values at the burn she goes back to, stage 4-0), and its sleep debt's
// factor. What the place puts on it (the cold) it only feels.
func (w *World) ownBurn(b *Body, ahead bool) float64 {
	burn := w.cfg.EnergyBurn
	if b.Build.EnergyBurn > 0 {
		burn = b.Build.EnergyBurn
	}
	if w.resting(b) && !(ahead && w.cfg.RestAhead) {
		burn *= w.cfg.RestBurn // a resting mother keeps still (stage 3-4)
	}
	return burn * w.debtBurn(b)
}

// TempAt returns the temperature on tile (x, y) now (stage 7-4): normal on
// a place to rest (Shelter, stage 6-1a). ok is false on a map without
// temperature.
func (w *World) TempAt(x, y int) (float64, bool) {
	t, ok := w.m.Temp(w.m.RegionAt(x, y), w.Brightness())
	if ok && w.cfg.Shelter && w.m.ShelterAt(x, y) {
		return w.cfg.NormalTemp, true
	}
	return t, ok
}

// tempBurn is the factor the temperature on body b's tile puts on its burn
// (stage 7-4): 1 at NormalTemp, rising by 1 for each TempBurn degrees off
// it, either way.
func (w *World) tempBurn(b *Body) float64 {
	if !w.cfg.Temperature {
		return 1
	}
	t, ok := w.TempAt(int(math.Floor(b.X)), int(math.Floor(b.Y)))
	if !ok {
		return 1
	}
	return 1 + math.Abs(t-w.cfg.NormalTemp)/w.cfg.TempBurn
}

// TempBurn is the factor the temperature puts on body b's burn now.
func (w *World) TempBurn(b Body) float64 { return w.tempBurn(&b) }

// debtBurn is the factor body b's sleep debt puts on its burn (stage 7-3b).
func (w *World) debtBurn(b *Body) float64 {
	if !w.cfg.Sleep || w.cfg.DebtBurn <= 0 {
		return 1
	}
	return 1 + b.Debt/w.cfg.DebtBurn
}

// debtAbility is the factor body b's sleep debt puts on its ability (stage
// 7-3b).
// It moves in steps of AgeStep, to the nearest, so that a small debt does
// not cost a step and bodies share survival tables.
func (w *World) debtAbility(b *Body) float64 {
	if !w.cfg.Sleep || w.cfg.DebtAbility <= 0 {
		return 1
	}
	f := math.Max(w.cfg.DebtFloor, 1-b.Debt/w.cfg.DebtAbility)
	if step := w.cfg.AgeStep; step > 0 {
		f = math.Max(w.cfg.DebtFloor, math.Round(f/step)*step)
	}
	return f
}

// sleepTick moves body b's sleep pressure and debt on by a tick, asleep or
// awake, and wakes it once the pressure has fallen to WakeAt (stage 7-3b).
// Woken, its intent is a wait, which is decided again next tick.
func (w *World) sleepTick(b *Body) {
	if !w.cfg.Sleep {
		return
	}
	if b.Asleep {
		b.Pressure -= b.Pressure / w.cfg.PressureAsleep
		b.Debt = math.Max(0, b.Debt-w.cfg.DebtRepay)
		if w.tick >= b.Woke || w.touched(b) {
			b.Asleep = false
			b.Intent = Action{Kind: ActWait}
		}
		return
	}
	b.Debt += b.Pressure
	b.Pressure += (1 - b.Pressure) / w.cfg.PressureAwake
}

// sleepDue reports whether body b's sleep is due (stage 7-3d): awake, with
// its pressure at ForceAt or above.
func (w *World) sleepDue(b *Body) bool {
	return w.cfg.Sleep && w.cfg.ForceAt > 0 && !b.Asleep && b.Pressure >= w.cfg.ForceAt
}

// dueSleep is what body b, its sleep due, does instead of a: it sleeps,
// unless its pressure is below ForceMax and sleeping now would raise its
// chance of dying within the window by more than StayAwake over its best
// waking option (life first), read from the decision just made.
func (w *World) dueSleep(b *Body, a Action) Action {
	sleep := Action{Kind: ActSleep}
	if b.Pressure >= w.cfg.ForceMax {
		return sleep
	}
	v := &w.valuation
	asleep, awake := math.Inf(1), math.Inf(1)
	for j, o := range v.Options {
		if j >= len(v.Risk[0]) {
			break
		}
		if o.Kind == ActSleep {
			asleep = v.Risk[0][j]
		} else {
			awake = math.Min(awake, v.Risk[0][j])
		}
	}
	if math.IsInf(asleep, 1) || asleep <= awake+w.cfg.StayAwake {
		return sleep
	}
	return a
}

// touched reports whether another body that is awake stands on sleeping
// body b's tile: the contact that wakes it (stage 7-3b; broadcasts and
// threats will wake it too, once there are any).
func (w *World) touched(b *Body) bool {
	if w.grid == nil {
		return false
	}
	for _, j := range w.grid[w.tileOf(b.X, b.Y)] {
		if o := &w.bodies[j]; o.ID != b.ID && !o.Asleep {
			return true
		}
	}
	return false
}

// planBurn is the burn body b values its options at: its own, but for a
// resting mother with RestAhead the burn she goes back to when her rest
// ends (stage 4-0).
func (w *World) planBurn(b *Body) float64 {
	return w.ownBurn(b, true) * w.tempBurn(b)
}

// readBurn is the burn body b values its options at (stage 7-3a): planBurn
// now, and with BurnRead, its average over the window if it keeps rising as
// the body feels it rising, in steps of BurnStep above planBurn. A burn
// that is not rising reads as planBurn exactly.
func (w *World) readBurn(b *Body) float64 {
	return w.aheadBurn(w.planBurn(b), b.BurnRise)
}

// aheadBurn is readBurn for a burn and a rise per tick: the burn itself
// when it is not rising, else the window's average rounded up to a step of
// BurnStep above it. (The steps once kept survival tables few; since the
// tables count grains of energy they no longer matter to them.)
func (w *World) aheadBurn(burn, rise float64) float64 {
	if !w.cfg.BurnRead || rise <= 0 || w.cfg.Window <= 0 {
		return burn
	}
	ahead := burn + rise*float64(w.cfg.Window)/2
	if w.cfg.BurnStep <= 1 {
		return ahead
	}
	k := math.Ceil(math.Log(ahead/burn)/math.Log(w.cfg.BurnStep) - 1e-9)
	return burn * math.Pow(w.cfg.BurnStep, k)
}

// feelBurn has body b feel its burn after a tick: how far planBurn moved
// since it last felt it, smoothed into BurnRise (stage 7-3a).
func (w *World) feelBurn(b *Body) {
	if !w.cfg.BurnRead {
		return
	}
	burn := w.planBurn(b)
	if b.BurnFelt > 0 && w.cfg.BurnSpan > 0 {
		b.BurnRise += (burn - b.BurnFelt - b.BurnRise) / float64(w.cfg.BurnSpan)
	}
	b.BurnFelt = burn
}

// restReserve is, for a resting mother with RestAhead, the grains of energy
// the rest of her rest saves her: she burns RestBurn of planBurn each tick
// until Rested. Zero for every other body.
func (w *World) restReserve(b *Body) int {
	if !w.cfg.RestAhead || !w.resting(b) || w.cfg.RestBurn >= 1 {
		return 0
	}
	return int(math.Floor(float64(b.Rested-w.tick) * (1 - w.cfg.RestBurn) * w.planBurn(b) / w.cfg.Grain))
}

// SetBuild gives the living body with the given ID a build, for experiments
// that set bodies apart; no rule calls it. Its energy is held to the new
// most. It reports whether the body was found.
func (w *World) SetBuild(id int64, build Build) bool {
	for i := range w.bodies {
		b := &w.bodies[i]
		if b.ID == id {
			b.Build = build
			if m := w.maxOf(b); b.Energy > m {
				b.Energy = m
			}
			return true
		}
	}
	return false
}

// allot gives body b its share of its budget for speed and the build the
// share buys: speed (2 share)^AllotCurve times the config's, the most
// energy (2 (1 - share))^AllotCurve times the config's. The share is one of
// AllotLevels evenly spaced values within AllotSpread of one half. A body
// with no parents draws its level, each as likely; with AllotInherit a
// child takes the level of one of its parents, drawn, and with chance
// AllotMutation moves it one level up or down (inward at the ends).
// Without AllotInherit a child draws too. Its energy is held to its most.
func (w *World) allot(b *Body, parents ...*Body) {
	b.Budget = w.cfg.Budget
	w.stats.BudgetIn += b.Budget
	if !w.cfg.Allot {
		return
	}
	n := w.cfg.AllotLevels
	k := 0
	switch {
	case n <= 1:
	case w.cfg.AllotInherit && len(parents) == 2:
		k = parents[w.rng.Intn(2)].Level
		if w.rng.Float64() < w.cfg.AllotMutation {
			if w.rng.Intn(2) == 0 {
				k--
			} else {
				k++
			}
			if k < 0 {
				k = 1
			}
			if k > n-1 {
				k = n - 2
			}
		}
	default:
		k = w.rng.Intn(n)
	}
	s := 0.5
	if n > 1 {
		s += w.cfg.AllotSpread * (2*float64(k)/float64(n-1) - 1)
	}
	b.Level, b.Share = k, s
	b.Build = Build{
		Speed:     w.cfg.Speed * math.Pow(2*s, w.cfg.AllotCurve),
		EnergyMax: w.cfg.EnergyMax * math.Pow(2*(1-s), w.cfg.AllotCurve),
	}
	b.Energy = math.Min(b.Energy, w.maxOf(b))
}

// BudgetLedger is the running account of the budgets: every body brings
// one in when it is born and takes it out when it dies. The budget of a
// body is Config.Budget for now; the account is kept so that it closes when
// budgets come to depend on the parents'.
type BudgetLedger struct {
	Living  float64 // the budgets of the living, summed
	In, Out float64
}

// Balanced reports whether the account closes: the living hold what came
// in less what went out, to rounding.
func (l BudgetLedger) Balanced() bool {
	return math.Abs(l.Living-(l.In-l.Out)) <= 1e-9*math.Max(1, l.In)
}

// BudgetLedger returns the budget account.
func (w *World) BudgetLedger() BudgetLedger {
	l := BudgetLedger{In: w.stats.BudgetIn, Out: w.stats.BudgetOut}
	for i := range w.bodies {
		l.Living += w.bodies[i].Budget
	}
	return l
}
