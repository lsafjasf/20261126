package timeline

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestSpeedRationalMapping(t *testing.T) {
	s := Speed{Num: 3, Den: 2}
	cases := map[int64]int64{0: 0, 1: 1, 2: 3, 3: 4, 4: 6, 5: 7}
	for in, want := range cases {
		got, err := s.Map(in)
		if err != nil || got != want {
			t.Fatalf("Map(%d)=%d,%v want %d", in, got, err, want)
		}
	}
	if got := (Speed{Num: 6, Den: 4}).Normalize(); got != (Speed{Num: 3, Den: 2}) {
		t.Fatalf("normalize = %v", got)
	}
	prod := (Speed{Num: 2, Den: 3}).Mul(Speed{Num: 3, Den: 2})
	if prod != SpeedOne {
		t.Fatalf("2/3 * 3/2 = %v want 1", prod)
	}
}

func TestStagedFloorNotAssociative(t *testing.T) {
	// floor(floor(t * 2/3) * 3/2) 与 floor(t * 1) 不同，这就是漂移根源。
	outer := Speed{Num: 2, Den: 3}
	inner := Speed{Num: 3, Den: 2}
	at := int64(4)
	stage1, _ := outer.Map(at)
	stage2, _ := inner.Map(stage1)
	naive, _ := outer.Mul(inner).Map(at)
	if stage2 == naive {
		t.Fatalf("expected divergence at t=%d, both %d", at, stage2)
	}
}

func TestDemoResolveAndFreeze(t *testing.T) {
	p := DemoProject()
	r, err := NewResolver(p)
	if err != nil {
		t.Fatal(err)
	}

	// 根级冻结段（根帧 8、9）：源帧恒为 77，内部速率固定为 1，
	// 外部速率为 1（不参与任何变速乘积）。
	for _, f := range []int64{8, 9} {
		rec, err := r.Frame(f)
		if err != nil {
			t.Fatal(err)
		}
		if rec.Kind != FrameFreeze || rec.SourceFrame != 77 || rec.InternalSpeed != SpeedOne {
			t.Fatalf("frame %d freeze record wrong: %+v", f, rec)
		}
		if rec.ExternalSpeed != SpeedOne {
			t.Fatalf("freeze external speed = %v want 1", rec.ExternalSpeed)
		}
	}

}

func TestNestedFreezeAndGap(t *testing.T) {
	p := &Project{
		Sources: map[string]*Source{"s": {ID: "s", Frames: 50}},
		Compounds: map[string]*Compound{
			"c": {
				ID: "c", Dur: 4,
				Items: []*Item{
					{ID: "c-leaf", Kind: KindLeaf, SourceID: "s", Start: 0, Dur: 2,
						SrcIn: 0, SrcOut: 10, Speed: SpeedOne},
					{ID: "c-frz", Kind: KindLeaf, SourceID: "s", Start: 2, Dur: 2,
						SrcIn: 33, SrcOut: 34, Freeze: true},
				},
			},
		},
		Root: &Compound{
			ID: "c", Dur: 6,
			Items: []*Item{
				{ID: "r-ref", Kind: KindRef, CompoundID: "c", Start: 0, Dur: 4, Speed: SpeedOne},
			},
		},
	}
	r, err := NewResolver(p)
	if err != nil {
		t.Fatal(err)
	}
	frz, err := r.Frame(3)
	if err != nil {
		t.Fatal(err)
	}
	if frz.Kind != FrameFreeze || frz.SourceFrame != 33 || frz.InternalSpeed != SpeedOne {
		t.Fatalf("nested freeze wrong: %+v", frz)
	}
	gap, err := r.Frame(5)
	if err != nil {
		t.Fatal(err)
	}
	if gap.Kind != FrameGap {
		t.Fatalf("frame 5 kind = %s want gap", gap.Kind)
	}
}

func TestFlattenMatchesResolver(t *testing.T) {
	p := DemoProject()
	r, err := NewResolver(p)
	if err != nil {
		t.Fatal(err)
	}
	flat, err := Flatten(p)
	if err != nil {
		t.Fatal(err)
	}
	mismatches, err := VerifyFrames(r, flat)
	if err != nil {
		t.Fatal(err)
	}
	if len(mismatches) != 0 {
		t.Fatalf("frame mismatches: %v", mismatches)
	}
	markerMismatches, err := VerifyMarkers(r, flat)
	if err != nil {
		t.Fatal(err)
	}
	if len(markerMismatches) != 0 {
		t.Fatalf("marker mismatches: %v", markerMismatches)
	}
}

func TestNaiveDriftDetected(t *testing.T) {
	drifts, err := CompareMarkers(DemoProject())
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, d := range drifts {
		if d.MarkerID == "mk-inner-leaf" && d.DriftFrames != 0 {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected drift on mk-inner-leaf, got %+v", drifts)
	}
}

func TestCycleDetected(t *testing.T) {
	_, err := NewResolver(CyclicProject())
	var ce *CycleError
	if !errors.As(err, &ce) {
		t.Fatalf("expected *CycleError, got %v", err)
	}
	want := []string{"A", "B", "A"}
	if len(ce.Path) != 3 || ce.Path[0] != want[0] || ce.Path[1] != want[1] || ce.Path[2] != want[2] {
		t.Fatalf("cycle path = %v want %v", ce.Path, want)
	}
}

func TestExportsRoundTrip(t *testing.T) {
	r, err := NewResolver(DemoProject())
	if err != nil {
		t.Fatal(err)
	}
	records, err := r.Frames()
	if err != nil {
		t.Fatal(err)
	}

	jsonBytes, err := ExportJSON(records)
	if err != nil {
		t.Fatal(err)
	}
	var back []FrameRecord
	if err := json.Unmarshal(jsonBytes, &back); err != nil {
		t.Fatal(err)
	}
	if len(back) != len(records) || back[8].SourceFrame != 77 {
		t.Fatalf("json round trip wrong: %+v", back)
	}

	csvBytes, err := ExportCSV(records)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := csv.NewReader(strings.NewReader(string(csvBytes))).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != len(records)+1 {
		t.Fatalf("csv rows = %d want %d", len(rows), len(records)+1)
	}
	if rows[1][5] != "3/2" || rows[1][6] != "2/3" {
		t.Fatalf("csv speed columns wrong: %v", rows[1])
	}
}

func TestValidationRejectsBadLeaf(t *testing.T) {
	p := DemoProject()
	p.Compounds["inner"].Items[0].SrcOut = 11 // 区间不足以承载变速
	if err := p.Validate(); err == nil {
		t.Fatal("expected validation error for overflowing leaf")
	}
}
