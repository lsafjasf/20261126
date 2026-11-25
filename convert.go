package ssastyle

import "sort"

// ConvertedStyle 是一个样式正确解析继承链后的扁平目标表示。
type ConvertedStyle struct {
	Name   string            `json:"name"`
	Chain  []string          `json:"chain,omitempty"`
	Fields map[string]string `json:"fields"`
}

// ConversionReport 是一次“正确转换”的产物：映射后的样式 + 单独的不可映射报告。
type ConversionReport struct {
	Styles      []ConvertedStyle   `json:"styles"`
	Unmapped    []UnmappedProperty `json:"unmapped_properties,omitempty"`
	Diagnostics []Diagnostic       `json:"diagnostics,omitempty"`
}

// ConvertOptions 控制转换。
type ConvertOptions struct {
	Merge   MergeOptions
	Mapping []PropertyMapping
}

// Convert 先按继承链与空值语义求出生效样式，再应用属性映射。
// 无法映射/只能有损派生的属性进入 Unmapped 单独报告，不会静默丢失。
func Convert(doc *Document, opts ConvertOptions) *ConversionReport {
	idx := buildMappingIndex(opts.Mapping)
	cat := NewCatalog(opts.Merge, doc)
	eff := cat.ResolveAll()
	dialect := docDialect(doc)

	rep := &ConversionReport{Diagnostics: append([]Diagnostic(nil), cat.Diagnostics...)}
	for _, name := range cat.Names() {
		es := eff[name]
		rep.Diagnostics = append(rep.Diagnostics, es.Diagnostics...)
		cs := ConvertedStyle{Name: name, Chain: es.Chain, Fields: map[string]string{}}
		for _, k := range sortedFieldKeys(es.Fields) {
			if isMetaKey(k) {
				continue
			}
			rf := es.Fields[k]
			if rf.Reset {
				continue
			}
			rule := idx.classify(k)
			if rule.Status == MapUnmapped || rule.Status == MapDerived {
				rep.Unmapped = append(rep.Unmapped, UnmappedProperty{
					Style: name, Field: k, RawValue: rf.Raw, Status: rule.Status,
					Reason: rule.Note, Source: rf.Source, SourceStyle: rf.SourceStyle,
				})
				continue
			}
			target := rule.Target
			if target == "" {
				target = k
			}
			val, err := NormalizeValue(k, rf.Raw, dialect)
			if err != nil {
				// 归一化失败时保留原始词法值，同时给诊断，绝不丢字段。
				val = rf.Raw
				rep.Diagnostics = append(rep.Diagnostics, Diagnostic{
					Code: DiagInvalidValue, Level: "warning", Style: name, Field: k,
					Message: err.Error(), Source: rf.Source,
				})
			}
			cs.Fields[target] = val
		}
		rep.Styles = append(rep.Styles, cs)
	}
	sort.SliceStable(rep.Styles, func(i, j int) bool { return rep.Styles[i].Name < rep.Styles[j].Name })
	rep.Diagnostics = DedupeDiagnostics(rep.Diagnostics)
	return rep
}
