// Package view draws a world for people to look at: the map, the food, the
// bodies, their recent trails, the history of the counts since tick 0 and the
// last decision of one followed body. It uses the standard image package
// only, so that cmd/client can put the picture in a window and cmd/shot can
// write it to PNG files where there is no screen (ebiten needs one as soon
// as it is imported).
package view

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"sort"
	"strings"

	"github.com/toku463ne/cld_generic_dangeon/engine"
)

// View is what the client shows besides the world itself: the history of
// the counts since tick 0, the recent trail of every body, and the last
// decision of the body it follows. It draws with the standard image package
// only, so that the window and the PNG files of -shot show the same picture.
//
// Nothing here changes the world: the trace it sets draws no random number
// and writes nothing back.
type View struct {
	W     *engine.World
	scale int
	// mapW and MapH are the pixels of the map part; the history panel sits
	// under it.
	mapW, MapH int
	ground     *image.RGBA

	// hist has one sample per tick, from tick 0.
	hist   []sample
	trails map[int64]*trail
	// ShowTrails draws every body's trail; the followed body's is drawn
	// either way.
	ShowTrails bool
	// zoom is an index into ticksPerPixel, for the history panel.
	zoom int

	follow int64 // ID of the followed body, or noBody
	last   *decision

	// drive plays a body, when its ID is set (drive.go).
	drive Driver
}

const (
	noBody = -1
	// trailLen is how many ticks of a body's path are kept: at the default
	// speed, about 75 tiles.
	trailLen = 300
	panelH   = 110
)

// ticksPerPixel are the zooms of the history panel. A column always shows
// the least and the most of its ticks, not a sample of them, so a rhythm
// faster than a column shows as a band instead of a slow false wave.
var ticksPerPixel = []int{1, 4, 16, 64, 256}

type sample struct{ bodies, food int32 }

type trail struct {
	pts  [trailLen][2]float32
	n    int // how many of pts are filled
	next int // where the next point goes
	seen int64
}

func (t *trail) push(x, y float32, tick int64) {
	t.pts[t.next] = [2]float32{x, y}
	t.next = (t.next + 1) % trailLen
	t.n = min(t.n+1, trailLen)
	t.seen = tick
}

// decision is a copy of one traced decision of the followed body. The
// engine reuses the valuation's slices, so they are copied out.
type decision struct {
	tick   int64
	body   engine.Body
	opts   []engine.Action
	risk   []float64 // first window
	child  []float64
	seen   []engine.Food
	plan   []int
	arrive []int
	chosen engine.Action
	why    engine.Trigger
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
	waterColor  = color.RGBA{0x4a, 0x78, 0xb0, 0xff}
	foodColor   = color.RGBA{0xe0, 0x50, 0x40, 0xff}
	trailColor  = color.RGBA{0x30, 0x30, 0x30, 0xff}
	followColor = color.RGBA{0xff, 0xd0, 0x20, 0xff}
	panelColor  = color.RGBA{0x18, 0x18, 0x20, 0xff}
	gridColor   = color.RGBA{0x38, 0x38, 0x48, 0xff}
	bodiesLine  = color.RGBA{0xf0, 0xf0, 0xf0, 0xff}
)

func New(w *engine.World, scale int) *View {
	m := w.Map()
	v := &View{
		W: w, scale: scale,
		mapW: m.Width * scale, MapH: m.Height * scale,
		trails: map[int64]*trail{},
		follow: noBody,
		zoom:   1,
		drive:  Driver{ID: noBody},
	}
	v.ground = image.NewRGBA(image.Rect(0, 0, v.mapW, v.MapH))
	for y := 0; y < m.Height; y++ {
		for x := 0; x < m.Width; x++ {
			c := regionColors[int(m.RegionAt(x, y))%len(regionColors)]
			if m.TerrainAt(x, y) == engine.TerrainWater {
				c = waterColor
			}
			fillRect(v.ground, x*scale, y*scale, scale, scale, c)
		}
	}
	w.SetTrace(v.trace)
	w.SetChooser(&v.drive)
	v.observe(true)
	return v
}

// Size is the picture's size in pixels.
func (v *View) Size() (int, int) { return v.mapW, v.MapH + panelH }

// Step advances the world one tick and records it. Trails are kept only
// when asked, so that a long headless run does not pay for them.
func (v *View) Step(trails bool) {
	v.W.Step()
	v.observe(trails)
}

func (v *View) observe(trails bool) {
	bodies := v.W.Bodies()
	v.hist = append(v.hist, sample{int32(len(bodies)), int32(v.W.FoodLedger().OnGround)})
	if !trails {
		return
	}
	tick := v.W.Tick()
	for _, b := range bodies {
		t := v.trails[b.ID]
		if t == nil {
			t = &trail{}
			v.trails[b.ID] = t
		}
		t.push(float32(b.X), float32(b.Y), tick)
	}
	for id, t := range v.trails {
		if t.seen != tick {
			delete(v.trails, id)
		}
	}
}

