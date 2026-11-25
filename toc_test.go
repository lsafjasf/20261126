package opusalign

import "testing"

func TestParseTOCByteDurations(t *testing.T) {
	tests := []struct {
		name string
		toc  byte
		want int
	}{
		{"silk-nb-10ms", 0x00, 480},
		{"silk-nb-20ms-code1", 0x01, 960},
		{"silk-nb-20ms-code2", 0x02, 960},
		{"silk-nb-60ms", 0x18, 2880},
		{"hybrid-swb-10ms", 0x60, 480},
		{"hybrid-fb-20ms", 0x78, 960},
		{"celt-nb-2_5ms", 0x80, 120},
		{"celt-fb-20ms", 0xf8, 960},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseTOCByte(tt.toc)
			if !got.DurationKnown || got.DurationSamples != tt.want {
				t.Fatalf("ParseTOCByte(%#x) duration = %d known=%v, want %d", tt.toc, got.DurationSamples, got.DurationKnown, tt.want)
			}
		})
	}
}

func TestPacketDurationCodeThree(t *testing.T) {
	duration, toc, err := PacketDurationSamples([]byte{0x03, 0x03})
	if err != nil {
		t.Fatal(err)
	}
	if duration != 1440 || toc.FrameCount != 3 || toc.VBR || toc.Padding {
		t.Fatalf("code 3 duration=%d toc=%+v", duration, toc)
	}

	_, _, err = PacketDurationSamples([]byte{0x03, 0x00})
	if err == nil {
		t.Fatal("frame count zero must be rejected")
	}

	_, _, err = PacketDurationSamples([]byte{0x1b, 0x03})
	if err == nil {
		t.Fatal("180 ms packet must be rejected")
	}
}

func TestPacketDurationTruncatedCodeThree(t *testing.T) {
	_, _, err := PacketDurationSamples([]byte{0x03})
	if err == nil {
		t.Fatal("code 3 without frame count byte must fail")
	}
}

func TestParseOpusHeadPreSkip(t *testing.T) {
	head := make([]byte, 19)
	copy(head[0:8], "OpusHead")
	head[8] = 1
	head[9] = 2
	head[10] = 0xe8
	head[11] = 0x03

	got, err := PreSkipFromOpusHead(head)
	if err != nil {
		t.Fatal(err)
	}
	if want := 1000; got != want {
		t.Fatalf("pre-skip = %d, want %d", got, want)
	}
}

func TestParseOpusHeadRejectsBadMagic(t *testing.T) {
	head := make([]byte, 19)
	copy(head[0:8], "OpusNope")
	if _, err := ParseOpusHead(head); err == nil {
		t.Fatal("bad magic must fail")
	}
}
