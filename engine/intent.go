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
	// TriggerSleepy is a tick its sleep is due (ForceAt, stage 7-3d).
	TriggerSleepy
	// TriggerOutside is a tick a chooser decides for the body (chooser.go)
	// and nothing else made it decide.
	TriggerOutside
	// TriggerThreat is a hunter in sight (every tick it is), one going
	// out of sight, or a blow landing (Fear, stage 10-2b).
	TriggerThreat
	// TriggerCall is a call of its kind struck heard (Calls, stage 10-4).
	TriggerCall
	NumTriggers

	// noTrigger is a tick the body follows its intent.
	noTrigger Trigger = -1
)

// TriggerNames name the triggers for reports, in order.
var TriggerNames = [NumTriggers]string{"everytick", "first", "underfoot", "sight", "partner", "step", "blocked", "path", "recheck", "waited", "sleepy", "outside", "threat", "call"}

// turn returns the action body b takes this tick: its intent, or a new
// decision when a trigger is true.
func (w *World) turn(b *Body) Action {
	why := TriggerEveryTick
	taken := w.takes(b)
	w.tell(b)
	if b.Work.Site != 0 {
		// Building is the skill's routine (stage 8-2b).
		a := w.work(b)
		w.howSleep(b, a)
		b.Intent, b.Goal, b.Decided = Action{Kind: ActWait}, -1, w.tick
		return a
	}
	if w.cfg.Recheck > 0 {
		under, saw := w.perceive(b)
		mates := w.sawMates(b)
		threats := w.sawThreats(b)
		w.noteSightings(b) // where the hunters stand, for later (Wary)
		w.noteHunters(b)   // which were a threat to it (Ties, stage 12-3)
		why = w.trigger(b, under, saw, mates, threats)
		b.Under, b.Saw, b.Mates, b.Threats = under, saw, mates, threats
		if why == noTrigger && w.sleepDue(b) {
			why = TriggerSleepy
		}
		if why == noTrigger {
			if !taken {
				return b.Intent
			}
			why = TriggerOutside
		}
	}
	w.stats.Decisions[why]++
	b.Struck, b.Called = false, false
	w.noteSpots(b)
	w.noteShelters(b)
	w.noteShops(b)
	w.valuation.Why = why
	a := w.decide(b)
	if taken {
		a = w.chosen(b, a)
	}
	if w.sleepDue(b) {
		a = w.dueSleep(b, a)
	}
	a = w.settleInstead(b, a)
	w.howSleep(b, a)
	w.craft(b, a)
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
			f := v.unit(v.Plan[j])
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
		under = w.foodFor(b, t) >= 0
	}
	// FNV-1a over the tiles of the food in sight, row by row as inSight
	// lists them, and their count.
	const prime = 1099511628211
	saw = 14695981039346656037
	sight := w.sightOf(b)
	if sight < 0 {
		return under, saw
	}
	bx, by := int(math.Floor(b.X)), int(math.Floor(b.Y))
	n := uint64(0)
	for y := by - sight; y <= by+sight; y++ {
		for x := bx - sight; x <= bx+sight; x++ {
			if w.m.InBounds(x, y) && w.seesFrom(b, bx, by, x, y) {
				if t := w.m.index(x, y); w.foodFor(b, t) >= 0 {
					off := (y-by+sight)*(2*sight+1) + x - bx + sight
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
func (w *World) trigger(b *Body, under bool, saw, mates, threats uint64) Trigger {
	switch {
	case b.Decided < 0:
		return TriggerFirst
	case b.Struck, threats != 0, threats != b.Threats:
		return TriggerThreat // a hunter in sight: decide every tick
	case b.Called:
		return TriggerCall
	case under != b.Under:
		return TriggerUnderfoot
	case saw != b.Saw:
		return TriggerSight
	case mates != b.Mates:
		return TriggerPartner
	case b.Intent.Kind == ActWait, b.Intent.Kind == ActMate, b.Intent.Kind == ActPick, b.Intent.Kind == ActAttack, b.Intent.Kind == ActTrade, b.Intent.Kind == ActBuy, b.Intent.Kind == ActSell, b.Intent.Kind == ActSellTo, b.Intent.Kind == ActBuyFrom, b.Intent.Kind == ActList, b.Intent.Kind == ActBuyListed, b.Intent.Kind == ActHelp:
		return TriggerWaited
	}
	if a := b.Intent; a.Kind == ActMove {
		d := moveDirs[a.Dir]
		speed := w.speedOf(b)
		nx, ny := b.X+d[0]*speed, b.Y+d[1]*speed
		next := w.tileOf(nx, ny)
		if next < 0 || !w.mayEnter(b, next) || w.m.Region[next] != w.m.Region[w.tileOf(b.X, b.Y)] {
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