func (v *View) trace(b engine.Body, val engine.Valuation, a engine.Action) {
	if b.ID != v.follow {
		return
	}
	d := &decision{
		tick: v.W.Tick(), body: b, chosen: a, why: val.Why,
		opts:   append([]engine.Action(nil), val.Options...),
		seen:   append([]engine.Food(nil), val.Seen...),
		plan:   append([]int(nil), val.Plan...),
		arrive: append([]int(nil), val.Arrive...),
	}
	if len(val.Risk) > 0 {
		d.risk = append([]float64(nil), val.Risk[0]...)
	}
	d.child = append([]float64(nil), val.Child...)
	v.last = d
}

// followID starts following the body with the given ID, or stops with
// noBody.
func (v *View) followID(id int64) {
	v.follow = id
	v.last = nil
	v.drive.ID = noBody
}

// FollowHungriest follows the living body with the least energy.
func (v *View) FollowHungriest() {
	bodies := v.W.Bodies()
	if len(bodies) == 0 {
		v.followID(noBody)
		return
	}
	best := bodies[0]
	for _, b := range bodies[1:] {
		if b.Energy < best.Energy {
			best = b
		}
	}
	v.followID(best.ID)
}

// FollowMedianSpeed follows the living body whose speed is the median, the
// one the player's body is judged by (PARAMETERS.md "速度の分布").
func (v *View) FollowMedianSpeed() {
	bodies := v.W.Bodies()
	if len(bodies) == 0 {
		v.followID(noBody)
		return
	}
	speed := func(b engine.Body) float64 {
		if b.Build.Speed > 0 {
			return b.Build.Speed
		}
		return v.W.Config().Speed
	}
	sort.SliceStable(bodies, func(i, j int) bool {
		si, sj := speed(bodies[i]), speed(bodies[j])
		if si != sj {
			return si < sj
		}
		return bodies[i].ID < bodies[j].ID
	})
	v.followID(bodies[len(bodies)/2].ID)
}

// Pick follows the body nearest to pixel (px, py) of the map, within one
// tile, or stops following if none is that close.
func (v *View) Pick(px, py int) {
	x, y := float64(px)/float64(v.scale), float64(py)/float64(v.scale)
	id, best := int64(noBody), 1.0
	for _, b := range v.W.Bodies() {
		if d := math.Hypot(b.X-x, b.Y-y); d < best {
			id, best = b.ID, d
		}
	}
	v.followID(id)
}

// followed returns the followed body, if it is alive.
func (v *View) followed() (engine.Body, bool) {
	if v.follow == noBody {
		return engine.Body{}, false
	}
	for _, b := range v.W.Bodies() {
		if b.ID == v.follow {
			return b, true
		}
	}
	return engine.Body{}, false
}

// Render draws the world and the history panel into dst, which must be
// v.Size() large.
func (v *View) Render(dst *image.RGBA) {
	copy(dst.Pix[:len(v.ground.Pix)], v.ground.Pix)
	s := float32(v.scale)
	px := func(x float64) int { return int(float32(x) * s) }

	if v.ShowTrails {
		for id, t := range v.trails {
			if id != v.follow {
				v.drawTrail(dst, t, trailColor)
			}
		}
	}
	for _, f := range v.W.Foods() {
		fillRect(dst, f.X*v.scale+v.scale/4, f.Y*v.scale+v.scale/4, v.scale/2, v.scale/2, foodColor)
	}

	fb, following := v.followed()
	if following {
		if t := v.trails[fb.ID]; t != nil {
			v.drawTrail(dst, t, followColor)
		}
		// What it sees.
		r := v.W.Config().Sight
		tx, ty := int(math.Floor(fb.X)), int(math.Floor(fb.Y))
		if r >= 0 {
			strokeRect(dst, (tx-r)*v.scale, (ty-r)*v.scale, (2*r+1)*v.scale, (2*r+1)*v.scale, followColor)
		}
		// Where its last decision planned to eat.
		if d := v.last; d != nil && d.body.ID == fb.ID {
			if j := optionIndex(d.opts, d.chosen); j >= 0 && j < len(d.plan) && d.plan[j] >= 0 {
				f := d.seen[d.plan[j]]
				line(dst, px(fb.X), px(fb.Y), f.X*v.scale+v.scale/2, f.Y*v.scale+v.scale/2, followColor)
			}
		}
	}

	full := v.W.Config().EnergyMax
	rad := max(v.scale/3, 1)
	for _, b := range v.W.Bodies() {
		g := uint8(255 * min(max(b.Energy/full, 0), 1))
		fillCircle(dst, px(b.X), px(b.Y), rad, color.RGBA{g, g, g, 0xff})
	}
	if following {
		strokeCircle(dst, px(fb.X), px(fb.Y), rad+2, followColor)
		if fb.Heading >= 0 {
			a := float64(fb.Heading) * math.Pi / 4
			l := float64(v.scale) * 1.2
			line(dst, px(fb.X), px(fb.Y), px(fb.X)+int(l*math.Cos(a)), px(fb.Y)+int(l*math.Sin(a)), followColor)
		}
	}
	v.renderPanel(dst)
}

