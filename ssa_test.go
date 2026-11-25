package ssastyle

import (
	"strings"
	"testing"
)

const sampleSSA = `[Script Info]
ScriptType: v4.00

[V4 Styles]
Format: Name, Fontname, Fontsize, PrimaryColour, SecondaryColour, TertiaryColour, BackColour, Bold, Italic, BorderStyle, Outline, Shadow, Alignment, MarginL, MarginR, MarginV, AlphaLevel, Encoding
Style: Default,Arial,20,&H00FFFFFF,&H000000FF,&H00000000,&H80000000,0,0,1,2,2,2,10,10,10,0,1
Style: Default,Arial,22,,,,,,,,,,,,,,,,,1
Style: Title,Impact,48,&H000000FF,&H000000FF,&H00000000,&H00000000,-1,0,1,3,0,5,20,20,30,0,1
`

func TestParseSSADefaultColumnsAndDuplicates(t *testing.T) {
	doc, diags, err := ParseSSA(strings.NewReader(sampleSSA), "sample.ssa")
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Styles) != 3 {
		t.Fatalf("应保留 3 条 Style 记录, got %d", len(doc.Styles))
	}
	cat := NewCatalog(DefaultMergeOptions(), doc)
	eff := cat.Resolve("Default")
	// 第二条重复 Default 只给了 Fontsize=22 和空列；空列不得覆盖第一条。
	if got, _ := eff.Value("fontname"); got != "Arial" {
		t.Fatalf("重复 Style 行的空列覆盖了 Fontname: %q", got)
	}
	if got, _ := eff.Value("fontsize"); got != "22" {
		t.Fatalf("Fontsize 应被重复行的有效值更新为 22, got %q", got)
	}
	if got, _ := eff.Value("primarycolor"); got != "&H00FFFFFF" {
		t.Fatalf("空颜色列不应覆盖, got %q", got)
	}
	foundDup := false
	for _, d := range cat.Diagnostics {
		if d.Code == DiagDuplicateStyle {
			foundDup = true
		}
	}
	if !foundDup {
		t.Fatalf("应对重复样式名给出诊断: %+v / parse %+v", cat.Diagnostics, diags)
	}
}

func TestParseSSAAlignmentDialect(t *testing.T) {
	doc, _, err := ParseSSA(strings.NewReader(sampleSSA), "sample.ssa")
	if err != nil {
		t.Fatal(err)
	}
	cat := NewCatalog(DefaultMergeOptions(), doc)
	title := cat.Resolve("Title")
	raw, _ := title.Value("alignment")
	if raw != "5" {
		t.Fatalf("raw alignment = %q, want 5", raw)
	}
	norm, err := NormalizeValue("alignment", raw, DialectSSA)
	if err != nil || norm != "7" {
		t.Fatalf("SSA alignment 5 应换算为 ASS 7, got %q err=%v", norm, err)
	}
}

const sampleASS = `[V4+ Styles]
Format: Name, Fontname, Fontsize, PrimaryColour, SecondaryColour, OutlineColour, BackColour, Bold, Italic, Underline, StrikeOut, ScaleX, ScaleY, Spacing, Angle, BorderStyle, Outline, Shadow, Alignment, MarginL, MarginR, MarginV, Encoding
Style: Default,Calibri,42,&H00112233,&H00445566,&H00778899,&HAA000000,-1,0,0,0,100,100,0,0,1,2.5,1,5,12,12,24,1
`

func TestParseASSColumnsAndColor(t *testing.T) {
	doc, _, err := ParseSSA(strings.NewReader(sampleASS), "x.ass")
	if err != nil {
		t.Fatal(err)
	}
	if doc.Dialect != DialectASS {
		t.Fatalf("dialect = %q", doc.Dialect)
	}
	cat := NewCatalog(DefaultMergeOptions(), doc)
	eff := cat.Resolve("Default")
	if got, _ := eff.Value("fontsize"); got != "42" {
		t.Fatalf("fontsize=%q", got)
	}
	if got, _ := eff.Value("outline"); got != "2.5" {
		t.Fatalf("outline=%q", got)
	}
	c, err := ParseColor("&H80112233")
	if err != nil {
		t.Fatal(err)
	}
	// AA=0x80 -> conventional A=127; BBGGRR: B=0x11 G=0x22 R=0x33.
	if c.R != 0x33 || c.G != 0x22 || c.B != 0x11 || c.A != 127 {
		t.Fatalf("color decode wrong: %+v", c)
	}
}

func TestNormalizeEquivalentForms(t *testing.T) {
	cases := []struct {
		key, a, b string
	}{
		{"fontsize", "20", "20.0"},
		{"bold", "-1", "true"},
		{"bold", "1", "yes"},
		{"primarycolor", "&H00FFFFFF", "#FFFFFF"},
		{"marginl", "010", "10"},
	}
	for _, tc := range cases {
		na, err := NormalizeValue(tc.key, tc.a, DialectASS)
		if err != nil {
			t.Fatalf("%s=%q: %v", tc.key, tc.a, err)
		}
		nb, err := NormalizeValue(tc.key, tc.b, DialectASS)
		if err != nil {
			t.Fatalf("%s=%q: %v", tc.key, tc.b, err)
		}
		if na != nb {
			t.Fatalf("%s 不等价: %q->%q vs %q->%q", tc.key, tc.a, na, tc.b, nb)
		}
	}
}

func TestParseJSONNullPresenceAndDuplicates(t *testing.T) {
	jsonDoc := []byte(`{
      "dialect":"json",
      "styles":[
        {"name":"A","parent":"P","fields":[["fontname","Arial"],["fontname",null],["fontsize",20]]},
        {"name":"P","fields":{"fontname":"Times","bold":1}}
      ]
    }`)
	doc, err := ParseJSONDocument(jsonDoc, "x.json")
	if err != nil {
		t.Fatal(err)
	}
	cat := NewCatalog(DefaultMergeOptions(), doc)
	eff := cat.Resolve("A")
	if got, _ := eff.Value("fontname"); got != "Arial" {
		t.Fatalf("重复字段中的 null 不应覆盖 Arial, got %q", got)
	}
	if got, _ := eff.Value("bold"); got != "1" {
		t.Fatalf("应继承 P.bold=1, got %q", got)
	}
}
