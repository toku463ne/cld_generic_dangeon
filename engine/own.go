package engine

// Owning things on the ground (stage 6-4c, the user's choice).
//
// Anything that lies on the ground can have an owner: today only food, by
// and by goods and the meat of a beaten foe. An owned thing can be taken
// (eaten, picked up) only by its owner; to every other body it is not
// there - not in sight, not remembered, not evidence, not an option - so
// no valuation plans around what it cannot take. When the owner dies its
// things on the ground lose their owner and anyone may take them (VISION:
// a dead node's belongings can be recovered). Nothing yet makes a thing
// change owner otherwise; taking another's (stealing) and handing over
// (trading) will be rules of their own, written as exceptions to mayTake.
//
// Food that comes back has no owner, and no rule yet gives a thing one.

// noOwner is the owner of a thing nobody owns.
const noOwner int64 = -1

// mayTake reports whether body b may take a thing owned by owner.
func (w *World) mayTake(b *Body, owner int64) bool {
	return owner == noOwner || owner == b.ID
}

// foodFor returns the index of the unit on tile t that body b may take,
// or -1: the food as b perceives it.
func (w *World) foodFor(b *Body, t int) int {
	i := w.foodOn(t)
	if i < 0 || !w.mayTake(b, w.food.owner[i]) {
		return -1
	}
	return i
}

// release gives up the things on the ground body id owned: anyone may take
// them now. Every kind of thing that can be owned lets go here.
func (w *World) release(id int64) {
	for i, o := range w.food.owner {
		if o == id {
			w.food.owner[i] = noOwner
		}
	}
}