func (v *View) drawTrail(dst *image.RGBA, t *trail, c color.RGBA) {
	s := float32(v.scale)
	for i := 0; i < t.n; i++ {
		p := t.pts[i]
		dst.SetRGBA(int(p[0]*s), int(p[1]*s), c)
	}
}

// renderPanel draws the history under the map: bodies in white, food on the
// ground in red, both on one scale from zero, the latest tick at the right
// edge.
// Grid lines mark every 1000 ticks (every 10000 when zoomed far out).
func (v *View) renderPanel(dst *image.RGBA) {
	top := v.MapH
	fillRect(dst, 0, top, v.mapW, panelH, panelColor)
	tpp := ticksPerPixel[v.zoom]
	end := len(v.hist) // one past the latest tick
	top0 := end - v.mapW*tpp
	every := 1000
	if tpp > 16 {
		every = 10000
	}
	ceil := panelCeil(v.hist[max(top0, 0):end])
	yOf := func(n int32) int { return top + panelH - 1 - int(float64(n)*float64(panelH-4)/float64(ceil)) }
	for col := 0; col < v.mapW; col++ {
		lo, hi := top0+col*tpp, top0+(col+1)*tpp
		if hi <= 0 {
			continue
		}
		lo = max(lo, 0)
		if lo/every != (hi-1)/every || lo%every == 0 {
			vline(dst, col, top, top+panelH-1, gridColor)
		}
		bmin, bmax, fmin, fmax := envelope(v.hist[lo:hi])
		vline(dst, col, yOf(fmax), yOf(fmin), foodColor)
		vline(dst, col, yOf(bmax), yOf(bmin), bodiesLine)
	}
}

// panelCeil is the top of the panel's scale: the most of either count in
// the ticks shown, rounded up to a multiple of 50 so that the scale holds
// still while the counts move within it. The bottom is always zero.
func panelCeil(hist []sample) int32 {
	_, bmax, _, fmax := envelope(hist)
	return (max(bmax, fmax, 1) + 49) / 50 * 50
}

// envelope returns the least and most of the bodies and of the food over s.
func envelope(s []sample) (bmin, bmax, fmin, fmax int32) {
	bmin, fmin = math.MaxInt32, math.MaxInt32
	for _, x := range s {
		bmin, bmax = min(bmin, x.bodies), max(bmax, x.bodies)
		fmin, fmax = min(fmin, x.food), max(fmax, x.food)
	}
	return
}

// Status is the first lines of text shown with the picture.
func (v *View) Status() string {
	h := v.hist[len(v.hist)-1]
	tpp := ticksPerPixel[v.zoom]
	return fmt.Sprintf("tick %d  bodies %d  food %d  history: %d ticks (%d/px, grid %s)",
		v.W.Tick(), h.bodies, h.food, v.mapW*tpp, tpp, map[bool]string{true: "10000", false: "1000"}[tpp > 16])
}

