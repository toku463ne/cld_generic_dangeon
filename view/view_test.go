package view

import (
	"image"
	"strings"
	"testing"

	"github.com/toku463ne/cld_generic_dangeon/engine"
	"github.com/toku463ne/cld_generic_dangeon/variant"
	"github.com/toku463ne/cld_generic_dangeon/worldmap"
)

// stepped is the constrained world of seed 9 run step by step, without the
// client, as the experiment runs it.
func stepped(t *testing.T, ticks int) *engine.World {
	t.Helper()
	cfg, _ := variant.Config(variant.Base, 9)
	w, err := engine.NewWorld(cfg, worldmap.Constrained(32, 24))
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < ticks; i++ {
		w.Step()
	}
	return w
}

// The world the client starts drawing from is the one the experiment passed
// through at that tick: same seed, run again from tick 0.
func TestPrepareMatchesExperimentRun(t *testing.T) {
	o := Options{Map: "constrained", Width: 32, Height: 24, Variant: variant.Base, Seed: 9, FromTick: 1500}
	got, err := Prepare(o)
	if err != nil {
		t.Fatal(err)
	}
	want := stepped(t, 1500)
	if got.W.Tick() != 1500 {
		t.Fatalf("prepared world is at tick %d, want 1500", got.W.Tick())
	}
	if got.W.Fingerprint() != want.Fingerprint() {
		t.Fatal("prepared world differs from the world run step by step")
	}
	if len(got.hist) != 1501 {
		t.Fatalf("history has %d samples, want one per tick from 0 (1501)", len(got.hist))
	}
}

// Following a body, keeping trails and drawing change nothing in the world:
// the view only reads it.
func TestViewChangesNothing(t *testing.T) {
	o := Options{Map: "constrained", Width: 32, Height: 24, Variant: variant.Base, Seed: 9, FromTick: 1000, Follow: "hungriest"}
	v, err := Prepare(o)
	if err != nil {
		t.Fatal(err)
	}
	v.ShowTrails = true
	w, h := v.Size()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < 500; i++ {
		v.Step(true)
		if i%50 == 0 {
			v.Render(img)
			_ = v.FollowText()
		}
	}
	if v.W.Fingerprint() != stepped(t, 1500).Fingerprint() {
		t.Fatal("the world shown by the client differs from the world run alone")
	}
	if v.last == nil {
		t.Fatal("the followed body's decisions were not traced")
	}
	if !strings.Contains(v.FollowText(), "risk") && !strings.Contains(v.FollowText(), "dead") {
		t.Fatalf("follow text shows no decision: %q", v.FollowText())
	}
}

// A rhythm faster than a column of the history panel shows as a band from
// its least to its most, not as whatever value the column happened to
// sample: reading a 300-tick cycle through coarse samples once made a false
// 6000-tick wave.
func TestEnvelopeKeepsFastRhythm(t *testing.T) {
	var s []sample
	for i := 0; i < 64; i++ {
		f := int32(100)
		if i%8 < 4 {
			f = 200
		}
		s = append(s, sample{bodies: 50, food: f})
	}
	bmin, bmax, fmin, fmax := envelope(s)
	if fmin != 100 || fmax != 200 || bmin != 50 || bmax != 50 {
		t.Fatalf("envelope = %d %d %d %d, want bodies 50..50 and food 100..200", bmin, bmax, fmin, fmax)
	}
}

// Without -from-tick the client draws from tick 0.
func TestPrepareFromStart(t *testing.T) {
	v, err := Prepare(Options{Map: "flat", Width: 8, Height: 8, Variant: variant.Base, Seed: 1})
	if err != nil {
		t.Fatal(err)
	}
	if v.W.Tick() != 0 {
		t.Fatalf("world is at tick %d, want 0", v.W.Tick())
	}
}

func TestPrepareRejectsBadOptions(t *testing.T) {
	for _, o := range []Options{
		{Map: "flat", Variant: variant.Base},
		{Map: "flat", Width: 8, Height: 8, Variant: "nope"},
		{Map: "flat", Width: 8, Height: 8, Variant: variant.Base, FromTick: -1},
		{Map: "flat", Width: 8, Height: 8, Variant: variant.Base, Follow: "someone"},
	} {
		if _, err := Prepare(o); err == nil {
			t.Fatalf("prepare accepted %+v", o)
		}
	}
}
