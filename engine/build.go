package engine

// Builds.
//
// A body's build is its own speed, most energy and energy burned per tick.
// Zero in a field stands for the config's value, so a world whose bodies
// all have the zero build is the world the config describes, fingerprint
// for fingerprint. No rule sets a build yet: stage 1-4 will draw them from a
// budget. Until then only an experiment sets one (SetBuild), to set some
// bodies apart from the rest in the same world.
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
