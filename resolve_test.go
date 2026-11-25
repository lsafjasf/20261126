package ssastyle

import "testing"

func TestNullDoesNotOverwriteEarlierValue(t *testing.T) {
	s := &Style{Name: "X"}
	s.AddField("Fontname", "Arial")
	s.AddNullField("Fontname") // 后出现的显式 null

	cat := NewCatalog(DefaultMergeOptions(), &Document{Styles: []*Style{s}})
	eff := cat.Resolve("X")
	got, ok := eff.Value("fontname")
	if !ok || got != "Arial" {
		t.Fatalf("null 覆盖了有效值: got=%q ok=%v", got, ok)
	}
}

func TestEmptyStringTreatedAsNullByDefault(t *testing.T) {
	s := &Style{Name: "X"}
	s.AddField("Fontname", "Arial")
	s.AddField("Fontname", "   ")
	cat := NewCatalog(DefaultMergeOptions(), &Document{Styles: []*Style{s}})
	if got, ok := cat.Resolve("X").Value("fontname"); !ok || got != "Arial" {
		t.Fatalf("空白串不应覆盖: got=%q ok=%v", got, ok)
	}

	opts := DefaultMergeOptions()
	opts.EmptyIsUnset = false
	cat2 := NewCatalog(opts, &Document{Styles: []*Style{s}})
	if got, ok := cat2.Resolve("X").Value("fontname"); !ok || got != "   " {
		t.Fatalf("KeepEmpty 模式下空白串应作为显式值生效: got=%q ok=%v", got, ok)
	}
}

func TestZeroAndFalseAreRealValues(t *testing.T) {
	s := &Style{Name: "X"}
	s.AddField("Bold", "1")
	s.AddField("Bold", "0") // 零值是真实覆盖，不是空值
	s.AddField("Outline", "2")
	s.AddField("Outline", "0")
	cat := NewCatalog(DefaultMergeOptions(), &Document{Styles: []*Style{s}})
	eff := cat.Resolve("X")
	if got, _ := eff.Value("bold"); got != "0" {
		t.Fatalf("Bold=0 应覆盖为 0, got %q", got)
	}
	if got, _ := eff.Value("outline"); got != "0" {
		t.Fatalf("Outline=0 应覆盖为 0, got %q", got)
	}
}

func TestLastEffectiveValueWinsAmongDuplicates(t *testing.T) {
	s := &Style{Name: "X"}
	s.AddField("Fontsize", "20")
	s.AddNullField("Fontsize")
	s.AddField("Fontsize", "24")
	cat := NewCatalog(DefaultMergeOptions(), &Document{Styles: []*Style{s}})
	if got, _ := cat.Resolve("X").Value("fontsize"); got != "24" {
		t.Fatalf("最后一个有效值应为 24, got %q", got)
	}
	f := cat.Resolve("X").Fields["fontsize"]
	if f.SourceStyle != "X" {
		t.Fatalf("来源样式应为 X, got %q", f.SourceStyle)
	}
}

func TestMultiLayerInheritanceChain(t *testing.T) {
	title := (&Style{Name: "Title"}).SetParent("Base").AddField("Fontsize", "48").AddNullField("Bold")
	base := (&Style{Name: "Base"}).SetParent("Global").AddField("Fontname", "Helvetica").AddField("Bold", "1")
	global := (&Style{Name: "Global"}).AddField("Fontname", "Arial").AddField("MarginL", "10")
	doc := &Document{Styles: []*Style{
		title, base, global,
	}}
	cat := NewCatalog(DefaultMergeOptions(), doc)
	eff := cat.Resolve("Title")

	wantChain := []string{"Global", "Base", "Title"}
	if len(eff.Chain) != 3 || eff.Chain[0] != wantChain[0] || eff.Chain[2] != "Title" {
		t.Fatalf("chain = %v, want %v", eff.Chain, wantChain)
	}
	// Title 的 null Bold 不覆盖 Base 的 1。
	if got, _ := eff.Value("bold"); got != "1" {
		t.Fatalf("应继承 Base.Bold=1, got %q", got)
	}
	// Base 覆盖 Global 的 Fontname。
	got, _ := eff.Value("fontname")
	src := eff.Fields["fontname"].SourceStyle
	if got != "Helvetica" || src != "Base" {
		t.Fatalf("fontname=%q src=%q, want Helvetica/Base", got, src)
	}
	// Title 自身值优先。
	got, _ = eff.Value("fontsize")
	src = eff.Fields["fontsize"].SourceStyle
	if got != "48" || src != "Title" {
		t.Fatalf("fontsize=%q src=%q", got, src)
	}
	// 未被任何近层覆盖的 Global 字段保留。
	if got, _ := eff.Value("marginl"); got != "10" {
		t.Fatalf("marginl 应来自 Global, got %q", got)
	}
}

func TestInheritanceCycleDetected(t *testing.T) {
	a := (&Style{Name: "A"}).SetParent("B")
	b := (&Style{Name: "B"}).SetParent("C")
	c := (&Style{Name: "C"}).SetParent("A")
	doc := &Document{Styles: []*Style{
		a, b, c,
	}}
	cat := NewCatalog(DefaultMergeOptions(), doc)
	eff := cat.Resolve("A")
	found := false
	for _, d := range eff.Diagnostics {
		if d.Code == DiagInheritanceCycle && d.Level == "error" {
			found = true
		}
	}
	if !found {
		t.Fatalf("应报告继承环, diagnostics=%+v", cat.Diagnostics)
	}
	if len(eff.Chain) == 0 || eff.Chain[0] != "C" {
		t.Fatalf("环应被安全截断并给出已遍历链, chain=%v", eff.Chain)
	}
}

func TestUnknownParentDiagnostic(t *testing.T) {
	a := (&Style{Name: "A"}).SetParent("Ghost").AddField("Bold", "1")
	doc := &Document{Styles: []*Style{a}}
	cat := NewCatalog(DefaultMergeOptions(), doc)
	eff := cat.Resolve("A")
	found := false
	for _, d := range eff.Diagnostics {
		if d.Code == DiagUnknownParent {
			found = true
		}
	}
	if !found {
		t.Fatal("应报告未知父样式")
	}
	if got, _ := eff.Value("bold"); got != "1" {
		t.Fatalf("断链后自身字段仍应生效, got %q", got)
	}
}

func TestExplicitNullResetMode(t *testing.T) {
	child := (&Style{Name: "Child"}).SetParent("P").AddNullField("Fontname")
	parent := (&Style{Name: "P"}).AddField("Fontname", "Arial")
	doc := &Document{Styles: []*Style{
		child, parent,
	}}
	opts := DefaultMergeOptions()
	opts.ExplicitNullResets = true
	cat := NewCatalog(opts, doc)
	eff := cat.Resolve("Child")
	f, ok := eff.Fields["fontname"]
	if !ok || !f.Reset {
		t.Fatalf("null-reset 模式下应留下 reset 标记, field=%+v ok=%v", f, ok)
	}
	if _, has := eff.Value("fontname"); has {
		t.Fatal("reset 后不应再取到继承值")
	}
}
