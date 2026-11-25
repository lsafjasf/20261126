package wavfix_test

import (
	"bytes"
	"errors"
	"math"
	"testing"

	"wavfix"
)

func approxEqual(t *testing.T, name string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-12 {
		t.Fatalf("%s = %.12f, want %.12f", name, got, want)
	}
}

func TestStereoG711DataByteCountDoublesDuration(t *testing.T) {
	unknownBefore := testChunk{id: "uNKn", payload: []byte{0xde, 0xad, 0xbe}}
	unknownAfter := testChunk{id: "uNK2", payload: []byte("preserve-me")}
	fixture := newG711Fixture(t)
	fixture.factCount = uint32Ptr(uint32(len(fixture.audio)))
	fixture.beforeFact = []testChunk{unknownBefore}
	fixture.afterData = []testChunk{unknownAfter}
	input := fixture.build()

	parsed, err := wavfix.Parse(input)
	if err != nil {
		t.Fatal(err)
	}
	report := parsed.BuildReport()

	if report.Channels != 2 || report.BlockAlign != 2 || report.FrameBytes != 2 {
		t.Fatalf("unexpected frame geometry: %+v", report)
	}
	if report.DataBytes != 2000 || report.DataFrames != 1000 {
		t.Fatalf("data relation = %d bytes / %d frames, want 2000 / 1000", report.DataBytes, report.DataFrames)
	}
	if report.FactDeclaredCount != 2000 {
		t.Fatalf("fact = %d, want buggy 2000", report.FactDeclaredCount)
	}
	approxEqual(t, "fact duration", report.DurationFromFact, 0.25)
	approxEqual(t, "data duration", report.DurationFromData, 0.125)
	approxEqual(t, "naive duration", report.NaiveByteCountSeconds, 0.25)
	if factor := report.DurationErrorFactor(); factor != 2 {
		t.Fatalf("error factor = %v, want 2", factor)
	}
	if report.FloatValues != 2000 {
		t.Fatalf("float values = %d, want one float per G.711 byte", report.FloatValues)
	}
	approxEqual(t, "interleaved float duration", report.FloatDurationSeconds, 0.125)

	output, repairReport, err := wavfix.Repair(input)
	if err != nil {
		t.Fatal(err)
	}
	if !repairReport.FactChanged || repairReport.FactInserted {
		t.Fatalf("repair flags = changed:%v inserted:%v, want changed only", repairReport.FactChanged, repairReport.FactInserted)
	}
	if repairReport.RepairedFactCount != 1000 {
		t.Fatalf("repaired fact = %d, want 1000", repairReport.RepairedFactCount)
	}

	repaired, err := wavfix.Parse(output)
	if err != nil {
		t.Fatal(err)
	}
	if repaired.Fact == nil || repaired.Fact.SampleCount != 1000 {
		t.Fatalf("output fact = %+v, want 1000", repaired.Fact)
	}
	if !bytes.Equal(repaired.Data.Payload, fixture.audio) {
		t.Fatal("data payload changed")
	}
	for _, wantChunk := range []testChunk{unknownBefore, unknownAfter} {
		got := repaired.ChunkByID(wantChunk.id)
		if got == nil {
			t.Fatalf("unknown chunk %s was removed", wantChunk.id)
		}
		if !bytes.Equal(got.Payload, wantChunk.payload) {
			t.Fatalf("unknown chunk %s payload = %x, want %x", wantChunk.id, got.Payload, wantChunk.payload)
		}
	}
	if repaired.RIFFSize != uint32(len(output)-8) {
		t.Fatalf("RIFF size = %d, want %d", repaired.RIFFSize, len(output)-8)
	}
}

func TestMissingFactIsInserted(t *testing.T) {
	fixture := newG711Fixture(t)
	fixture.channels = 1
	fixture.blockAlign = 1
	fixture.audio = bytes.Repeat([]byte{0x55}, 501)
	fixture.omitFact = true
	fixture.afterData = []testChunk{{id: "META", payload: []byte("ok")}}
	input := fixture.build()

	output, report, err := wavfix.Repair(input)
	if err != nil {
		t.Fatal(err)
	}
	if !report.FactInserted || !report.FactChanged {
		t.Fatalf("report = inserted:%v changed:%v, want both true", report.FactInserted, report.FactChanged)
	}
	if len(output) != len(input)+12 {
		t.Fatalf("output length = %d, want input %d + fact chunk 12", len(output), len(input))
	}

	parsed, err := wavfix.Parse(output)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Fact == nil || parsed.Fact.SampleCount != 501 {
		t.Fatalf("inserted fact = %+v, want 501", parsed.Fact)
	}
	if parsed.ChunkByID("META") == nil {
		t.Fatal("unknown after-data chunk missing")
	}
	if !bytes.Equal(parsed.Data.Payload, fixture.audio) {
		t.Fatal("audio changed")
	}
}

func TestValidFileIsContainerByteIdentical(t *testing.T) {
	fixture := newG711Fixture(t)
	fixture.channels = 1
	fixture.blockAlign = 1
	fixture.audio = []byte{0x01, 0x02, 0x03, 0x04, 0x05}
	fixture.factCount = uint32Ptr(5)
	fixture.beforeFact = []testChunk{{id: "jUnk", payload: []byte{0xaa, 0xbb, 0xcc}}}
	input := fixture.build()

	output, report, err := wavfix.Repair(input)
	if err != nil {
		t.Fatal(err)
	}
	if report.FactChanged {
		t.Fatal("correct fact marked changed")
	}
	if !bytes.Equal(output, input) {
		t.Fatal("correct file was rewritten non-identically")
	}
}

func TestMisalignedStereoDataIsRejected(t *testing.T) {
	fixture := newG711Fixture(t)
	fixture.audio = []byte{0x80, 0x7f, 0x00}
	fixture.factCount = uint32Ptr(1)
	input := fixture.build()

	_, _, err := wavfix.Repair(input)
	if !errors.Is(err, wavfix.ErrMisaligned) {
		t.Fatalf("err = %v, want ErrMisaligned", err)
	}
}
