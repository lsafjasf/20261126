package ssastyle

import "testing"

func findStyle(rep *Report, name string) *StyleReport {
	for _, s := range rep.Styles {
		if s.Name == name {
			return s
		}
	}
	return nil
}

func findField(sr *StyleReport, key string) *FieldComparison {
	for _, f := range sr.Fields {
		if f.Key == key {
			return f
		}
	}
	return nil
}

func TestCompareEqualAfterNullSafeResolution(t *testing.T) {
	legacy, err := ParseJSONDocument([]byte(`{
      "dialect":"ssa",
      "styles":[
        {"name":"Global","fields":{"fontname":"Arial","marginl":"10"}},
        {"name":"Title","parent":"Global","fields":{"fontsize":"48","bold":"-1"}}
      ]}`), "legacy.json")
	if err != nil {
		t.Fatal(err)
	}
	// 转换侧是多层记录流，后出现的 null 与空串按正确语义不得覆盖。
	converted, err := ParseJSONDocument([]byte(`{
      "dialect":"ass",
      "styles":[
        {"name":"Global","fields":{"fontname":"Arial","marginl":"10"}},
        {"name":"Title","parent":"Global","fields":[
          ["fontsize",48],["bold",true],["fontsize",null],["bold",null],["fontsize",48]
        ]}
      ]}`), "converted.json")
	if err != nil {
		t.Fatal(err)
	}
	rep := Compare(legacy, converted, DefaultCompareOptions())
	if !rep.Equal {
		for _, s := range rep.Styles {
			for _, f := range s.Fields {
				if f.Status != FieldEqual {
					t.Errorf("unexpected %s: %+v", f.Status, f)
				}
			}
		}
		t.Fatalf("null 安全解析后应一致, diagnostics=%+v", rep.Diagnostics)
	}
}

func TestCompareCatchesClobberedFlatOutput(t *testing.T) {
	legacy, _ := ParseJSONDocument([]byte(`{"styles":[
      {"name":"Title","fields":{"fontname":"Arial","fontsize":"48","bold":"-1"}}]}`), "l.json")
	// 有缺陷的转换器产出的扁平结果：有效值真的被空值抹掉了。
	converted, _ := ParseJSONDocument([]byte(`{"Title":{"fontname":"","fontsize":null,"bold":false}}`), "c.json")
	rep := Compare(legacy, converted, DefaultCompareOptions())
	if rep.Equal {
		t.Fatal("应检测到转换结果中的覆盖/丢失")
	}
	sr := findStyle(rep, "Title")
	if sr == nil {
		t.Fatal("缺少 Title 报告")
	}
	if f := findField(sr, "fontname"); f == nil || f.Status != FieldMissingInConverted {
		t.Fatalf("fontname 应 missing, got %+v", f)
	}
	if f := findField(sr, "fontsize"); f == nil || f.Status != FieldMissingInConverted {
		t.Fatalf("fontsize 应 missing(null), got %+v", f)
	}
	if f := findField(sr, "bold"); f == nil || f.Status != FieldEqual {
		// false/0 是真实零值：旧 -1/true 归一为 true，新 false 应为 different。
		if f == nil || f.Status != FieldDifferent {
			t.Fatalf("bold 应为 different（零值是真实值）, got %+v", f)
		}
	}
}

func TestCompareFieldDifferenceAndProvenance(t *testing.T) {
	legacy, _ := ParseJSONDocument([]byte(`{"dialect":"ssa","styles":[
      {"name":"A","fields":{"fontsize":"20","alignment":"5"}}]}`), "l.json")
	converted, _ := ParseJSONDocument([]byte(`{"dialect":"ass","styles":[
      {"name":"A","fields":{"fontsize":"18","alignment":"7"}}]}`), "c.json")
	rep := Compare(legacy, converted, DefaultCompareOptions())
	sr := findStyle(rep, "A")
	if f := findField(sr, "fontsize"); f == nil || f.Status != FieldDifferent ||
		f.LegacyNormalized != "20" || f.ConvertedNormalized != "18" {
		t.Fatalf("fontsize diff 报告错误: %+v", f)
	}
	if f := findField(sr, "alignment"); f == nil || f.Status != FieldEqual {
		t.Fatalf("SSA 5 与 ASS 7 应在方言归一后相等: %+v", f)
	}
}

