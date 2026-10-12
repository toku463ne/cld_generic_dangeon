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
	// shades are the ground darkened in steps, drawn as the light falls
	// (stage 7-1): shades[k] at light k/(len-1).
	shades []*image.RGBA

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

	// meetings are the recent meetings at which bodies passed evidence.
	meetings []meeting
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

// A sample is one tick of the history: the bodies, the food on the ground,
// and the share of the living's path evidence whose observers are dead, in
// thousandths (-1 where there is none to read).
type sample struct {
	bodies, food int32
	culture      int16
}

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
	tick  int64
	body  engine.Body
	opts  []engine.Action
	risk  []float64 // first window
	child []float64
	seen  []engine.Food
	// units are what a plan can point to: food seen, then remembered, then
	// a place to sleep (6-1b).
	units []engine.Food
	// kinds name what each of units is.
	kinds  []string
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
	waterColor = color.RGBA{0x4a, 0x78, 0xb0, 0xff}
	foodColor  = color.RGBA{0xe0, 0x50, 0x40, 0xff}
	// foodColors is one colour per kind of food (stage 9-7b-1): the
	// first kind is foodColor; the client tells kinds apart, as bodies do.
	foodColors  = []color.RGBA{foodColor, {0xe0, 0xb0, 0x30, 0xff}, {0x60, 0xc0, 0x60, 0xff}, {0xc0, 0x70, 0xe0, 0xff}}
	trailColor  = color.RGBA{0x30, 0x30, 0x30, 0xff}
	followColor = color.RGBA{0xff, 0xd0, 0x20, 0xff}
	panelColor  = color.RGBA{0x18, 0x18, 0x20, 0xff}
	gridColor   = color.RGBA{0x38, 0x38, 0x48, 0xff}
	bodiesLine  = color.RGBA{0xf0, 0xf0, 0xf0, 0xff}
	meetColor   = color.RGBA{0x40, 0xd0, 0xe0, 0xff}
	// shelterColor is a place to rest (stage 6-1a).
	shelterColor = color.RGBA{0xb0, 0x80, 0x50, 0xff}
	// tentColor marks tents: pitched round a sleeper, folded on the ground,
	// or carried by a body awake (stage 8-2a).
	tentColor = color.RGBA{0xff, 0xd0, 0x30, 0xff}
	// coinColor marks the coins lying on the ground (stage 11-2a).
	coinColor = color.RGBA{0xff, 0xff, 0xff, 0xff}
	// shopColor outlines the map's shops (stage 11-2b).
	shopColor = color.RGBA{0xc0, 0x40, 0xff, 0xff}
	// Sight rings (stage 9-3): a body that drew sight 0 wears a dark
	// ring, one at SightMost a white one; the neutral sight is bare.
	sightLowColor  = color.RGBA{0x10, 0x10, 0x18, 0xff}
	sightHighColor = color.RGBA{0xff, 0xff, 0xff, 0xff}
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
			if m.TerrainAt(x, y) == engine.TerrainForest {
				// Forest is the region's colour, darker and greener
				// (stage 10-2h).
				c = color.RGBA{c.R * 2 / 3, c.G * 4 / 5, c.B * 2 / 3, 0xff}
			}
			if m.TerrainAt(x, y) == engine.TerrainWater {
				c = waterColor
			}
			if m.ShelterAt(x, y) && w.Config().Shelter {
				c = shelterColor
			}
			fillRect(v.ground, x*scale, y*scale, scale, scale, c)
		}
	}
	const steps = 8
	for k := 0; k <= steps; k++ {
		img := image.NewRGBA(v.ground.Rect)
		f := 0.4 + 0.6*float64(k)/steps // dark ground keeps 40% of its colour
		for i, p := range v.ground.Pix {
			if i%4 == 3 {
				img.Pix[i] = p
			} else {
				img.Pix[i] = uint8(float64(p) * f)
			}
		}
		v.shades = append(v.shades, img)
	}
	w.SetTrace(v.trace)
	w.SetChooser(&v.drive)
	w.SetMeet(v.meet)
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
	keep := v.meetings[:0]
	for _, m := range v.meetings {
		if v.W.Tick()-m.tick < meetShown {
			keep = append(keep, m)
		}
	}
	v.meetings = keep
	v.hist = append(v.hist, sample{int32(len(bodies)), int32(v.W.FoodLedger().OnGround), v.culture()})
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

