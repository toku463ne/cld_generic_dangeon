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

// allot draws body b's share of its budget for speed and gives it the
// build the share buys: speed (2 share)^AllotCurve times the config's, the
// most energy (2 (1 - share))^AllotCurve times the config's. The share is
// one of AllotLevels evenly spaced values within AllotSpread of one half,
// each as likely. Its energy is held to its most.
func (w *World) allot(b *Body) {
	if !w.cfg.Allot {
		return
	}
	s := 0.5
	if n := w.cfg.AllotLevels; n > 1 {
		s += w.cfg.AllotSpread * (2*float64(w.rng.Intn(n))/float64(n-1) - 1)
	}
	b.Share = s
	b.Build = Build{
		Speed:     w.cfg.Speed * math.Pow(2*s, w.cfg.AllotCurve),
		EnergyMax: w.cfg.EnergyMax * math.Pow(2*(1-s), w.cfg.AllotCurve),
	}
	b.Energy = math.Min(b.Energy, w.maxOf(b))
}