func TestUnmappedPropertiesReportedSeparately(t *testing.T) {
	legacy, _ := ParseJSONDocument([]byte(`{"dialect":"ssa","styles":[
      {"name":"A","fields":{"fontname":"Arial","alphalevel":"0","x-studio-id":"ZZ-9"}}]}`), "l.json")
	converted, _ := ParseJSONDocument([]byte(`{"dialect":"ass","styles":[
      {"name":"A","fields":{"fontname":"Arial"}}]}`), "c.json")
	rep := Compare(legacy, converted, DefaultCompareOptions())
	if !rep.Equal {
		t.Fatalf("不可映射属性不应算作字段差异: %+v", rep.Styles[0].Fields)
	}
	if rep.Summary.Unmapped != 2 {
		t.Fatalf("应有 2 个单独报告的不可映射/派生属性, got %d (%+v)", rep.Summary.Unmapped, rep.Unmapped)
	}
	var hasAlpha, hasUnknown bool
	for _, u := range rep.Unmapped {
		if u.Field == "alphalevel" && u.Status == MapDerived {
			hasAlpha = true
		}
		if u.Field == "xstudioid" && u.Status == MapUnmapped {
			hasUnknown = true
		}
	}
	if !hasAlpha || !hasUnknown {
		t.Fatalf("缺少 alphalevel/unknown 报告: %+v", rep.Unmapped)
	}
}

func TestTertiaryColourRenamesToOutlineColour(t *testing.T) {
	legacy, _ := ParseJSONDocument([]byte(`{"dialect":"ssa","styles":[
      {"name":"A","fields":{"tertiarycolor":"&H00112233"}}]}`), "l.json")
	converted, _ := ParseJSONDocument([]byte(`{"dialect":"ass","styles":[
      {"name":"A","fields":{"outlinecolor":"&H00112233"}}]}`), "c.json")
	rep := Compare(legacy, converted, DefaultCompareOptions())
	if !rep.Equal {
		t.Fatalf("TertiaryColour->OutlineColour 应识别为同字段: %+v", rep.Styles[0].Fields)
	}
}

func TestConvertRoundTripIsComparable(t *testing.T) {
	doc, _ := ParseJSONDocument([]byte(`{"dialect":"ssa","styles":[
      {"name":"G","fields":{"fontname":"Arial"}},
      {"name":"A","parent":"G","fields":{"fontsize":"20","alignment":"9","alphalevel":"128"}}]}`), "l.json")
	cr := Convert(doc, ConvertOptions{Merge: DefaultMergeOptions()})
	if len(cr.Unmapped) != 1 || cr.Unmapped[0].Field != "alphalevel" {
		t.Fatalf("convert 应单独报告 alphalevel, got %+v", cr.Unmapped)
	}

	out := NewDocument(DialectJSON)
	for _, cs := range cr.Styles {
		st := &Style{Name: cs.Name}
		for k, v := range cs.Fields {
			st.AddField(k, v)
		}
		out.Styles = append(out.Styles, st)
	}
	rep := Compare(doc, out, DefaultCompareOptions())
	if !rep.Equal {
		for _, sr := range rep.Styles {
			for _, f := range sr.Fields {
				if f.Status != FieldEqual {
					t.Errorf("roundtrip %s.%s status=%s %q vs %q", sr.Name, f.Key, f.Status, f.LegacyNormalized, f.ConvertedNormalized)
				}
			}
		}
		t.Fatal("Convert 产物与旧生效样式逐字段不一致")
	}
}

func TestMissingConvertedStyleReported(t *testing.T) {
	legacy, _ := ParseJSONDocument([]byte(`{"styles":[{"name":"A","fields":{"bold":"1"}}]}`), "l.json")
	converted, _ := ParseJSONDocument([]byte(`{"styles":[{"name":"B","fields":{"bold":"1"}}]}`), "c.json")
	rep := Compare(legacy, converted, DefaultCompareOptions())
	if rep.Equal {
		t.Fatal("缺少 A 样式应判不一致")
	}
	a := findStyle(rep, "A")
	if a == nil || a.InConverted {
		t.Fatalf("A 应标记为转换侧缺失: %+v", a)
	}
	b := findStyle(rep, "B")
	if b == nil || b.InLegacy {
		t.Fatalf("B 应作为旧侧不存在的额外样式: %+v", b)
	}
}
