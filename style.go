package ttmlevents

import (
	"sort"
	"strings"
)

var canonicalStyleNames = map[string]string{
	"color":           "color",
	"backgroundcolor": "backgroundColor",
	"fontfamily":      "fontFamily",
	"fontsize":        "fontSize",
	"fontweight":      "fontWeight",
	"fontstyle":       "fontStyle",
	"textdecoration":  "textDecoration",
	"textalign":       "textAlign",
	"visibility":      "visibility",
	"display":         "display",
	"opacity":         "opacity",
	"lineheight":      "lineHeight",
	"wrapoption":      "wrapOption",
	"direction":       "direction",
	"unicodebidi":     "unicodeBidi",
	"ruby":            "ruby",
	"rubyposition":    "rubyPosition",
	"textemphasis":    "textEmphasis",
	"textshadow":      "textShadow",
	"outline":         "outline",
	"padding":         "padding",
	"margin":          "margin",
	"extent":          "extent",
	"origin":          "origin",
	"zindex":          "zIndex",
	"showbackground":  "showBackground",
	"linethickness":   "lineThickness",
	"textoutline":     "textOutline",
}

var inheritableProperties = map[string]bool{
	"color":          true,
	"fontFamily":     true,
	"fontSize":       true,
	"fontWeight":     true,
	"fontStyle":      true,
	"textDecoration": true,
	"textAlign":      true,
	"lineHeight":     true,
	"direction":      true,
	"unicodeBidi":    true,
	"wrapOption":     true,
	"visibility":     true,
	"ruby":           true,
	"rubyPosition":   true,
	"textEmphasis":   true,
	"textShadow":     true,
	"textOutline":    true,
}

func canonicalPropertyName(raw string) string {
	name := strings.TrimSpace(raw)
	if i := strings.LastIndex(name, ":"); i >= 0 {
		name = name[i+1:]
	}
	compact := strings.ReplaceAll(strings.ToLower(name), "-", "")
	if canonical, ok := canonicalStyleNames[compact]; ok {
		return canonical
	}
	return name
}

func isInheritableProperty(raw string) bool {
	return inheritableProperties[canonicalPropertyName(raw)]
}

type propertyMap map[string]Property

type styleResolver struct {
	doc   *Document
	cache map[int]propertyMap
}

func newStyleResolver(doc *Document) *styleResolver {
	return &styleResolver{doc: doc, cache: make(map[int]propertyMap)}
}

func (r *styleResolver) baseProperties(n *Node) propertyMap {
	if n == nil || n.Kind != ElementNode {
		return propertyMap{}
	}
	if cached, ok := r.cache[n.DocOrder]; ok {
		return clonePropertyMap(cached)
	}

	result := propertyMap{}
	if n.Parent != nil && n.Parent.Kind == ElementNode {
		parent := r.baseProperties(n.Parent)
		for name, prop := range parent {
			if inheritableProperties[name] {
				prop.Inherited = true
				result[name] = prop
			}
		}
	}

	if rawStyle, ok := n.AttrValue("style"); ok {
		applyStyleAttribute(rawStyle, n.Path, pathDepth(n.Path), result, r)
	}
	r.applyInlineAttributes(n, result)
	r.cache[n.DocOrder] = clonePropertyMap(result)
	return result
}

func applyStyleAttribute(raw, nodePath string, depth int, out propertyMap, resolver *styleResolver) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return
	}
	if strings.Contains(raw, ":") {
		for _, declaration := range strings.Split(raw, ";") {
			nameValue := strings.SplitN(declaration, ":", 2)
			if len(nameValue) != 2 {
				continue
			}
			compact := strings.ReplaceAll(strings.ToLower(strings.TrimSpace(nameValue[0])), "-", "")
			name, known := canonicalStyleNames[compact]
			if !known {
				continue
			}
			out[name] = Property{
				Name:             name,
				Value:            strings.TrimSpace(nameValue[1]),
				Source:           nodePath + "/@style",
				DeclarationDepth: depth,
			}
		}
		return
	}
	for _, id := range splitIDRefs(raw) {
		resolver.applyStyleRule(id, out, map[string]bool{}, depth)
	}
}

func (r *styleResolver) applyStyleRule(id string, out propertyMap, active map[string]bool, depth int) {
	rule, ok := r.doc.Styles[id]
	if !ok || active[id] {
		if !ok {
			r.doc.Warnings = append(r.doc.Warnings, Warning{Code: "missing-style", Message: id})
		}
		return
	}
	active[id] = true
	defer delete(active, id)

	for _, parentID := range rule.ParentRefs {
		r.applyStyleRule(parentID, out, active, depth)
	}
	for _, prop := range rule.Properties {
		prop.Inherited = false
		prop.DeclarationDepth = depth
		out[prop.Name] = prop
	}
}

func (r *styleResolver) applyInlineAttributes(n *Node, out propertyMap) {
	for _, attr := range n.Attrs {
		name := canonicalPropertyName(attr.Name)
		if !isStyleAttribute(attr, name) {
			continue
		}
		out[name] = Property{
			Name:             name,
			Value:            strings.TrimSpace(attr.Value),
			Source:           n.Path + "/@" + attr.Name,
			DeclarationDepth: pathDepth(n.Path),
		}
	}
}

func isStyleAttribute(attr Attr, canonical string) bool {
	if strings.Contains(attr.Namespace, "ttml") && strings.Contains(attr.Namespace, "styling") {
		return true
	}
	if _, ok := canonicalStyleNames[strings.ToLower(attr.Name)]; !ok {
		return false
	}
	switch canonical {
	case "id":
		return false
	}
	switch strings.ToLower(attr.Name) {
	case "begin", "end", "dur", "timing", "timecontainer", "fill", "repeatcount",
		"region", "style", "agent", "role", "lang", "xmllang", "space":
		return false
	default:
		return true
	}
}

func clonePropertyMap(in propertyMap) propertyMap {
	out := make(propertyMap, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func sortedProperties(m propertyMap) []Property {
	names := make([]string, 0, len(m))
	for name := range m {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]Property, 0, len(names))
	for _, name := range names {
		out = append(out, m[name])
	}
	return out
}
