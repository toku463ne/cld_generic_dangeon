package engine

import "testing"

// A replayed source must continue exactly where the original was.
func TestReplayContinuesStream(t *testing.T) {
	r, c := newCountingRand(7)
	for i := 0; i < 1000; i++ {
		if i%3 == 0 {
			r.Float64()
		} else {
			r.Intn(100)
		}
	}
	r2, c2 := replayTo(7, c.draws)
	if c2.draws != c.draws {
		t.Fatalf("draws after replay = %d, want %d", c2.draws, c.draws)
	}
	for i := 0; i < 1000; i++ {
		if a, b := r.Int63(), r2.Int63(); a != b {
			t.Fatalf("draw %d after replay: %d, want %d", i, b, a)
		}
	}
}
