package engine

import (
	"fmt"
	"math"
)

// Tastes and ornaments (stage 11-4, VISION "好みがあり、特定のアイテムを欲
// しがることがある" and "飾り物"): an ornament has no use, and the one
// exception to valuing everything in the two currencies (NODE.md "値付け
// は2通貨だけ") is that a body may want one for itself. A body is born
// with a taste (Genome.Taste, outside the budget, passed on as boldness
// is), each kind of ornament answers to some tastes (OrnamentSpec), and a
// body that holds no ornament its taste answers to wants one: to it such
// an ornament is worth WantEnergy (the user's choice, 2026-10-12: the want
// is put in energy, the unit every food has in common, so that it stays
// defined when foods differ). One is enough; a second is worth nothing
// more. The want is read only where a thing is weighed against coins or
// against what it costs to make - in buying, selling and asking a price -
// never added to the energy a body reads its survival from: a body does
// not think itself fed by an ornament.
//
// A body born with the skill (CraftBorn) may make an ornament as it falls
// asleep, for CraftEnergy, where what it reads the ornament at is more:
// the kind it wants, if it wants one, or else what a shop it knows pays
// for one, coins read at what they buy. Ornaments are carried
// (OrnamentCarry at most), dropped where their holder dies and taken up
// by a body standing there that has room, sold to a shop and bought there,
// and sold between bodies (stages 11-2c and 11-3). A body that stands at a
// shop selling an ornament it wants buys it there and then, without a
// turn, where the want outweighs the price: the want is weighed against
// coins directly rather than through a valued action, since no option's
// survival reading has room for it.

// OrnamentSpec is a kind of ornament: its name, and the tastes it answers
// to.
type OrnamentSpec struct {
	Name   string
	Tastes []int
}

// ornamentItem is the first sale item (Action.Dir) that is an ornament:
// item ornamentItem+k is an ornament of kind k (sale.go).
const ornamentItem = 1 << 16

// goodOrnament is the first shop good kind that is an ornament: pool
// goodOrnament-k is kind k (shop.go).
const goodOrnament = -10

// ornaments reports whether ornaments are in the world.
func (w *World) ornaments() bool { return len(w.cfg.Ornaments) > 0 && w.cfg.Tastes > 0 }

// tasteOf is body b's taste, -1 for none.
func (w *World) tasteOf(b *Body) int { return b.Taste - 1 }

// likes reports whether body b's taste answers to ornament kind k.
func (w *World) likes(b *Body, k int) bool {
	t := w.tasteOf(b)
	if t < 0 || k < 0 || k >= len(w.cfg.Ornaments) || w.spec(b).NoCoins {
		return false
	}
	for _, x := range w.cfg.Ornaments[k].Tastes {
		if x == t {
			return true
		}
	}
	return false
}

// liked is how many ornaments body b holds that it likes.
func (w *World) liked(b *Body) int {
	n := 0
	for _, k := range b.Ornaments {
		if w.likes(b, k) {
			n++
		}
	}
	return n
}

// wants reports whether body b wants an ornament of kind k: it likes the
// kind, holds none it likes, and has room for one.
func (w *World) wants(b *Body, k int) bool {
	return w.ornaments() && w.likes(b, k) && w.liked(b) == 0 && len(b.Ornaments) < w.cfg.OrnamentCarry
}

// wantGrains is WantEnergy in grains.
func (w *World) wantGrains() int { return w.units(w.cfg.WantEnergy) }

// ornamentLoss is what body b reads parting with one of its ornaments of
// kind k at, in grains: its want, if it is the only one it likes.
func (w *World) ornamentLoss(b *Body, k int) int {
	if w.likes(b, k) && w.liked(b) == 1 {
		return w.wantGrains()
	}
	return 0
}

// heldOrnament is the index in body b's ornaments of one of kind k, -1
// for none.
func heldOrnament(b *Body, k int) int {
	for i, x := range b.Ornaments {
		if x == k {
			return i
		}
	}
	return -1
}

// dropOrnament takes one ornament of kind k out of body b's hold.
func dropOrnament(b *Body, k int) {
	if i := heldOrnament(b, k); i >= 0 {
		b.Ornaments = append(b.Ornaments[:i], b.Ornaments[i+1:]...)
	}
}

// ornamentOffer is the most coins a shop body b knows pays for an
// ornament of kind k, read at what coins buy, in grains.
func (w *World) ornamentOffer(b *Body, k int) float64 {
	best := 0
	for _, i := range b.Memory.Shops {
		if i >= len(w.shops) {
			continue
		}
		for g, gs := range w.shops[i].goods {
			if gs.pool == goodOrnament-k && w.m.Shops[i].Goods[g].Offer > best {
				best = w.m.Shops[i].Goods[g].Offer
			}
		}
	}
	return float64(best) * w.coinWorth(b)
}

