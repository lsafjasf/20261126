package ssastyle

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

// 默认 SSA（V4，18 列）样式 Format。
var defaultSSAColumns = []string{
	"Name", "Fontname", "Fontsize", "PrimaryColour", "SecondaryColour",
	"TertiaryColour", "BackColour", "Bold", "Italic", "BorderStyle",
	"Outline", "Shadow", "Alignment", "MarginL", "MarginR", "MarginV",
	"AlphaLevel", "Encoding",
}

// 默认 ASS（V4+，23 列）样式 Format。
var defaultASSColumns = []string{
	"Name", "Fontname", "Fontsize", "PrimaryColour", "SecondaryColour",
	"OutlineColour", "BackColour", "Bold", "Italic", "Underline",
	"StrikeOut", "ScaleX", "ScaleY", "Spacing", "Angle", "BorderStyle",
	"Outline", "Shadow", "Alignment", "MarginL", "MarginR", "MarginV",
	"Encoding",
}

// ParseSSA 解析 SSA/ASS 文件中的样式段。
// 只读取 [V4 Styles] 与 [V4+ Styles]，按最近一条 Format: 行动态映射列；
// 同名 Style 行全部保留（重复定义），空列记为 ExplicitNull。
func ParseSSA(r io.Reader, source string) (*Document, []Diagnostic, error) {
	doc := NewDocument("")
	doc.Source = source
	var diags []Diagnostic

	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	section := ""
	dialect := ""
	var columns []string
	lineNo := 0
	for sc.Scan() {
		lineNo++
		line := strings.TrimPrefix(sc.Text(), "\uFEFF")
		trim := strings.TrimSpace(line)
		if trim == "" || strings.HasPrefix(trim, ";") || strings.HasPrefix(trim, "!:") {
			continue
		}
		if strings.HasPrefix(trim, "[") && strings.HasSuffix(trim, "]") {
			section = strings.ToLower(strings.TrimSpace(trim[1 : len(trim)-1]))
			columns = nil
			if strings.Contains(section, "v4+ styles") {
				dialect = DialectASS
				columns = append([]string(nil), defaultASSColumns...)
			} else if strings.Contains(section, "v4 styles") {
				dialect = DialectSSA
				columns = append([]string(nil), defaultSSAColumns...)
			}
			continue
		}
		if dialect == "" || !isStyleSection(section) {
			continue
		}
		if doc.Dialect == "" {
			doc.Dialect = dialect
		}

		colon := strings.IndexByte(line, ':')
		if colon < 0 {
			continue
		}
		key := strings.TrimSpace(line[:colon])
		payload := strings.TrimSpace(line[colon+1:])
		loc := fmt.Sprintf("%s:%d", source, lineNo)

		switch strings.ToLower(key) {
		case "format":
			columns = splitFormat(payload)
		case "style":
			if columns == nil {
				columns = defaultColumnsFor(dialect)
			}
			st, d := parseStyleLine(payload, columns, dialect, loc)
			diags = append(diags, d...)
			if st != nil {
				doc.Styles = append(doc.Styles, st)
			}
		}
	}
	if err := sc.Err(); err != nil {
		return nil, diags, fmt.Errorf("%s: 读取失败: %w", source, err)
	}
	if doc.Dialect == "" {
		doc.Dialect = DialectSSA
	}
	return doc, diags, nil
}

func isStyleSection(s string) bool {
	return strings.Contains(s, "v4 styles") || strings.Contains(s, "v4+ styles")
}

func defaultColumnsFor(dialect string) []string {
	if dialect == DialectASS {
		return append([]string(nil), defaultASSColumns...)
	}
	return append([]string(nil), defaultSSAColumns...)
}

func splitFormat(payload string) []string {
	parts := strings.Split(payload, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		out = append(out, strings.TrimSpace(p))
	}
	return out
}

func parseStyleLine(payload string, columns []string, dialect, loc string) (*Style, []Diagnostic) {
	// Style 行字段数与 Format 列数一致；用 SplitN 保留末列内可能出现的逗号。
	values := strings.SplitN(payload, ",", len(columns))
	st := &Style{Source: loc}
	var diags []Diagnostic

	if len(values) != len(columns) {
		diags = append(diags, Diagnostic{
			Code: DiagColumnMismatch, Level: "warning", Source: loc,
			Message: fmt.Sprintf("Style 字段数 %d 与 Format 列数 %d 不一致，按位置尽力映射", len(values), len(columns)),
		})
	}
	for i, col := range columns {
		var raw string
		if i < len(values) {
			raw = strings.TrimSpace(values[i])
		}
		key := CanonicalKey(col)
		if key == "name" {
			st.Name = raw
			continue
		}
		if key == "basedon" || key == "parent" || key == "inherits" || key == "inherit" {
			st.ParentSet = true
			st.Parent = raw
			st.ParentNull = raw == ""
			continue
		}
		if i >= len(values) || raw == "" {
			// 缺列或空位置列：显式 null，默认合并策略下不覆盖。
			st.Fields = append(st.Fields, FieldOccurrence{
				Key: key, Alias: col, Value: Null(), Index: len(st.Fields), Source: loc,
			})
			continue
		}
		st.appendField(key, col, Literal(raw), loc)
	}
	if st.Name == "" {
		st.Name = fmt.Sprintf("<unnamed@%s>", loc)
	}
	return st, diags
}
