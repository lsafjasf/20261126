package mp3timeline

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"
	"time"
)

func analyzeBytes(t *testing.T, data []byte) *Timeline {
	t.Helper()
	parsed, err := Analyze(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	return parsed
}

func TestAnalyzeComparesSourcesAndGapless(t *testing.T) {
	data := synthMP3(synthOptions{
		frames: 10, xingFrames: 10, delay: 576, padding: 841,
		id3v2: true, id3v1: true,
	})
	tl := analyzeBytes(t, data)

	if tl.Format.MPEGVersion != "1" || tl.Format.Layer != 3 ||
		tl.Format.SampleRate != 44100 || tl.Format.SamplesPerFrame != 1152 {
		t.Fatalf("unexpected format: %+v", tl.Format)
	}
	if tl.ID3v2Bytes != 10 || tl.ID3v1Bytes != 128 {
		t.Fatalf("id3 bytes = %d/%d", tl.ID3v2Bytes, tl.ID3v1Bytes)
	}
	if tl.FirstMPEGOffset != 10 || tl.AudioStart != 10+testFrameSize {
		t.Fatalf("offsets = first:%d audio:%d", tl.FirstMPEGOffset, tl.AudioStart)
	}
	if len(tl.Frames) != 10 || tl.Source != SourceFrameScan {
		t.Fatalf("frames=%d source=%q", len(tl.Frames), tl.Source)
	}
	if tl.RawSamples != 10*1152 || tl.PresentationSamples != 10*1152-576-841 {
		t.Fatalf("samples raw=%d presentation=%d", tl.RawSamples, tl.PresentationSamples)
	}
	if declared, ok := tl.DeclaredDuration(); !ok || declared != sampleDuration(10*1152, 44100) {
		t.Fatalf("declared duration = %s/%t", declared, ok)
	}
	if scanned := tl.ScannedDuration(); scanned != sampleDuration(10*1152, 44100) {
		t.Fatalf("scanned duration = %s", scanned)
	}
	if tl.EncoderDelay != 576 || tl.TailPadding != 841 {
		t.Fatalf("delay/padding = %d/%d", tl.EncoderDelay, tl.TailPadding)
	}
	if tl.Xing == nil || tl.Xing.FrameCount != 10 || tl.Gapless == nil {
		t.Fatalf("xing missing: %+v gapless:%+v", tl.Xing, tl.Gapless)
	}

	report := tl.Report(false)
	if report.Comparison.Relation != "same" || report.Xing.CountsMetadataFrame {
		t.Fatalf("comparison = %+v", report.Comparison)
	}
	if !report.Scan.Complete || report.Scan.AudioFrames != 10 {
		t.Fatalf("scan = %+v", report.Scan)
	}
	if report.Gapless == nil || report.Gapless.PresentationSamples != 10103 {
		t.Fatalf("gapless report = %+v", report.Gapless)
	}
	if report.Mapping.Authority != SourceFrameScan ||
		report.Mapping.EncoderDelaySamples != 576 ||
		report.Mapping.TailPaddingSamples != 841 {
		t.Fatalf("mapping = %+v", report.Mapping)
	}
}

func TestStableMappingRoundTrips(t *testing.T) {
	data := synthMP3(synthOptions{
		frames: 10, xingFrames: 10, delay: 576, padding: 841,
	})
	tl := analyzeBytes(t, data)
	if got, want := tl.Duration(), sampleDuration(10103, 44100); got != want {
		t.Fatalf("duration = %s, want %s", got, want)
	}

	for i := 0; i < len(tl.Frames); i++ {
		at, ok := tl.TimeAtFrame(i)
		if !ok {
			t.Fatalf("TimeAtFrame(%d) failed", i)
		}
		index, ok := tl.FrameAt(at)
		if !ok || index != i {
			t.Fatalf("round trip frame %d -> %s -> %d", i, at, index)
		}
		point, ok := tl.SeekPointAt(at)
		if !ok || point.Offset != tl.Frames[i].Offset || point.Frame != i {
			t.Fatalf("seek point at %s = %+v", at, point)
		}
	}

	end, ok := tl.SeekPointAt(tl.Duration())
	if !ok || !end.AtEnd || end.Frame != len(tl.Frames)-1 {
		t.Fatalf("end seek = %+v", end)
	}
	if _, ok := tl.TimeAtFrame(len(tl.Frames)); ok {
		t.Fatal("out-of-range frame should fail")
	}
}

func TestEncoderDelaySpanningFirstFrames(t *testing.T) {
	data := synthMP3(synthOptions{frames: 10, xingFrames: 10, delay: 2000, padding: 500})
	tl := analyzeBytes(t, data)
	report := tl.Report(false)
	if report.Mapping.FirstFrame != 1 {
		t.Fatalf("first presentation frame = %d", report.Mapping.FirstFrame)
	}
	if index, ok := tl.FrameAt(0); !ok || index != 1 {
		t.Fatalf("FrameAt(0) = %d/%t", index, ok)
	}
	for i := report.Mapping.FirstFrame; i < len(tl.Frames); i++ {
		at, _ := tl.TimeAtFrame(i)
		if index, ok := tl.FrameAt(at); !ok || index != i {
			t.Fatalf("round trip frame %d -> %s -> %d", i, at, index)
		}
	}
}

func TestXingCountsMetadataFrame(t *testing.T) {
	data := synthMP3(synthOptions{frames: 10, xingFrames: 11, delay: 576, padding: 841})
	tl := analyzeBytes(t, data)
	report := tl.Report(false)
	if report.Comparison.Relation != "xing_counts_metadata_frame" ||
		!report.Xing.CountsMetadataFrame ||
		report.Xing.NormalizedAudioFrames != 10 {
		t.Fatalf("comparison = %+v xing = %+v", report.Comparison, report.Xing)
	}
	if tl.Source != SourceFrameScan || tl.AudioFrames != 10 ||
		tl.PresentationSamples != 10103 {
		t.Fatalf("canonical mapping = source:%q frames:%d samples:%d",
			tl.Source, tl.AudioFrames, tl.PresentationSamples)
	}
	hasWarning(t, tl, "includes the metadata frame")
}

func TestXingMismatchUsesFrameScan(t *testing.T) {
	data := synthMP3(synthOptions{frames: 10, xingFrames: 15})
	tl := analyzeBytes(t, data)
	report := tl.Report(false)
	if report.Comparison.Relation != "mismatch" ||
		report.Comparison.FrameDelta != 5 ||
		tl.Source != SourceFrameScan || tl.AudioFrames != 10 {
		t.Fatalf("comparison = %+v source=%q frames=%d",
			report.Comparison, tl.Source, tl.AudioFrames)
	}
	hasWarning(t, tl, "declares 15 frames")
}

func TestTruncatedScanFallsBackToXingDuration(t *testing.T) {
	data := synthMP3(synthOptions{frames: 10, xingFrames: 10})
	data = data[:len(data)-100]
	tl := analyzeBytes(t, data)
	report := tl.Report(false)
	if len(tl.Frames) != 9 || !tl.Truncated || tl.Source != SourceXingDeclared {
		t.Fatalf("frames=%d truncated=%t source=%q", len(tl.Frames), tl.Truncated, tl.Source)
	}
	if tl.RawSamples != 10*1152 || tl.AudioFrames != 10 {
		t.Fatalf("fallback samples raw=%d frames=%d", tl.RawSamples, tl.AudioFrames)
	}
	if !report.Scan.Truncated || report.Scan.Complete {
		t.Fatalf("scan report should be incomplete: %+v", report.Scan)
	}
	if point, ok := tl.SeekPointAt(tl.Duration()); !ok || !point.AtEnd || point.Frame != 8 {
		t.Fatalf("seek at fallback end = %+v", point)
	}
	hasWarning(t, tl, "extends past")
}

func TestNoXingUsesFrameScan(t *testing.T) {
	data := synthMP3(synthOptions{frames: 5, noXing: true})
	tl := analyzeBytes(t, data)
	if tl.Xing != nil || tl.Source != SourceFrameScan || tl.AudioFrames != 5 {
		t.Fatalf("xing=%+v source=%q frames=%d", tl.Xing, tl.Source, tl.AudioFrames)
	}
	if tl.Duration() != sampleDuration(5*1152, 44100) {
		t.Fatalf("duration = %s", tl.Duration())
	}
}

func TestVBRIHeader(t *testing.T) {
	tag := mpegTestFrame()
	copy(tag[36:40], "VBRI")
	binary.BigEndian.PutUint16(tag[40:42], 1)
	binary.BigEndian.PutUint32(tag[46:50], uint32(testFrameSize*6))
	binary.BigEndian.PutUint32(tag[50:54], 5)
	data := append(tag, bytes.Repeat(mpegTestFrame(), 5)...)

	tl := analyzeBytes(t, data)
	if tl.Xing == nil || tl.Xing.Kind != "VBRI" || tl.Xing.FrameCount != 5 {
		t.Fatalf("VBRI header = %+v", tl.Xing)
	}
	if tl.Source != SourceFrameScan || tl.AudioFrames != 5 {
		t.Fatalf("source=%q frames=%d", tl.Source, tl.AudioFrames)
	}
}

func TestMPEG2Layer3FrameClock(t *testing.T) {
	frame := make([]byte, 208)
	copy(frame, []byte{0xff, 0xf3, 0x80, 0xc0})
	data := bytes.Repeat(frame, 5)
	tl := analyzeBytes(t, data)
	if tl.Format.MPEGVersion != "2" || tl.Format.SampleRate != 22050 ||
		tl.Format.SamplesPerFrame != 576 || tl.AudioFrames != 5 {
		t.Fatalf("format=%+v frames=%d", tl.Format, tl.AudioFrames)
	}
	if tl.RawSamples != 5*576 {
		t.Fatalf("raw samples = %d", tl.RawSamples)
	}
}

func hasWarning(t *testing.T, tl *Timeline, fragment string) {
	t.Helper()
	for _, warning := range tl.Warnings {
		if strings.Contains(warning, fragment) {
			return
		}
	}
	t.Fatalf("warning containing %q not found in %q", fragment, tl.Warnings)
}

func sampleDuration(samples int64, rate int) time.Duration {
	return time.Duration(samples) * time.Second / time.Duration(rate)
}
