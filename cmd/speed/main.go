// Command speed answers, in wall-clock seconds, how long the engine takes
// to run a world of a given size: it grows a world, then times a stretch of
// ticks and the first ticks after the grown world is saved and loaded back
// (a player starts from a saved world, and the survival tables are rebuilt
// on the way). Run it with GOMAXPROCS=1 for one core, and built with
// GOOS=js GOARCH=wasm under node for the browser (PARAMETERS.md "性能の上限").
package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/toku463ne/cld_generic_dangeon/engine"
	"github.com/toku463ne/cld_generic_dangeon/variant"
	"github.com/toku463ne/cld_generic_dangeon/worldmap"
)

func main() {
	mapName := flag.String("map", "constrained", "map: "+strings.Join(worldmap.Names, " | "))
	width := flag.Int("w", 64, "map width in tiles")
	height := flag.Int("h", 48, "map height in tiles")
	v := flag.String("variant", variant.Base, "variant")
	seed := flag.Int64("seed", 5, "seed")
	food := flag.Int("food", 0, "food cap, which sets the population (0: the variant's)")
	grow := flag.Int("grow", 20000, "ticks run before timing")
	ticks := flag.Int("ticks", 10000, "ticks timed")
	cold := flag.Int("cold", 1000, "ticks timed after saving and loading the grown world")
	flag.Parse()

	m, err := worldmap.Build(*mapName, *width, *height)
	if err != nil {
		fail(err)
	}
	cfg, err := variant.Config(*v, *seed)
	if err != nil {
		fail(err)
	}
	if *food > 0 {
		cfg.FoodCap = *food
	}
	w, err := engine.NewWorld(cfg, m)
	if err != nil {
		fail(err)
	}
	for i := 0; i < *grow; i++ {
		w.Step()
	}
	var saved bytes.Buffer
	if err := w.Save(&saved); err != nil {
		fail(err)
	}
	bodies, samples := 0.0, 0.0
	start := time.Now()
	for i := 0; i < *ticks; i++ {
		w.Step()
		if i%100 == 0 {
			bodies += float64(len(w.Bodies()))
			samples++
		}
	}
	warm := time.Since(start)
	l, err := engine.Load(&saved)
	if err != nil {
		fail(err)
	}
	start = time.Now()
	for i := 0; i < *cold; i++ {
		l.Step()
	}
	coldT := time.Since(start)
	fmt.Printf("%s %dx%d %s seed %d, GOMAXPROCS=%d %s/%s\n", *mapName, *width, *height, *v, *seed, runtime.GOMAXPROCS(0), runtime.GOOS, runtime.GOARCH)
	fmt.Printf("  %d ticks after %d: %.2f s (%.0f us/tick), mean bodies %.0f\n", *ticks, *grow, warm.Seconds(), warm.Seconds()/float64(*ticks)*1e6, bodies/samples)
	fmt.Printf("  first %d ticks after loading the grown world: %.2f s (%.0f us/tick)\n", *cold, coldT.Seconds(), coldT.Seconds()/float64(*cold)*1e6)
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "speed:", err)
	os.Exit(2)
}
