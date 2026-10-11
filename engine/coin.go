package engine

import (
	"fmt"
	"sort"
)

// Coins (stage 11-2a, VISION "コイン"): a thing with no use of its own,
// weightless, that never rots. Coins are issued onto the ground the way
// bitcoin is: CoinRate a tick at first, halved every CoinHalving ticks,
// and never more than CoinCap in all. Once issued a coin is never lost:
// a body standing on coins takes them up (they take no hand), and a body
// that dies leaves them where it lies for anyone to take (VISION "死んだ
// ノードの持ち物は回収可能"). So the coins issued are always those on the
// ground plus those held (CoinLedger), and the number in the world only
// grows, to the cap. Where they are issued is the map's to say
// (Map.RegionCoin). A species whose row says NoCoins leaves them lying.
// Nothing reads a coin's worth yet: what one buys comes with the stall
// (stage 11-2b), and its worth is learned from that.

// coinState is the coins of the world: those lying on each tile, the
// fraction of a coin owed by the issue so far, and how many have been
// issued.
type coinState struct {
	at     []int32
	due    float64
	issued int64
}

// GroundCoin is the coins lying on a tile.
type GroundCoin struct {
	Tile int `json:"tile"`
	N    int `json:"n"`
}

// CoinLedger is the coins of the world: those issued, and where they are.
type CoinLedger struct {
	Issued, Ground, Held int64
}

// Balanced reports whether every coin issued is on the ground or held.
func (l CoinLedger) Balanced() bool { return l.Issued == l.Ground+l.Held }

// validCoins checks the issue's settings.
func validCoins(cfg Config) error {
	if cfg.Coins && (!(cfg.CoinRate > 0) || cfg.CoinHalving <= 0 || cfg.CoinCap < 0) {
		return fmt.Errorf("Coins needs CoinRate and CoinHalving above zero and CoinCap at least zero, got %v %d %d", cfg.CoinRate, cfg.CoinHalving, cfg.CoinCap)
	}
	return nil
}

// initCoins makes room for the coins on the ground.
func (w *World) initCoins() {
	if w.cfg.Coins {
		w.coins.at = make([]int32, w.m.Width*w.m.Height)
	}
}

// coinRate is the coins issued at tick t: CoinRate halved once for each
// CoinHalving ticks gone (a power of two, exact on every platform).
func (w *World) coinRate(t int64) float64 {
	epoch := (t - 1) / int64(w.cfg.CoinHalving)
	if epoch >= 62 {
		return 0
	}
	return w.cfg.CoinRate / float64(uint64(1)<<epoch)
}

// coinShares is the regions' shares of the coins issued: the map's, or
// those of its food.
func (w *World) coinShares() []float64 {
	if len(w.m.RegionCoin) > 0 {
		return w.m.RegionCoin
	}
	return w.m.RegionFood
}

// issueCoins puts this tick's issue on the ground: each coin on a ground
// tile (land or forest) of a region drawn by the map's shares. Once the
// cap is reached nothing more is owed.
func (w *World) issueCoins() {
	if !w.cfg.Coins {
		return
	}
	c := &w.coins
	if c.issued >= int64(w.cfg.CoinCap) {
		c.due = 0
		return
	}
	c.due += w.coinRate(w.tick)
	for c.due >= 1 && c.issued < int64(w.cfg.CoinCap) {
		t := w.coinTile()
		if t < 0 {
			return // no ground to put it on; it stays owed
		}
		c.at[t]++
		c.issued++
		c.due--
	}
}

// coinTile draws a ground tile for a coin: a region by the shares among
// those with ground, then a tile of it. -1 if no region has any.
func (w *World) coinTile() int {
	shares := w.coinShares()
	sum := 0.0
	for r, s := range shares {
		if w.groundOf(RegionID(r)) > 0 {
			sum += s
		}
	}
	if !(sum > 0) {
		return -1
	}
	x := w.rng.Float64() * sum
	r := -1
	for i, s := range shares {
		if s == 0 || w.groundOf(RegionID(i)) == 0 {
			continue
		}
		r = i
		if x < s {
			break
		}
		x -= s
	}
	land, forest := w.food.regionTiles[TerrainLand][r], w.food.regionTiles[TerrainForest][r]
	i := w.rng.Intn(len(land) + len(forest))
	if i < len(land) {
		return land[i]
	}
	return forest[i-len(land)]
}

// groundOf is the number of ground tiles (land and forest) of region r.
func (w *World) groundOf(r RegionID) int {
	return len(w.food.regionTiles[TerrainLand][r]) + len(w.food.regionTiles[TerrainForest][r])
}

// pickCoins has body b take up the coins lying where it stands.
func (w *World) pickCoins(b *Body) {
	if !w.cfg.Coins || w.spec(b).NoCoins {
		return
	}
	t := w.tileOf(b.X, b.Y)
	if t < 0 || w.coins.at[t] == 0 {
		return
	}
	n := int(w.coins.at[t])
	w.coins.at[t] = 0
	b.Coins += n
	w.stats.CoinsPicked += int64(n)
}

// dropCoins leaves the coins of body b, dead, where it lies.
func (w *World) dropCoins(b *Body) {
	if !w.cfg.Coins || b.Coins == 0 {
		return
	}
	t := w.tileOf(b.X, b.Y)
	if t < 0 {
		return // never: a body stands on the map
	}
	w.coins.at[t] += int32(b.Coins)
	w.stats.CoinsDropped += int64(b.Coins)
	b.Coins = 0
}

// GroundCoins returns the coins lying on the ground, by tile.
func (w *World) GroundCoins() []GroundCoin {
	var g []GroundCoin
	for t, n := range w.coins.at {
		if n > 0 {
			g = append(g, GroundCoin{Tile: t, N: int(n)})
		}
	}
	return g
}

// CoinLedger counts the coins of the world.
func (w *World) CoinLedger() CoinLedger {
	l := CoinLedger{Issued: w.coins.issued}
	for _, n := range w.coins.at {
		l.Ground += int64(n)
	}
	for i := range w.bodies {
		l.Held += int64(w.bodies[i].Coins)
	}
	return l
}

// loadCoins puts saved coins back on the ground.
func (w *World) loadCoins(g []GroundCoin, due float64, issued int64) error {
	w.initCoins()
	if len(g) > 0 && !w.cfg.Coins {
		return fmt.Errorf("snapshot has coins on the ground without Coins")
	}
	sort.Slice(g, func(i, j int) bool { return g[i].Tile < g[j].Tile })
	for _, c := range g {
		if c.Tile < 0 || c.Tile >= len(w.coins.at) || c.N <= 0 {
			return fmt.Errorf("coins %d on tile %d", c.N, c.Tile)
		}
		w.coins.at[c.Tile] += int32(c.N)
	}
	w.coins.due, w.coins.issued = due, issued
	return nil
}
