package aiffmeta

import (
	"bytes"
	"math"
	"strings"
	"testing"
)

func TestParseUnusualChunkOrderAndMetadata(t *testing.T) {
	data := unusualAIFC(t)
	file, err := ParseBytes(data)
	if err != nil {
		t.Fatalf("ParseBytes: %v", err)
	}
	if file.Format != FormatAIFC {
		t.Fatalf("format = %q, want AIFC", file.Format)
	}
	if len(file.Chunks) != 10 {
		t.Fatalf("chunks = %d, want 10", len(file.Chunks))
	}
	if got := file.Chunks[0].ID; got != "SSND" {
		t.Fatalf("first chunk = %q, want SSND", got)
	}
	if got := file.Chunks[8].ID; got != "COMM" {
		t.Fatalf("chunk 8 = %q, want COMM", got)
	}
	for i, chunk := range file.Chunks {
		if chunk.HeaderOffset != (func() int64 {
			if i == 0 {
				return 12
			}
			return file.Chunks[i-1].PaddedEnd
		}()) {
			t.Fatalf("chunk %d header offset = %d, table is not contiguous", i, chunk.HeaderOffset)
		}
		if chunk.DataOffset != chunk.HeaderOffset+8 {
			t.Fatalf("chunk %d data offset = %d", i, chunk.DataOffset)
		}
		if chunk.DataEnd != chunk.DataOffset+int64(chunk.Size) {
			t.Fatalf("chunk %d data end = %d", i, chunk.DataEnd)
		}
	}

	if file.COMM == nil {
		t.Fatal("COMM not parsed")
	}
	if file.COMM.Channels != 1 || file.COMM.SampleFrames != 8 ||
		file.COMM.SampleSize != 16 || file.COMM.SampleRate != 44100 {
		t.Fatalf("COMM = %+v", *file.COMM)
	}
	if file.COMM.CompressionType != "sowt" || file.COMM.CompressionName != "little-endian PCM" {
		t.Fatalf("AIFC compression = %q / %q",
			file.COMM.CompressionType, file.COMM.CompressionName)
	}
	if file.SSND == nil {
		t.Fatal("SSND not parsed")
	}
	if file.SSND.Offset != 5 || file.SSND.AudioSize != 16 {
		t.Fatalf("SSND = %+v", *file.SSND)
	}
	audio, err := readSection(file, file.SSND.AudioOffset, file.SSND.AudioSize)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(audio, testAudio()) {
		t.Fatalf("audio = %x, want %x", audio, testAudio())
	}
	if file.Text.Name != "Take 7" || file.Text.Author != "Audio Lab" ||
		file.Text.Copyright != "2026 Example" {
		t.Fatalf("text = %+v", file.Text)
	}
	if len(file.Text.Annotations) != 1 || file.Text.Annotations[0] != "first annotation" {
		t.Fatalf("annotations = %#v", file.Text.Annotations)
	}
	if len(file.Text.Comments) != 2 || file.Text.Comments[1].Text != "second" {
		t.Fatalf("comments = %#v", file.Text.Comments)
	}
	if len(file.Text.Markers) != 2 || file.Text.Markers[1].Position != 8 ||
		file.Text.Markers[1].Name != "Outro" {
		t.Fatalf("markers = %#v", file.Text.Markers)
	}
	if len(file.Warnings) != 0 {
		t.Fatalf("unexpected warnings: %#v", file.Warnings)
	}
}

func readSection(file *File, offset, size int64) ([]byte, error) {
	buf := make([]byte, size)
	_, err := file.source.ReadAt(buf, offset)
	return buf, err
}

func TestDecodeExtended80(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want float64
	}{
		{"44100", []byte{0x40, 0x0e, 0xac, 0x44, 0, 0, 0, 0, 0, 0}, 44100},
		{"48000", []byte{0x40, 0x0e, 0xbb, 0x80, 0, 0, 0, 0, 0, 0}, 48000},
		{"zero", make([]byte, 10), 0},
		{"negative", []byte{0xc0, 0x0e, 0xac, 0x44, 0, 0, 0, 0, 0, 0}, -44100},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := decodeExtended80(test.data)
			if err != nil {
				t.Fatalf("decodeExtended80: %v", err)
			}
			if math.Abs(got-test.want) > 1e-9 {
				t.Fatalf("got %g, want %g", got, test.want)
			}
		})
	}
}

func TestParseTrailingJunkAndTruncation(t *testing.T) {
	data := unusualAIFC(t)
	withJunk := append(append([]byte(nil), data...), []byte("trailing junk")...)
	file, err := ParseBytes(withJunk)
	if err != nil {
		t.Fatalf("ParseBytes with junk: %v", err)
	}
	if file.TrailingBytes != int64(len("trailing junk")) {
		t.Fatalf("trailing bytes = %d", file.TrailingBytes)
	}
	if file.COMM == nil || file.Text.Name != "Take 7" {
		t.Fatal("metadata changed by trailing bytes")
	}

	truncated := data[:len(data)-3]
	file, err = ParseBytes(truncated)
	if err != nil {
		t.Fatalf("ParseBytes truncated: %v", err)
	}
	if !file.FormTruncated {
		t.Fatal("FormTruncated = false")
	}
	if len(file.Warnings) == 0 {
		t.Fatal("truncation produced no warning")
	}
}

func TestInspectionContainsOffsetTableAndLostData(t *testing.T) {
	file, err := ParseBytes(unusualAIFC(t))
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := WriteInspection(&out, file); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Chunk offset table", "SSND", "COMM", "first annotation",
		"first comment", "Outro", "audio-size=16",
	} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("inspection missing %q:\n%s", want, out.String())
		}
	}
}
