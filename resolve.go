package ssastyle

import (
	"fmt"
	"sort"
	"strings"
)

// ResolvedField 是继承链合并后某字段的最终生效结果。
type ResolvedField struct {
	Key         string `json:"key"`
	Raw         string `json:"raw,omitempty"`
	Reset       bool   `json:"reset,omitempty"`
	SourceStyle string `json:"source_style,omitempty"`
	Source      string `json:"source,omitempty"`
}

// EffectiveStyle 是一条样式沿继承链解析后的最终生效样式。
type EffectiveStyle struct {
	Name        string                   `json:"name"`
	Chain       []string                 `json:"chain"`
	Fields      map[string]ResolvedField `json:"fields"`
	Diagnostics []Diagnostic             `json:"diagnostics,omitempty"`
}

// ValueOrEmpty 返回生效原始值；被 reset 或无值时返回空串与 false。
func (e *EffectiveStyle) Value(key string) (string, bool) {
	f, ok := e.Fields[CanonicalKey(key)]
	if !ok || f.Reset {
		return "", false
	}
	return f.Raw, true
}

// layered 是同名样式记录叠加后的内部结构（同一层）。
type layered struct {
	name         string
	parent       string
	parentSource string
	parentChosen bool
	occs         []FieldOccurrence
	recordCount  int
}

// Catalog 把一个或多个 Document 按顺序叠加成可解析继承链的样式目录。
// 后加入的 Document/同名 Style 行不会用空值抹掉先出现的有效值。
type Catalog struct {
	opts        MergeOptions
	styles      map[string]*layered
	order       []string
	Diagnostics []Diagnostic
}

// NewCatalog 按给定顺序构建目录（靠后的 Document 仅在提供有效值时覆盖）。
func NewCatalog(opts MergeOptions, docs ...*Document) *Catalog {
	c := &Catalog{opts: opts, styles: map[string]*layered{}}
	for _, d := range docs {
		if d == nil {
			continue
		}
		for _, st := range d.Styles {
			c.addStyle(st)
		}
	}
	c.detectDuplicateConflicts()
	return c
}

func (c *Catalog) addStyle(st *Style) {
	if st == nil {
		return
	}
	l, ok := c.styles[st.Name]
	if !ok {
		l = &layered{name: st.Name}
		c.styles[st.Name] = l
		c.order = append(c.order, st.Name)
	} else {
		l.recordCount++
		c.Diagnostics = append(c.Diagnostics, Diagnostic{
			Code: DiagDuplicateStyle, Level: "warning", Style: st.Name,
			Message: fmt.Sprintf("样式 %q 重复定义，%d 条记录将按出现顺序叠加", st.Name, l.recordCount+1),
			Source:  st.Source,
		})
	}
	if st.ParentSet {
		nullParent := st.ParentNull || strings.TrimSpace(st.Parent) == ""
		switch {
		case nullParent && c.opts.ExplicitNullResets:
			l.parent, l.parentChosen = "", false
			l.parentSource = st.Source
		case nullParent:
			// 默认空值语义：null/空 parent 不覆盖已有继承关系。
			if l.parentChosen {
				c.Diagnostics = append(c.Diagnostics, warning(DiagEmptyIgnored,
					fmt.Sprintf("样式 %q 后出现的空 parent 被忽略，保留继承 %q", st.Name, l.parent)).withStyle(st.Name, st.Source))
			}
		default:
			l.parent, l.parentChosen = st.Parent, true
			l.parentSource = st.Source
		}
	}
	l.occs = append(l.occs, st.Fields...)
}

func (c *Catalog) detectDuplicateConflicts() {
	for _, name := range c.order {
		l := c.styles[name]
		byKey := map[string][]FieldOccurrence{}
		for _, o := range l.occs {
			if o.Value.IsNullLike(c.opts) {
				continue
			}
			byKey[o.Key] = append(byKey[o.Key], o)
		}
		keys := make([]string, 0, len(byKey))
		for k := range byKey {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			occs := byKey[k]
			if len(occs) < 2 {
				continue
			}
			first := strings.TrimSpace(occs[0].Value.Raw)
			conflict := false
			for _, o := range occs[1:] {
				if strings.TrimSpace(o.Value.Raw) != first {
					conflict = true
					break
				}
			}
			if !conflict {
				continue
			}
			d := Diagnostic{
				Code: DiagDuplicateField, Style: name, Field: k,
				Source: occs[len(occs)-1].Source,
				Message: fmt.Sprintf("样式 %q 的字段 %q 在同层重复且取值不一致，按最后一个有效值 %q 生效",
					name, k, occs[len(occs)-1].Value.Raw),
			}
			if c.opts.DuplicateConflictIsError {
				d.Level = "error"
			} else {
				d.Level = "warning"
			}
			c.Diagnostics = append(c.Diagnostics, d)
		}
	}
}

