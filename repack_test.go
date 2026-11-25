package aiffmeta

import (
	"bytes"
	"os"
	"testing"
)

func TestRepackPreservesWholeFileAndAudio(t *testing.T) {
	original := unusualAIFC(t)
	source, err := ParseBytes(original)
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	report, err := source.Repack(&output, RepackOptions{})
	if err != nil {
		t.Fatalf("Repack: %v", err)
	}
	if !bytes.Equal(output.Bytes(), original) {
		t.Fatal("repack changed a valid file despite preserving all chunks")
	}
	if report.KeptChunks != len(source.Chunks) || len(report.DroppedChunks) != 0 {
		t.Fatalf("report = %+v", report)
	}
	if report.AudioBytes != 16 || report.AudioSHA256 == "" {
		t.Fatalf("audio report = %+v", report)
	}
}

func TestRepackDropsOnlyTrailingJunkOutsideFORM(t *testing.T) {
	original := unusualAIFC(t)
	withJunk := append(append([]byte(nil), original...), []byte("outside FORM")...)
	source, err := ParseBytes(withJunk)
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	report, err := source.Repack(&output, RepackOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(output.Bytes(), original) {
		t.Fatal("repack did not normalize content outside FORM")
	}
	if report.TrailingBytesOmitted != int64(len("outside FORM")) {
		t.Fatalf("trailing bytes omitted = %d", report.TrailingBytesOmitted)
	}
}

func TestRepackDropReportsLostMetadata(t *testing.T) {
	source, err := ParseBytes(unusualAIFC(t))
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	report, err := source.Repack(&output, RepackOptions{
		Drop: []string{"ANNO", "COMT", "MARK", "NAME"},
	})
	if err != nil {
		t.Fatalf("Repack: %v", err)
	}
	if len(report.DroppedChunks) != 4 {
		t.Fatalf("dropped = %d, want 4", len(report.DroppedChunks))
	}
	if !hasLost(report.Lost, "annotation", "first annotation") ||
		!hasLost(report.Lost, "comment", "first comment") ||
		!hasLost(report.Lost, "marker", "Intro") ||
		!hasLost(report.Lost, "name", "Take 7") {
		t.Fatalf("lost metadata incomplete: %#v", report.Lost)
	}

	resaved, err := ParseBytes(output.Bytes())
	if err != nil {
		t.Fatalf("parse repacked file: %v", err)
	}
	sourceAudio, sourceBytes, err := source.AudioDigest()
	if err != nil {
		t.Fatal(err)
	}
	resultAudio, resultBytes, err := resaved.AudioDigest()
	if err != nil {
		t.Fatal(err)
	}
	if sourceBytes != resultBytes || sourceAudio != resultAudio {
		t.Fatal("audio payload changed while dropping metadata")
	}
	if resaved.Text.Name != "" || len(resaved.Text.Comments) != 0 || len(resaved.Text.Markers) != 0 {
		t.Fatalf("dropped metadata still present: %+v", resaved.Text)
	}
	if resaved.SSND == nil || resaved.SSND.Offset != 5 {
		t.Fatalf("SSND parameters changed: %+v", resaved.SSND)
	}
}

func TestRepackRefusesRequiredChunks(t *testing.T) {
	source, err := ParseBytes(unusualAIFC(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"COMM", "SSND", "FVER"} {
		var output bytes.Buffer
		if _, err := source.Repack(&output, RepackOptions{Drop: []string{id}}); err == nil {
			t.Fatalf("dropping %s was allowed", id)
		}
	}
}

func TestCompareFindsLostMetadataAndEqualAudio(t *testing.T) {
	sourceData := unusualAIFC(t)
	source, err := ParseBytes(sourceData)
	if err != nil {
		t.Fatal(err)
	}
	var resavedData bytes.Buffer
	if _, err := source.Repack(&resavedData, RepackOptions{
		Drop: []string{"ANNO", "COMT", "MARK"},
	}); err != nil {
		t.Fatal(err)
	}
	resaved, err := ParseBytes(resavedData.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	report, err := Compare(source, resaved)
	if err != nil {
		t.Fatalf("Compare: %v", err)
	}
	if !report.Audio.Comparable || !report.Audio.Equal {
		t.Fatalf("audio comparison = %+v", report.Audio)
	}
	if !report.COMM.Comparable || report.COMM.Changed {
		t.Fatalf("COMM comparison = %+v", report.COMM)
	}
	if len(report.Metadata.MissingChunks) != 3 {
		t.Fatalf("missing chunks = %#v", report.Metadata.MissingChunks)
	}
	if !hasLost(report.Metadata.Lost, "annotation", "first annotation") ||
		!hasLost(report.Metadata.Lost, "comment", "first comment") ||
		!hasLost(report.Metadata.Lost, "marker", "Outro") {
		t.Fatalf("lost metadata incomplete: %#v", report.Metadata.Lost)
	}
	if report.Metadata.String() == "metadata: unchanged" {
		t.Fatal("metadata diff reported unchanged")
	}
}

func TestRepackToFileIsAtomicOnRefusal(t *testing.T) {
	dir := t.TempDir()
	sourcePath := dir + "/source.aifc"
	outputPath := dir + "/output.aifc"
	if err := os.WriteFile(sourcePath, unusualAIFC(t), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(outputPath, []byte("keep me"), 0o600); err != nil {
		t.Fatal(err)
	}
	source, err := ReadFile(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	if _, err := source.RepackToFile(outputPath, RepackOptions{Drop: []string{"SSND"}}); err == nil {
		t.Fatal("expected refusal")
	}
	got, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "keep me" {
		t.Fatalf("existing output was replaced with %q", got)
	}
}

func hasLost(items []LostItem, kind, text string) bool {
	for _, item := range items {
		if item.Kind == kind && item.Text == text {
			return true
		}
	}
	return false
}
