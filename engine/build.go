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
	if b.Build.Speed > 0 {
		return b.Build.Speed
	}
	return w.cfg.Speed
}

func (w *World) maxOf(b *Body) float64 {
	if b.Build.EnergyMax > 0 {
		return b.Build.EnergyMax
	}
	return w.cfg.EnergyMax
}

func (w *World) burnOf(b *Body) float64 {
	if b.Build.EnergyBurn > 0 {
		return b.Build.EnergyBurn
	}
	return w.cfg.EnergyBurn
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
