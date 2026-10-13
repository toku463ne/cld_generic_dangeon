// Package lineages holds the saves of bred bodies the production maps'
// nests send out (stage 13-3), embedded so that a build for the browser
// carries them: human.json and beast.json, each the living bodies of seed
// 1 of its training stage after 40000 ticks (stage 13-2, cmd/train).
package lineages

import (
	_ "embed"

	"github.com/toku463ne/cld_generic_dangeon/engine"
	"github.com/toku463ne/cld_generic_dangeon/lineage"
)

var (
	//go:embed human.json
	human []byte
	//go:embed beast.json
	beast []byte
)

// Stocks returns the stocks of both saves, by the config's species.
func Stocks(cfg engine.Config) ([]engine.Stock, error) {
	var saves []lineage.File
	for _, b := range [][]byte{human, beast} {
		f, err := lineage.Parse(b)
		if err != nil {
			return nil, err
		}
		saves = append(saves, f)
	}
	return lineage.Stocks(cfg, saves...)
}
