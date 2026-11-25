package smptett

import "testing"

func TestApplyRawStyleUsesDeterministicDependencies(t *testing.T) {
	style := defaultStyle()
	applyRawStyle(&style, map[string]string{
		"fontSize":   "20px",
		"lineHeight": "150%",
		"origin":     "10px 20%",
		"extent":     "50% 30px",
	}, 640, 360)
	if style.FontSize != 20 {
		t.Fatalf("font size = %v", style.FontSize)
	}
	if style.LineHeight != 30 {
		t.Fatalf("line height = %v, want 30", style.LineHeight)
	}
	if style.OriginX != 10 || style.OriginY != 20 || style.OriginXUnit != "px" || style.OriginYUnit != "%" {
		t.Fatalf("mixed origin = %+v", style)
	}
	if style.ExtentW != 50 || style.ExtentH != 30 || style.ExtentXUnit != "%" || style.ExtentYUnit != "px" {
		t.Fatalf("mixed extent = %+v", style)
	}
}

func TestParseColorFormats(t *testing.T) {
	tests := map[string]string{
		"#fff":      "#ffffffff",
		"#abcd":     "#aabbccdd",
		"#112233":   "#112233ff",
		"#11223344": "#11223344",
		"white":     "#ffffffff",
	}
	for input, want := range tests {
		c, ok := parseColor(input)
		if !ok {
			t.Fatalf("parseColor(%q) failed", input)
		}
		got := "#" + hexByte(c.R) + hexByte(c.G) + hexByte(c.B) + hexByte(c.A)
		if got != want {
			t.Errorf("parseColor(%q) = %s, want %s", input, got, want)
		}
	}
}

func hexByte(v uint8) string {
	const digits = "0123456789abcdef"
	return string([]byte{digits[v>>4], digits[v&0x0f]})
}
