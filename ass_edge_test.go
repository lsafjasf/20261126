package asskaraoke

import "testing"

func TestParseASSTimeErrors(t *testing.T) {
	for _, s := range []string{"", "12.34", "1:2:3:4", "a:00:00.00", "0:60:00.00"} {
		if _, err := ParseASSTime(s); err == nil {
			t.Fatalf("expected error for %q", s)
		}
	}
}

func TestParseColorVariants(t *testing.T) {
	c, ok := parseASSColor("&HFF0000&") // 6 位 BBGGRR：纯蓝
	if !ok || c != (Color{R: 0, G: 0, B: 255, A: 0}) {
		t.Fatalf("6-bit color = %+v ok=%v", c, ok)
	}
	c, ok = parseASSColor("&H80FF0000&") // 8 位 AABBGGRR：半透明蓝
	if !ok || c.A != 0x80 || c.B != 255 {
		t.Fatalf("8-bit color = %+v", c)
	}
	if _, ok := parseASSColor("not a color"); ok {
		t.Fatal("invalid color should fail")
	}
}

func TestUnclosedTagBlock(t *testing.T) {
	doc, err := ParseASS("[Events]\nFormat: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Text\n" +
		"Dialogue: 0,0:00:00.00,0:00:01.00,S,,0,0,0,{\\kf10}abc { unclosed\n")
	if err != nil {
		t.Fatal(err)
	}
	if got := doc.Events[0].Plain; got != "abc { unclosed" {
		t.Fatalf("unclosed block plain = %q", got)
	}
}

func TestTextWithCommasAndHardSpace(t *testing.T) {
	doc, err := ParseASS("[Events]\nFormat: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Text\n" +
		"Dialogue: 0,0:00:00.00,0:00:01.00,S,,0,0,0,{\\kf10}a,\\hb\n")
	if err != nil {
		t.Fatal(err)
	}
	if got := doc.Events[0].Plain; got != "a, b" {
		t.Fatalf("comma/hardspace plain = %q", got)
	}
}
