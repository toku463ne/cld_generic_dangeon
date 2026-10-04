package engine

import "testing"

// Without tents and with building, the world is stage 8-1b: its
// fingerprint is the one pinned then.
func TestHousedIsStage81b(t *testing.T) {
	const want = uint64(0xb14340e47db514ca)
	cfg := testConfig(1)
	cfg.Building, cfg.Tents = true, false
	w, err := NewWorld(cfg, tempMap())
	if err != nil {
		t.Fatal(err)
	}
	run(w, 1000)
	if got := w.Fingerprint(); got != want {
		t.Fatalf("fingerprint = %#x, want %#x", got, want)
	}
}

// A body with the skill falling asleep makes a tent where that reads as
// less risk, paying for it; one with a tent pitches it and sleeps out of
// the cold; the tent is folded on waking, and when it wears out the body
// learns the nights it used it. A body without the skill makes none, and
// nothing changes how a body sleeps but a sleep.
func TestTent(t *testing.T) {
	w := pairWorld(t, Body{ID: 0, X: 5.5, Y: 5.5, Energy: 60, Pressure: 0.5}, Body{ID: 1, X: 20.5, Y: 20.5, Energy: 60, Pressure: 0.5})
	w.cfg.Tents = true
	b, o := &w.bodies[0], &w.bodies[1]
	b.Skill[SkillTent] = 1
	w.howSleep(o, Action{Kind: ActSleep})
	if o.Tent.Has {
		t.Fatal("a body without the skill made a tent")
	}
	w.howSleep(b, Action{Kind: ActWait})
	if b.Tent.Has {
		t.Fatal("a tent was made without a sleep")
	}
	// Having learned the warmth and expecting nights in it, it makes one.
	b.Memory.Warm = [2]float64{1, 1}
	if w.tentCredit(b) != 0 {
		t.Fatal("credit for a tent before the cold outside was felt")
	}
	b.Memory.Cold = [2]float64{1.3, 1}
	if w.tentCredit(b) <= 0 {
		t.Fatal("no credit for a tent with cold and warmth felt")
	}
	w.cfg.ReusePrior = 50 // nights enough to outweigh the cost whatever the cold
	w.howSleep(b, Action{Kind: ActSleep})
	if !b.Tent.Has || !b.Tent.Pitched || b.Tent.Uses != 1 || b.Energy != 60-w.cfg.TentEnergy || w.stats.TentsMade != 1 {
		t.Fatalf("tent %+v, energy %v", b.Tent, b.Energy)
	}
	if w.cfg.Temperature && w.tempBurn(b) != 1 {
		t.Fatal("a pitched tent does not keep the cold out")
	}
	w.act(0, b, Action{Kind: ActSleep})
	b.Asleep = false
	w.foldTent(b)
	if b.Tent.Pitched || !b.Tent.Has {
		t.Fatalf("not folded on waking: %+v", b.Tent)
	}
	w.howSleep(b, Action{Kind: ActSleep})
	if b.Tent.Uses != 2 || w.stats.TentsMade != 1 {
		t.Fatalf("pitching again made another or did not count: %+v", b.Tent)
	}
	w.tick = b.Tent.Until
	w.foldTent(b)
	if b.Tent.Has || b.Memory.TentUses != [2]float64{2, 1} || w.stats.TentsWorn != 1 {
		t.Fatalf("worn out: %+v, uses %v", b.Tent, b.Memory.TentUses)
	}
}

// A body with nothing learned of warmth reads making a tent as the sleep
// paid for: never less risk than sleeping as it is.
func TestTentUnlearned(t *testing.T) {
	w := pairWorld(t, Body{ID: 0, X: 5.5, Y: 5.5, Energy: 30, Pressure: 0.5}, Body{ID: 1, X: 20.5, Y: 20.5, Energy: 60})
	b := &w.bodies[0]
	if c := w.tentCredit(b); c != 0 {
		t.Fatalf("credit %d with no warmth learned", c)
	}
	var rs rates
	w.ratesOf(b, &rs)
	win := w.pred.windows[0]
	plain := w.sleepRiskAt(b, &rs, win, 0, w.planBurn(b), b.Energy)
	made := w.sleepRiskAt(b, &rs, win, w.tentCredit(b), w.tentWarm(b), b.Energy-w.cfg.TentEnergy)
	if made < plain {
		t.Fatalf("an unlearned tent reads safer: %v < %v", made, plain)
	}
}

