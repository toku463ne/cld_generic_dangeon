package engine

import (
	"math"
	"testing"
)

// A body knows how many ticks of waking take its pressure to ForceAt; with
// that in the window, waking options read the sleep taken off, and the
// mean sight of the waking hours follows the light it learned: a sleep
// now, at dusk, leaves brighter hours awake than one forced at noon
// tomorrow. Unlearned, the light now; off, or due, nothing.
func TestSleepAhead(t *testing.T) {
	w, b := soloWorld(t, 80)
	w.cfg.Sleep, w.cfg.ForceAt, w.cfg.SleepAhead, w.cfg.SightRead, w.cfg.DuskRead = true, 0.625, true, true, true
	b.Pressure = 0.5
	due := w.ticksToForce(b)
	p := b.Pressure
	for i := 0; i < due; i++ {
		p += (1 - p) / w.cfg.PressureAwake
	}
	if p < w.cfg.ForceAt || 1-(1-0.5)*math.Pow(1-1/w.cfg.PressureAwake, float64(due-1)) >= w.cfg.ForceAt {
		t.Fatalf("forced in %d ticks: pressure then %v", due, p)
	}
	w.light, w.lightFor = 1, w.tick+1
	lump, awake, slept := w.sleepFore(b)
	if lump != w.grains(w.cfg.SleepTicks, w.cfg.EnergyBurn) || awake != 1 || slept != 1 {
		t.Fatalf("unlearned: lump %d, factors %v %v", lump, awake, slept)
	}
	// In full light, dark in 100 ticks and light again in 600: the sleep
	// due (some 350 ticks on) falls in the night, a sleep now in the day.
	b.LightKnown, b.LightSeen = true, 1
	b.LightKey = w.lightKey(1, 0, -1)
	b.Dusk = []DuskRow{{Key: b.LightKey, ToDark: [2]float64{100, 1}, ToLight: [2]float64{600, 1}}}
	_, awake, slept = w.sleepFore(b)
	if !(awake > slept) {
		t.Fatalf("noon: waking factor %v, sleeping now %v: sleeping through the night should leave brighter hours", awake, slept)
	}
	s := lightSched{lit: true, s1: 100, s2: 600}
	if got, want := s.mean(1000, w), (100*3+500*1+400*3)/1000.0; math.Abs(got-want) > 1e-12 {
		t.Fatalf("mean with no sleep in the window %v, want %v", got, want)
	}
	if got, want := s.mean(100, w), (100*3+167*1+400*3)/667.0; math.Abs(got-want) > 1e-12 {
		t.Fatalf("mean around a sleep at 100 %v, want %v", got, want)
	}
	b.Pressure = 0.7
	if lump, awake, slept := w.sleepFore(b); lump != 0 || awake != 1 || slept != 1 {
		t.Fatal("foresees a sleep already due")
	}
	b.Pressure = 0.5
	w.cfg.SleepAhead = false
	if lump, awake, slept := w.sleepFore(b); lump != 0 || awake != 1 || slept != 1 {
		t.Fatal("foresees without SleepAhead")
	}
}

// Without foreseeing a forced sleep, the world is stage 7-3d: its
// fingerprint is the one pinned then.
func TestSleepblindIsStage73d(t *testing.T) {
	const want = uint64(0x897e8d0ad6f500c1)
	cfg := testConfig(1)
	cfg.HideShelter, cfg.HomeRead = false, false
	cfg.FatherSends, cfg.Provision = false, true
	cfg.SleepAhead = false
	w, err := NewWorld(cfg, tempMap())
	if err != nil {
		t.Fatal(err)
	}
	run(w, 1000)
	if got := w.Fingerprint(); got != want {
		t.Fatalf("fingerprint = %#x, want %#x", got, want)
	}
}
