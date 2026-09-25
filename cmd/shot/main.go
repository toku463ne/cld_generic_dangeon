// Command shot writes pictures of a world as PNG files, where there is no
// screen to open cmd/client's window on.
//
// It takes the same options as cmd/client, runs the world headless up to
// -from-tick, and then writes -shots pictures, -every ticks apart, each
// with the text cmd/client shows beside it printed to standard output. The
// picture is the same one the window shows (package view).
package main

import (
	"flag"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"

	"github.com/toku463ne/cld_generic_dangeon/variant"
	"github.com/toku463ne/cld_generic_dangeon/view"
	"github.com/toku463ne/cld_generic_dangeon/worldmap"
)

func main() {
	var o view.Options
	var dir string
	var shots, every int
	flag.StringVar(&o.Map, "map", "", "map: "+strings.Join(worldmap.Names, " | ")+" (required)")
	flag.IntVar(&o.Width, "w", 0, "map width in tiles (required)")
	flag.IntVar(&o.Height, "h", 0, "map height in tiles (required)")
	flag.StringVar(&o.Variant, "variant", variant.Base, "variant: "+strings.Join(variant.Names(), " | "))
	flag.Int64Var(&o.Seed, "seed", 1, "seed of the world")
	flag.IntVar(&o.FromTick, "from-tick", 0, "run headless up to this tick, then take the first picture")
	flag.IntVar(&o.Scale, "scale", 12, "pixels per tile")
	flag.StringVar(&o.Follow, "follow", "", "body to follow: an ID, or \"hungriest\" at -from-tick")
	trails := flag.Bool("trails", false, "draw every body's trail")
	flag.StringVar(&dir, "out", "", "directory to write the PNG files into (required)")
	flag.IntVar(&shots, "shots", 1, "how many pictures")
	flag.IntVar(&every, "every", 300, "ticks between pictures")
	flag.Parse()
	if dir == "" {
		fmt.Fprintln(os.Stderr, "shot: -out is required")
		os.Exit(2)
	}

	v, err := view.Prepare(o)
	if err != nil {
		fmt.Fprintln(os.Stderr, "shot:", err)
		os.Exit(2)
	}
	v.ShowTrails = *trails
	prefix := fmt.Sprintf("%s-%s-s%d", o.Map, o.Variant, o.Seed)
	if err := write(v, dir, prefix, shots, every); err != nil {
		fmt.Fprintln(os.Stderr, "shot:", err)
		os.Exit(1)
	}
}

// write writes n pictures, every ticks apart, the first at the current
// tick, and prints the path and text of each.
func write(v *view.View, dir, prefix string, n, every int) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	w, h := v.Size()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < n; i++ {
		if i > 0 {
			for j := 0; j < every; j++ {
				v.Step(true)
			}
		}
		v.Render(img)
		path := filepath.Join(dir, fmt.Sprintf("%s-t%07d.png", prefix, v.W.Tick()))
		f, err := os.Create(path)
		if err != nil {
			return err
		}
		if err := png.Encode(f, img); err != nil {
			f.Close()
			return err
		}
		if err := f.Close(); err != nil {
			return err
		}
		fmt.Println(path)
		fmt.Println(v.Status())
		if t := v.FollowText(); t != "" {
			fmt.Println(t)
		}
		fmt.Println()
	}
	return nil
}
