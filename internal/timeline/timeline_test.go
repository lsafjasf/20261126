package timeline

import "testing"

func TestRationalSnapPolicies(t *testing.T) {
	half := NewRat(25, 2)

	if got := half.Floor(); got != 12 {
		t.Fatalf("Floor() = %d, want 12", got)
	}
	if got := half.Ceil(); got != 13 {
		t.Fatalf("Ceil() = %d, want 13", got)
	}
	if got := half.NearestEven(); got != 12 {
		t.Fatalf("NearestEven() = %d, want 12", got)
	}
	if got := half.NearestUp(); got != 13 {
		t.Fatalf("NearestUp() = %d, want 13", got)
	}
}

func TestMediaAnchorUsesLocalCoordinate(t *testing.T) {
	marker := Marker{
		ID:                "media-1",
		AnchorKind:        AnchorMedia,
		AnchorFrame:       1,
		AnchorRate:        25,
		GlobalTimeSeconds: MustParseRat("1201/25"),
		RolePolicy:        PolicyFloor,
		MediaOffsetFrames: 1200,
	}

	placement := BuildPlacement(marker, 24)
	if placement.Exact {
		t.Fatal("expected non-integer target frame")
	}
	first := placement.Candidates[0]
	if first.Frame != 1153 || first.AnchorCoordinate.RatString() != "25/24" {
		t.Fatalf("unexpected first candidate: frame=%d coordinate=%s", first.Frame, first.AnchorCoordinate.RatString())
	}
	if len(first.SafeReversePolicies) != 2 {
		t.Fatalf("safe reverse policies = %v, want two policies", first.SafeReversePolicies)
	}
}

func TestClipAndGlobalAnchorCoordinates(t *testing.T) {
	clip := Marker{
		ID:                "clip-1",
		AnchorKind:        AnchorClip,
		AnchorFrame:       6,
		AnchorRate:        25,
		GlobalTimeSeconds: MustParseRat("1206/25"),
		RolePolicy:        PolicyNearest,
		ClipOffsetFrames:  1200,
	}
	clipPlacement := BuildPlacement(clip, 24)
	if got := clipPlacement.Candidates[0].AnchorCoordinate.RatString(); got != "25/4" {
		t.Fatalf("clip anchor coordinate = %s, want 25/4", got)
	}

	global := Marker{
		ID:                "global-1",
		AnchorKind:        AnchorGlobal,
		AnchorFrame:       250,
		AnchorRate:        25,
		GlobalTimeSeconds: MustParseRat("10"),
		RolePolicy:        PolicyNearest,
	}
	globalPlacement := BuildPlacement(global, 24)
	if got := globalPlacement.Candidates[0].AnchorCoordinate.RatString(); got != "250" {
		t.Fatalf("global anchor coordinate = %s, want 250", got)
	}
}

func TestBoundaryTieUsesEvenNearest(t *testing.T) {
	marker := Marker{
		ID:                "half-even",
		AnchorKind:        AnchorMedia,
		AnchorFrame:       12,
		AnchorRate:        24,
		GlobalTimeSeconds: NewRat(1, 2),
		RolePolicy:        PolicyNearest,
		NearestEvenTie:    true,
	}

	placement := BuildPlacement(marker, 25)
	nearest := findTestCandidate(placement, PolicyNearest)
	if nearest.Frame != 12 {
		t.Fatalf("nearest frame = %d, want even frame 12", nearest.Frame)
	}
}

func TestDemoReportRoundTripAndCounts(t *testing.T) {
	report, err := BuildReport(DemoTimeline())
	if err != nil {
		t.Fatal(err)
	}

	wantCounts := MarkerCounts{
		Markers:         149,
		ExactOnAllGrids: 16,
		NeedingChoice:   133,
		RoundTripOK:     149,
		SnapRecoverable: 149,
	}
	wantCounts.ByAnchor = map[string]int{AnchorMedia: 110, AnchorClip: 24, AnchorGlobal: 15}
	if report.Counts.Markers != wantCounts.Markers ||
		report.Counts.ExactOnAllGrids != wantCounts.ExactOnAllGrids ||
		report.Counts.NeedingChoice != wantCounts.NeedingChoice ||
		report.Counts.RoundTripOK != wantCounts.RoundTripOK ||
		report.Counts.SnapRecoverable != wantCounts.SnapRecoverable ||
		report.Counts.ByAnchor[AnchorMedia] != 110 ||
		report.Counts.ByAnchor[AnchorClip] != 24 ||
		report.Counts.ByAnchor[AnchorGlobal] != 15 {
		t.Fatalf("counts = %+v, want %+v", report.Counts, wantCounts)
	}
	for _, marker := range report.Markers {
		if !marker.RoundTripTwice.OK || !marker.RoundTripTwice.CrossRateOK {
			t.Fatalf("marker %s failed round trip: %+v", marker.ID, marker.RoundTripTwice)
		}
		if marker.RoundTripTwice.OriginalFrame != marker.RoundTripTwice.FirstFrame ||
			marker.RoundTripTwice.FirstFrame != marker.RoundTripTwice.SecondFrame {
			t.Fatalf("marker %s did not return to the original frame", marker.ID)
		}
	}
}

func findTestCandidate(placement Placement, tag string) Candidate {
	for _, candidate := range placement.Candidates {
		for _, candidateTag := range candidate.Tags {
			if candidateTag == tag {
				return candidate
			}
		}
	}
	return placement.Candidates[0]
}
