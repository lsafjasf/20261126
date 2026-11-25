package anchor

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestSaveLoadRoundTripIsStable(t *testing.T) {
	p := DemoProject()
	key := PieceKey("SUB1", Anchor{MediaID: "media-A", Start: 7 * testTick, End: 9 * testTick})
	if err := p.SetSegmentDecision(key, "seg-1", "canonical"); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "project.json")
	if err := p.Save(path); err != nil {
		t.Fatal(err)
	}

	reloaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}

	before, err := p.Checksum()
	if err != nil {
		t.Fatal(err)
	}
	after, err := reloaded.Checksum()
	if err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Fatalf("checksum changed after reload: %s vs %s", before, after)
	}

	r1, err := p.Map()
	if err != nil {
		t.Fatal(err)
	}
	r2, err := reloaded.Map()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(r1, r2) {
		t.Fatal("mapping results differ after reload")
	}

	// Repeated saves must produce byte-identical files.
	path2 := filepath.Join(dir, "project-copy.json")
	if err := reloaded.Save(path2); err != nil {
		t.Fatal(err)
	}
	sum1, _ := checksumFile(path)
	sum2, _ := checksumFile(path2)
	if sum1 != sum2 {
		t.Fatal("saved bytes are not deterministic")
	}
}

func TestRepeatedMappingIsDeterministic(t *testing.T) {
	p := DemoProject()
	r1, err := p.Map()
	if err != nil {
		t.Fatal(err)
	}
	r2, err := p.Map()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(r1, r2) {
		t.Fatal("mapping is not deterministic across calls")
	}
}

func TestValidationRejectsBadProject(t *testing.T) {
	p := testProject()
	p.TicksPerSecond = 0
	if err := p.Validate(); err == nil {
		t.Fatal("expected error for non-positive timebase")
	}

	p2 := testProject()
	p2.Edl[0].SrcEnd = p2.Edl[0].SrcStart
	if err := p2.Validate(); err == nil {
		t.Fatal("expected error for empty EDL interval")
	}

	p3 := testProject()
	p3.Events[0].Anchor.End = p3.Events[0].Anchor.Start
	if err := p3.Validate(); err == nil {
		t.Fatal("expected error for empty anchor interval")
	}
}

func TestParseTimeForms(t *testing.T) {
	p := testProject()
	cases := []struct {
		in   string
		want Ticks
	}{
		{"1500000", 1_500_000},
		{"2s", 2_000_000},
		{"250ms", 250_000},
		{"1500us", 1_500},
		{"00:00:01.500", 1_500_000},
		{"01:02:03", 3723 * testTick},
	}
	for _, tc := range cases {
		got, err := p.ParseTime(tc.in)
		if err != nil {
			t.Fatalf("ParseTime(%q): %v", tc.in, err)
		}
		if got != tc.want {
			t.Fatalf("ParseTime(%q) = %d, want %d", tc.in, got, tc.want)
		}
	}

	if _, err := p.ParseTime("0.5us"); err == nil {
		t.Fatal("expected sub-tick precision error")
	}
	if _, err := p.ParseTime("nonsense"); err == nil {
		t.Fatal("expected parse error for garbage input")
	}
}

func TestFormatTime(t *testing.T) {
	p := testProject()
	if got := p.FormatTime(3723 * testTick); got != "01:02:03.000" {
		t.Fatalf("FormatTime = %q", got)
	}
	if got := p.FormatTime(1_500_000); got != "00:00:01.500" {
		t.Fatalf("FormatTime = %q", got)
	}
}
