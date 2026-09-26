package engine

import "math"

// Deciding on events (stage 1-2e).
//
// With Recheck above zero a body keeps the action it chose as an intent -
// eat, walk to a unit of food it saw, or keep moving - and follows it one
// step a tick without valuing anything. It decides again, through decide and
// nothing else, only on a tick when one of the triggers is true:
//
//	underfoot  whether there is food on its tile changed ("eat" enters or
//	           leaves the options)
//	sight      the food in sight changed
//	partner    the adults in sight changed (with Breed, breed.go)
//	step       the next step of the intent leaves the land or the map, or
//	           enters another region
//	blocked    the next step of the intent is into a tile another body
//	           stands on (with Collide)
//	path       walking to food, the next step no longer takes a tick off
//	           the walk as the valuation counts it (walkTicks): the food
//	           is level with it on one axis, or reached
//	recheck    Recheck ticks have passed since it last decided
//	waited     it waited or asked to mate last tick: neither is an intent
//
// Waiting is a tick not moved, not a plan; so is a mate, which a birth
// ends or the partner's not mating back leaves as a wait. Held as an intent it would
// last until Recheck, since a body standing still sees nothing change.
//
// A trigger says "think again", never what to do (NODE.md): what the body
// does after a trigger is whatever decide values best, as on any tick of
// stage 1-2r. With Recheck zero every tick is a decision, which is that
// stage draw for draw.

// Trigger is why a body decided on a tick.
type Trigger int

const (
	// TriggerEveryTick is every decision when Recheck is zero.
	TriggerEveryTick Trigger = iota
	// TriggerFirst is a body's first decision.
	TriggerFirst
	TriggerUnderfoot
	TriggerSight
	TriggerPartner
	TriggerStep
	TriggerBlocked
	TriggerPath
	TriggerRecheck
	TriggerWaited
	// TriggerOutside is a tick a chooser decides for the body (chooser.go)
	// and nothing else made it decide.
	TriggerOutside
	NumTriggers

	// noTrigger is a tick the body follows its intent.
	noTrigger Trigger = -1
)

// TriggerNames name the triggers for reports, in order.
var TriggerNames = [NumTriggers]string{"everytick", "first", "underfoot", "sight", "partner", "step", "blocked", "path", "recheck", "waited", "outside"}

// turn returns the action body b takes this tick: its intent, or a new
// decision when a trigger is true.
func (w *World) turn(b *Body) Action {
	why := TriggerEveryTick
	taken := w.takes(b)
	if w.cfg.Recheck > 0 {
		under, saw := w.perceive(b)
		mates := w.sawMates(b)
		why = w.trigger(b, under, saw, mates)
		b.Under, b.Saw, b.Mates = under, saw, mates
		if why == noTrigger {
			if !taken {
				return b.Intent
			}
			why = TriggerOutside
		}
	}
	w.stats.Decisions[why]++
	w.valuation.Why = why
	a := w.decide(b)
	if taken {
		a = w.chosen(b, a)
	}
	if w.trace != nil {
		w.trace(*b, w.valuation, a)
	}
	// A birth reads the partner's intent (breed.go), so the intent is kept
	// even when every tick is a decision.
	b.Intent, b.Goal, b.Decided = a, w.goalOf(b, a), w.tick
	return a
}

// goalOf is the tile of the food body b walks to by taking action a: the
// unit the decision under way read a's risk from, if a brings the body a
// tick closer to it. Otherwise, or if a is not a move, it is -1 and the
// intent is to keep on the move. Where food is plentiful every option can
// carry the same risk; the heading then picks a move that may lead away
// from the unit read, and that move is not a walk to it.
func (w *World) goalOf(b *Body, a Action) int {
	v := &w.valuation
	if a.Kind != ActMove {
		return -1
	}
	for j, o := range v.Options {
		if o == a && j < len(v.Plan) && v.Plan[j] >= 0 {
			f := v.Seen[v.Plan[j]]
			if v.Arrive[j] < walkTicks(gapTo(b.X, f.X), gapTo(b.Y, f.Y), w.speedOf(b)) {
				return w.m.index(f.X, f.Y)
			}
		}
	}
	return -1
}

// perceive returns what body b perceives now: whether there is food on its
// tile, and a hash of the food in sight.
func (w *World) perceive(b *Body) (under bool, saw uint64) {
	if t := w.tileOf(b.X, b.Y); t >= 0 {
		under = w.foodOn(t) >= 0
	}
	// FNV-1a over the tiles of the food in sight, row by row as inSight
	// lists them, and their count.
	const prime = 1099511628211
	saw = 14695981039346656037
	if w.cfg.Sight < 0 {
		return under, saw
	}
	bx, by := int(math.Floor(b.X)), int(math.Floor(b.Y))
	n := uint64(0)
	for y := by - w.cfg.Sight; y <= by+w.cfg.Sight; y++ {
		for x := bx - w.cfg.Sight; x <= bx+w.cfg.Sight; x++ {
			if w.m.InBounds(x, y) {
				if t := w.m.index(x, y); w.foodOn(t) >= 0 {
					off := (y-by+w.cfg.Sight)*(2*w.cfg.Sight+1) + x - bx + w.cfg.Sight
					saw = (saw ^ uint64(off)) * prime
					n++
				}
			}
		}
	}
	return under, (saw ^ n) * prime
}

// trigger returns the first trigger true of body b, which perceives under,
// saw and mates now, or noTrigger.
func (w *World) trigger(b *Body, under bool, saw, mates uint64) Trigger {
	switch {
	case b.Decided < 0:
		return TriggerFirst
	case under != b.Under:
		return TriggerUnderfoot
	case saw != b.Saw:
		return TriggerSight
	case mates != b.Mates:
		return TriggerPartner
	case b.Intent.Kind == ActWait, b.Intent.Kind == ActMate:
		return TriggerWaited
	}
	if a := b.Intent; a.Kind == ActMove {
		d := moveDirs[a.Dir]
		speed := w.speedOf(b)
		nx, ny := b.X+d[0]*speed, b.Y+d[1]*speed
		next := w.tileOf(nx, ny)
		if next < 0 || w.m.Terrain[next] != TerrainLand || w.m.Region[next] != w.m.Region[w.tileOf(b.X, b.Y)] {
			return TriggerStep
		}
		if w.blocks(b, nx, ny) {
			return TriggerBlocked
		}
		if b.Goal >= 0 {
			gx, gy := b.Goal%w.m.Width, b.Goal/w.m.Width
			if walkTicks(gapTo(nx, gx), gapTo(ny, gy), speed) >= walkTicks(gapTo(b.X, gx), gapTo(b.Y, gy), speed) {
				return TriggerPath
			}
		}
	}
	if w.tick-b.Decided >= int64(w.cfg.Recheck) {
		return TriggerRecheck
	}
	return noTrigger
}

// octile is the length of the shortest path over dx and dy with moves in the
// eight directions.
func octile(dx, dy float64) float64 {
	return math.Max(dx, dy) + (math.Sqrt2-1)*math.Min(dx, dy)
}
