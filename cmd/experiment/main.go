// Command experiment runs variants of the world over a range of seeds and
// prints the report in the form MEASURE.md fixes.
//
// Every variant is a rewrite of engine.Config, never a branch in the code, so
// that all of them are in the same binary and can be paired seed by seed.
//
// Stage 1-0 has no rules yet, so apart from the conditions table the report is
// the fixed columns with no rows.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/toku463ne/cld_generic_dangeon/engine"
	"github.com/toku463ne/cld_generic_dangeon/worldmap"
)

// variants maps a variant name to the rewrite of the config it stands for.
var variants = map[string]func(*engine.Config){
	"base": func(*engine.Config) {},
}

type options struct {
	variants      []string
	seeds         int
	seed0         int64
	ticks         int
	mapName       string
	width, height int
}

func main() {
	var o options
	var names string
	flag.StringVar(&names, "variants", "base", "comma-separated variants; the first is the base")
	// Seeds, ticks and map size have no defaults yet: PLAN.md decides them
	// from measurements, and a made-up default would be quietly used.
	flag.IntVar(&o.seeds, "seeds", 0, "number of seeds (required)")
	flag.Int64Var(&o.seed0, "seed0", 1, "first seed")
	flag.IntVar(&o.ticks, "ticks", 0, "ticks per run (required)")
	flag.StringVar(&o.mapName, "map", "", "map: "+strings.Join(worldmap.Names, " | ")+" (required)")
	flag.IntVar(&o.width, "w", 0, "map width in tiles (required)")
	flag.IntVar(&o.height, "h", 0, "map height in tiles (required)")
	flag.Parse()
	o.variants = strings.Split(names, ",")

	// The reproduction line names the package rather than the binary, whose
	// path under go run is a temporary directory.
	command := strings.Join(append([]string{"go run ./cmd/experiment"}, os.Args[1:]...), " ")
	if err := run(os.Stdout, os.Stderr, o, command); err != nil {
		fmt.Fprintln(os.Stderr, "experiment:", err)
		os.Exit(2)
	}
}

func run(out, log io.Writer, o options, command string) error {
	if o.seeds <= 0 || o.ticks <= 0 || o.width <= 0 || o.height <= 0 || o.mapName == "" {
		return fmt.Errorf("-seeds, -ticks, -map, -w and -h are required")
	}
	for _, v := range o.variants {
		if _, ok := variants[v]; !ok {
			return fmt.Errorf("unknown variant %q (known: %s)", v, strings.Join(knownVariants(), ", "))
		}
	}
	m, err := worldmap.Build(o.mapName, o.width, o.height)
	if err != nil {
		return err
	}

	for _, v := range o.variants {
		for s := 0; s < o.seeds; s++ {
			cfg := engine.Config{Seed: o.seed0 + int64(s)}
			variants[v](&cfg)
			w, err := engine.NewWorld(cfg, m)
			if err != nil {
				return err
			}
			start := time.Now()
			for t := 0; t < o.ticks; t++ {
				w.Step()
			}
			// Wall time goes to the log, not the report: it describes the
			// machine, not the world.
			fmt.Fprintf(log, "%s seed %d: %d ticks in %v\n", v, cfg.Seed, o.ticks, time.Since(start))
		}
	}

	writeReport(out, o, command)
	return nil
}

func knownVariants() []string {
	names := make([]string, 0, len(variants))
	for n := range variants {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// commit names the binary's source: the short hash, with -dirty when tracked
// files have uncommitted changes, or "(none)" outside a repository with
// commits.
func commit() string {
	hash, err := exec.Command("git", "rev-parse", "--short", "HEAD").Output()
	if err != nil {
		return "(none)"
	}
	c := strings.TrimSpace(string(hash))
	status, err := exec.Command("git", "status", "--porcelain", "--untracked-files=no").Output()
	if err != nil || len(strings.TrimSpace(string(status))) > 0 {
		c += "-dirty"
	}
	return c
}

// writeReport prints sections 2 to 9 of MEASURE.md with their fixed columns.
// Section 1 is written by whoever reads the numbers.
func writeReport(out io.Writer, o options, command string) {
	p := func(format string, a ...any) { fmt.Fprintf(out, format, a...) }

	p("### 2. 条件\n\n")
	p("| 項目 | 値 |\n| --- | --- |\n")
	p("| コミット | %s |\n", commit())
	p("| マップ | %s (%dx%d) |\n", o.mapName, o.width, o.height)
	p("| tick 数 | %d |\n", o.ticks)
	p("| シード数 | %d（最初のシード %d） |\n", o.seeds, o.seed0)
	base := o.variants[0]
	p("| 比較対象（base） | %s |\n", base)
	p("| 詳細レポート | |\n\n")
	p("**再現方法**:\n\n```\n%s\n```\n\n", command)

	p("### 3. 人口推移\n\n")
	p("| tick | 人口（全体） | 人間 | 獣 | 鳥 | 魚 |\n| --- | --- | --- | --- | --- | --- |\n\n")

	p("### 4. 出来事の回数\n\n")
	p("| 出来事 | 回数 |\n| --- | --- |\n")
	for _, e := range []string{"交配", "出生", "種族内の戦い", "種族間の戦い", "コインを拾った", "売買", "体力消耗（合計）", "死亡"} {
		p("| %s | |\n", e)
	}
	p("\n")

	p("### 5. 行動の選択割合\n\n")
	p("| 行動 | 割合 |\n| --- | --- |\n\n")

	p("### 6. 記憶\n\n")
	p("| 種別 | 上位10 |\n| --- | --- |\n")
	p("| 合成されていない高頻度記憶 | |\n| 合成された高頻度記憶 | |\n| ノード平均が高得点の中間項 | |\n\n")

	p("### 7. スキル別伝承数\n\n")
	p("| スキル | 伝承された回数 | 保有率 |\n| --- | --- | --- |\n\n")

	p("### 8. 死因別死亡数\n\n")
	p("| 死因 | 回数 | 割合 |\n| --- | --- | --- |\n\n")

	p("### 9. 対の差\n\n")
	if len(o.variants) < 2 {
		p("（比較対象なし）\n")
		return
	}
	p("| 指標 | %s | %s | 差 |\n| --- | --- | --- | --- |\n", base, strings.Join(o.variants[1:], " | "))
}
