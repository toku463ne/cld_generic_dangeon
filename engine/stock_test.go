package engine

import (
	"reflect"
	"testing"
)

// A world's stocks are its living bodies of a species, each with the
// genome it was born with and the skills it holds; a world begun from them
// has one first body per stock, with that genome and those skills and an
// empty memory, and hands the same stocks back (stage 13-2).
func TestStocksRoundTrip(t *testing.T) {
	w := newTestWorld(t, 4)
	run(w, 300)
	var stocks []Stock
	for s := 0; s < w.SpeciesCount(); s++ {
		got := w.Stocks(s)
		n := 0
		for _, b := range w.Bodies() {
			if b.Species == s {
				if got[n].Genome != w.GenomeOf(b) || got[n].Skill != b.Skill {
					t.Fatalf("stock %d of species %d is %+v, want the body's %+v %v", n, s, got[n], w.GenomeOf(b), b.Skill)
				}
				n++
			}
		}
		if n != len(got) {
			t.Fatalf("species %d: %d stocks for %d living bodies", s, len(got), n)
		}
		stocks = append(stocks, got...)
	}
	skilled, learned := 0, false
	for _, b := range w.Bodies() {
		if b.Skill != ([NumSkills]float64{}) {
			skilled++
		}
		learned = learned || !reflect.DeepEqual(b.Memory, Memory{})
	}
	if skilled == 0 || !learned {
		t.Fatalf("the saved world has %d skilled bodies, learned %v: the test reads nothing", skilled, learned)
	}

	cfg := testConfig(11)
	v, err := NewWorldFromStocks(cfg, testMap(), stocks)
	if err != nil {
		t.Fatal(err)
	}
	var back []Stock
	for s := 0; s < v.SpeciesCount(); s++ {
		back = append(back, v.Stocks(s)...)
	}
	if !reflect.DeepEqual(back, stocks) {
		t.Fatalf("stocks read back as\n%+v\nwant\n%+v", back, stocks)
	}
	for _, b := range v.Bodies() {
		if !reflect.DeepEqual(b.Memory, Memory{}) {
			t.Fatalf("body %d of a saved stock begins with a memory", b.ID)
		}
		if b.Energy != v.maxOf(&b) {
			t.Fatalf("body %d begins with energy %v, want its most", b.ID, b.Energy)
		}
	}
	if !v.BudgetLedger().Balanced() {
		t.Fatal("budget ledger does not close for saved stocks")
	}
	run(v, 200)
	if !v.BudgetLedger().Balanced() {
		t.Fatal("budget ledger does not close after running")
	}

	// The same seed saves the same stocks.
	u := newTestWorld(t, 4)
	run(u, 300)
	if !reflect.DeepEqual(u.Stocks(0), w.Stocks(0)) {
		t.Fatal("the same run saved different stocks")
	}
}

// Stocks the config cannot hold are refused: a skill below zero, or any
// skill on a species without skills.
func TestStocksRefused(t *testing.T) {
	cfg := testConfig(2)
	w := newTestWorld(t, 2)
	beast := -1
	for s := 0; s < w.SpeciesCount(); s++ {
		if w.specOf(s).NoSkills {
			beast = s
		}
	}
	if beast < 0 {
		t.Fatal("the test config has no species without skills")
	}
	human := Stock{Genome: w.drawGenome()}
	human.Genome.Species = 1 - beast
	if _, err := NewWorldFromStocks(cfg, testMap(), []Stock{human}); err != nil {
		t.Fatalf("a stock with no skill was refused: %v", err)
	}
	neg := human
	neg.Skill[SkillTent] = -1
	skilledBeast := human
	skilledBeast.Genome.Species = beast
	skilledBeast.Skill[SkillBuild] = 1
	for _, bad := range []Stock{neg, skilledBeast} {
		if _, err := NewWorldFromStocks(cfg, testMap(), []Stock{bad}); err == nil {
			t.Fatalf("stock %+v was accepted", bad)
		}
	}
}
