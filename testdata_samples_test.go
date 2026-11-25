package wavfix_test

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"wavfix"
)

func readSample(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestCheckedInBadFactSample(t *testing.T) {
	input := readSample(t, "g711-stereo-bad-fact.wav")
	file, err := wavfix.Parse(input)
	if err != nil {
		t.Fatal(err)
	}
	report := file.BuildReport()
	if report.DataFrames != 1000 || report.FactDeclaredCount != 2000 {
		t.Fatalf("bad fact sample relation = frames %d fact %d", report.DataFrames, report.FactDeclaredCount)
	}
	if report.DurationErrorFactor() != 2 {
		t.Fatalf("factor = %v, want exactly 2", report.DurationErrorFactor())
	}

	output, repairReport, err := wavfix.Repair(input)
	if err != nil {
		t.Fatal(err)
	}
	if repairReport.RepairedFactCount != 1000 {
		t.Fatalf("fact = %d, want 1000", repairReport.RepairedFactCount)
	}
	repaired, err := wavfix.Parse(output)
	if err != nil {
		t.Fatal(err)
	}
	if string(repaired.ChunkByID("xUNK").Payload) != "unknown chunk before data" {
		t.Fatal("xUNK payload not preserved")
	}
	if string(repaired.ChunkByID("META").Payload) != "preserve after data" {
		t.Fatal("META payload not preserved")
	}
	if !bytes.Equal(repaired.Data.Payload, file.Data.Payload) {
		t.Fatal("checked-in sample audio changed")
	}
}

func TestCheckedInMissingFactSample(t *testing.T) {
	input := readSample(t, "g711-mono-missing-fact.wav")
	if _, err := wavfix.Parse(input); err != nil {
		t.Fatal(err)
	}
	output, report, err := wavfix.Repair(input)
	if err != nil {
		t.Fatal(err)
	}
	if !report.FactInserted || report.RepairedFactCount != 1601 {
		t.Fatalf("report = inserted:%v count:%d", report.FactInserted, report.RepairedFactCount)
	}
	if len(output) != len(input)+12 {
		t.Fatalf("missing-fact output delta = %d, want 12", len(output)-len(input))
	}
	repaired, err := wavfix.Parse(output)
	if err != nil {
		t.Fatal(err)
	}
	if repaired.Fact.SampleCount != 1601 || repaired.ChunkByID("LIST") == nil {
		t.Fatalf("fact=%+v LIST=%v", repaired.Fact, repaired.ChunkByID("LIST") != nil)
	}
}

func TestCheckedInCorrectALawAndBadRIFFSamples(t *testing.T) {
	correct := readSample(t, "g711-mono-alaw-correct.wav")
	fixed, _, err := wavfix.Repair(correct)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(fixed, correct) {
		t.Fatal("correct A-law sample was rewritten")
	}

	badRIFF := readSample(t, "g711-stereo-bad-riff-size.wav")
	output, _, err := wavfix.Repair(badRIFF)
	if err != nil {
		t.Fatal(err)
	}
	repaired, err := wavfix.Parse(output)
	if err != nil {
		t.Fatal(err)
	}
	if repaired.RIFFSize != uint32(len(output)-8) || repaired.ChunkByID("TAIL") == nil {
		t.Fatal("RIFF size or trailing unknown chunk not repaired")
	}
}

func TestCheckedInOverlongDataSample(t *testing.T) {
	input := readSample(t, "g711-stereo-overlong-data.wav")
	parsed, parseErr := wavfix.Parse(input)
	if !errors.Is(parseErr, wavfix.ErrTruncated) {
		t.Fatalf("parse error = %v, want ErrTruncated", parseErr)
	}

	output, report, err := wavfix.Repair(input)
	if err != nil {
		t.Fatal(err)
	}
	if report.DataBytes != 1000 || report.DataFrames != 500 || report.RepairedFactCount != 500 {
		t.Fatalf("report bytes=%d frames=%d fact=%d", report.DataBytes, report.DataFrames, report.RepairedFactCount)
	}
	repaired, err := wavfix.Parse(output)
	if err != nil {
		t.Fatal(err)
	}
	if repaired.Data.Size != 1000 || !bytes.Equal(repaired.Data.Payload, parsed.Data.Payload) {
		t.Fatal("available data bytes were not preserved with corrected length")
	}
}