// cultureEvery is how often, in ticks, the history reads where the path
// evidence came from; between reads it repeats the last.
const cultureEvery = 10

// culture reads the share of the living's path evidence whose observers
// are dead, in thousandths, every cultureEvery ticks.
func (v *View) culture() int16 {
	c := v.W.Config()
	if !c.Learn || !c.Tell {
		return -1
	}
	if v.W.Tick()%cultureEvery != 0 && len(v.hist) > 0 {
		return v.hist[len(v.hist)-1].culture
	}
	for _, rp := range v.W.Provenance() {
		if rp.Name == "path" && rp.Held > 0 {
			return int16(math.Round(1000 * rp.Orphan / rp.Held))
		}
	}
	return -1
}

// meeting is two bodies that passed evidence, and when.
type meeting struct {
	a, b int64
	tick int64
}

// meetShown is how many ticks a meeting's line stays on the map.
const meetShown = 30

func (v *View) meet(a, b engine.Body) {
	v.meetings = append(v.meetings, meeting{a.ID, b.ID, v.W.Tick()})
}

func (v *View) trace(b engine.Body, val engine.Valuation, a engine.Action) {
	if b.ID != v.follow {
		return
	}
	d := &decision{
		tick: v.W.Tick(), body: b, chosen: a, why: val.Why,
		opts:   append([]engine.Action(nil), val.Options...),
		seen:   append([]engine.Food(nil), val.Seen...),
		units:  append(append(append([]engine.Food(nil), val.Seen...), val.Recalled...), val.Shelter...),
		kinds:  unitKinds(val),
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
	ground := v.shades[int(math.Round(v.W.Brightness()*float64(len(v.shades)-1)))]
	copy(dst.Pix[:len(ground.Pix)], ground.Pix)
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
		fillRect(dst, f.X*v.scale+v.scale/4, f.Y*v.scale+v.scale/4, v.scale/2, v.scale/2, foodColors[v.W.KindOf(f)%len(foodColors)])
	}

	fb, following := v.followed()
	if following {
		if t := v.trails[fb.ID]; t != nil {
			v.drawTrail(dst, t, followColor)
		}
		// What it sees.
		r := v.W.BodySight(fb)
		tx, ty := int(math.Floor(fb.X)), int(math.Floor(fb.Y))
		if r >= 0 {
			strokeRect(dst, (tx-r)*v.scale, (ty-r)*v.scale, (2*r+1)*v.scale, (2*r+1)*v.scale, followColor)
		}
		// Where its last decision planned to eat.
		if d := v.last; d != nil && d.body.ID == fb.ID {
			if j := optionIndex(d.opts, d.chosen); j >= 0 && j < len(d.plan) && d.plan[j] >= 0 {
				f := d.units[d.plan[j]]
				line(dst, px(fb.X), px(fb.Y), f.X*v.scale+v.scale/2, f.Y*v.scale+v.scale/2, followColor)
			}
		}
	}

	// Meetings at which bodies passed evidence, lately.
	if len(v.meetings) > 0 {
		at := map[int64][2]float64{}
		for _, b := range v.W.Bodies() {
			at[b.ID] = [2]float64{b.X, b.Y}
		}
		for _, m := range v.meetings {
			pa, oka := at[m.a]
			pb, okb := at[m.b]
			if oka && okb {
				line(dst, px(pa[0]), px(pa[1]), px(pb[0]), px(pb[1]), meetColor)
			}
		}
	}

	full := v.W.Config().EnergyMax
	rad := max(v.scale/3, 1)
	now := v.W.Tick()
	// Places built (stage 8-1) change while the world runs, so they are
	// drawn over the ground, not into it.
	m := v.W.Map()
	for _, p := range v.W.Places() {
		for _, t := range p.Tiles {
			strokeRect(dst, (t%m.Width)*v.scale, (t/m.Width)*v.scale, v.scale, v.scale, shelterColor)
		}
	}
	// Tents lying folded on the ground: a small square.
	for _, g := range v.W.GroundTents() {
		s := max(v.scale/3, 1)
		fillRect(dst, (g.Tile%m.Width)*v.scale+v.scale/2-s/2, (g.Tile/m.Width)*v.scale+v.scale/2-s/2, s, s, tentColor)
	}
	// The map's shops: the tile filled purple, and the tiles a body buys
	// from (the shop's and those next to it) outlined.
	for _, s := range m.Shops {
		fillRect(dst, s.X*v.scale, s.Y*v.scale, v.scale, v.scale, shopColor)
		strokeRect(dst, (s.X-1)*v.scale, (s.Y-1)*v.scale, 3*v.scale, 3*v.scale, shopColor)
	}
	// Ornaments lying on the ground: a purple dot in the tile's other
	// corner (stage 11-4).
	for _, g := range v.W.GroundOrnaments() {
		s := max(v.scale/5, 1)
		fillRect(dst, (g.Tile%m.Width)*v.scale+v.scale-1-s, (g.Tile/m.Width)*v.scale+1, s, s, shopColor)
	}
	// Coins lying on the ground: a white dot in the tile's corner.
	for _, g := range v.W.GroundCoins() {
		s := max(v.scale/5, 1)
		fillRect(dst, (g.Tile%m.Width)*v.scale+1, (g.Tile/m.Width)*v.scale+1, s, s, coinColor)
	}
	for _, b := range v.W.Bodies() {
		switch {
		case b.Tent.Pitched:
			// A pitched tent: a triangle round its sleeper.
			x, y, h := px(b.X), px(b.Y), v.scale/2
			line(dst, x-h, y+h, x+h, y+h, tentColor)
			line(dst, x-h, y+h, x, y-h, tentColor)
			line(dst, x+h, y+h, x, y-h, tentColor)
		case b.Tent.Has:
			// A tent carried: a mark above the body.
			line(dst, px(b.X)-rad, px(b.Y)-rad-2, px(b.X)+rad, px(b.Y)-rad-2, tentColor)
		}
		if cfg := v.W.Config(); cfg.AllotSight {
			// The sight ring follows the body's shape: a square round a
			// square, so that a beast never reads as round.
			ring := func(c color.RGBA) {
				if b.Species == 1 {
					strokeRect(dst, px(b.X)-rad-2, px(b.Y)-rad-2, 2*rad+5, 2*rad+5, c)
				} else {
					strokeCircle(dst, px(b.X), px(b.Y), rad+2, c)
				}
			}
			switch v.W.BodySight(b) {
			case 0:
				ring(sightLowColor)
			case cfg.SightMost:
				ring(sightHighColor)
			}
		}
		if cfg := v.W.Config(); cfg.AllotFertility {
			// Fertility (stage 9-4): a bar under the body, dark at level
			// 0 and white at FertMost; the neutral level is bare.
			switch v.W.BodyFertility(b) {
			case 0:
				line(dst, px(b.X)-rad, px(b.Y)+rad+2, px(b.X)+rad, px(b.Y)+rad+2, sightLowColor)
			case cfg.FertMost:
				line(dst, px(b.X)-rad, px(b.Y)+rad+2, px(b.X)+rad, px(b.Y)+rad+2, sightHighColor)
			}
		}
		c := bodyColor(b, now, b.Energy/full)
		if b.Species >= 2 {
			// The third species (fish) is a flat bar (stage 9-7b-2c).
			if b.Asleep {
				strokeRect(dst, px(b.X)-rad-1, px(b.Y)-rad/2, 2*rad+3, rad+1, c)
			} else {
				fillRect(dst, px(b.X)-rad-1, px(b.Y)-rad/2, 2*rad+3, rad+1, c)
			}
			continue
		}
		if b.Species != 0 {
			// Any species but the first is a square of one hue, its sex
			// not shown (stage 9-7a; the user's choice 2026-10-11, so
			// that a beast is never taken for a human); a sleeper is
			// hollow either way.
			c = beastColor(b.Energy / full)
			if b.Asleep {
				strokeRect(dst, px(b.X)-rad, px(b.Y)-rad, 2*rad+1, 2*rad+1, c)
			} else {
				fillRect(dst, px(b.X)-rad, px(b.Y)-rad, 2*rad+1, 2*rad+1, c)
			}
			continue
		}
		// A human is a triangle: a woman's points up, a man's down (the
		// user's choice 2026-10-11); a sleeper is hollow.
		up := b.Sex != engine.Male
		if b.Asleep {
			strokeTriangle(dst, px(b.X), px(b.Y), rad+1, up, c)
			continue
		}
		fillTriangle(dst, px(b.X), px(b.Y), rad+1, up, c)
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

// Body hues: a child's until it comes of age, then a woman's or a man's.
// A world without sexes draws bodies grey.
var (
	childHue  = color.RGBA{0x70, 0xe0, 0x70, 0xff}
	femaleHue = color.RGBA{0xff, 0x70, 0xb0, 0xff}
	maleHue   = color.RGBA{0x60, 0xa0, 0xff, 0xff}
	greyHue   = color.RGBA{0xff, 0xff, 0xff, 0xff}
)

// beastHue is every body's of the species past the first: one hue, as
// their sex is not shown.
var beastHue = color.RGBA{0xe8, 0xb0, 0x60, 0xff}

// beastColor is beastHue darker the less energy the body has (full).
func beastColor(full float64) color.RGBA {
	f := 0.2 + 0.8*min(max(full, 0), 1)
	return color.RGBA{uint8(float64(beastHue.R) * f), uint8(float64(beastHue.G) * f), uint8(float64(beastHue.B) * f), 0xff}
}

// bodyColor is body b's hue at tick now, darker the less energy it has
// (full, its energy over the config's most): the brightness still reads
// as how near starving it is.
func bodyColor(b engine.Body, now int64, full float64) color.RGBA {
	hue := greyHue
	switch {
	case b.Sex == engine.NoSex:
	case now < b.Mature:
		hue = childHue
	case b.Sex == engine.Female:
		hue = femaleHue
	default:
		hue = maleHue
	}
	f := min(max(full, 0), 1)
	if hue != greyHue {
		f = 0.2 + 0.8*f // a starving body keeps a trace of its hue
	}
	return color.RGBA{uint8(float64(hue.R) * f), uint8(float64(hue.G) * f), uint8(float64(hue.B) * f), 0xff}
}

// sexName names body b's sex and stage at tick now, for the status line.
func sexName(b engine.Body, now int64) string {
	switch {
	case b.Sex == engine.NoSex:
		return ""
	case now < b.Mature:
		return "  child"
	case b.Sex == engine.Female:
		return "  female"
	}
	return "  male"
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
	yShare := yShareAt(top)
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
		if cmin, cmax, ok := cultureEnvelope(v.hist[lo:hi]); ok {
			vline(dst, col, yShare(cmax), yShare(cmin), meetColor)
		}
	}
}

// yShare places a share in thousandths on the panel: 0 at the bottom, all
// at the top.
func yShareAt(top int) func(int16) int {
	return func(c int16) int { return top + panelH - 1 - int(float64(c)*float64(panelH-4)/1000) }
}

// cultureEnvelope is the least and most of the culture share over s, if
// any tick of it has one.
func cultureEnvelope(s []sample) (lo, hi int16, ok bool) {
	lo, hi = math.MaxInt16, -1
	for _, x := range s {
		if x.culture < 0 {
			continue
		}
		lo, hi, ok = min(lo, x.culture), max(hi, x.culture), true
	}
	return
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
	culture := ""
	if h.culture >= 0 {
		culture = fmt.Sprintf("  path knowledge from the dead %.0f%%", float64(h.culture)/10)
	}
	night := ""
	if l := v.W.Brightness(); l < 1 {
		night = fmt.Sprintf("  light %.2f", l)
	}
	if lo, hi, ok := v.temps(); ok {
		if lo == hi {
			night += fmt.Sprintf("  %.1f°", lo)
		} else {
			night += fmt.Sprintf("  %.1f-%.1f°", lo, hi)
		}
	}
	return fmt.Sprintf("tick %d%s  bodies %d  food %d%s  history: %d ticks (%d/px, grid %s)",
		v.W.Tick(), night, h.bodies, h.food, culture, v.mapW*tpp, tpp, map[bool]string{true: "10000", false: "1000"}[tpp > 16])
}

// ShopStatus is a line on each of the map's shops as it stands (stage
// 11-2): each good's stock over its cap, the coins it holds over its cap,
// and the things on their way to it. Empty without shops.
func (v *View) ShopStatus() string {
	m := v.W.Map()
	shops := v.W.Shops()
	if len(shops) == 0 {
		return ""
	}
	coming := make([]int, len(shops))
	for _, p := range v.W.Parcels() {
		coming[p.Shop]++
	}
	var sb strings.Builder
	for i, s := range shops {
		if i > 0 {
			sb.WriteString("  ")
		}
		fmt.Fprintf(&sb, "shop %d:", i)
		for g, gs := range s.Goods {
			name := m.Shops[i].Goods[g].Item
			if strings.HasSuffix(name, " food") {
				name = "food"
			}
			fmt.Fprintf(&sb, " %s %d/%d", name, gs.Stock, m.Shops[i].Goods[g].StockCap)
		}
		fmt.Fprintf(&sb, "  fund %d/%d  on the way %d", s.Fund, m.Shops[i].FundCap, coming[i])
	}
	return sb.String()
}

// temps is the lowest and highest temperature over the regions now, and ok
// false where the temperature touches no burn.
func (v *View) temps() (lo, hi float64, ok bool) {
	if !v.W.Config().Temperature {
		return 0, 0, false
	}
	m := v.W.Map()
	for r := range m.RegionTemp {
		t, _ := m.Temp(engine.RegionID(r), v.W.Brightness())
		if !ok || t < lo {
			lo = t
		}
		if !ok || t > hi {
			hi = t
		}
		ok = true
	}
	return lo, hi, ok
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
	fmt.Fprintf(&sb, "%s #%d%s  energy %.1f  age %d  sight %d  fert %d  bold %d  heading %s", v.W.SpeciesName(b.Species), b.ID, sexName(b, v.W.Tick()), b.Energy, v.W.Tick()-b.Born, v.W.BodySight(b), v.W.BodyFertility(b), v.W.BodyBold(b), dirName(b.Heading))
	if v.W.Config().Sleep {
		state := "awake"
		if b.Asleep {
			state = fmt.Sprintf("asleep until %d", b.Woke)
		}
		fmt.Fprintf(&sb, "  %s  pressure %.2f  debt %.0f", state, b.Pressure, b.Debt)
	}
	if temp, ok := v.W.TempAt(int(math.Floor(b.X)), int(math.Floor(b.Y))); ok && v.W.Config().Temperature {
		fmt.Fprintf(&sb, "  %.1f°  burn x%.2f", temp, v.W.TempBurn(b))
	}
	if b.Tent.Has {
		state := "carried"
		if b.Tent.Pitched {
			state = "pitched"
		}
		fmt.Fprintf(&sb, "  tent %s, %d nights, wears out at %d", state, b.Tent.Uses, b.Tent.Until)
	}
	if b.Coins > 0 {
		fmt.Fprintf(&sb, "  coins %d", b.Coins)
	}
	if cfg := v.W.Config(); b.Taste > 0 {
		fmt.Fprintf(&sb, "  taste %d", b.Taste-1)
		for _, k := range b.Ornaments {
			fmt.Fprintf(&sb, " %s", cfg.Ornaments[k].Name)
		}
	}
	if warm := b.Memory.Warm; warm[1] > 0 {
		fmt.Fprintf(&sb, "  knows %d shelter tiles, warm x%.2f", len(b.Memory.Shelters), warm[0]/warm[1])
	}
	if toDark, toLight, ok := v.W.DuskAhead(b); ok {
		fmt.Fprintf(&sb, "  dark in ~%.0f, light in ~%.0f", toDark, toLight)
	}
	if b.Build.Speed > 0 {
		fmt.Fprintf(&sb, "  speed %.3f  most %.0f", b.Build.Speed, b.Build.EnergyMax)
	}
	if v.drive.ID == b.ID {
		sb.WriteString("  PLAYED")
	}
	if v.W.Config().Learn {
		sb.WriteString(v.beliefText(b))
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
			f := d.units[d.plan[j]]
			via = fmt.Sprintf("walk to %s (%d,%d) in %d", d.kinds[d.plan[j]], f.X, f.Y, d.arrive[j])
		}
		if j < len(d.child) && d.child[j] > 0 {
			via += fmt.Sprintf(", child %.0f", d.child[j])
		}
		fmt.Fprintf(&sb, "\n  %-8s risk %.4f  via %s", actionName(d.opts[j]), d.risk[j], via)
	}
	return sb.String()
}

