package ssastyle

// FieldOccurrence 是同一个字段在某一层记录中的一次出现。
// 重复字段不会被 map 静默吞掉：每次出现都保留原始值、次序与来源。
type FieldOccurrence struct {
	// Key 是规范化后的字段键（小写、去下划线/连字符/空格）。
	Key string `json:"key"`
	// Alias 是输入中实际使用的列名/键名，便于回溯。
	Alias string `json:"alias,omitempty"`
	Value Value  `json:"value"`
	// Index 是该记录内的出现序号（从 0 开始）。
	Index int `json:"index"`
	// Source 是 “文件:行号” 之类的可定位来源。
	Source string `json:"source,omitempty"`
}

// Style 是一条原始样式记录（某一层、某一次 Style 行或 JSON 节点）。
// 同名 Style 可以有多条，它们会按出现顺序叠加。
type Style struct {
	Name   string `json:"name"`
	Parent string `json:"parent,omitempty"`
	// ParentSet 区分 “没有 parent 键” 与 “parent: null/空”。
	ParentSet bool `json:"parent_set,omitempty"`
	// ParentNull 表示 parent 被显式写成 null/空串。
	ParentNull bool `json:"parent_null,omitempty"`

	// Fields 保留全部字段出现，包括重复项与空值。
	Fields []FieldOccurrence `json:"fields,omitempty"`

	// Source 是该记录的可定位来源。
	Source string `json:"source,omitempty"`
}

// Document 是一个来源（旧 SSA 文件、转换结果 JSON、样式层等）的样式集合。
type Document struct {
	// Dialect 决定对齐码、颜色等归一化规则："ssa" | "ass" | "json"。
	Dialect string   `json:"dialect,omitempty"`
	Source  string   `json:"source,omitempty"`
	Styles  []*Style `json:"styles"`
}

// NewDocument 返回指定方言的空文档。
func NewDocument(dialect string) *Document {
	return &Document{Dialect: dialect}
}

// appendField 追加一次字段出现。
func (s *Style) appendField(key, alias string, v Value, source string) {
	ck := CanonicalKey(key)
	if spec, ok := LookupField(ck); ok && spec.Key != "" {
		ck = spec.Key
	}
	s.Fields = append(s.Fields, FieldOccurrence{
		Key:    ck,
		Alias:  alias,
		Value:  v,
		Index:  len(s.Fields),
		Source: source,
	})
}

// AddField 以编程方式追加一个真实字段值。
func (s *Style) AddField(key, raw string) *Style {
	s.appendField(CanonicalKey(key), key, Literal(raw), s.Source)
	return s
}

// AddNullField 以编程方式追加一个显式 null 字段。
func (s *Style) AddNullField(key string) *Style {
	s.appendField(CanonicalKey(key), key, Null(), s.Source)
	return s
}

// SetParent 设置继承的父样式名。
func (s *Style) SetParent(parent string) *Style {
	s.Parent = parent
	s.ParentSet = true
	s.ParentNull = parent == ""
	return s
}
