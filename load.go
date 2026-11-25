package ssastyle

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// LoadDocumentFile 按扩展名解析 .ssa/.ass 或 .json 样式文档。
func LoadDocumentFile(path string) (*Document, []Diagnostic, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	return ParseDocumentBytes(data, path)
}

// ParseDocumentBytes 按 source 文件名扩展名选择解析器。
func ParseDocumentBytes(data []byte, source string) (*Document, []Diagnostic, error) {
	ext := strings.ToLower(filepath.Ext(source))
	switch ext {
	case ".json":
		d, err := ParseJSONDocument(data, source)
		return d, nil, err
	case ".ssa", ".ass", ".txt":
		return ParseSSA(strings.NewReader(string(data)), source)
	default:
		// 未知扩展名优先尝试 JSON，失败后按 SSA 解析。
		if d, err := ParseJSONDocument(data, source); err == nil {
			return d, nil, nil
		}
		return ParseSSA(strings.NewReader(string(data)), source)
	}
}

// Case 是一个自包含的比对任务：旧 SSA 侧、转换后侧与选项。
type Case struct {
	Styles         []string          `json:"styles"`
	ReportExtra    bool              `json:"report_extra"`
	StrictUnmapped bool              `json:"strict_unmapped"`
	Mapping        []PropertyMapping `json:"mapping,omitempty"`
	Merge          MergeOptions      `json:"merge"`
	Legacy         *Document         `json:"-"`
	Converted      *Document         `json:"-"`

	legacyRaw    json.RawMessage
	convertedRaw json.RawMessage
	baseDir      string
}

type caseWire struct {
	Styles         []string          `json:"styles"`
	ReportExtra    *bool             `json:"report_extra"`
	StrictUnmapped bool              `json:"strict_unmapped"`
	Mapping        []PropertyMapping `json:"mapping"`
	Merge          *MergeOptions     `json:"merge"`
	Legacy         json.RawMessage   `json:"legacy"`
	Converted      json.RawMessage   `json:"converted"`
}

// LoadCaseFile 读取 case JSON。其中 legacy/converted 既可以是内联文档对象，
// 也可以是相对于 case 文件目录的路径字符串。
func LoadCaseFile(path string) (*Case, []Diagnostic, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	var w caseWire
	if err := json.Unmarshal(data, &w); err != nil {
		return nil, nil, fmt.Errorf("case 解析失败: %w", err)
	}
	c := &Case{
		Styles:         w.Styles,
		StrictUnmapped: w.StrictUnmapped,
		Mapping:        w.Mapping,
		Merge:          DefaultMergeOptions(),
		ReportExtra:    true,
		baseDir:        filepath.Dir(path),
	}
	if w.Merge != nil {
		c.Merge = *w.Merge
	}
	if w.ReportExtra != nil {
		c.ReportExtra = *w.ReportExtra
	}
	var diags []Diagnostic
	if c.Legacy, diags, err = loadSide(w.Legacy, c.baseDir); err != nil {
		return nil, diags, fmt.Errorf("legacy: %w", err)
	}
	conv, diags2, err2 := loadSide(w.Converted, c.baseDir)
	if err2 != nil {
		return nil, append(diags, diags2...), fmt.Errorf("converted: %w", err)
	}
	c.Converted = conv
	diags = append(diags, diags2...)
	return c, diags, nil
}

func loadSide(raw json.RawMessage, baseDir string) (*Document, []Diagnostic, error) {
	if len(raw) == 0 {
		return NewDocument(""), nil, nil
	}
	var p string
	if err := json.Unmarshal(raw, &p); err == nil {
		if !filepath.IsAbs(p) {
			p = filepath.Join(baseDir, p)
		}
		return LoadDocumentFile(p)
	}
	d, err := ParseJSONDocument(raw, "<case-inline>")
	return d, nil, err
}

// CompareOptions 从 Case 构造比对选项。
func (c *Case) CompareOptions() CompareOptions {
	return CompareOptions{
		Merge:       c.Merge,
		Mapping:     c.Mapping,
		Styles:      c.Styles,
		ReportExtra: c.ReportExtra,
	}
}
