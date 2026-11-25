package opusalign

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func buildOggPage(t *testing.T, headerType byte, granule int64, sequence uint32, payloads ...[]byte) []byte {
	t.Helper()
	segmentTable := make([]byte, 0, len(payloads))
	data := make([]byte, 0)
	for _, payload := range payloads {
		if len(payload) >= 255 {
			t.Fatal("test helper only supports packets shorter than 255 bytes")
		}
		segmentTable = append(segmentTable, byte(len(payload)))
		data = append(data, payload...)
	}

	page := make([]byte, 27+len(segmentTable)+len(data))
	copy(page[0:4], "OggS")
	page[4] = 0
	page[5] = headerType
	binary.LittleEndian.PutUint64(page[6:14], uint64(granule))
	binary.LittleEndian.PutUint32(page[14:18], 1)
	binary.LittleEndian.PutUint32(page[18:22], sequence)
	page[26] = byte(len(segmentTable))
	copy(page[27:27+len(segmentTable)], segmentTable)
	copy(page[27+len(segmentTable):], data)
	return page
}

func TestAnalyzeOggGranuleBackwards(t *testing.T) {
	head := make([]byte, 19)
	copy(head[0:8], "OpusHead")
	head[8] = 1
	head[9] = 1
	binary.LittleEndian.PutUint16(head[10:12], 0)

	var stream bytes.Buffer
	stream.Write(buildOggPage(t, oggFlagBOS, 0, 0, head))
	stream.Write(buildOggPage(t, 0, 480, 1, []byte{0x00}))
	stream.Write(buildOggPage(t, oggFlagEOS, 1920, 2, []byte{0x00}, []byte{0x00}))

	report, parsedHead, err := AnalyzeOgg(&stream)
	if err != nil {
		t.Fatal(err)
	}
	if int(parsedHead.PreSkip) != 0 {
		t.Fatalf("pre-skip = %d", parsedHead.PreSkip)
	}
	if len(report.Intervals) != 3 {
		t.Fatalf("intervals = %d", len(report.Intervals))
	}
	last := report.Intervals[2]
	if last.Raw != (SampleRange{1440, 1920}) {
		t.Fatalf("last raw range = %+v", last.Raw)
	}
	if len(report.MissingRanges) != 1 || report.MissingRanges[0].Range != (SampleRange{480, 960}) {
		t.Fatalf("missing ranges = %+v", report.MissingRanges)
	}
	if !last.Received || !last.EndOfStream {
		t.Fatalf("last packet EOS state wrong: %+v", last)
	}
}

func TestReadOggRejectsBadMagic(t *testing.T) {
	_, err := ReadOggPackets(bytes.NewReader([]byte("not ogg data at all")))
	if err == nil {
		t.Fatal("bad Ogg data must fail")
	}
}

func TestAnalyzeOggEOSTrimDoesNotShiftEarlierPacket(t *testing.T) {
	head := make([]byte, 19)
	copy(head[0:8], "OpusHead")
	head[9] = 1

	var stream bytes.Buffer
	stream.Write(buildOggPage(t, oggFlagBOS, 0, 0, head))
	stream.Write(buildOggPage(t, 0, 480, 1, []byte{0x00}))
	stream.Write(buildOggPage(t, oggFlagEOS, 1380, 2, []byte{0x00}, []byte{0x00}))

	report, _, err := AnalyzeOgg(&stream)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Intervals) != 3 {
		t.Fatalf("intervals = %d", len(report.Intervals))
	}
	middle := report.Intervals[1]
	last := report.Intervals[2]
	if middle.Raw != (SampleRange{480, 960}) {
		t.Fatalf("middle raw range = %+v", middle.Raw)
	}
	if last.Raw != (SampleRange{960, 1380}) || last.EndTrimSamples != 60 {
		t.Fatalf("last raw=%+v trim=%d", last.Raw, last.EndTrimSamples)
	}
	for _, issue := range last.Issues {
		if issue.Code == IssueOverlap || issue.Code == IssueDuplicate {
			t.Fatalf("EOS trim misclassified: %+v", last.Issues)
		}
	}
}
