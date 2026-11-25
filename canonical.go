package fcpxml

import (
	"sort"
	"strconv"
	"strings"
)

// canonicalXMLNode produces a deterministic, human-readable representation of
// an XML subtree. It is used only for unmapped values; Parameter.Raw retains
// the exact imported bytes separately.
func canonicalXMLNode(n *xmlNode) string {
	if n == nil {
		return ""
	}
	var b strings.Builder
	writeCanonicalNode(&b, n)
	return b.String()
}

func writeCanonicalNode(b *strings.Builder, n *xmlNode) {
	b.WriteByte('<')
	b.WriteString(n.local())

	attrs := append([]xmlAttr(nil), convertAttrs(n)...)
	sort.SliceStable(attrs, func(i, j int) bool {
		if attrs[i].Name != attrs[j].Name {
			return attrs[i].Name < attrs[j].Name
		}
		return attrs[i].Value < attrs[j].Value
	})
	for _, attr := range attrs {
		b.WriteByte(' ')
		b.WriteString(attr.Name)
		b.WriteString("=")
		b.WriteString(strconv.Quote(attr.Value))
	}

	text := strings.TrimSpace(n.text.String())
	if len(n.children) == 0 && text == "" {
		b.WriteString("/>")
		return
	}
	b.WriteByte('>')
	if text != "" {
		b.WriteString(text)
	}
	childStrings := make([]string, 0, len(n.children))
	for _, child := range n.children {
		childStrings = append(childStrings, canonicalXMLNode(child))
	}
	sort.Strings(childStrings)
	for _, child := range childStrings {
		b.WriteString(child)
	}
	b.WriteString("</")
	b.WriteString(n.local())
	b.WriteByte('>')
}

type xmlAttr struct {
	Name  string
	Value string
}

func convertAttrs(n *xmlNode) []xmlAttr {
	out := make([]xmlAttr, 0, len(n.attrs))
	for _, attr := range n.attrs {
		name := attr.Name.Local
		if attr.Name.Space != "" {
			name = attr.Name.Space + "#" + name
		}
		out = append(out, xmlAttr{Name: name, Value: attr.Value})
	}
	return out
}
