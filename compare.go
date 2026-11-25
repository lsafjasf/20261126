package ssastyle

import (
	"sort"
)

// FieldStatus 是单个字段在两侧生效样式中的比对结论。
type FieldStatus string

const (
	FieldEqual              FieldStatus = "equal"
	FieldDifferent          FieldStatus = "different"
	FieldMissingInConverted FieldStatus = "missing_in_converted"
	FieldExtraInConverted   FieldStatus = "extra_in_converted"
	FieldInvalidValue       FieldStatus = "invalid_value"
)

// FieldComparison 是逐字段比较的一行结果，包含两侧原始值、归一化值与来源。
type FieldComparison struct {
	Key                  string      `json:"key"`
	Target               string      `json:"target,omitempty"`
	Status               FieldStatus `json:"status"`
	LegacyRaw            string      `json:"legacy_raw,omitempty"`
	LegacyNormalized     string      `json:"legacy_normalized,omitempty"`
	LegacySourceStyle    string      `json:"legacy_source_style,omitempty"`
	LegacySource         string      `json:"legacy_source,omitempty"`
	ConvertedRaw         string      `json:"converted_raw,omitempty"`
	ConvertedNormalized  string      `json:"converted_normalized,omitempty"`
	ConvertedSourceStyle string      `json:"converted_source_style,omitempty"`
	Note                 string      `json:"note,omitempty"`
}

// StyleReport 是一个具名样式的完整比对报告。
type StyleReport struct {
	Name           string             `json:"name"`
	InLegacy       bool               `json:"in_legacy"`
	InConverted    bool               `json:"in_converted"`
	LegacyChain    []string           `json:"legacy_chain,omitempty"`
	ConvertedChain []string           `json:"converted_chain,omitempty"`
	Equal          bool               `json:"equal"`
	Fields         []*FieldComparison `json:"fields,omitempty"`
	Unmapped       []UnmappedProperty `json:"unmapped_properties,omitempty"`
}

// ReportSummary 是计数汇总。
type ReportSummary struct {
	Styles             int `json:"styles"`
	EqualStyles        int `json:"equal_styles"`
	FieldsEqual        int `json:"fields_equal"`
	Different          int `json:"different"`
	MissingInConverted int `json:"missing_in_converted"`
	ExtraInConverted   int `json:"extra_in_converted"`
	InvalidValues      int `json:"invalid_values"`
	Unmapped           int `json:"unmapped_properties"`
	Diagnostics        int `json:"diagnostics"`
	Errors             int `json:"errors"`
}

// Report 是完整比对结果，可直接序列化为稳定 JSON。
type Report struct {
	Equal       bool               `json:"equal"`
	OK          bool               `json:"ok"`
	Summary     ReportSummary      `json:"summary"`
	Styles      []*StyleReport     `json:"styles"`
	Unmapped    []UnmappedProperty `json:"unmapped_properties,omitempty"`
	Diagnostics []Diagnostic       `json:"diagnostics,omitempty"`
}

// CompareOptions 控制比对行为。
type CompareOptions struct {
	Merge       MergeOptions
	Mapping     []PropertyMapping
	Styles      []string
	ReportExtra bool
}

// DefaultCompareOptions 返回默认比对选项（空值不覆盖、报告多余字段）。
func DefaultCompareOptions() CompareOptions {
	return CompareOptions{Merge: DefaultMergeOptions(), ReportExtra: true}
}

// Compare 解析两侧的继承链得到最终生效样式，再逐字段比较。
// 旧 SSA 中无法映射到目标模型的属性进入 Report.Unmapped 单独报告，
// 不参与字段一致性判定，也绝不会被静默丢弃。
func Compare(legacyDoc, convertedDoc *Document, opts CompareOptions) *Report {
	idx := buildMappingIndex(opts.Mapping)
	legacyCat := NewCatalog(opts.Merge, legacyDoc)
	convCat := NewCatalog(opts.Merge, convertedDoc)
	legacyEff := legacyCat.ResolveAll()
	convEff := convCat.ResolveAll()

	rep := &Report{Equal: true}
	rep.Diagnostics = append(rep.Diagnostics, legacyCat.Diagnostics...)
	rep.Diagnostics = append(rep.Diagnostics, convCat.Diagnostics...)

	lDialect := docDialect(legacyDoc)
	cDialect := docDialect(convertedDoc)
	names := selectStyleNames(opts.Styles, legacyCat, convCat)
	for _, name := range names {
		sr := compareStyle(name, legacyCat, convCat, legacyEff, convEff, idx, opts, lDialect, cDialect)
		rep.Styles = append(rep.Styles, sr)
		if es := legacyEff[name]; es != nil {
			rep.Diagnostics = append(rep.Diagnostics, es.Diagnostics...)
		}
		if es := convEff[name]; es != nil {
			rep.Diagnostics = append(rep.Diagnostics, es.Diagnostics...)
		}
		if !sr.Equal {
			rep.Equal = false
		}
		rep.Unmapped = append(rep.Unmapped, sr.Unmapped...)
	}

	rep.Diagnostics = DedupeDiagnostics(rep.Diagnostics)
	for _, d := range rep.Diagnostics {
		if d.Level == "error" {
			rep.Equal = false
		}
	}
	rep.OK = rep.Equal
	rep.summarize()
	return rep
}

// DedupeDiagnostics 去除完全相同的诊断（同一祖先层会被多个后代样式解析）。
func DedupeDiagnostics(ds []Diagnostic) []Diagnostic {
	seen := map[Diagnostic]bool{}
	out := make([]Diagnostic, 0, len(ds))
	for _, d := range ds {
		if seen[d] {
			continue
		}
		seen[d] = true
		out = append(out, d)
	}
	return out
}

