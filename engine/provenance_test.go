package engine

import "testing"

// Without a way for evidence to pass between bodies, every piece of it is
// its holder's own: none is heard, none is an orphan.
func TestNoHeardEvidenceWithoutPassing(t *testing.T) {
	cfg := testConfig(4)
	cfg.Tell = false
	w, err := NewWorld(cfg, testMap())
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 6; i++ {
		run(w, 500)
		for _, rp := range w.Provenance() {
			if rp.Heard != 0 || rp.Orphan != 0 || rp.Hearers != 0 {
				t.Fatalf("tick %d %s: %+v", w.Tick(), rp.Name, rp)
			}
			if rp.Name == "region 0" && rp.Held == 0 {
				t.Fatalf("tick %d: no evidence held for region 0", w.Tick())
			}
		}
	}
}

// Evidence handed from one body to another counts as heard; once its
// observer dies it is an orphan, and its age is the ticks since that death.
func TestOrphanEvidence(t *testing.T) {
	cfg := testConfig(4)
	cfg.Tell = false // only the evidence handed over below is heard
	w, err := NewWorld(cfg, testMap())
	if err != nil {
		t.Fatal(err)
	}
	run(w, 300)
	a, b := &w.bodies[0], &w.bodies[1]
	aID, bID := a.ID, b.ID
	for len(b.Memory.Regions) < 1 {
		b.Memory.Regions = append(b.Memory.Regions, Tally{})
	}
	r := &b.Memory.Regions[0]
	r.N, r.K = r.N+10, r.K+1
	r.Heard = map[int64]Count{aID: {N: 10, K: 1}}
	r.HN, r.HK = 10, 1
	region0 := func() RowProvenance {
		for _, rp := range w.Provenance() {
			if rp.Name == "region 0" {
				return rp
			}
		}
		t.Fatal("no region 0")
		return RowProvenance{}
	}
	if rp := region0(); rp.Heard != 10 || rp.Orphan != 0 || rp.Hearers == 0 {
		t.Fatalf("observer alive: %+v", rp)
	}
	a.Energy = 0 // starves at the end of the next tick
	keep := func() {
		for i := range w.bodies {
			if w.bodies[i].ID == bID {
				w.bodies[i].Energy = w.cfg.EnergyMax
			}
		}
	}
	keep()
	w.Step()
	died := w.Tick()
	if _, ok := w.died[aID]; !ok {
		t.Fatal("the observer's death was not recorded")
	}
	for i := 0; i < 40; i++ {
		keep()
		w.Step()
	}
	rp := region0()
	if rp.Orphan != 10 || len(rp.Ages) != 1 || rp.Ages[0][0] != float64(w.Tick()-died) || rp.Ages[0][1] != 10 {
		t.Fatalf("40 ticks after the observer died: %+v", rp)
	}
}

// Reading the instrument changes nothing.
func TestProvenanceChangesNothing(t *testing.T) {
	a, b := newTestWorld(t, 9), newTestWorld(t, 9)
	for i := 0; i < 600; i++ {
		a.Step()
		b.Step()
		b.Provenance()
	}
	if a.Fingerprint() != b.Fingerprint() {
		t.Fatal("reading provenance changed the world")
	}
}
