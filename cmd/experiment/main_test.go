package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/toku463ne/cld_generic_dangeon/engine"
	"github.com/toku463ne/cld_generic_dangeon/worldmap"
)

func TestRunRequiresSizes(t *testing.T) {
	var out, log bytes.Buffer
	err := run(&out, &log, options{variants: []string{"base"}, mapName: "flat"}, "cmd")
	if err == nil {
		t.Fatal("run accepted missing -seeds/-ticks/-w/-h")
	}
}

func TestRunRejectsUnknownVariant(t *testing.T) {
	var out, log bytes.Buffer
	o := options{variants: []string{"nope"}, seeds: 1, ticks: 1, mapName: "flat", width: 4, height: 4}
	if err := run(&out, &log, o, "cmd"); err == nil {
		t.Fatal("run accepted an unknown variant")
	}
}

// The report must carry every section of MEASURE.md from 2 to 9, in order,
// and the command that reproduces it.
func TestReportHasEverySection(t *testing.T) {
	var out, log bytes.Buffer
	o := options{variants: []string{"base"}, seeds: 2, seed0: 5, ticks: 10, mapName: "constrained", width: 16, height: 8}
	if err := run(&out, &log, o, "go run ./cmd/experiment -seeds 2"); err != nil {
		t.Fatal(err)
	}
	report := out.String()
	last := -1
	for _, h := range []string{"### 2. 条件", "### 3. 人口推移", "### 4. 出来事の回数", "### 5. 行動の選択割合",
		"### 6. 記憶", "### 7. スキル別伝承数", "### 8. 死因別死亡数", "### 9. 対の差"} {
		i := strings.Index(report, h)
		if i < 0 {
			t.Fatalf("report lacks %q", h)
		}
		if i < last {
			t.Fatalf("%q is out of order", h)
		}
		last = i
	}
	for _, want := range []string{"| コミット |", "go run ./cmd/experiment -seeds 2", "constrained (16x8)", "最初のシード 5"} {
		if !strings.Contains(report, want) {
			t.Fatalf("report lacks %q", want)
		}
	}
}

// A seed that falls to the floor stops, and every checkpoint after it is
// recorded as not surviving, so the per-tick tables average over the seeds
// still alive.
func TestRunOneStopsAtFloor(t *testing.T) {
	cfg := engine.DefaultConfig()
	cfg.FoodCap = 0   // everybody starves at the same tick
	cfg.Breed = false // and no one pays for a child first
	cfg.Allot = false // or holds more than the rest
	m := worldmap.Flat(16, 12)
	const ticks = 2000
	r, err := runOne(cfg, m, ticks, 0)
	if err != nil {
		t.Fatal(err)
	}
	every := ticks / checkpoints
	want := ticks/every + 1
	if len(r.pop) != want || len(r.surviving) != want || len(r.food) != want {
		t.Fatalf("got %d/%d/%d checkpoints, want %d", len(r.pop), len(r.surviving), len(r.food), want)
	}
	starve := int(cfg.EnergyMax / cfg.EnergyBurn)
	if r.fellAt < starve || r.fellAt > starve+1 {
		t.Fatalf("fell at tick %d, want about %d", r.fellAt, starve)
	}
	for i, s := range r.surviving {
		if alive := i*every < r.fellAt; s != alive {
			t.Fatalf("checkpoint %d (tick %d): surviving %v, want %v", i, i*every, s, alive)
		}
	}
	if r.regionOK {
		t.Fatal("region shares recorded although nobody reached the second half")
	}
}

func seriesResult(fellAt int, xs ...int32) result {
	return result{series: xs, fellAt: fellAt}
}

// Collapsed seeds are not candidates, the seed whose own average is closest
// to the experiment's is chosen, and the steady-state tick is the first one
// within a standard deviation of the experiment's average.
func TestChooseReplay(t *testing.T) {
	rs := []result{
		seriesResult(3, 200, 100, 10, 0), // collapsed: ignored even though it passes the average
		seriesResult(-1, 200, 150, 60, 60),
		seriesResult(-1, 200, 120, 100, 96), // the closest
		seriesResult(-1, 200, 200, 180, 180),
	}
	// Over the three candidates: mean 145.5, sd 53.3; own averages 117.5, 129, 190.
	p := chooseReplay(rs, 5)
	if !p.ok || p.seed != 7 {
		t.Fatalf("chose %+v, want seed 7", p)
	}
	if p.steady != 1 {
		t.Fatalf("steady tick %d, want 1 (200 is outside the band, 120 inside)", p.steady)
	}
}

func TestChooseReplayAllCollapsed(t *testing.T) {
	rs := []result{seriesResult(2, 5, 1, 0), seriesResult(1, 5, 0)}
	if p := chooseReplay(rs, 1); p.ok {
		t.Fatalf("chose %+v although every seed collapsed", p)
	}
}

// The report carries the replay command, and it names the map and size the
// experiment ran on.
func TestReportHasReplayCommand(t *testing.T) {
	var out, log bytes.Buffer
	o := options{variants: []string{"base"}, seeds: 2, seed0: 3, ticks: 50, mapName: "flat", width: 16, height: 8}
	if err := run(&out, &log, o, "cmd"); err != nil {
		t.Fatal(err)
	}
	report := out.String()
	for _, want := range []string{"| 代表シード・定常化 tick | シード ", "go run ./cmd/client -map flat -w 16 -h 8 -seed "} {
		if !strings.Contains(report, want) {
			t.Fatalf("report lacks %q:\n%s", want, report)
		}
	}
}
