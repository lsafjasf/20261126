package ssastyle

import (
	"os"
	"strings"
	"testing"
)

func TestASSWriterRoundTripAgainstLegacy(t *testing.T) {
	data, err := os.ReadFile("testdata/legacy.ssa")
	if err != nil {
		t.Fatal(err)
	}
	legacy, _, err := ParseSSA(strings.NewReader(string(data)), "legacy.ssa")
	if err != nil {
		t.Fatal(err)
	}
	cr := Convert(legacy, ConvertOptions{Merge: DefaultMergeOptions()})
	ass := FormatASSStyles(cr)
	if !strings.Contains(ass, "[V4+ Styles]") || !strings.Contains(ass, "Style: Title,Impact,48") {
		t.Fatalf("ASS 输出缺少预期内容:\n%s", ass)
	}

	back, _, err := ParseSSA(strings.NewReader(ass), "generated.ass")
	if err != nil {
		t.Fatal(err)
	}
	rep := Compare(legacy, back, DefaultCompareOptions())
	for _, sr := range rep.Styles {
		for _, f := range sr.Fields {
			if f.Status != FieldEqual {
				t.Errorf("%s.%s status=%s legacy=%q conv=%q note=%s",
					sr.Name, f.Key, f.Status, f.LegacyNormalized, f.ConvertedNormalized, f.Note)
			}
		}
	}
	if !rep.Equal {
		t.Fatal("生成的 ASS 读回后应与旧 SSA 生效样式逐字段一致")
	}
	// AlphaLevel 与 XStudioID 必须作为不可映射属性保留在报告里。
	var nAlpha, nX int
	for _, u := range cr.Unmapped {
		if u.Field == "alphalevel" {
			nAlpha++
		}
		if u.Field == "xstudioid" {
			nX++
		}
	}
	if nAlpha != 3 || nX != 3 {
		t.Fatalf("每个生效样式都应带 alphalevel/xstudioid: alpha=%d x=%d", nAlpha, nX)
	}
}
