package wavfix_test

import (
	"bytes"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"wavfix"
)

func TestWrongRIFFSizeIsRecomputed(t *testing.T) {
	fixture := newG711Fixture(t)
	fixture.afterData = []testChunk{{id: "TAIL", payload: []byte("tail")}}
	input := fixture.build()
	wrong := uint32(len(input) - 8 - 40)
	copy(input[4:8], []byte{byte(wrong), byte(wrong >> 8), byte(wrong >> 16), byte(wrong >> 24)})

	parsed, err := wavfix.Parse(input)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, issue := range parsed.Reports {
		if issue.Code == "riff_size_mismatch" {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing RIFF size issue: %+v", parsed.Reports)
	}

	output, _, err := wavfix.Repair(input)
	if err != nil {
		t.Fatal(err)
	}
	repaired, err := wavfix.Parse(output)
	if err != nil {
		t.Fatal(err)
	}
	if repaired.RIFFSize != uint32(len(output)-8) {
		t.Fatalf("RIFF size = %d, want %d", repaired.RIFFSize, len(output)-8)
	}
	if repaired.ChunkByID("TAIL") == nil {
		t.Fatal("tail unknown chunk lost while repairing RIFF size")
	}
}

func TestShortFactChunkIsCanonicalized(t *testing.T) {
	fixture := newG711Fixture(t)
	fixture.factPayload = []byte{0xe8, 0x03}
	input := fixture.build()

	parsed, err := wavfix.Parse(input)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Fact.Valid {
		t.Fatal("two-byte fact incorrectly marked valid")
	}

	output, report, err := wavfix.Repair(input)
	if err != nil {
		t.Fatal(err)
	}
	if !report.FactChanged {
		t.Fatal("short fact should be replaced")
	}
	repaired, err := wavfix.Parse(output)
	if err != nil {
		t.Fatal(err)
	}
	if repaired.Fact.Size != 4 || repaired.Fact.SampleCount != 1000 {
		t.Fatalf("repaired fact = %+v, want size 4 count 1000", repaired.Fact)
	}
}

func TestOverlongDataLengthCanBeCorrectedOrRejected(t *testing.T) {
	fixture := newG711Fixture(t)
	input := fixture.build()
	truncated := input[:len(input)-8]

	parsed, parseErr := wavfix.Parse(truncated)
	if !errors.Is(parseErr, wavfix.ErrTruncated) {
		t.Fatalf("parse error = %v, want ErrTruncated", parseErr)
	}
	if parsed == nil || parsed.Data == nil || len(parsed.Data.Payload) != 1992 {
		t.Fatalf("truncated data = %+v", parsed.Data)
	}

	output, report, err := wavfix.Repair(truncated)
	if err != nil {
		t.Fatal(err)
	}
	if report.DataFrames != 996 || report.RepairedFactCount != 996 {
		t.Fatalf("frames=%d repaired=%d, want 996", report.DataFrames, report.RepairedFactCount)
	}
	repaired, err := wavfix.Parse(output)
	if err != nil {
		t.Fatal(err)
	}
	if repaired.Data.Size != 1992 || !bytes.Equal(repaired.Data.Payload, parsed.Data.Payload) {
		t.Fatal("overlong data header was not corrected while preserving available bytes")
	}

	_, _, err = wavfix.RepairWithOptions(truncated, wavfix.Options{StrictTruncation: true})
	if !errors.Is(err, wavfix.ErrTruncated) {
		t.Fatalf("strict repair err = %v, want ErrTruncated", err)
	}
}

func TestALawMonoAndFloatDecodeDuration(t *testing.T) {
	fixture := newG711Fixture(t)
	fixture.tag = wavfix.FormatALaw
	fixture.channels = 1
	fixture.blockAlign = 1
	fixture.audio = bytes.Repeat([]byte{0x5a}, 333)
	fixture.factCount = uint32Ptr(333)
	input := fixture.build()

	file, err := wavfix.Parse(input)
	if err != nil {
		t.Fatal(err)
	}
	if !file.Format.IsG711() || file.Format.EffectiveTag != wavfix.FormatALaw {
		t.Fatalf("format = %+v, want A-law", file.Format)
	}
	values := file.Float64Samples()
	if len(values) != 333 {
		t.Fatalf("decoded values = %d, want 333", len(values))
	}
	duration := wavfix.FloatDuration(len(values), int(file.Format.Channels), file.Format.SampleRate)
	approxEqual(t, "A-law float duration", duration, 333.0/8000.0)

	output, _, err := wavfix.Repair(input)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(output, input) {
		t.Fatal("valid A-law file should remain byte-identical")
	}
}

func TestRepairFileWritesUnchangedAudio(t *testing.T) {
	fixture := newG711Fixture(t)
	fixture.factCount = uint32Ptr(uint32(len(fixture.audio)))
	inputPath := filepath.Join(t.TempDir(), "input.wav")
	outputPath := filepath.Join(t.TempDir(), "fixed.wav")
	if err := os.WriteFile(inputPath, fixture.build(), 0o644); err != nil {
		t.Fatal(err)
	}

	report, err := wavfix.RepairFile(inputPath, outputPath, wavfix.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if report.AudioSHA256 == "" || report.RepairedFactCount != 1000 {
		t.Fatalf("bad report: %+v", report)
	}
	output, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := wavfix.Parse(output)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(parsed.Data.Payload, fixture.audio) {
		t.Fatal("file repair changed audio payload")
	}
}

func TestMissingFormatReturnsReportableError(t *testing.T) {
	var buffer bytes.Buffer
	buffer.WriteString("RIFF")
	buffer.Write([]byte{0, 0, 0, 0})
	buffer.WriteString("WAVE")
	writeTestChunk(&buffer, "data", []byte{0x01, 0x02})
	raw := buffer.Bytes()
	binary.LittleEndian.PutUint32(raw[4:8], uint32(len(raw)-8))

	_, report, err := wavfix.Repair(raw)
	if !errors.Is(err, wavfix.ErrNoFormat) {
		t.Fatalf("err = %v, want ErrNoFormat", err)
	}
	if report == nil || len(report.Issues) == 0 {
		t.Fatal("expected a report with missing_fmt issue")
	}
}

func TestDuplicateFactChunksAreCorrectedInPlace(t *testing.T) {
	fixture := newG711Fixture(t)
	fixture.factCount = uint32Ptr(uint32(len(fixture.audio)))
	input := fixture.build()
	parsed, err := wavfix.Parse(input)
	if err != nil {
		t.Fatal(err)
	}
	firstFact := parsed.ChunkByID("fact")

	withDuplicate := append([]byte{}, input[:firstFact.NextOffset]...)
	withDuplicate = append(withDuplicate, firstFact.Raw...)
	withDuplicate = append(withDuplicate, input[firstFact.NextOffset:]...)

	output, _, err := wavfix.Repair(withDuplicate)
	if err != nil {
		t.Fatal(err)
	}
	repaired, err := wavfix.Parse(output)
	if err != nil {
		t.Fatal(err)
	}
	factCount := 0
	for _, chunk := range repaired.Chunks {
		if chunk.ID == "fact" {
			factCount++
			if binary.LittleEndian.Uint32(chunk.Payload[:4]) != 1000 {
				t.Fatal("duplicate fact was not corrected")
			}
		}
	}
	if factCount != 2 || repaired.Fact.SampleCount != 1000 {
		t.Fatalf("fact chunks=%d first count=%d", factCount, repaired.Fact.SampleCount)
	}
}
