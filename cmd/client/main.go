// Command client shows a world as it runs.
//
// It builds the world from a map, a variant and a seed, runs it headless up
// to -from-tick, and draws it from there on. With the representative seed and
// steady-state tick that cmd/experiment prints, it replays the typical run of
// an experiment; without -from-tick it shows any world from the start.
//
// The picture is drawn by package view; cmd/shot writes the same picture to
// PNG files where there is no screen.
//
// The client imports the engine; the engine knows nothing about drawing.
package main

import (
	"flag"
	"fmt"
	"image"
	"image/color"
	"os"
	"strings"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/toku463ne/cld_generic_dangeon/variant"
	"github.com/toku463ne/cld_generic_dangeon/view"
	"github.com/toku463ne/cld_generic_dangeon/worldmap"
)

func main() {
	var o view.Options
	flag.StringVar(&o.Map, "map", "", "map: "+strings.Join(worldmap.Names, " | ")+" (required)")
	flag.IntVar(&o.Width, "w", 0, "map width in tiles (required)")
	flag.IntVar(&o.Height, "h", 0, "map height in tiles (required)")
	flag.StringVar(&o.Variant, "variant", variant.Base, "variant: "+strings.Join(variant.Names(), " | "))
	flag.Int64Var(&o.Seed, "seed", 1, "seed of the world")
	flag.IntVar(&o.FromTick, "from-tick", 0, "run headless up to this tick, then start drawing")
	flag.IntVar(&o.Scale, "scale", 12, "pixels per tile")
	flag.StringVar(&o.Follow, "follow", "", "body to follow: an ID, or \"hungriest\" at -from-tick")
	flag.Parse()

	start := time.Now()
	v, err := view.Prepare(o)
	if err != nil {
		fmt.Fprintln(os.Stderr, "client:", err)
		os.Exit(2)
	}
	if o.FromTick > 0 {
		fmt.Fprintf(os.Stderr, "ran %d ticks headless in %v\n", o.FromTick, time.Since(start))
	}

	g := newGame(v)
	ebiten.SetWindowSize(g.frame.Bounds().Dx(), g.frame.Bounds().Dy())
	ebiten.SetWindowTitle(fmt.Sprintf("%s seed %d (%s)", o.Map, o.Seed, o.Variant))
	if err := ebiten.RunGame(g); err != nil {
		fmt.Fprintln(os.Stderr, "client:", err)
		os.Exit(1)
	}
}

// speeds are the ticks per frame the viewer can step through.
var speeds = []int{1, 2, 4, 8, 16, 32, 64}

type game struct {
	v      *view.View
	frame  *image.RGBA
	screen *ebiten.Image
	speed  int // index into speeds
	paused bool
}

func newGame(v *view.View) *game {
	w, h := v.Size()
	return &game{v: v, frame: image.NewRGBA(image.Rect(0, 0, w, h)), screen: ebiten.NewImage(w, h)}
}

const keyHelp = "space pause  right step  up/down speed  [ ] history zoom\nclick follow  f hungriest  t trails\nc play the followed body: wasd move  e eat  r mate"

// Update takes the keys and advances the world.
func (g *game) Update() error {
	v := g.v
	switch {
	case inpututil.IsKeyJustPressed(ebiten.KeySpace):
		g.paused = !g.paused
	case inpututil.IsKeyJustPressed(ebiten.KeyArrowUp) && g.speed < len(speeds)-1:
		g.speed++
	case inpututil.IsKeyJustPressed(ebiten.KeyArrowDown) && g.speed > 0:
		g.speed--
	case inpututil.IsKeyJustPressed(ebiten.KeyBracketRight):
		v.ZoomOut()
	case inpututil.IsKeyJustPressed(ebiten.KeyBracketLeft):
		v.ZoomIn()
	case inpututil.IsKeyJustPressed(ebiten.KeyT):
		v.ShowTrails = !v.ShowTrails
	case inpututil.IsKeyJustPressed(ebiten.KeyF):
		v.FollowHungriest()
	case inpututil.IsKeyJustPressed(ebiten.KeyC):
		v.Play()
	case inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft):
		if x, y := ebiten.CursorPosition(); y < v.MapH {
			v.Pick(x, y)
		}
	}
	d := v.Driver()
	d.Dx, d.Dy = held(ebiten.KeyD)-held(ebiten.KeyA), held(ebiten.KeyS)-held(ebiten.KeyW)
	d.Eat, d.Mate = ebiten.IsKeyPressed(ebiten.KeyE), ebiten.IsKeyPressed(ebiten.KeyR)
	switch {
	case !g.paused:
		for i := 0; i < speeds[g.speed]; i++ {
			v.Step(true)
		}
	case inpututil.IsKeyJustPressed(ebiten.KeyArrowRight):
		v.Step(true)
	}
	return nil
}

// held is 1 while key k is held down, 0 otherwise.
func held(k ebiten.Key) int {
	if ebiten.IsKeyPressed(k) {
		return 1
	}
	return 0
}

var textBack = color.RGBA{0, 0, 0, 0xa0}

// Draw paints the view, then the text over the map's top left corner on a
// dark box so that it stays readable over any ground.
func (g *game) Draw(screen *ebiten.Image) {
	g.v.Render(g.frame)
	g.screen.WritePixels(g.frame.Pix)
	screen.DrawImage(g.screen, nil)
	state := fmt.Sprintf("x%d", speeds[g.speed])
	if g.paused {
		state = "paused"
	}
	text := g.v.Status() + "  " + state + "\n" + keyHelp
	if t := g.v.FollowText(); t != "" {
		text += "\n" + t
	}
	lines := strings.Split(text, "\n")
	width := 0
	for _, l := range lines {
		width = max(width, len(l))
	}
	// ebitenutil's debug font is 6 by 16 pixels.
	vector.FillRect(screen, 0, 0, float32(width*6+6), float32(len(lines)*16+2), textBack, false)
	ebitenutil.DebugPrintAt(screen, text, 3, 0)
}

func (g *game) Layout(int, int) (int, int) { return g.v.Size() }