// Reading a night's saving at the burn of falling asleep, the world is
// stage 8-2a: its fingerprint is the one pinned then.
func TestAtduskIsStage82a(t *testing.T) {
	const want = uint64(0x998e11891b32b064)
	cfg := testConfig(1)
	cfg.NightSaving, cfg.DropTents = false, false
	w, err := NewWorld(cfg, tempMap())
	if err != nil {
		t.Fatal(err)
	}
	run(w, 1000)
	if got := w.Fingerprint(); got != want {
		t.Fatalf("fingerprint = %#x, want %#x", got, want)
	}
}

// Asleep in the dark off any place to rest and out of a tent, a body
// learns the cold it felt over its own burn; in a pitched tent it learns
// the warmth instead.
func TestColdFelt(t *testing.T) {
	w, err := NewWorld(testConfig(1), tempMap())
	if err != nil {
		t.Fatal(err)
	}
	for w.Brightness() >= 0.5 {
		w.tick++
	}
	b := &w.bodies[0]
	for w.m.ShelterAt(int(b.X), int(b.Y)) {
		b.X++
	}
	b.Asleep = true
	w.feelWarm(b)
	if b.Memory.Cold[1] != 1 || b.Memory.Cold[0] != w.planBurn(b)/w.ownBurn(b, true) || b.Memory.Warm[1] != 0 {
		t.Fatalf("cold %v warm %v", b.Memory.Cold, b.Memory.Warm)
	}
	b.Tent = Tent{Has: true, Pitched: true, Until: w.tick + 10}
	w.feelWarm(b)
	if b.Memory.Cold[1] != 1 || b.Memory.Warm[1] != 1 {
		t.Fatalf("in a tent: cold %v warm %v", b.Memory.Cold, b.Memory.Warm)
	}
}

// Losing tents with their owners, the world is stage 8-2a2: its
// fingerprint is the one pinned then.
func TestBuriedIsStage82a2(t *testing.T) {
	const want = uint64(0x4cf99317cf4b51a6)
	cfg := testConfig(1)
	cfg.DropTents = false
	w, err := NewWorld(cfg, tempMap())
	if err != nil {
		t.Fatal(err)
	}
	run(w, 1000)
	if got := w.Fingerprint(); got != want {
		t.Fatalf("fingerprint = %#x, want %#x", got, want)
	}
}

// A dead body's tent stays on its tile until it would have worn out; a
// body with no tent standing there takes it up, skill or not, and one
// with a tent leaves it.
func TestDropTent(t *testing.T) {
	w := pairWorld(t, Body{ID: 0, X: 5.5, Y: 5.5, Energy: 0}, Body{ID: 1, X: 6.5, Y: 5.5, Energy: 60})
	w.cfg.DropTents = true
	dead, o := &w.bodies[0], &w.bodies[1]
	dead.Tent = Tent{Has: true, Until: w.tick + 500, Uses: 3}
	w.removeDead()
	if len(w.bodies) != 1 || len(w.groundTents) != 1 || w.groundTents[0].Until != w.tick+500 {
		t.Fatalf("ground tents %+v", w.groundTents)
	}
	o = &w.bodies[0]
	o.Tent = Tent{Has: true, Until: w.tick + 100}
	o.X = 5.5
	w.pickTent(o)
	if len(w.groundTents) != 1 {
		t.Fatal("a body with a tent took up another")
	}
	o.Tent = Tent{}
	w.pickTent(o)
	if !o.Tent.Has || o.Tent.Until != w.tick+500 || o.Tent.Uses != 0 || len(w.groundTents) != 0 {
		t.Fatalf("taken up: %+v, ground %v", o.Tent, w.groundTents)
	}
	w.groundTents = []GroundTent{{Tile: 3, Until: w.tick + 1}}
	w.tick++
	w.wearGroundTents()
	if len(w.groundTents) != 0 {
		t.Fatal("a tent on the ground did not wear out")
	}
}
