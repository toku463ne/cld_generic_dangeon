package lineage

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/toku463ne/cld_generic_dangeon/engine"
	"github.com/toku463ne/cld_generic_dangeon/worldmap"
)

// A save written and read back holds the same stocks, and a world begun
// from them has their genomes and skills (stage 13-2).
func TestTrainWriteRead(t *testing.T) {
	cfg := engine.DefaultConfig()
	cfg.Seed = 3
	cfg.Bodies = 40
	m := worldmap.Train(24, 18, "human")
	species, err := Train(cfg, m, 300)
	if err != nil {
		t.Fatal(err)
	}
	if len(species) != 1 || len(species["human"]) == 0 {
		t.Fatalf("a human training stage saved %d species, %d humans", len(species), len(species["human"]))
	}
	f := File{Map: "trainhuman", Width: 24, Height: 18, Variant: "base", Seed: 3, Ticks: 300, Species: species}
	path := filepath.Join(t.TempDir(), "human.json")
	if err := f.Write(path); err != nil {
		t.Fatal(err)
	}
	g, err := Read(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(f, g) {
		t.Fatalf("save read back as %+v, want %+v", g, f)
	}
	st, err := g.Stocks("human")
	if err != nil {
		t.Fatal(err)
	}
	w, err := engine.NewWorldFromStocks(cfg, m, st)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(w.Stocks(0), st) {
		t.Fatal("a world begun from the save does not hand its stocks back")
	}
	if _, err := g.Stocks("beast"); err == nil {
		t.Fatal("a save with no beast gave beast stocks")
	}
}