func docDialect(d *Document) string {
	if d == nil || d.Dialect == "" {
		return DialectSSA
	}
	return d.Dialect
}

func selectStyleNames(want []string, a, b *Catalog) []string {
	if len(want) > 0 {
		return append([]string(nil), want...)
	}
	seen := map[string]bool{}
	var names []string
	for _, n := range a.Names() {
		if !seen[n] {
			seen[n] = true
			names = append(names, n)
		}
	}
	extra := []string{}
	for _, n := range b.Names() {
		if !seen[n] {
			seen[n] = true
			extra = append(extra, n)
		}
	}
	sort.Strings(extra)
	return append(names, extra...)
}

func compareStyle(name string, lc, cc *Catalog, le, ce map[string]*EffectiveStyle,
	idx *mappingIndex, opts CompareOptions, lDialect, cDialect string) *StyleReport {
	sr := &StyleReport{Name: name, Equal: true}
	ls, inL := le[name]
	cs, inC := ce[name]
	sr.InLegacy, sr.InConverted = inL && lc.Has(name), inC && cc.Has(name)
	if ls != nil {
		sr.LegacyChain = ls.Chain
	}
	if cs != nil {
		sr.ConvertedChain = cs.Chain
	}
	if !sr.InLegacy {
		sr.Equal = false
		return sr
	}
	if !sr.InConverted {
		sr.Equal = false
		keys := sortedFieldKeys(ls.Fields)
		for _, k := range keys {
			if isMetaKey(k) || ls.Fields[k].Reset {
				continue
			}
			rf := ls.Fields[k]
			sr.Fields = append(sr.Fields, &FieldComparison{
				Key: k, Status: FieldMissingInConverted,
				LegacyRaw: rf.Raw, LegacySourceStyle: rf.SourceStyle, LegacySource: rf.Source,
			})
		}
		return sr
	}

	coveredTargets := map[string]bool{}
	for _, k := range sortedFieldKeys(ls.Fields) {
		if isMetaKey(k) {
			continue
		}
		rf := ls.Fields[k]
		if rf.Reset {
			continue
		}
		rule := idx.classify(k)
		if rule.Status == MapUnmapped || rule.Status == MapDerived {
			sr.Unmapped = append(sr.Unmapped, UnmappedProperty{
				Style: name, Field: k, RawValue: rf.Raw, Status: rule.Status,
				Reason: rule.Note, Source: rf.Source, SourceStyle: rf.SourceStyle,
			})
			continue
		}
		target := rule.Target
		if target == "" {
			target = k
		}
		coveredTargets[target] = true
		fc := &FieldComparison{Key: k, Target: target, LegacyRaw: rf.Raw,
			LegacySourceStyle: rf.SourceStyle, LegacySource: rf.Source}
		lNorm, lerr := NormalizeValue(k, rf.Raw, lDialect)
		if lerr != nil {
			fc.Status = FieldInvalidValue
			fc.Note = lerr.Error()
			sr.Equal = false
			sr.Fields = append(sr.Fields, fc)
			continue
		}
		fc.LegacyNormalized = lNorm

		cf, ok := cs.Fields[target]
		if !ok || cf.Reset {
			fc.Status = FieldMissingInConverted
			sr.Equal = false
			sr.Fields = append(sr.Fields, fc)
			continue
		}
		fc.ConvertedRaw, fc.ConvertedSourceStyle = cf.Raw, cf.SourceStyle
		cNorm, cerr := NormalizeValue(target, cf.Raw, cDialect)
		if cerr != nil {
			fc.Status = FieldInvalidValue
			fc.Note = cerr.Error()
			sr.Equal = false
			sr.Fields = append(sr.Fields, fc)
			continue
		}
		fc.ConvertedNormalized = cNorm
		if lNorm == cNorm {
			fc.Status = FieldEqual
		} else {
			fc.Status = FieldDifferent
			sr.Equal = false
		}
		sr.Fields = append(sr.Fields, fc)
	}

	if opts.ReportExtra {
		for _, k := range sortedFieldKeys(cs.Fields) {
			if isMetaKey(k) || coveredTargets[k] {
				continue
			}
			cf := cs.Fields[k]
			sr.Fields = append(sr.Fields, &FieldComparison{
				Key: k, Target: k, Status: FieldExtraInConverted,
				ConvertedRaw: cf.Raw, ConvertedNormalized: cf.Raw,
				ConvertedSourceStyle: cf.SourceStyle,
			})
			// 多余字段不破坏一致性结论，只作提示。
		}
	}
	return sr
}

func sortedFieldKeys(m map[string]ResolvedField) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func (r *Report) summarize() {
	s := ReportSummary{Styles: len(r.Styles), Unmapped: len(r.Unmapped), Diagnostics: len(r.Diagnostics)}
	for _, d := range r.Diagnostics {
		if d.Level == "error" {
			s.Errors++
		}
	}
	for _, st := range r.Styles {
		if st.Equal {
			s.EqualStyles++
		}
		for _, f := range st.Fields {
			switch f.Status {
			case FieldEqual:
				s.FieldsEqual++
			case FieldDifferent:
				s.Different++
			case FieldMissingInConverted:
				s.MissingInConverted++
			case FieldExtraInConverted:
				s.ExtraInConverted++
			case FieldInvalidValue:
				s.InvalidValues++
			}
		}
	}
	r.Summary = s
}
