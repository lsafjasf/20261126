package main

import (
	"os"
	"strings"
	"testing"
)

func loadSample(t *testing.T) *EDL {
	t.Helper()
	f, err := os.Open("testdata/sample.edl")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	edl, err := ParseEDL(f)
	if err != nil {
		t.Fatal(err)
	}
	if len(edl.Events) != 14 {
		t.Fatalf("期望 14 条事件，实际 %d；警告: %v", len(edl.Events), edl.Warnings)
	}
	return edl
}

func findResult(rs []GroupResult, kind, name string) *GroupResult {
	for i := range rs {
		if rs[i].Kind == kind && rs[i].Name == name {
			return &rs[i]
		}
	}
	return nil
}

func TestInferReels(t *testing.T) {
	rs := Analyze(loadSample(t))

	a := findResult(rs, "卷", "A001")
	if a == nil || a.Best != "25" {
		t.Fatalf("A001 应推断为 25，实际 %+v", a)
	}
	if !strings.Contains(a.Decision, "采用推断帧率") {
		t.Fatalf("A001 应采用推断值: %s", a.Decision)
	}

	b := findResult(rs, "卷", "B002")
	if b == nil || b.Best != "29.97 DF" {
		t.Fatalf("B002 应推断为 29.97 DF，实际 %+v", b)
	}
	if !strings.Contains(b.Decision, "声明") {
		t.Fatalf("B002 有声明，决定应提及声明: %s", b.Decision)
	}

	c := findResult(rs, "卷", "C003")
	if c == nil {
		t.Fatal("缺少 C003 结果")
	}
	if c.Best != "" || !strings.Contains(c.Decision, "不自动修正") {
		t.Fatalf("C003 证据不足，不得自动修正: %+v", c)
	}
}

func TestInferMediaAndRecord(t *testing.T) {
	rs := Analyze(loadSample(t))

	m := findResult(rs, "媒体", "BROLL_B.mov")
	if m == nil || m.Best != "29.97 DF" {
		t.Fatalf("BROLL_B.mov 应推断为 29.97 DF，实际 %+v", m)
	}

	rec := findResult(rs, "时间线", "(记录侧)")
	if rec == nil || rec.Best != "29.97 DF" {
		t.Fatalf("记录侧应推断为 29.97 DF，实际 %+v", rec)
	}
}

func TestDeclaredConflictNotSilentlyAdopted(t *testing.T) {
	text := "TITLE: CONFLICT\n" +
		"001  R001  V  C  01:00:00:00 01:00:01:29 00:00:00:00 00:00:01:29\n" +
		"* FRAME RATE: 25\n"
	edl, err := ParseEDL(strings.NewReader(text))
	if err != nil {
		t.Fatal(err)
	}
	rs := Analyze(edl)
	r := findResult(rs, "卷", "R001")
	if r == nil {
		t.Fatal("缺少 R001 结果")
	}
	if !strings.Contains(r.Decision, "冲突") {
		t.Fatalf("声明与证据冲突应被显式报告: %s", r.Decision)
	}
}

func TestDFBoundaryEvidence(t *testing.T) {
	tc, err := parseTimecode("08:01:00:02")
	if err != nil {
		t.Fatal(err)
	}
	at, dropped := tc.boundaryInfo()
	if !at || dropped {
		t.Fatalf("08:01:00:02 应为丢帧边界上的正证据: at=%v dropped=%v", at, dropped)
	}
	df := Rate{Nominal: 30, Drop: true}
	ndf := Rate{Nominal: 30}
	in, _ := parseTimecode("08:00:59:29")
	out, _ := parseTimecode("08:01:00:02")
	if out.frames(df)-in.frames(df) != 1 {
		t.Fatal("DF 计数跨丢帧边界应为 1 帧")
	}
	if out.frames(ndf)-in.frames(ndf) != 3 {
		t.Fatal("NDF 计数跨边界应为 3 帧")
	}
}
