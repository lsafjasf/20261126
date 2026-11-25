package cea608

import "testing"

func TestDuplicateTabOffsetMovesOnce(t *testing.T) {
	d := NewWithOptions(Options{Parity: ParityOff})
	d.Decode([]byte{0x14, CmdRDC, 0x14, 0x40})
	d.Decode([]byte{0x17, 0x22, 0x17, 0x22}) // tab offset 2, sent twice
	d.Decode([]byte("AB"))
	if got := d.Lines()[13]; got != "  AB" {
		t.Fatalf("重复 tab offset 应只移动一次（+2 列），got %q", got)
	}
}

func TestDuplicateSpecialCharacterWrittenOnce(t *testing.T) {
	d := NewWithOptions(Options{Parity: ParityOff})
	d.Decode([]byte{0x14, CmdRDC, 0x14, 0x40})
	d.Decode([]byte{0x11, 0x37, 0x11, 0x37}) // musical note, repeated for robustness
	if got := d.Lines()[13]; got != "♪" {
		t.Fatalf("重复特殊字符对应只应写一个，got %q", got)
	}
}
