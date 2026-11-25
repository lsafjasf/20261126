package ssastyle

import "strings"

// FieldKind 决定字段值如何归一化以进行逐字段比较。
type FieldKind string

const (
	KindString    FieldKind = "string"
	KindBool      FieldKind = "bool"
	KindInt       FieldKind = "int"
	KindFloat     FieldKind = "float"
	KindColor     FieldKind = "color"
	KindAlignment FieldKind = "alignment"
)

// FieldSpec 是已知 SSA/ASS 样式字段的登记表项。
type FieldSpec struct {
	Key     string
	Aliases []string
	Kind    FieldKind
	// SSAOnly 表示该字段只存在于旧 SSA（V4），ASS V4+ 已改名或取消。
	SSAOnly bool
}

// CanonicalKey 把各种书写形式归一为小写无分隔符键，
// 使 Fontname / FontName / font_name / font-name 指向同一字段。
func CanonicalKey(k string) string {
	k = strings.ToLower(strings.TrimSpace(k))
	var b strings.Builder
	for _, r := range k {
		switch r {
		case '_', '-', ' ', '\t':
			continue
		}
		b.WriteRune(r)
	}
	s := b.String()
	// 英美颜色拼写统一。
	s = strings.ReplaceAll(s, "colour", "color")
	return s
}

// fieldRegistry 以 canonical key 索引已知字段。
var fieldRegistry = map[string]FieldSpec{}

func register(spec FieldSpec) {
	fieldRegistry[spec.Key] = spec
	for _, a := range spec.Aliases {
		fieldRegistry[CanonicalKey(a)] = spec
	}
}

func init() {
	known := []FieldSpec{
		{Key: "name", Kind: KindString},
		{Key: "fontname", Aliases: []string{"font"}, Kind: KindString},
		{Key: "fontsize", Kind: KindFloat},
		{Key: "primarycolor", Kind: KindColor},
		{Key: "secondarycolor", Kind: KindColor},
		{Key: "tertiarycolor", Kind: KindColor, SSAOnly: true},
		{Key: "outlinecolor", Kind: KindColor},
		{Key: "backcolor", Kind: KindColor},
		{Key: "bold", Kind: KindBool},
		{Key: "italic", Kind: KindBool},
		{Key: "underline", Kind: KindBool},
		{Key: "strikeout", Aliases: []string{"strikethrough"}, Kind: KindBool},
		{Key: "scalex", Kind: KindFloat},
		{Key: "scaley", Kind: KindFloat},
		{Key: "spacing", Kind: KindFloat},
		{Key: "angle", Kind: KindFloat},
		{Key: "borderstyle", Kind: KindInt},
		{Key: "outline", Kind: KindFloat},
		{Key: "shadow", Kind: KindFloat},
		{Key: "alignment", Kind: KindAlignment},
		{Key: "marginl", Aliases: []string{"marginleft"}, Kind: KindInt},
		{Key: "marginr", Aliases: []string{"marginright"}, Kind: KindInt},
		{Key: "marginv", Aliases: []string{"margint", "marginvertical"}, Kind: KindInt},
		{Key: "alphalevel", Kind: KindInt, SSAOnly: true},
		{Key: "encoding", Kind: KindInt},
		// 继承扩展列（非标准 SSA，但旧工具链/转换记录里可能出现）。
		{Key: "basedon", Aliases: []string{"parent", "inherits", "inherit", "parentstyle"}, Kind: KindString},
	}
	for _, f := range known {
		register(f)
	}
}

// LookupField 返回字段登记信息；未知字段 ok=false。
func LookupField(key string) (spec FieldSpec, ok bool) {
	spec, ok = fieldRegistry[CanonicalKey(key)]
	return
}
