package ssastyle

// MapStatus 描述旧 SSA 属性到新样式模型的可映射程度。
type MapStatus string

const (
	MapDirect      MapStatus = "direct"      // 同名/同语义直接映射
	MapRenamed     MapStatus = "renamed"     // 改名映射（如 TertiaryColour -> OutlineColour）
	MapTransformed MapStatus = "transformed" // 需要值变换（如旧对齐码 -> ASS 小键盘码）
	MapDerived     MapStatus = "derived"     // 目标无独立字段，只能折算/派生，属有损或需人工确认
	MapUnmapped    MapStatus = "unmapped"    // 完全无法映射，必须单独报告
)

// PropertyMapping 是一条旧字段到目标字段的映射规则。
type PropertyMapping struct {
	Source string    `json:"source"`
	Target string    `json:"target,omitempty"`
	Status MapStatus `json:"status"`
	Note   string    `json:"note,omitempty"`
}

// UnmappedProperty 是“无法作为目标字段映射”的属性报告项。
// Status 为 MapUnmapped（无法映射）或 MapDerived（只能有损派生）。
type UnmappedProperty struct {
	Style       string    `json:"style"`
	Field       string    `json:"field"`
	Alias       string    `json:"alias,omitempty"`
	RawValue    string    `json:"raw_value,omitempty"`
	Status      MapStatus `json:"status"`
	Reason      string    `json:"reason"`
	Source      string    `json:"source,omitempty"`
	SourceStyle string    `json:"source_style,omitempty"`
}

// DefaultMapping 返回旧 SSA V4 -> ASS V4+/规范样式模型的默认映射表。
func DefaultMapping() []PropertyMapping {
	direct := []string{
		"fontname", "fontsize", "primarycolor", "secondarycolor", "backcolor",
		"bold", "italic", "underline", "strikeout", "scalex", "scaley",
		"spacing", "angle", "borderstyle", "outline", "shadow",
		"marginl", "marginr", "marginv", "encoding",
	}
	m := make([]PropertyMapping, 0, len(direct)+4)
	for _, k := range direct {
		m = append(m, PropertyMapping{Source: k, Target: k, Status: MapDirect})
	}
	m = append(m,
		PropertyMapping{Source: "alignment", Target: "alignment", Status: MapTransformed,
			Note: "旧 SSA 对齐码 5/6/7、9/10/11 需换算为 ASS 小键盘 7/8/9、4/5/6"},
		PropertyMapping{Source: "tertiarycolor", Target: "outlinecolor", Status: MapRenamed,
			Note: "SSA TertiaryColour 在 ASS V4+ 中更名为 OutlineColour"},
		PropertyMapping{Source: "alphalevel", Target: "", Status: MapDerived,
			Note: "ASS 无独立 AlphaLevel；可折算进四个颜色的 Alpha 字节，无法作为独立字段往返，需单独确认"},
	)
	return m
}

// mappingIndex 把映射表按 source canonical key 索引。
type mappingIndex struct {
	rules  map[string]PropertyMapping
	custom bool
}

func buildMappingIndex(rules []PropertyMapping) *mappingIndex {
	if rules == nil {
		rules = DefaultMapping()
	}
	idx := &mappingIndex{rules: map[string]PropertyMapping{}}
	for _, r := range rules {
		r.Source = CanonicalKey(r.Source)
		if r.Target != "" {
			r.Target = CanonicalKey(r.Target)
		}
		idx.rules[r.Source] = r
	}
	return idx
}

// classify 返回某旧字段的映射规则；未登记字段按“无法映射”处理。
func (idx *mappingIndex) classify(key string) PropertyMapping {
	if r, ok := idx.rules[key]; ok {
		return r
	}
	if _, known := LookupField(key); known {
		// 登记表内但映射表遗漏：保守报告为无法映射，而不是静默丢弃。
		return PropertyMapping{Source: key, Status: MapUnmapped,
			Note: "已知 SSA/ASS 字段，但当前映射表未给出目标"}
	}
	return PropertyMapping{Source: key, Status: MapUnmapped,
		Note: "未知扩展字段，目标样式模型中没有对应属性"}
}

// metaKeys 是不参与逐字段样式比较的身份/继承元数据。
func isMetaKey(key string) bool {
	switch CanonicalKey(key) {
	case "name", "basedon":
		return true
	}
	return false
}
