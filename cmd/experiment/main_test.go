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
	cfg.FoodCap = 0 // everybody starves at the same tick
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
