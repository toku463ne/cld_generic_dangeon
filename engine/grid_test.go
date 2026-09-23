package engine

import (
	"math/rand"
	"slices"
	"testing"
)

// The index must hand back a superset of a linear scan's answer, in ascending
// order, whatever the cell size.
func TestGridMatchesLinearScan(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	const width, height = 100.0, 60.0
	points := make([]Point, 700)
	for i := range points {
		// A few points stray outside the world on purpose.
		points[i] = Point{X: rng.Float64()*110 - 5, Y: rng.Float64()*70 - 5}
	}
	for _, cell := range []float64{0, 0.01, 3, 7.5, 1000} {
		g := newSpatialGrid(width, height, cell)
		g.rebuild(points)
		for q := 0; q < 200; q++ {
			x, y := rng.Float64()*width, rng.Float64()*height
			r := rng.Float64() * 20
			got := g.appendNear(nil, x, y, r)
			if !slices.IsSorted(got) {
				t.Fatalf("cell %v: candidates not in ascending order", cell)
			}
			for i, p := range points {
				inside := p.X >= x-r && p.X <= x+r && p.Y >= y-r && p.Y <= y+r
				if inside {
					if _, found := slices.BinarySearch(got, i); !found {
						t.Fatalf("cell %v: point %d at (%v,%v) missing near (%v,%v) r=%v", cell, i, p.X, p.Y, x, y, r)
					}
				}
			}
		}
	}
}

func TestGridCapsCellCount(t *testing.T) {
	g := newSpatialGrid(1e6, 1e6, 1)
	if g.cols*g.rows > maxGridCells {
		t.Fatalf("grid has %d cells, cap is %d", g.cols*g.rows, maxGridCells)
	}
}
