// Command train runs a training stage and saves the stocks of its living
// bodies (stage 13-2): each body's genome and skills, nothing it learned.
// The nests of a production map send bodies out of the save (13-3).
//
//	go run ./cmd/train -map trainhuman -w 64 -h 48 -seed 1 -out lineages/human.json
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/toku463ne/cld_generic_dangeon/lineage"
	"github.com/toku463ne/cld_generic_dangeon/variant"
	"github.com/toku463ne/cld_generic_dangeon/worldmap"
)

func main() {
	var f lineage.File
	var out string
	flag.StringVar(&f.Map, "map", "", "training map: trainhuman | trainbeast (any map of "+strings.Join(worldmap.Names, " | ")+")")
	flag.IntVar(&f.Width, "w", 64, "map width in tiles")
	flag.IntVar(&f.Height, "h", 48, "map height in tiles")
	flag.StringVar(&f.Variant, "variant", variant.Warmed, "variant to breed under")
	flag.Int64Var(&f.Seed, "seed", 1, "seed")
	flag.IntVar(&f.Ticks, "ticks", 40000, "ticks to breed (10 years of 4000 ticks)")
	flag.StringVar(&out, "out", "", "file to write the save to (required)")
	flag.Parse()
	if err := run(f, out); err != nil {
		fmt.Fprintln(os.Stderr, "train:", err)
		os.Exit(2)
	}
}

func run(f lineage.File, out string) error {
	if f.Map == "" || out == "" {
		return fmt.Errorf("-map and -out are required")
	}
	cfg, err := variant.Config(f.Variant, f.Seed)
	if err != nil {
		return err
	}
	m, err := worldmap.Build(f.Map, f.Width, f.Height)
	if err != nil {
		return err
	}
	if f.Species, err = lineage.Train(cfg, m, f.Ticks); err != nil {
		return err
	}
	if c, err := exec.Command("git", "rev-parse", "--short", "HEAD").Output(); err == nil {
		f.Commit = strings.TrimSpace(string(c))
	}
	if err := f.Write(out); err != nil {
		return err
	}
	for name, st := range f.Species {
		fmt.Printf("%s: %d bodies saved to %s\n", name, len(st), out)
	}
	return nil
}
