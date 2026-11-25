package clipmerge

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func loadRequest(t *testing.T, name string) MergeRequest {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	var req MergeRequest
	if err := json.Unmarshal(data, &req); err != nil {
		t.Fatal(err)
	}
	return req
}

// TestGoldenReports regenerates each scenario report and compares it
// byte-for-byte with the checked-in golden file.
func TestGoldenReports(t *testing.T) {
	scenarios := []string{"auto-merge", "dependency-conflict", "direct-conflict"}
	for _, scenario := range scenarios {
		t.Run(scenario, func(t *testing.T) {
			req := loadRequest(t, scenario+"-request.json")
			rep, err := Merge(req)
			if err != nil {
				t.Fatal(err)
			}
			golden, err := os.ReadFile(filepath.Join("testdata", scenario+"-report.golden.json"))
			if err != nil {
				t.Fatal(err)
			}
			if got := rep.Marshal(); !bytes.Equal(got, golden) {
				t.Fatalf("report mismatch\ngot:\n%s\nwant:\n%s", got, golden)
			}
		})
	}
}

// TestReproducible runs the same merge twice and requires identical bytes,
// including the content-derived reportId.
func TestReproducible(t *testing.T) {
	req := loadRequest(t, "dependency-conflict-request.json")
	first, err := Merge(req)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Merge(req)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first.Marshal(), second.Marshal()) {
		t.Fatal("identical requests produced different reports")
	}
	if first.ReportID == "" || first.ReportID != second.ReportID {
		t.Fatalf("reportId not reproducible: %q vs %q", first.ReportID, second.ReportID)
	}
}

// TestNoLastWriterWins permutes edit arrival order; the report must not
// change, proving resolution never depends on arrival or timestamps.
func TestNoLastWriterWins(t *testing.T) {
	req := loadRequest(t, "direct-conflict-request.json")
	base, err := Merge(req)
	if err != nil {
		t.Fatal(err)
	}
	if base.Decision.Code != CodeManualResolutionRequired {
		t.Fatalf("expected manual resolution, got %s", base.Decision.Code)
	}
	// Reverse arrival order: under last-write-wins the "winner" would flip.
	req.Edits[0], req.Edits[1] = req.Edits[1], req.Edits[0]
	permuted, err := Merge(req)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(base.Marshal(), permuted.Marshal()) {
		t.Fatal("report changed when edit arrival order changed")
	}
	if permuted.PropertyResolutions[1].EffectiveValue != nil {
		t.Fatal("unresolved property must not pick any value")
	}
}

// TestAutoMergeOrderIndependent checks the same for a clean merge.
func TestAutoMergeOrderIndependent(t *testing.T) {
	req := loadRequest(t, "auto-merge-request.json")
	want, err := Merge(req)
	if err != nil {
		t.Fatal(err)
	}
	e := req.Edits
	e[0], e[1], e[2] = e[2], e[0], e[1]
	got, err := Merge(req)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(want.Marshal(), got.Marshal()) {
		t.Fatal("auto-merge report changed when edit order changed")
	}
}
