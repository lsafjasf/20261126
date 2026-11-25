package handlefix

import "testing"

func TestDamagedFixtureCanBeTracedAndRestored(t *testing.T) {
	damaged, err := LoadProject("testdata/damaged-reexport.json")
	if err != nil {
		t.Fatal(err)
	}
	edits, err := LoadEdgeEdits("testdata/trim-edits.json")
	if err != nil {
		t.Fatal(err)
	}

	report, err := AuditTrace(damaged, edits)
	if err != nil {
		t.Fatal(err)
	}
	if report.Total != 2 || len(report.Locations) != 4 {
		t.Fatalf("report total/locations = %d/%d", report.Total, len(report.Locations))
	}

	restored, err := ReverseEdits(damaged, edits)
	if err != nil {
		t.Fatal(err)
	}
	exported, exportReport, err := PrepareExport(restored)
	if err != nil {
		t.Fatal(err)
	}
	if exportReport.HardCutCount != 0 {
		t.Fatalf("restored export has hard cuts: %+v", exportReport)
	}
	if err := exported.validate(); err != nil {
		t.Fatal(err)
	}
}
