package main

import (
	"testing"

	"github.com/toku463ne/cld_generic_dangeon/engine"
	"github.com/toku463ne/cld_generic_dangeon/variant"
	"github.com/toku463ne/cld_generic_dangeon/worldmap"
)

// The world the client starts drawing from is the one the experiment passed
// through at that tick: same seed, run again from tick 0.
func TestPrepareMatchesExperimentRun(t *testing.T) {
	o := options{mapName: "constrained", width: 32, height: 24, variant: variant.Base, seed: 9, fromTick: 1500}
	got, err := prepare(o)
	if err != nil {
		t.Fatal(err)
	}
	cfg, _ := variant.Config(variant.Base, 9)
	want, err := engine.NewWorld(cfg, worldmap.Constrained(32, 24))
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 1500; i++ {
		want.Step()
	}
	if got.Tick() != 1500 {
		t.Fatalf("prepared world is at tick %d, want 1500", got.Tick())
	}
	if got.Fingerprint() != want.Fingerprint() {
		t.Fatal("prepared world differs from the world run step by step")
	}
}

// Without -from-tick the client draws from tick 0.
func TestPrepareFromStart(t *testing.T) {
	w, err := prepare(options{mapName: "flat", width: 8, height: 8, variant: variant.Base, seed: 1})
	if err != nil {
		t.Fatal(err)
	}
	if w.Tick() != 0 {
		t.Fatalf("world is at tick %d, want 0", w.Tick())
	}
}

func TestPrepareRejectsBadOptions(t *testing.T) {
	for _, o := range []options{
		{mapName: "flat", variant: variant.Base},
		{mapName: "flat", width: 8, height: 8, variant: "nope"},
		{mapName: "flat", width: 8, height: 8, variant: variant.Base, fromTick: -1},
	} {
		if _, err := prepare(o); err == nil {
			t.Fatalf("prepare accepted %+v", o)
		}
	}
}
