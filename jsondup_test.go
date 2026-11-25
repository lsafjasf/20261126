package ssastyle

import "testing"

func TestDuplicateJSONKeysPreserveEarlierValue(t *testing.T) {
	// 同一个样式对象内 fontname 重复出现，最后一个是 null。
	// 普通 map 反序列化在解析阶段就只剩 null；保序解析必须保留两次出现，
	// 再由空值语义保住第一个有效值。
	raw := []byte(`{
      "styles": [{
        "name": "X",
        "fontname": "Arial",
        "fontsize": 20,
        "fontname": null,
        "fontsize": "",
        "fontsize": 24
      }]
    }`)
	doc, err := ParseJSONDocument(raw, "dup.json")
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Styles[0].Fields) < 5 {
		t.Fatalf("重复键的每次出现都应保留, fields=%+v", doc.Styles[0].Fields)
	}
	cat := NewCatalog(DefaultMergeOptions(), doc)
	eff := cat.Resolve("X")
	if got, _ := eff.Value("fontname"); got != "Arial" {
		t.Fatalf("重复键中的后发 null 不应覆盖 Arial, got %q", got)
	}
	if got, _ := eff.Value("fontsize"); got != "24" {
		t.Fatalf("fontsize 应取最后一个有效值 24, got %q", got)
	}
}

func TestDuplicateTopLevelStyleNames(t *testing.T) {
	raw := []byte(`{
      "A": {"fontname": "Arial"},
      "A": {"fontname": null, "fontsize": 20}
    }`)
	doc, err := ParseJSONDocument(raw, "dupstyles.json")
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Styles) != 2 {
		t.Fatalf("重复顶层样式名应保留为两条记录, got %d", len(doc.Styles))
	}
	cat := NewCatalog(DefaultMergeOptions(), doc)
	eff := cat.Resolve("A")
	if got, _ := eff.Value("fontname"); got != "Arial" {
		t.Fatalf("后一条记录的 null 不应抹掉前一条的 Arial, got %q", got)
	}
	if got, _ := eff.Value("fontsize"); got != "20" {
		t.Fatalf("后一条的有效 fontsize 应叠加生效, got %q", got)
	}
}
