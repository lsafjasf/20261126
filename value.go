package ssastyle

import (
	"encoding/json"
	"strings"
)

// Presence 描述一个样式字段的“存在性”。
//
// 旧 SSA 样式在多层继承、重复 Style 行以及格式转换记录中，同一个字段会
// 反复出现。转换链路经常把“没有值”的序列化成 null 或空串；若按普通
// map 的“后者覆盖前者”合并，后出现的空值就会把真正生效的值抹掉。
// 因此这里把“值是什么”和“值是否存在”分开表示。
type Presence uint8

const (
	// Absent 表示记录中根本没有这个字段（未声明）。
	Absent Presence = iota
	// ExplicitNull 表示字段出现了，但值是显式 null（或 SSA 位置列留空）。
	ExplicitNull
	// Set 表示字段带有真实值。注意 0、-1、false 也是 Set，绝不能当空值。
	Set
)

// Value 是带存在性跟踪的字段值。零值等价于 Missing()。
type Value struct {
	Presence Presence
	// Raw 保留未经归一化的原始文本，便于报告与追溯。
	Raw string
}

// Missing 返回 Absent 值。
func Missing() Value { return Value{Presence: Absent} }

// Null 返回显式 null 值。
func Null() Value { return Value{Presence: ExplicitNull} }

// Literal 返回一个真实值。
func Literal(raw string) Value { return Value{Presence: Set, Raw: raw} }

// IsSet 报告该值是否为真实设置（Presence == Set）。
func (v Value) IsSet() bool { return v.Presence == Set }

// IsNullLike 按合并策略判断该值是否“没有资格覆盖”已有值：
// Absent/ExplicitNull 永远算空；Set 的空白字符串仅在 EmptyIsUnset 时算空。
func (v Value) IsNullLike(opts MergeOptions) bool {
	switch v.Presence {
	case Absent, ExplicitNull:
		return true
	case Set:
		return opts.EmptyIsUnset && strings.TrimSpace(v.Raw) == ""
	default:
		return true
	}
}

// MergeOptions 控制重复字段与空值的合并语义。
type MergeOptions struct {
	// EmptyIsUnset 为 true（默认）时，空白字符串与 null 同等对待，
	// 不会覆盖继承/先出现的值。SSA 的空位置列也依赖这一语义。
	EmptyIsUnset bool `json:"empty_is_unset"`

	// ExplicitNullResets 为 true 时，更近一层的显式 null 被解释为
	// “有意清空/重置”，继承链在此截断（最终字段为 Reset 状态），
	// 而不是继续向父层取值。默认 false：null 只代表“本层未提供”，
	// 继续继承——这正是修复“null 覆盖有效值”缺陷所需的语义。
	ExplicitNullResets bool `json:"explicit_null_resets"`

	// DuplicateConflictIsError 为 true 时，同一作用域内重复字段给出
	// 互不相同的有效值将产生 error 级诊断；默认只告警并按
	// “最后一个有效的值生效”处理。
	DuplicateConflictIsError bool `json:"duplicate_conflict_is_error"`
}

// DefaultMergeOptions 返回修复空值覆盖问题所需的默认策略。
func DefaultMergeOptions() MergeOptions {
	return MergeOptions{EmptyIsUnset: true}
}

// UnmarshalJSON 使省略的选项保持 DefaultMergeOptions 的默认值。
func (m *MergeOptions) UnmarshalJSON(data []byte) error {
	type wire struct {
		EmptyIsUnset             *bool `json:"empty_is_unset"`
		ExplicitNullResets       *bool `json:"explicit_null_resets"`
		DuplicateConflictIsError *bool `json:"duplicate_conflict_is_error"`
	}
	var w wire
	if err := json.Unmarshal(data, &w); err != nil {
		return err
	}
	*m = DefaultMergeOptions()
	if w.EmptyIsUnset != nil {
		m.EmptyIsUnset = *w.EmptyIsUnset
	}
	if w.ExplicitNullResets != nil {
		m.ExplicitNullResets = *w.ExplicitNullResets
	}
	if w.DuplicateConflictIsError != nil {
		m.DuplicateConflictIsError = *w.DuplicateConflictIsError
	}
	return nil
}

// DiagnosticCode 是稳定的机器可读诊断码。
type DiagnosticCode string

const (
	DiagDuplicateStyle   DiagnosticCode = "duplicate_style"
	DiagDuplicateField   DiagnosticCode = "duplicate_field"
	DiagEmptyIgnored     DiagnosticCode = "empty_value_ignored"
	DiagNullReset        DiagnosticCode = "null_reset"
	DiagUnknownParent    DiagnosticCode = "unknown_parent"
	DiagInheritanceCycle DiagnosticCode = "inheritance_cycle"
	DiagColumnMismatch   DiagnosticCode = "column_mismatch"
	DiagUnresolvedField  DiagnosticCode = "unresolved_field"
	DiagUnmappedProperty DiagnosticCode = "unmapped_property"
	DiagInvalidValue     DiagnosticCode = "invalid_value"
)

// Diagnostic 描述解析、合并、继承或比对过程中的可复核事件。
type Diagnostic struct {
	Code    DiagnosticCode `json:"code"`
	Level   string         `json:"level"` // "warning" | "error"
	Style   string         `json:"style,omitempty"`
	Field   string         `json:"field,omitempty"`
	Message string         `json:"message"`
	Source  string         `json:"source,omitempty"`
}

func warning(code DiagnosticCode, msg string) Diagnostic {
	return Diagnostic{Code: code, Level: "warning", Message: msg}
}

func errDiag(code DiagnosticCode, msg string) Diagnostic {
	return Diagnostic{Code: code, Level: "error", Message: msg}
}
