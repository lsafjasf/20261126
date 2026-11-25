package ssastyle

import (
	"fmt"
	"io"
	"sort"
	"strings"
)

// RenderTextReport 输出人类可读的逐字段比对报告。
func RenderTextReport(w io.Writer, rep *Report) {
	fmt.Fprintln(w, "SSA 样式继承生效值 · 逐字段比对")
	for _, sr := range rep.Styles {
		fmt.Fprintln(w, strings.Repeat("=", 68))
		switch {
		case !sr.InLegacy:
			fmt.Fprintf(w, "样式 %q：旧 SSA 中不存在（转换侧额外样式）\n", sr.Name)
		case !sr.InConverted:
			fmt.Fprintf(w, "样式 %q：转换结果中缺失！继承链 %s\n", sr.Name, chainArrow(sr.LegacyChain))
		case sr.Equal:
			fmt.Fprintf(w, "样式 %q：全部一致  继承链 %s\n", sr.Name, chainArrow(sr.LegacyChain))
		default:
			fmt.Fprintf(w, "样式 %q：存在差异  旧链 %s | 新链 %s\n",
				sr.Name, chainArrow(sr.LegacyChain), chainArrow(sr.ConvertedChain))
		}
		for _, f := range sr.Fields {
			switch f.Status {
			case FieldEqual:
				label := f.Key
				if f.Target != "" && f.Target != f.Key {
					label = f.Key + "->" + f.Target
				}
				fmt.Fprintf(w, "  [ equal ] %-28s = %s   (生效来源: %s)\n",
					label, f.LegacyNormalized, sourceOr(f.LegacySourceStyle))
			case FieldDifferent:
				fmt.Fprintf(w, "  [ DIFF  ] %-15s 旧=%q [%s] -> 新=%q [%s]\n",
					f.Key, f.LegacyNormalized, sourceOr(f.LegacySourceStyle),
					f.ConvertedNormalized, sourceOr(f.ConvertedSourceStyle))
			case FieldMissingInConverted:
				fmt.Fprintf(w, "  [ MISS  ] %-15s 转换侧缺失/被空值覆盖；旧生效值=%q (来源 %s)\n",
					f.Key, f.LegacyNormalized, sourceOr(f.LegacySourceStyle))
			case FieldExtraInConverted:
				fmt.Fprintf(w, "  [ EXTRA ] %-15s 转换侧多出字段=%q\n", f.Key, f.ConvertedRaw)
			case FieldInvalidValue:
				fmt.Fprintf(w, "  [ BADVAL] %-15s 无法归一比较: %s\n", f.Key, f.Note)
			}
		}
		if len(sr.Unmapped) > 0 {
			fmt.Fprintf(w, "  -- 不可映射属性（%d，单独报告，不计入字段差异）--\n", len(sr.Unmapped))
			for _, u := range sr.Unmapped {
				fmt.Fprintf(w, "  [ %-8s] %s = %q : %s\n", u.Status, u.Field, u.RawValue, u.Reason)
			}
		}
	}

	fmt.Fprintln(w, strings.Repeat("=", 68))
	fmt.Fprintf(w, "汇总: 样式 %d（一致 %d），字段 equal=%d diff=%d missing=%d extra=%d bad=%d；不可映射 %d；诊断 %d（错误 %d）\n",
		rep.Summary.Styles, rep.Summary.EqualStyles, rep.Summary.FieldsEqual,
		rep.Summary.Different, rep.Summary.MissingInConverted, rep.Summary.ExtraInConverted,
		rep.Summary.InvalidValues, rep.Summary.Unmapped, rep.Summary.Diagnostics, rep.Summary.Errors)

	if len(rep.Diagnostics) > 0 {
		fmt.Fprintln(w, "诊断:")
		for _, d := range rep.Diagnostics {
			where := d.Style
			if d.Field != "" {
				where += "." + d.Field
			}
			fmt.Fprintf(w, "  (%s/%s) %s %s\n", d.Level, d.Code, where, d.Message)
		}
	}

	if rep.Equal {
		fmt.Fprintln(w, "结论: 所有可映射字段的最终生效样式一致。")
	} else {
		fmt.Fprintln(w, "结论: 存在生效样式差异/错误，通常即“后出现的空值覆盖有效值”或转换丢失。")
	}
}

// RenderEffectiveText 输出 resolve 子命令的生效样式。
func RenderEffectiveText(w io.Writer, cat *Catalog, eff map[string]*EffectiveStyle) {
	for _, name := range cat.Names() {
		es := eff[name]
		fmt.Fprintf(w, "样式 %q  继承链: %s\n", name, chainArrow(es.Chain))
		keys := make([]string, 0, len(es.Fields))
		for k := range es.Fields {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			f := es.Fields[k]
			if f.Reset {
				fmt.Fprintf(w, "  %-15s <reset by explicit null>  (来源 %s)\n", k, sourceOr(f.SourceStyle))
				continue
			}
			fmt.Fprintf(w, "  %-15s = %q  (来源 %s)\n", k, f.Raw, sourceOr(f.SourceStyle))
		}
	}
}

func chainArrow(chain []string) string {
	if len(chain) == 0 {
		return "(无)"
	}
	return strings.Join(chain, " -> ")
}

func sourceOr(s string) string {
	if strings.TrimSpace(s) == "" {
		return "?"
	}
	return s
}
