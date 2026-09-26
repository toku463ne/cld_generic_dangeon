package variant

import (
	"testing"

	"github.com/toku463ne/cld_generic_dangeon/engine"
)

func TestBaseIsDefault(t *testing.T) {
	cfg, err := Config(Base, 7)
	if err != nil {
		t.Fatal(err)
	}
	want := engine.DefaultConfig()
	want.Seed = 7
	if cfg != want {
		t.Fatalf("base config %+v, want %+v", cfg, want)
	}
}

func TestUnknownVariant(t *testing.T) {
	if _, err := Config("nope", 1); err == nil {
		t.Fatal("unknown variant accepted")
	}
}

// Every variant but base is an earlier stage, so the rules added since are
// off in it: collisions in all of them, the budget in all before 1-4, and so
// on down.
func TestEarlierStagesLackLaterRules(t *testing.T) {
	// The stage each variant stands for, as the rules it still has.
	for _, c := range []struct {
		name                            string
		collide, allot, breed, onEvents bool
	}{
		{Bounced, true, true, true, true},
		{Drawn, true, true, true, true},
		{Overlap, false, true, true, true},
		{Fixed, false, false, true, true},
		{Nobreed, false, false, false, true},
		{Everytick, false, false, false, false},
		{Straightback, false, false, false, false},
		{Restless, false, false, false, false},
		{Blind, false, false, false, false},
		{Random, false, false, false, false},
	} {
		cfg, err := Config(c.name, 1)
		if err != nil {
			t.Fatal(err)
		}
		if !cfg.Bounce {
			t.Errorf("%s: an earlier stage without the bounce", c.name)
		}
		if cfg.Collide != c.collide || cfg.Allot != c.allot || cfg.Breed != c.breed || (cfg.Recheck > 0) != c.onEvents {
			t.Errorf("%s: collide %v allot %v breed %v recheck %d", c.name, cfg.Collide, cfg.Allot, cfg.Breed, cfg.Recheck)
		}
	}
}
