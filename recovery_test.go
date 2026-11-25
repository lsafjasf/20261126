package cea608

import "testing"

func TestBadParityContinuesCaption(t *testing.T) {
	d := New()
	bad := []byte{0x41, 0x42} // both have even parity and are invalid on the wire.
	d.Decode(catBytes(testRCL, testPACRow14, encodeText("AB"), bad, encodeText("CD"), testEOC))
	if d.BadPairs != 1 {
		t.Fatalf("应统计 1 个坏字节对，got %d", d.BadPairs)
	}
	if got := d.Lines()[13]; got != "ABCD" {
		t.Fatalf("坏字节后应继续解码，got %q", got)
	}
}

func TestFirstBadControlRecordedOnce(t *testing.T) {
	d := New()
	badOne := ccPair(0x14, 0x10)
	badTwo := ccPair(0x15, 0x11)
	d.Decode(catBytes(testRCL, testPACRow14, encodeText("OK"), badOne, badTwo, testEOC))

	if d.BadControls != 2 {
		t.Fatalf("应记录两个坏控制码，got %d", d.BadControls)
	}
	if d.FirstBadControl == nil {
		t.Fatal("必须记录首个坏控制码")
	}
	if got := d.FirstBadControl.Code; got != 0x1410 {
		t.Fatalf("首个坏控制码应为 1410，got %04X", got)
	}
	if d.FirstBadCode != 0x1410 {
		t.Fatalf("FirstBadCode 应为 1410，got %04X", d.FirstBadCode)
	}
	if d.FirstBadControl.Offset != 6 {
		t.Fatalf("首个坏控制码字节偏移应为 6，got %d", d.FirstBadControl.Offset)
	}
	if got := d.Lines()[13]; got != "OK" {
		t.Fatalf("坏控制码不应污染字幕且后续 EOC 应上屏，got %q", got)
	}
}

func TestPlainSevenBitInputDoesNotRequireParity(t *testing.T) {
	d := New()
	d.Decode([]byte{0x14, CmdRCL, 0x14, 0x40, 'H', 'i', 0x14, CmdEOC})
	if d.FirstBadControl != nil {
		t.Fatalf("七比特输入不应误报坏控制码: %#v", d.FirstBadControl)
	}
	if got := d.Lines()[13]; got != "Hi" {
		t.Fatalf("普通七比特控制码/文本应解码，got %q", got)
	}
}

func TestPendingPairCompletesAcrossWrites(t *testing.T) {
	d := New()
	full := catBytes(testRDC, testPACRow15, encodeText("LIVE"))
	for i := 0; i < len(full); i++ {
		d.Decode(full[i : i+1])
	}
	if got := d.Lines()[14]; got != "LIVE" {
		t.Fatalf("逐字节喂入也应拼对并显示，got %q", got)
	}
}

func TestTruncatedPairCanResync(t *testing.T) {
	d := New()
	d.Decode(catBytes(testRCL, testPACRow14, encodeText("STALE")))
	d.Decode([]byte{0x14}) // first byte of a truncated two-byte control pair
	if pending, ok := d.PendingByte(); !ok || pending != 0x14 {
		t.Fatalf("应缓存截断对的首字节，pending=%#v ok=%v", pending, ok)
	}
	if !d.Resync() {
		t.Fatal("Resync 应丢弃半个截断对")
	}
	d.Decode(catBytes(testENM, testPACRow15, encodeText("FRESH"), testEOC))
	if got := d.Lines()[14]; got != "FRESH" {
		t.Fatalf("Resync 后新字幕应正常上屏，got %q", got)
	}
	if d.TruncatedPairs != 1 || d.Recoveries < 1 {
		t.Fatalf("应记录截断/恢复，trunc=%d recover=%d", d.TruncatedPairs, d.Recoveries)
	}
}

func TestDecodeSegmentDiscardsInterruptedHalfPair(t *testing.T) {
	d := New()
	d.Decode(catBytes(testRCL, testPACRow14, encodeText("STALE"), []byte{0x14}))
	d.DecodeSegment(catBytes(testENM, testPACRow15, encodeText("FRESH"), testEOC))
	if got := d.Lines()[14]; got != "FRESH" {
		t.Fatalf("新段应从双字节边界恢复，got %q", got)
	}
	if d.TruncatedPairs != 1 {
		t.Fatalf("应记录一次截断，got %d", d.TruncatedPairs)
	}
}

func TestParseReportsTrailingTruncationAndContinuesResult(t *testing.T) {
	result := Parse(catBytes(testRDC, testPACRow15, encodeText("ABC"), []byte{0x14}))
	if result.TruncatedPairs != 1 {
		t.Fatalf("尾部单字节应记为截断，got %d", result.TruncatedPairs)
	}
	if result.DisplayText != "ABC" {
		t.Fatalf("截断前字幕应保留，got %q", result.DisplayText)
	}
}

func TestResetClearsBadAndPending(t *testing.T) {
	d := New()
	d.Decode(catBytes(ccPair(0x14, 0x10), []byte{'X'}))
	d.Reset()
	if d.Mode() != ModeNone || d.BadPairs != 0 || d.FirstBadControl != nil {
		t.Fatal("Reset 后模式、计数和坏码记录应清空")
	}
	if _, ok := d.PendingByte(); ok {
		t.Fatal("Reset 后不应保留半个字节对")
	}
}

func TestOptionalRewindRecovery(t *testing.T) {
	d := NewWithOptions(Options{Parity: ParityOff, RewindOnBadLead: true})
	// 14 30 is not a legal 0x14-class command here. Recovery treats 30 as
	// the first byte of the next pair, completed by 41 ('A').
	d.Decode([]byte{0x14, 0x30, 0x41})
	if d.FirstBadCode != 0x1430 {
		t.Fatalf("应记录 1430，got %04x", d.FirstBadCode)
	}
	if d.Recoveries != 1 {
		t.Fatalf("应记录一次回卷恢复，got %d", d.Recoveries)
	}
	if d.Caption.Text != "0A" {
		t.Fatalf("回卷后应保留下一对字节，got %q", d.Caption.Text)
	}
}
