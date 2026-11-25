package ssastyle

import (
	"fmt"
	"strings"
)

// assWriteColumns 是写出 ASS V4+ Style 行时使用的列顺序。
var assWriteColumns = defaultASSColumns

// FormatASSStyles 把 Convert 的结果渲染成可被标准解析器读回的
// [V4+ Styles] 文本。字段值使用归一化结果并反向格式化为 ASS 词法
// （布尔 -1/0、颜色 &HAABBGGRR）。不可映射属性不在此输出，
// 它们保留在 ConversionReport.Unmapped 中。
func FormatASSStyles(cr *ConversionReport) string {
	var b strings.Builder
	b.WriteString("[V4+ Styles]\n")
	b.WriteString("Format: " + strings.Join(assWriteColumns, ", ") + "\n")
	for _, st := range cr.Styles {
		vals := make([]string, 0, len(assWriteColumns))
		for _, col := range assWriteColumns {
			key := CanonicalKey(col)
			if key == "name" {
				vals = append(vals, st.Name)
				continue
			}
			norm, ok := st.Fields[key]
			if !ok {
				vals = append(vals, "")
				continue
			}
			vals = append(vals, formatASSValue(key, norm))
		}
		b.WriteString("Style: " + strings.Join(vals, ",") + "\n")
	}
	return b.String()
}

// formatASSValue 把内部规范值转回 ASS 词法表示。
func formatASSValue(canonicalKey, norm string) string {
	spec, known := LookupField(canonicalKey)
	if !known {
		return norm
	}
	switch spec.Kind {
	case KindBool:
		if norm == "true" {
			return "-1"
		}
		return "0"
	case KindColor:
		c, err := ParseColor(norm) // norm 形如 #RRGGBBAA
		if err != nil {
			return norm
		}
		assAlpha := 255 - c.A
		return fmt.Sprintf("&H%02X%02X%02X%02X", assAlpha, c.B, c.G, c.R)
	default:
		return norm
	}
}