// craft has body b, falling asleep (a is ActSleep), make an ornament if
// it has the skill, the room and the energy, and the ornament is worth
// more to it than CraftEnergy: the kind it wants, or else the kind a shop
// it knows pays most for (the first such kind on a tie).
func (w *World) craft(b *Body, a Action) {
	if a.Kind != ActSleep || !w.ornaments() || b.Skill[SkillCraft] <= 0 || b.Mature > w.tick ||
		len(b.Ornaments) >= w.cfg.OrnamentCarry || b.Energy <= w.cfg.CraftEnergy {
		return
	}
	kind, worth := -1, 0.0
	for k := range w.cfg.Ornaments {
		v := w.ornamentOffer(b, k)
		if w.wants(b, k) {
			v = math.Max(v, float64(w.wantGrains()))
		}
		if v > worth {
			kind, worth = k, v
		}
	}
	if kind < 0 || worth <= float64(w.units(w.cfg.CraftEnergy)) {
		return
	}
	b.Energy -= w.cfg.CraftEnergy
	b.Ornaments = append(b.Ornaments, kind)
	w.stats.OrnamentsMade++
}

// GroundOrnament is an ornament lying on a tile.
type GroundOrnament struct {
	Tile int `json:"tile"`
	Kind int `json:"kind"`
}

// dropOrnaments leaves the ornaments of body b, dead, where it lies.
func (w *World) dropOrnaments(b *Body) {
	t := w.tileOf(b.X, b.Y)
	if t < 0 {
		return
	}
	for _, k := range b.Ornaments {
		w.groundOrnaments = append(w.groundOrnaments, GroundOrnament{Tile: t, Kind: k})
	}
	b.Ornaments = nil
}

// pickOrnaments has body b take up the ornaments lying where it stands,
// while it has room: a thing a body can sell or want, and that weighs
// nothing.
func (w *World) pickOrnaments(b *Body) {
	if len(w.groundOrnaments) == 0 || w.spec(b).NoCoins {
		return
	}
	t := w.tileOf(b.X, b.Y)
	kept := w.groundOrnaments[:0]
	for _, g := range w.groundOrnaments {
		if g.Tile == t && len(b.Ornaments) < w.cfg.OrnamentCarry {
			b.Ornaments = append(b.Ornaments, g.Kind)
			w.stats.OrnamentsPicked++
			continue
		}
		kept = append(kept, g)
	}
	w.groundOrnaments = kept
}

// shopOrnaments has body b, at a shop, buy an ornament it wants that the
// shop has on sale, where the want outweighs the price read at what coins
// buy for it: once, without a turn.
func (w *World) shopOrnaments(b *Body) {
	if !w.ornaments() || !w.trades() || b.Coins == 0 {
		return
	}
	for i := range w.shops {
		if !w.inReach(b, i) {
			continue
		}
		for g, gs := range w.shops[i].goods {
			k := goodOrnament - gs.pool
			if gs.pool > goodOrnament || !w.wants(b, k) || !w.onSale(i, g) {
				continue
			}
			price := w.m.Shops[i].Goods[g].Price
			if b.Coins < price || float64(w.wantGrains()) <= float64(price)*w.coinWorth(b) {
				continue
			}
			b.Coins -= price
			w.shops[i].fund += price
			w.shops[i].goods[g].stock--
			b.Ornaments = append(b.Ornaments, k)
			w.stats.OrnamentsBought++
			return
		}
	}
}

// GroundOrnaments returns the ornaments lying on the ground.
func (w *World) GroundOrnaments() []GroundOrnament {
	return append([]GroundOrnament(nil), w.groundOrnaments...)
}

// OrnamentCount counts the ornaments in the world: held, lying, in the
// shops and on their way to them.
func (w *World) OrnamentCount() int {
	n := len(w.groundOrnaments)
	for i := range w.bodies {
		n += len(w.bodies[i].Ornaments)
	}
	for _, s := range w.shops {
		for _, g := range s.goods {
			if g.pool <= goodOrnament {
				n += g.stock
			}
		}
	}
	for _, p := range w.parcels {
		if w.shops[p.Shop].goods[p.Good].pool <= goodOrnament {
			n++
		}
	}
	return n
}

// validOrnaments checks the kinds of ornament against the tastes.
func validOrnaments(cfg Config) error {
	if len(cfg.Ornaments) == 0 {
		return nil
	}
	if cfg.Tastes < 1 || cfg.OrnamentCarry < 1 || cfg.WantEnergy < 0 || cfg.CraftEnergy < 0 {
		return fmt.Errorf("ornaments need Tastes and OrnamentCarry of at least one and WantEnergy and CraftEnergy of at least zero, got %d %d %v %v", cfg.Tastes, cfg.OrnamentCarry, cfg.WantEnergy, cfg.CraftEnergy)
	}
	for _, o := range cfg.Ornaments {
		for _, t := range o.Tastes {
			if t < 0 || t >= cfg.Tastes {
				return fmt.Errorf("ornament %s answers to taste %d of %d", o.Name, t, cfg.Tastes)
			}
		}
	}
	return nil
}

// ornamentNamed is the kind of ornament named name, -1 for none.
func (w *World) ornamentNamed(name string) int {
	if !w.ornaments() {
		return -1
	}
	for k, o := range w.cfg.Ornaments {
		if o.Name == name {
			return k
		}
	}
	return -1
}
