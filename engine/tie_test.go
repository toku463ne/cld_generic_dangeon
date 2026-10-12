package engine

import (
	"math"
	"testing"
)

// A sale leaves each side a row of the other: what it gained, in energy,
// as it read the sale (stage 12-3). Liking is read off the row, aged by
// EvidenceHalfLife; a body keeps TieMemory rows, the weakest dropped.
func TestTies(t *testing.T) {
	cfg := shopConfig(17)
	cfg.Ties = true
	cfg.Bodies = 0
	w, err := NewWorld(cfg, shopMap(5, 100))
	if err != nil {
		t.Fatal(err)
	}
	w.bodies = append(w.bodies, shopper(0, 9.5, 5.5, 0), shopper(1, 10.5, 5.5, 4))
	w.nextID = 2
	w.coins.issued = 4
	w.buildGrid()
	s, p := &w.bodies[0], &w.bodies[1]
	f := w.food.foods[0]
	w.takeFood(int(w.food.foodAt[w.m.index(f.X, f.Y)])-1, s)
	w.rememberShop(s, 0)
	w.act(0, s, Action{Kind: ActSellTo, Mate: 1, Dir: 1})
	if w.Stats().Sales != 1 {
		t.Fatal("no sale")
	}
	g := cfg.Grain
	meal := float64(w.pred.meal)
	// The buyer knows no shop: coins are nothing to it, the meal all gain.
	if got, want := w.Liking(*p, 0), meal*g; got != want {
		t.Fatalf("buyer likes the seller %v, want %v", got, want)
	}
	// The seller read 3 coins at half a meal each, and parted with a meal.
	if got, want := w.Liking(*s, 1), (3*meal/2-meal)*g; math.Abs(got-want) > 1e-9 {
		t.Fatalf("seller likes the buyer %v, want %v", got, want)
	}
	w.tick += int64(cfg.EvidenceHalfLife)
	if got, want := w.Liking(*p, 0), meal*g/2; math.Abs(got-want) > 1e-9 {
		t.Fatalf("after a half life %v, want %v", got, want)
	}
	for id := int64(10); id < int64(10+cfg.TieMemory); id++ {
		w.tie(p, id, 1000)
	}
	if len(p.Memory.Ties) != cfg.TieMemory || w.Liking(*p, 0) != 0 {
		t.Fatalf("%d rows kept, the seller still %v", len(p.Memory.Ties), w.Liking(*p, 0))
	}
}

// With ties, the world is stage 12-3 (re-pinned at 12-4: a body tells a
// body it likes): its fingerprint is pinned here.
func TestTieFingerprint(t *testing.T) {
	const want = uint64(0x6c696bb954910382)
	cfg := ornamentConfig(1)
	cfg.Ties = true
	w, err := NewWorld(cfg, ornamentMap())
	if err != nil {
		t.Fatal(err)
	}
	run(w, 1000)
	if got := w.Fingerprint(); got != want {
		t.Fatalf("fingerprint = %#x, want %#x (ties noted %d)", got, want, w.Stats().TiesNoted)
	}
}

// A body tells a body it likes, as it tells its child, what it knows of
// the world - once for each change to its rows (stage 12-4); a stranger
// in sight hears nothing, and without TellLiked a liked body does not.
func TestTellFriend(t *testing.T) {
	cfg := testConfig(18)
	cfg.Ties, cfg.TellLiked = true, true
	cfg.Bodies = 0
	w, err := NewWorld(cfg, tempMap())
	if err != nil {
		t.Fatal(err)
	}
	mk := func(id int64, x float64) Body {
		return Body{ID: id, X: x, Y: 5.5, Energy: 80, Heading: -1, Goal: -1, Decided: -1, Parents: [2]int64{-1, -1}, Sight: 3, Born: -1500, Mature: -700}
	}
	w.bodies = append(w.bodies, mk(0, 2.5), mk(1, 3.5), mk(2, 4.5))
	w.nextID = 3
	w.buildGrid()
	b, friend, stranger := &w.bodies[0], &w.bodies[1], &w.bodies[2]
	w.rememberShelter(b, w.m.index(1, 1))
	w.tie(b, friend.ID, 30)
	w.tell(b)
	if !friend.Memory.Shelters[w.m.index(1, 1)] || stranger.Memory.Shelters[w.m.index(1, 1)] || w.Stats().FriendsTold != 1 {
		t.Fatalf("friend knows %v, stranger %v, told %d", friend.Memory.Shelters, stranger.Memory.Shelters, w.Stats().FriendsTold)
	}
	w.tell(b)
	if w.Stats().FriendsTold != 1 {
		t.Fatal("told again with nothing new")
	}
	w.cfg.TellLiked = false
	w.rememberShelter(b, w.m.index(2, 1))
	w.tell(b)
	if friend.Memory.Shelters[w.m.index(2, 1)] {
		t.Fatal("told a friend without TellLiked")
	}
}
