package cea608

import (
	"strings"
	"testing"
)

func pbyte(b byte) byte { return WithOddParity(b) }

func ccPair(b1, b2 byte) []byte { return []byte{pbyte(b1), pbyte(b2)} }

func encodeText(s string) []byte {
	out := make([]byte, 0, len(s)+2)
	for i := 0; i < len(s); i += 2 {
		b2 := byte(0)
		if i+1 < len(s) {
			b2 = s[i+1]
		}
		out = append(out, pbyte(s[i]), pbyte(b2))
	}
	return out
}

func catBytes(parts ...[]byte) []byte {
	var out []byte
	for _, part := range parts {
		out = append(out, part...)
	}
	return out
}

var (
	testRCL      = ccPair(0x14, 0x20)
	testRDC      = ccPair(0x14, 0x29)
	testRU2      = ccPair(0x14, 0x25)
	testRU3      = ccPair(0x14, 0x26)
	testCR       = ccPair(0x14, 0x2d)
	testEDM      = ccPair(0x14, 0x2c)
	testENM      = ccPair(0x14, 0x2e)
	testEOC      = ccPair(0x14, 0x2f)
	testPACRow14 = ccPair(0x14, 0x40)
	testPACRow15 = ccPair(0x14, 0x60)
)

func TestPopOnFlipsOnEOC(t *testing.T) {
	d := New()
	d.Decode(catBytes(testRCL, testPACRow14, encodeText("HELLO")))
	if got := strings.Join(d.VisibleLines(), ""); got != "" {
		t.Fatalf("EOC 前不应上屏，got %q", got)
	}
	d.Decode(testEOC)
	if got := d.Lines()[13]; got != "HELLO" {
		t.Fatalf("EOC 后第 14 行应为 HELLO，got %q", got)
	}
	if d.Mode() != ModePopOn {
		t.Fatalf("模式应为 pop-on，got %v", d.Mode())
	}
	if len(d.Caption.Cues) != 1 || d.Caption.Cues[0].Text != "HELLO" {
		t.Fatalf("应生成一条 HELLO cue，got %#v", d.Caption.Cues)
	}
}

func TestPopOnEraseNonDisplayed(t *testing.T) {
	d := New()
	d.Decode(catBytes(testRCL, testPACRow14, encodeText("DRAFT"), testENM, testEOC))
	if got := strings.Join(d.VisibleLines(), ""); got != "" {
		t.Fatalf("ENM 后 EOC 不应翻出内容，got %q", got)
	}
}

func TestPaintOnImmediateAndErase(t *testing.T) {
	d := New()
	d.Decode(catBytes(testRDC, testPACRow15, encodeText("LIVE")))
	if got := d.Lines()[14]; got != "LIVE" {
		t.Fatalf("paint-on 应逐字直接上屏，got %q", got)
	}
	d.Decode(testEDM)
	if got := strings.Join(d.VisibleLines(), ""); got != "" {
		t.Fatalf("EDM 应清空显示内存，got %q", got)
	}
}

func TestRollUpFastScroll(t *testing.T) {
	d := New()
	stream := catBytes(testRU2, testRU2, testPACRow15)
	lines := []string{"ONE", "TWO", "THREE", "FOUR"}
	for i, line := range lines {
		stream = catBytes(stream, encodeText(line))
		if i < len(lines)-1 {
			stream = catBytes(stream, testCR, testCR)
		}
	}
	d.Decode(stream)
	got := d.Lines()
	if got[13] != "THREE" || got[14] != "FOUR" {
		t.Fatalf("快速滚屏后应剩最后两行，got row14=%q row15=%q", got[13], got[14])
	}
}

func TestRollUpThreeLineWindow(t *testing.T) {
	d := New()
	stream := catBytes(testRU3, testPACRow15)
	lines := []string{"A", "B", "C", "D"}
	for i, line := range lines {
		stream = catBytes(stream, encodeText(line))
		if i < len(lines)-1 {
			stream = catBytes(stream, testCR)
		}
	}
	d.Decode(stream)
	got := d.Lines()
	if got[12] != "B" || got[13] != "C" || got[14] != "D" {
		t.Fatalf("3 行窗口应为 B/C/D，got %q/%q/%q", got[12], got[13], got[14])
	}
}

func TestSplitPairsRemainEquivalent(t *testing.T) {
	stream := catBytes(testRCL, testRCL, testPACRow14, encodeText("HELLO"),
		testPACRow15, encodeText("WORLD"), testEOC, testEOC)
	want := New()
	want.Decode(stream)

	for _, size := range []int{1, 3, 5, 7} {
		got := New()
		for i := 0; i < len(stream); i += size {
			end := min(i+size, len(stream))
			got.Decode(stream[i:end])
		}
		if got.String() != want.String() {
			t.Fatalf("按 %d 字节切碎后结果不一致:\ngot:\n%s\nwant:\n%s",
				size, got.String(), want.String())
		}
	}
}
