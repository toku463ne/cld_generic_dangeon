package engine

import "testing"

// A body's sleep is due at ForceAt, awake; then it can no longer mate. A
// fed body falls asleep; one for whom sleeping now is much riskier than
// its best waking option stays awake (life first) - until ForceMax.
func TestForcedSleep(t *testing.T) {
	w, b := soloWorld(t, 99)
	w.cfg.Sleep, w.cfg.ForceAt, w.cfg.ForceMax, w.cfg.StayAwake = true, 0.625, 0.9, 0.1
	b.Born, b.Mature = -10000, -9000 // an adult
	b.Pressure = 0.6
	if w.sleepDue(b) {
		t.Fatal("due below ForceAt")
	}
	b.Pressure = 0.7
	if !w.sleepDue(b) {
		t.Fatal("not due at 0.7")
	}
	if w.CanMate(*b) {
		t.Fatal("a body whose sleep is due can mate")
	}
	b.Asleep = true
	if w.sleepDue(b) {
		t.Fatal("due while asleep")
	}
	b.Asleep = false
	wait := Action{Kind: ActWait}
	w.decide(b)
	if a := w.dueSleep(b, wait); a.Kind != ActSleep {
		t.Fatalf("a fed body did %v, want sleep", a)
	}
	// Starving with food underfoot, sleeping now is far riskier than
	// eating: it stays awake, until the pressure reaches ForceMax.
	b.Energy = 3
	w.decide(b)
	eat := Action{Kind: ActEat}
	if a := w.dueSleep(b, eat); a != eat {
		t.Fatalf("a starving body did %v, want to eat", a)
	}
	b.Pressure = 0.95
	w.decide(b)
	if a := w.dueSleep(b, eat); a.Kind != ActSleep {
		t.Fatalf("past ForceMax it did %v, want sleep", a)
	}
	w.cfg.ForceAt = 0
	if w.sleepDue(b) {
		t.Fatal("due without ForceAt")
	}
}

// Without forced sleep, the world is stage 6-1b: its fingerprint is the
// one pinned then.
func TestUnforcedIsStage61b(t *testing.T) {
	const want = uint64(0x35148fa949adaa70)
	cfg := testConfig(1)
	cfg.AllotSight = false
	cfg.AllotFertility = false
	cfg.AllotBudget = false
	cfg.ReuseNights, cfg.SeekBed = false, false
	cfg.Building, cfg.Tents = false, false
	cfg.ShelterFoodLife = 0
	cfg.ForceAt = 0
	w, err := NewWorld(cfg, tempMap())
	if err != nil {
		t.Fatal(err)
	}
	run(w, 1000)
	if got := w.Fingerprint(); got != want {
		t.Fatalf("fingerprint = %#x, want %#x", got, want)
	}
}
