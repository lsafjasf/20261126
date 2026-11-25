package ac3select

import (
	"errors"
	"testing"
)

func TestParseHeaderChannelModes(t *testing.T) {
	for acmod := AudioCodingMode(0); acmod <= 7; acmod++ {
		for _, lfe := range []bool{false, true} {
			frame := makeTestFrame(testFrameSpec{
				bsmod: BSModeCompleteMain, acmod: acmod, lfeon: lfe, crc: true,
			})
			parsed, err := ParseFrame(frame)
			if err != nil {
				t.Fatalf("acmod=%d lfe=%v ParseFrame: %v", acmod, lfe, err)
			}
			h := parsed.Header
			if h.ACMod != acmod || h.LFEOn != lfe {
				t.Fatalf("header fields = acmod%d/lfe%v, want %d/%v", h.ACMod, h.LFEOn, acmod, lfe)
			}
			wantChannels := fullBandChannelCount[acmod] + boolToInt(lfe)
			if h.ChannelCount != wantChannels || len(h.Channels) != wantChannels {
				t.Fatalf("acmod=%d lfe=%v channels=%d/%d slice=%d, want %d",
					acmod, lfe, h.ChannelCount, h.FullBandChannels, len(h.Channels), wantChannels)
			}
			if h.SampleRate != 44100 || h.FrameSize != 834 {
				t.Fatalf("sample rate/frame size = %d/%d", h.SampleRate, h.FrameSize)
			}
			if h.CRC == nil || !h.CRC.Valid {
				t.Fatalf("acmod=%d lfe=%v CRC = %+v, want valid", acmod, lfe, h.CRC)
			}
		}
	}
}

func TestParse51Coupling(t *testing.T) {
	frame := makeTestFrame(testFrameSpec{
		bsmod:   BSModeCompleteMain,
		acmod:   ACMod3_2,
		lfeon:   true,
		couple:  true,
		coupled: []bool{true, true, true, true, true},
		start:   12,
		end:     17,
	})
	parsed, err := ParseFrame(frame)
	if err != nil {
		t.Fatalf("ParseFrame: %v", err)
	}
	c := parsed.Header.Coupling
	if !c.StrategyPresent || !c.InUse {
		t.Fatalf("coupling = %+v, want strategy and in-use", c)
	}
	wantLabels := []string{"L", "C", "R", "Ls", "Rs"}
	if !equalStrings(c.CoupledChannels, wantLabels) {
		t.Fatalf("CoupledChannels = %v, want %v", c.CoupledChannels, wantLabels)
	}
	if c.StartSubband != 12 || c.EndSubband != 17 {
		t.Fatalf("subbands = %d/%d", c.StartSubband, c.EndSubband)
	}
	if c.StartFrequency != 181 || c.EndFrequency != 241 {
		t.Fatalf("frequencies = %d/%d", c.StartFrequency, c.EndFrequency)
	}
	if c.BandCount != 4 || len(c.BandSizes) != 4 {
		t.Fatalf("bands = count%d sizes%v, want 4 explicit bands", c.BandCount, c.BandSizes)
	}
	for _, ch := range parsed.Header.Channels {
		if ch.Label != "LFE" && !ch.InCoupling {
			t.Fatalf("full-band channel %s was not marked coupled", ch.Label)
		}
		if ch.Label == "LFE" && ch.InCoupling {
			t.Fatal("LFE must not be marked coupled")
		}
	}
}

func TestProgramSegmentsSelectionAndBadFrameRecovery(t *testing.T) {
	main := makeTestFrame(testFrameSpec{bsmod: BSModeCompleteMain, acmod: ACMod3_2, lfeon: true})
	commentary := makeTestFrame(testFrameSpec{bsmod: BSModeCommentary, acmod: ACMod2_0})
	bad := []byte{0x0B, 0x77, 0xDE, 0xAD, 0xC0, 0x00}
	bad = append(bad, make([]byte, 94)...)

	stream := concat(main, main, bad, commentary, commentary, main)
	report := ParseBytes(stream)

	if report.FrameCount != 5 {
		t.Fatalf("FrameCount = %d, want 5; bad=%+v", report.FrameCount, report.BadFrames)
	}
	if report.BadCount != 1 || len(report.BadFrames) != 1 {
		t.Fatalf("BadFrames = %+v, want one recovered range", report.BadFrames)
	}
	if got := report.BadFrames[0]; got.Offset != 2*int64(len(main)) || got.Length != int64(len(bad)) {
		t.Fatalf("bad range = offset%d length%d, want %d/%d",
			got.Offset, got.Length, 2*len(main), len(bad))
	}
	if len(report.Segments) != 3 {
		t.Fatalf("Segments = %+v, want main/commentary/main runs", report.Segments)
	}
	wantCounts := []int{2, 2, 1}
	for i, want := range wantCounts {
		if report.Segments[i].FrameCount != want {
			t.Fatalf("segment %d frame count = %d, want %d", i, report.Segments[i].FrameCount, want)
		}
	}
	if report.Segments[1].BitstreamMode != BSModeCommentary ||
		!report.Segments[1].Track.IsCommentary {
		t.Fatalf("commentary segment = %+v", report.Segments[1])
	}
	if report.Selection.Main == nil || report.Selection.Main.SegmentIndex != 0 {
		t.Fatalf("main selection = %+v, want segment 0", report.Selection.Main)
	}
	if report.Selection.Commentary == nil || report.Selection.Commentary.SegmentIndex != 1 {
		t.Fatalf("commentary selection = %+v, want segment 1", report.Selection.Commentary)
	}
}

func TestInvalidCouplingFrameIsBypassedAtDeclaredBoundary(t *testing.T) {
	bad := makeTestFrame(testFrameSpec{acmod: ACMod1_0, couple: true})
	good := makeTestFrame(testFrameSpec{acmod: ACMod2_0})
	report := ParseBytes(concat(bad, good))

	if report.FrameCount != 1 {
		t.Fatalf("FrameCount = %d, want one recovered frame", report.FrameCount)
	}
	if report.BadCount != 1 || report.BadFrames[0].Length != int64(len(bad)) {
		t.Fatalf("BadFrames = %+v, want declared-size bypass", report.BadFrames)
	}
	if _, err := ParseFrame(bad); !errors.Is(err, ErrCoupling) {
		t.Fatalf("bad frame error = %v, want ErrCoupling", err)
	}
}

func TestStrictCRCBypassesCorruptFrame(t *testing.T) {
	corrupt := makeTestFrame(testFrameSpec{acmod: ACMod2_0, crc: true})
	corrupt[100] ^= 0xFF
	good := makeTestFrame(testFrameSpec{acmod: ACMod2_0, crc: true})

	lenient := ParseBytes(concat(corrupt, good))
	if lenient.FrameCount != 2 || lenient.BadCount != 0 {
		t.Fatalf("lenient scan frames=%d bad=%d, want metadata retained", lenient.FrameCount, lenient.BadCount)
	}
	strict := ParseBytesWithOptions(concat(corrupt, good), Options{StrictCRC: true})
	if strict.FrameCount != 1 || strict.BadCount != 1 {
		t.Fatalf("strict scan frames=%d bad=%+v, want corrupt bypassed", strict.FrameCount, strict.BadFrames)
	}
	if strict.Segments[0].StartOffset != int64(len(corrupt)) {
		t.Fatalf("recovered segment offset = %d, want %d", strict.Segments[0].StartOffset, len(corrupt))
	}
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func concat(parts ...[]byte) []byte {
	var out []byte
	for _, part := range parts {
		out = append(out, part...)
	}
	return out
}
