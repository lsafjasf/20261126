package encodingx

import (
	"testing"
)

func TestDecodeBOMs(t *testing.T) {
	cases := []struct {
		name string
		raw  []byte
		want Kind
	}{
		{"utf8 plain", []byte("hello 世界"), KindUTF8},
		{"utf8 bom", append([]byte{0xEF, 0xBB, 0xBF}, "hello"...), KindUTF8BOM},
		{"utf16le bom", append([]byte{0xFF, 0xFE}, 0x41, 0x00, 0x42, 0x00), KindUTF16LEBOM},
		{"utf16be bom", append([]byte{0xFE, 0xFF}, 0x00, 0x41, 0x00, 0x42), KindUTF16BEBOM},
		{"utf32le bom", append([]byte{0xFF, 0xFE, 0x00, 0x00}, 0x41, 0x00, 0x00, 0x00), KindUTF32LEBOM},
		{"utf32be bom", append([]byte{0x00, 0x00, 0xFE, 0xFF}, 0x00, 0x00, 0x00, 0x41), KindUTF32BEBOM},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Decode(tc.raw)
			if got.Encoding != tc.want {
				t.Fatalf("encoding = %s, want %s", got.Encoding, tc.want)
			}
			if got.ReplacementCount != 0 && tc.want != KindUnknown {
				t.Fatalf("unexpected replacement runes: %d", got.ReplacementCount)
			}
		})
	}
}

func TestDecodeGB18030(t *testing.T) {
	// "中文" in GB18030 is D6 D0 CE C4.
	raw := []byte{0xD6, 0xD0, 0xCE, 0xC4, 'a'}
	got := Decode(raw)
	if got.Encoding != KindGB18030 {
		t.Fatalf("encoding = %s, want gb18030", got.Encoding)
	}
	if got.Text != "中文a" {
		t.Fatalf("decoded = %q, want %q", got.Text, "中文a")
	}
}

func TestDecodeInvalidUTF8Counted(t *testing.T) {
	raw := []byte{'a', 0xFF, 0xFE, 'b'}
	got := Decode(raw)
	if !got.HasReplacement || got.ReplacementCount == 0 {
		t.Fatalf("expected U+FFFD accounting, got %+v", got)
	}
}

func TestMojibakeDetection(t *testing.T) {
	// Double-encode "é" style bytes: original UTF-8 bytes reinterpreted as
	// Latin-1 then saved as UTF-8 again.
	original := []byte{0xC3, 0xA9, 0xC3, 0xA8, 0xC3, 0xAA, 0xC2, 0xA0}
	got := Decode(original)
	if !got.Mojibake {
		t.Fatalf("expected mojibake flag for % x", original)
	}
}

func TestUTF16NoBOMHeuristic(t *testing.T) {
	// "AB" as raw UTF-16LE without BOM.
	got := Decode([]byte{'A', 0x00, 'B', 0x00})
	if got.Encoding != KindUTF16LENOBM {
		t.Fatalf("encoding = %s, want utf-16le no-bom", got.Encoding)
	}
	if got.Text != "AB" {
		t.Fatalf("text = %q, want AB", got.Text)
	}
}