// beliefText is the followed body's beliefs: each estimate, the evidence
// behind it, how far it still leans on its parent, and for the path how
// much of its evidence came from other bodies and from dead ones.
func (v *View) beliefText(b engine.Body) string {
	c := v.W.Config()
	bl := v.W.Belief(b)
	pull := func(weight, n float64) float64 { return 100 * weight / (weight + n) }
	path := b.Memory.Path
	// Evidence in tiles: the stable path row counts what the region was
	// believed to hold, a fraction of a tile each.
	tiles, weight := path.N, c.PathWeight
	if c.StableRows && bl.Here > 0 {
		tiles, weight = path.N/bl.Here, c.PathWeight*bl.Here
	}
	heard, dead := 0.0, 0.0
	if path.N > 0 {
		alive := map[int64]bool{}
		for _, o := range v.W.Bodies() {
			alive[o.ID] = true
		}
		scale := path.S
		if scale == 0 {
			scale = 1
		}
		for _, h := range path.Heard {
			heard += scale * h.N
			if !alive[h.ID] {
				dead += scale * h.N
			}
		}
		heard, dead = 100*heard/path.N, 100*dead/path.N
	}
	return fmt.Sprintf("\nbelieves: food/tile here %.4f (%.0f tiles, %.0f%% parent), land %.4f"+
		"\n  walked tiles hold x%.2f of it (evidence ~%.0f tiles, %.0f%% parent; %.0f%% heard, %.0f%% from the dead)"+
		"\n  a mate makes a child %.2f (%.0f asks, %.0f%% prior)",
		bl.Here, bl.HereN, pull(c.RegionWeight, bl.HereN), bl.Land,
		bl.PathRatio, tiles, pull(weight, path.N), heard, dead,
		bl.Child, bl.Asks, pull(c.ChildWeight, bl.Asks))
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
	case engine.ActPick:
		return "pick"
	case engine.ActMate:
		return fmt.Sprintf("mate #%d", a.Mate)
	case engine.ActAttack:
		return fmt.Sprintf("attack #%d", a.Mate)
	case engine.ActTrade:
		return fmt.Sprintf("trade #%d", a.Mate)
	case engine.ActBuy:
		return fmt.Sprintf("buy good %d at shop %d", a.Dir, a.Mate)
	case engine.ActSellTo:
		return fmt.Sprintf("sell item %d to #%d", a.Dir, a.Mate)
	case engine.ActSell:
		return fmt.Sprintf("send good %d to shop %d", a.Dir, a.Mate)
	case engine.ActSleep:
		return "sleep"
	case engine.ActBuild:
		return "build"
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

// fillTriangle fills a triangle r pixels from its centre to its apex and
// base, pointing up or down.
func fillTriangle(dst *image.RGBA, cx, cy, r int, up bool, c color.RGBA) {
	for y := -r; y <= r; y++ {
		// The half-width grows from the apex to the base.
		t := y + r
		if !up {
			t = r - y
		}
		h := r * t / (2 * r)
		for x := -h; x <= h; x++ {
			dst.SetRGBA(cx+x, cy+y, c)
		}
	}
}

// strokeTriangle draws the outline of the triangle fillTriangle fills.
func strokeTriangle(dst *image.RGBA, cx, cy, r int, up bool, c color.RGBA) {
	ax, ay, by := cx, cy-r, cy+r
	if !up {
		ay, by = cy+r, cy-r
	}
	line(dst, ax, ay, cx-r, by, c)
	line(dst, ax, ay, cx+r, by, c)
	line(dst, cx-r, by, cx+r, by, c)
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

// unitKinds names what each unit a plan can point to is, in the order of
// units: food seen, remembered, a place to sleep.
func unitKinds(v engine.Valuation) []string {
	var ks []string
	for _, n := range []struct {
		k string
		n int
	}{{"food", len(v.Seen)}, {"remembered food", len(v.Recalled)}, {"a place to sleep", len(v.Shelter)}} {
		for i := 0; i < n.n; i++ {
			ks = append(ks, n.k)
		}
	}
	return ks
}
