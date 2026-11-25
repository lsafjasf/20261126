package cuemap

import (
	"os"
	"strings"
	"testing"
)

func TestGeneratedFixtures(t *testing.T) {
	file, err := LoadFixtures("testdata/cues.json")
	if err != nil {
		t.Fatalf("load generated fixtures (run go generate): %v", err)
	}
	if len(file.Fixtures) < 5 {
		t.Fatalf("fixture count = %d, want at least 5", len(file.Fixtures))
	}

	for _, fixture := range file.Fixtures {
		t.Run(fixture.Name, func(t *testing.T) {
			result := Typeset(fixture.Cue, fixture.Options)
			if result.OK != fixture.WantOK {
				t.Fatalf("OK = %v, want %v; fallback=%+v", result.OK, fixture.WantOK, result.Fallback)
			}

			if fixture.WantOK {
				if collision := FindCollision(result); collision != nil {
					t.Fatalf("generated fixture collides: %v", collision)
				}
				for _, line := range result.Lines {
					assertMappingRoundTrip(t, line.Mapping)
					assertCellsInsideLineBounds(t, line)
				}
				return
			}

			if result.Fallback == nil {
				t.Fatal("expected fallback, got nil")
			}
			if result.Fallback.CueID != fixture.Cue.ID {
				t.Fatalf("Fallback.CueID = %q, want %q", result.Fallback.CueID, fixture.Cue.ID)
			}
			if result.Fallback.OriginalText != fixture.Cue.Raw {
				t.Fatalf("OriginalText = %q, want raw %q", result.Fallback.OriginalText, fixture.Cue.Raw)
			}
			if result.Fallback.CueRef != "cue:"+fixture.Cue.ID {
				t.Fatalf("CueRef = %q", result.Fallback.CueRef)
			}
		})
	}
}

func TestGeneratedSRTMaterialExists(t *testing.T) {
	data, err := os.ReadFile("testdata/trilingual.srt")
	if err != nil {
		t.Fatalf("read generated SRT: %v", err)
	}
	text := string(data)
	for _, want := range []string{"00:00:00,000 -->", "مرحبا", "<ruby>", "》"} {
		if !strings.Contains(text, want) {
			t.Fatalf("SRT material missing %q", want)
		}
	}
}

func assertCellsInsideLineBounds(t *testing.T, line LineLayout) {
	t.Helper()
	for _, cell := range line.Cells {
		if cell.Rect.X < line.Bounds.X || cell.Rect.Y < line.Bounds.Y ||
			cell.Rect.Right() > line.Bounds.Right()+1 || cell.Rect.Bottom() > line.Bounds.Bottom()+1 {
			t.Fatalf("cell %+v outside line bounds %+v", cell.Rect, line.Bounds)
		}
	}
}
