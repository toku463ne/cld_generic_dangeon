package engine

import (
	"math"
	"math/bits"
)

// This file holds the spatial index: a uniform grid of cells that says what is
// roughly where, so that a question about a neighbourhood does not have to walk
// the whole world. It is ported from the predecessor project.
//
// Nothing about the world's rules lives here, and a change here must be
// invisible in the results: the index narrows the field to a handful of cells
// and the callers keep their own exact distance test. The cell size is not in
// Config for that reason.
//
// The index is a snapshot. It is rebuilt from the points whenever something
// has moved, and a query in the middle of a movement loop would force a
// rebuild on every call: ask before moving, or after, not during.
//
// Stage 1-0 has nothing to index yet. The world starts holding one when
// bodies and food exist (stage 1-1).

// maxGridCells caps the grid so that a tiny cell size cannot ask for millions
// of buckets. Past the cap the cells simply get bigger, which costs speed and
// nothing else.
const maxGridCells = 1 << 14

// Point is a position in world units.
type Point struct {
	X, Y float64
}

// spatialGrid buckets point indices by cell.
type spatialGrid struct {
	cell       float64
	cols, rows int

	buckets [][]int

	// mark is a bitset of "this index is a candidate", which is how a query
	// puts its answer back into ascending order without comparing anything.
	mark []uint64

	// n is how many points the buckets hold, so that a query covering the
	// whole grid can hand back everything without going through them.
	n int
}

func newSpatialGrid(width, height, cell float64) *spatialGrid {
	if !(width > 0) {
		width = 1
	}
	if !(height > 0) {
		height = 1
	}
	// A missing or absurd cell degenerates into a single bucket, which is a
	// linear scan: slower, never wrong.
	if !(cell > 0) {
		cell = math.Max(width, height)
	}
	for {
		// Points on or past the far edge are clamped into the last cell, so
		// the grid only has to cover the world itself.
		cols := max(int(math.Ceil(width/cell)), 1)
		rows := max(int(math.Ceil(height/cell)), 1)
		if cols*rows <= maxGridCells {
			return &spatialGrid{cell: cell, cols: cols, rows: rows, buckets: make([][]int, cols*rows)}
		}
		cell *= 2
	}
}

// column and row clamp a coordinate into the grid, so that a point that has
// strayed outside the world is still found in the nearest edge cell instead of
// quietly disappearing from every query.
func (g *spatialGrid) column(x float64) int {
	return clampInt(int(math.Floor(x/g.cell)), 0, g.cols-1)
}

func (g *spatialGrid) row(y float64) int {
	return clampInt(int(math.Floor(y/g.cell)), 0, g.rows-1)
}

func (g *spatialGrid) cellIndex(x, y float64) int {
	return g.row(y)*g.cols + g.column(x)
}

// rebuild refills every bucket from scratch. The buckets keep their capacity,
// so a steady population stops allocating after the first few rebuilds.
func (g *spatialGrid) rebuild(points []Point) {
	for i := range g.buckets {
		g.buckets[i] = g.buckets[i][:0]
	}
	for i, p := range points {
		c := g.cellIndex(p.X, p.Y)
		g.buckets[c] = append(g.buckets[c], i)
	}
	g.n = len(points)
	for len(g.mark) < (g.n+63)/64 {
		g.mark = append(g.mark, 0)
	}
}

// appendNear appends the indices of every point in the cells the square of
// the given half-width around (x, y) touches. It is a superset of the answer:
// the caller keeps its own distance test.
func (g *spatialGrid) appendNear(dst []int, x, y, radius float64) []int {
	return g.appendInBox(dst, x-radius, y-radius, x+radius, y+radius)
}

// appendInBox collects the candidates in a rectangle.
//
// The indices come back in ascending order, which matters more than it looks:
// once a caller draws from the random source per candidate, the order they are
// visited in is part of what a seed reproduces, and ascending order is the
// order a linear scan uses. The bitset gives that order for one bit each,
// which the predecessor measured to be cheaper than sorting.
func (g *spatialGrid) appendInBox(dst []int, minX, minY, maxX, maxY float64) []int {
	if maxX < minX {
		minX, maxX = maxX, minX
	}
	if maxY < minY {
		minY, maxY = maxY, minY
	}
	c0, c1 := g.column(minX), g.column(maxX)
	r0, r1 := g.row(minY), g.row(maxY)

	// A question that reaches every cell narrows nothing down; handing back
	// everybody is the same answer for less work.
	if c0 == 0 && r0 == 0 && c1 == g.cols-1 && r1 == g.rows-1 {
		for i := 0; i < g.n; i++ {
			dst = append(dst, i)
		}
		return dst
	}

	lo, hi := len(g.mark), -1
	for r := r0; r <= r1; r++ {
		base := r * g.cols
		for c := c0; c <= c1; c++ {
			for _, i := range g.buckets[base+c] {
				word := i >> 6
				g.mark[word] |= 1 << uint(i&63)
				lo = min(lo, word)
				hi = max(hi, word)
			}
		}
	}
	// Reading the bits out in order, and clearing them on the way, leaves the
	// bitset ready for the next query.
	for word := lo; word <= hi; word++ {
		m := g.mark[word]
		if m == 0 {
			continue
		}
		g.mark[word] = 0
		for m != 0 {
			dst = append(dst, word<<6+bits.TrailingZeros64(m))
			m &= m - 1 // clear the lowest set bit
		}
	}
	return dst
}

func clampInt(v, lo, hi int) int {
	if hi < lo {
		return lo
	}
	return min(max(v, lo), hi)
}