// FollowText describes the followed body and its last decision, with the
// Options of least risk first.
func (v *View) FollowText() string {
	if v.follow == noBody {
		return ""
	}
	b, ok := v.followed()
	if !ok {
		return fmt.Sprintf("body #%d is dead", v.follow)
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "body #%d  energy %.1f  age %d  heading %s", b.ID, b.Energy, v.W.Tick()-b.Born, dirName(b.Heading))
	if b.Build.Speed > 0 {
		fmt.Fprintf(&sb, "  speed %.3f  most %.0f", b.Build.Speed, b.Build.EnergyMax)
	}
	if v.drive.ID == b.ID {
		sb.WriteString("  PLAYED")
	}
	if v.W.Config().Recheck > 0 && b.Decided >= 0 {
		intent := actionName(b.Intent)
		if b.Goal >= 0 {
			w := v.mapW / v.scale
			intent += fmt.Sprintf(" to food (%d,%d)", b.Goal%w, b.Goal/w)
		}
		fmt.Fprintf(&sb, "\nintent %s, decided at tick %d", intent, b.Decided)
	}
	d := v.last
	if d == nil || d.body.ID != b.ID || len(d.risk) == 0 {
		return sb.String()
	}
	fmt.Fprintf(&sb, "\ntick %d (%s): took %s, saw %d food", d.tick, engine.TriggerNames[d.why], actionName(d.chosen), len(d.seen))
	idx := make([]int, len(d.opts))
	for i := range idx {
		idx[i] = i
	}
	child := v.W.Config().ChildWorth
	score := func(j int) float64 {
		if j < len(d.child) {
			return d.risk[j] - child*d.child[j]
		}
		return d.risk[j]
	}
	sort.SliceStable(idx, func(i, j int) bool { return score(idx[i]) < score(idx[j]) })
	for n, j := range idx {
		if n == 4 {
			fmt.Fprintf(&sb, "\n  ... %d more", len(idx)-n)
			break
		}
		via := "keep moving in region"
		if d.plan[j] >= 0 {
			f := d.seen[d.plan[j]]
			via = fmt.Sprintf("walk to food (%d,%d) in %d", f.X, f.Y, d.arrive[j])
		}
		if j < len(d.child) && d.child[j] > 0 {
			via += fmt.Sprintf(", child %.0f", d.child[j])
		}
		fmt.Fprintf(&sb, "\n  %-8s risk %.4f  via %s", actionName(d.opts[j]), d.risk[j], via)
	}
	return sb.String()
}

func optionIndex(opts []engine.Action, a engine.Action) int {
	for i, o := range opts {
		if o == a {
			return i
		}
	}
	return -1
}

var dirNames = []string{"E", "SE", "S", "SW", "W", "NW", "N", "NE"}

func dirName(h int) string {
	if h < 0 {
		return "-"
	}
	return dirNames[h]
}

func actionName(a engine.Action) string {
	switch a.Kind {
	case engine.ActWait:
		return "wait"
	case engine.ActEat:
		return "eat"
	case engine.ActMate:
		return fmt.Sprintf("mate #%d", a.Mate)
	}
	return "move " + dirName(a.Dir)
}

func fillRect(dst *image.RGBA, x, y, w, h int, c color.RGBA) {
	r := image.Rect(x, y, x+w, y+h).Intersect(dst.Bounds())
	for yy := r.Min.Y; yy < r.Max.Y; yy++ {
		for xx := r.Min.X; xx < r.Max.X; xx++ {
			dst.SetRGBA(xx, yy, c)
		}
	}
}

func strokeRect(dst *image.RGBA, x, y, w, h int, c color.RGBA) {
	line(dst, x, y, x+w-1, y, c)
	line(dst, x, y+h-1, x+w-1, y+h-1, c)
	vline(dst, x, y, y+h-1, c)
	vline(dst, x+w-1, y, y+h-1, c)
}

func vline(dst *image.RGBA, x, y0, y1 int, c color.RGBA) {
	for y := min(y0, y1); y <= max(y0, y1); y++ {
		dst.SetRGBA(x, y, c)
	}
}

func fillCircle(dst *image.RGBA, cx, cy, r int, c color.RGBA) {
	for y := -r; y <= r; y++ {
		for x := -r; x <= r; x++ {
			if x*x+y*y <= r*r {
				dst.SetRGBA(cx+x, cy+y, c)
			}
		}
	}
}

func strokeCircle(dst *image.RGBA, cx, cy, r int, c color.RGBA) {
	for y := -r; y <= r; y++ {
		for x := -r; x <= r; x++ {
			if d := x*x + y*y; d <= r*r && d > (r-1)*(r-1) {
				dst.SetRGBA(cx+x, cy+y, c)
			}
		}
	}
}

// line draws from (x0, y0) to (x1, y1) (Bresenham).
func line(dst *image.RGBA, x0, y0, x1, y1 int, c color.RGBA) {
	dx, dy := abs(x1-x0), -abs(y1-y0)
	sx, sy := sign(x1-x0), sign(y1-y0)
	e := dx + dy
	for {
		dst.SetRGBA(x0, y0, c)
		if x0 == x1 && y0 == y1 {
			return
		}
		e2 := 2 * e
		if e2 >= dy {
			e += dy
			x0 += sx
		}
		if e2 <= dx {
			e += dx
			y0 += sy
		}
	}
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func sign(x int) int {
	switch {
	case x > 0:
		return 1
	case x < 0:
		return -1
	}
	return 0
}

// ZoomOut shows more ticks in the history panel, ZoomIn fewer.
func (v *View) ZoomOut() { v.zoom = min(v.zoom+1, len(ticksPerPixel)-1) }

// ZoomIn shows fewer ticks in the history panel.
func (v *View) ZoomIn() { v.zoom = max(v.zoom-1, 0) }