// Names 返回目录中全部样式名（首次出现顺序）。
func (c *Catalog) Names() []string { return append([]string(nil), c.order...) }

// Has 报告样式是否存在。
func (c *Catalog) Has(name string) bool { _, ok := c.styles[name]; return ok }

// ResolveAll 解析目录内全部样式的最终生效样式。
func (c *Catalog) ResolveAll() map[string]*EffectiveStyle {
	out := make(map[string]*EffectiveStyle, len(c.order))
	for _, name := range c.order {
		out[name] = c.Resolve(name)
	}
	return out
}

// Resolve 沿 parent 链解析单个样式。未知父样式与环会产生诊断并截断链条。
func (c *Catalog) Resolve(name string) *EffectiveStyle {
	eff := &EffectiveStyle{Name: name, Fields: map[string]ResolvedField{}}
	l, ok := c.styles[name]
	if !ok {
		eff.Diagnostics = append(eff.Diagnostics, errDiag(DiagUnknownParent,
			fmt.Sprintf("样式 %q 不存在", name)).withStyle(name, ""))
		return eff
	}

	// 自底向上收集链，再反转为 root -> leaf。
	var bottomUp []*layered
	seen := map[string]int{}
	cur := l
	for cur != nil {
		if at, repeated := seen[cur.name]; repeated {
			cycle := append(chainNames(bottomUp[at:]), cur.name)
			eff.Diagnostics = append(eff.Diagnostics, errDiag(DiagInheritanceCycle,
				fmt.Sprintf("检测到继承环: %s", strings.Join(cycle, " -> "))).withStyle(name, ""))
			break
		}
		seen[cur.name] = len(bottomUp)
		bottomUp = append(bottomUp, cur)
		if !cur.parentChosen || strings.TrimSpace(cur.parent) == "" {
			break
		}
		parent, ok := c.styles[cur.parent]
		if !ok {
			eff.Diagnostics = append(eff.Diagnostics, warning(DiagUnknownParent,
				fmt.Sprintf("样式 %q 继承的 %q 不存在，链条在 %q 处截断", cur.name, cur.parent, cur.name)).
				withStyle(cur.name, cur.parentSource))
			break
		}
		cur = parent
	}
	for i := len(bottomUp) - 1; i >= 0; i-- {
		eff.Chain = append(eff.Chain, bottomUp[i].name)
	}

	// root -> leaf 顺序应用全部出现；空值不覆盖有效值。
	for i := len(bottomUp) - 1; i >= 0; i-- {
		node := bottomUp[i]
		c.mergeLayer(eff, node, &eff.Diagnostics)
	}
	return eff
}

func chainNames(ls []*layered) []string {
	out := make([]string, 0, len(ls))
	for _, l := range ls {
		out = append(out, l.name)
	}
	return out
}

func (c *Catalog) mergeLayer(eff *EffectiveStyle, l *layered, diags *[]Diagnostic) {
	for _, occ := range l.occs {
		key := occ.Key
		if occ.Value.IsNullLike(c.opts) {
			prev, exists := eff.Fields[key]
			hasLiveValue := exists && !prev.Reset
			if c.opts.ExplicitNullResets && occ.Value.Presence == ExplicitNull {
				eff.Fields[key] = ResolvedField{Key: key, Reset: true, SourceStyle: l.name, Source: occ.Source}
				if hasLiveValue {
					*diags = append(*diags, Diagnostic{
						Code: DiagNullReset, Level: "warning", Style: l.name, Field: key,
						Message: fmt.Sprintf("字段 %q 的显式 null 按策略重置了已有/继承值", key), Source: occ.Source,
					})
				}
				continue
			}
			if hasLiveValue {
				*diags = append(*diags, Diagnostic{
					Code: DiagEmptyIgnored, Level: "warning", Style: l.name, Field: key,
					Message: fmt.Sprintf("后出现的空值（%s）未覆盖已有/继承值 %q", nullDesc(occ.Value), prev.Raw),
					Source:  occ.Source,
				})
			}
			continue
		}
		eff.Fields[key] = ResolvedField{
			Key: key, Raw: occ.Value.Raw, SourceStyle: l.name, Source: occ.Source,
		}
	}
}

func nullDesc(v Value) string {
	switch v.Presence {
	case ExplicitNull:
		return "null"
	case Absent:
		return "absent"
	default:
		return "空白字符串"
	}
}

func (d Diagnostic) withStyle(style, source string) Diagnostic {
	d.Style = style
	if source != "" {
		d.Source = source
	}
	return d
}
