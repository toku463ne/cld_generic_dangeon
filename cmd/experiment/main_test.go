package main

import (
	"bytes"
	"strings"
	"testing"
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
