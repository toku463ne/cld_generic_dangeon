// Command client shows a world as it runs.
//
// It builds the world from a map, a variant and a seed, runs it headless up
// to -from-tick, and draws it from there on. With the representative seed and
// steady-state tick that cmd/experiment prints, it replays the typical run of
// an experiment; without -from-tick it shows any world from the start.
//
// The client imports the engine; the engine knows nothing about drawing.
package main

import (
	"flag"
	"fmt"
	"image/color"
	"os"
	"strings"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/toku463ne/cld_generic_dangeon/engine"
	"github.com/toku463ne/cld_generic_dangeon/variant"
	"github.com/toku463ne/cld_generic_dangeon/worldmap"
)

func main() {
	var o options
	var scale int
	flag.StringVar(&o.mapName, "map", "", "map: "+strings.Join(worldmap.Names, " | ")+" (required)")
	flag.IntVar(&o.width, "w", 0, "map width in tiles (required)")
	flag.IntVar(&o.height, "h", 0, "map height in tiles (required)")
	flag.StringVar(&o.variant, "variant", variant.Base, "variant: "+strings.Join(variant.Names(), " | "))
	flag.Int64Var(&o.seed, "seed", 1, "seed of the world")
	flag.IntVar(&o.fromTick, "from-tick", 0, "run headless up to this tick, then start drawing")
	flag.IntVar(&scale, "scale", 12, "pixels per tile")
	flag.Parse()

	start := time.Now()
	w, err := prepare(o)
	if err != nil {
		fmt.Fprintln(os.Stderr, "client:", err)
		os.Exit(2)
	}
	if o.fromTick > 0 {
		fmt.Fprintf(os.Stderr, "ran %d ticks headless in %v\n", o.fromTick, time.Since(start))
	}

	g := newGame(w, scale)
	ebiten.SetWindowSize(g.screenW, g.screenH)
	ebiten.SetWindowTitle(fmt.Sprintf("%s seed %d (%s)", o.mapName, o.seed, o.variant))
	if err := ebiten.RunGame(g); err != nil {
		fmt.Fprintln(os.Stderr, "client:", err)
		os.Exit(1)
	}
}

// speeds are the ticks per frame the viewer can step through.
var speeds = []int{1, 2, 4, 8, 16, 32, 64}

type game struct {
	w     *engine.World
	scale int
	// ground is the terrain and region layers, drawn once: they do not
	// change while the world runs.
	ground           *ebiten.Image
	screenW, screenH int
	speed            int // index into speeds
	paused           bool
}

// regionColors tint the land by region, so that the regions of the
// constrained map can be told apart. Water is drawn over them.
var regionColors = []color.RGBA{
	{0x9c, 0xc0, 0x7a, 0xff},
	{0xc4, 0xb8, 0x82, 0xff},
	{0x86, 0xb0, 0x8e, 0xff},
	{0xb8, 0xa8, 0x9a, 0xff},
}

var (
	waterColor = color.RGBA{0x4a, 0x78, 0xb0, 0xff}
	foodColor  = color.RGBA{0xe0, 0x50, 0x40, 0xff}
)

func newGame(w *engine.World, scale int) *game {
	m := w.Map()
	g := &game{w: w, scale: scale, screenW: m.Width * scale, screenH: m.Height * scale}
	g.ground = ebiten.NewImage(g.screenW, g.screenH)
	s := float32(scale)
	for y := 0; y < m.Height; y++ {
		for x := 0; x < m.Width; x++ {
			c := regionColors[int(m.RegionAt(x, y))%len(regionColors)]
			if m.TerrainAt(x, y) == engine.TerrainWater {
				c = waterColor
			}
			vector.FillRect(g.ground, float32(x)*s, float32(y)*s, s, s, c, false)
		}
	}
	return g
}

// Update takes the keys and advances the world: space pauses, the right
// arrow steps one tick while paused, up and down change the speed.
func (g *game) Update() error {
	if inpututil.IsKeyJustPressed(ebiten.KeySpace) {
		g.paused = !g.paused
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyArrowUp) && g.speed < len(speeds)-1 {
		g.speed++
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyArrowDown) && g.speed > 0 {
		g.speed--
	}
	switch {
	case !g.paused:
		for i := 0; i < speeds[g.speed]; i++ {
			g.w.Step()
		}
	case inpututil.IsKeyJustPressed(ebiten.KeyArrowRight):
		g.w.Step()
	}
	return nil
}

// Draw paints the ground, the food and the bodies. A body's shade is its
// energy: white when full, black when about to starve.
func (g *game) Draw(screen *ebiten.Image) {
	screen.DrawImage(g.ground, nil)
	s := float32(g.scale)
	for _, f := range g.w.Foods() {
		vector.FillRect(screen, float32(f.X)*s+s/4, float32(f.Y)*s+s/4, s/2, s/2, foodColor, false)
	}
	full := g.w.Config().EnergyMax
	bodies := g.w.Bodies()
	for _, b := range bodies {
		v := uint8(255 * min(max(b.Energy/full, 0), 1))
		vector.FillCircle(screen, float32(b.X)*s, float32(b.Y)*s, s/3, color.RGBA{v, v, v, 0xff}, true)
	}
	state := fmt.Sprintf("x%d", speeds[g.speed])
	if g.paused {
		state = "paused"
	}
	ebitenutil.DebugPrint(screen, fmt.Sprintf("tick %d  bodies %d  food %d  %s\nspace: pause  right: step  up/down: speed",
		g.w.Tick(), len(bodies), g.w.FoodLedger().OnGround, state))
}

func (g *game) Layout(int, int) (int, int) { return g.screenW, g.screenH }
