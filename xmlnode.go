package smptett

import (
	"encoding/xml"
	"fmt"
	"io"
	"strings"
)

type xmlNode struct {
	Name     xml.Name
	Attr     []xml.Attr
	Children []*xmlNode
	Content  []xmlContent
	Text     string
	Comment  bool
}

type xmlContent struct {
	Text  string
	Child *xmlNode
}

func parseXMLTree(r io.Reader) (*xmlNode, error) {
	dec := xml.NewDecoder(r)
	dec.Strict = true
	var root *xmlNode
	var stack []*xmlNode

	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("decode XML: %w", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			n := &xmlNode{Name: t.Name, Attr: append([]xml.Attr(nil), t.Attr...)}
			if len(stack) == 0 {
				if root != nil {
					return nil, fmt.Errorf("multiple root elements")
				}
				root = n
			} else {
				parent := stack[len(stack)-1]
				parent.Children = append(parent.Children, n)
				parent.Content = append(parent.Content, xmlContent{Child: n})
			}
			stack = append(stack, n)
		case xml.EndElement:
			if len(stack) == 0 {
				return nil, fmt.Errorf("unexpected closing element %q", t.Name.Local)
			}
			stack = stack[:len(stack)-1]
		case xml.CharData:
			if len(stack) != 0 {
				parent := stack[len(stack)-1]
				text := string(t)
				parent.Text += text
				parent.Content = append(parent.Content, xmlContent{Text: text})
			}
		case xml.Comment:
			if len(stack) != 0 {
				stack[len(stack)-1].Children = append(stack[len(stack)-1].Children, &xmlNode{
					Name:    xml.Name{Local: "#comment"},
					Text:    string(t),
					Comment: true,
				})
			}
		}
	}
	if root == nil {
		return nil, fmt.Errorf("empty XML document")
	}
	if len(stack) != 0 {
		return nil, fmt.Errorf("unclosed XML element %q", stack[len(stack)-1].Name.Local)
	}
	return root, nil
}

func attrValue(n *xmlNode, local string) (string, bool) {
	for _, attr := range n.Attr {
		if attr.Name.Local == local {
			return strings.TrimSpace(attr.Value), true
		}
	}
	return "", false
}

func attrLocalMap(n *xmlNode) map[string]string {
	out := make(map[string]string, len(n.Attr))
	for _, attr := range n.Attr {
		out[attr.Name.Local] = strings.TrimSpace(attr.Value)
	}
	return out
}

func childElements(n *xmlNode, local string) []*xmlNode {
	var out []*xmlNode
	for _, child := range n.Children {
		if child.Comment || child.Name.Local == "#text" {
			continue
		}
		if local == "" || child.Name.Local == local {
			out = append(out, child)
		}
	}
	return out
}

func firstChild(n *xmlNode, local string) *xmlNode {
	children := childElements(n, local)
	if len(children) == 0 {
		return nil
	}
	return children[0]
}

func hasDescendant(n *xmlNode, local string) bool {
	for _, child := range n.Children {
		if child.Comment {
			continue
		}
		if child.Name.Local == local || hasDescendant(child, local) {
			return true
		}
	}
	return false
}

func hasDirectText(n *xmlNode) bool {
	for _, part := range n.Content {
		if part.Child == nil && strings.TrimSpace(part.Text) != "" {
			return true
		}
	}
	return false
}

func normalizeText(s string) string {
	lines := strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n")
	for i := range lines {
		lines[i] = strings.Join(strings.Fields(lines[i]), " ")
	}
	for len(lines) > 0 && lines[0] == "" {
		lines = lines[1:]
	}
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return strings.Join(lines, "\n")
}

func nodeTokens(n *xmlNode) []string {
	var out []string
	for _, name := range []string{"role", "class"} {
		if value, ok := attrValue(n, name); ok {
			for _, token := range strings.Fields(value) {
				out = append(out, strings.ToLower(token))
			}
		}
	}
	return out
}

func isAnnotationNode(n *xmlNode) bool {
	switch n.Name.Local {
	case "annotation", "comment", "note", "metadata", "desc":
		return true
	}
	for _, token := range nodeTokens(n) {
		switch token {
		case "annotation", "comment", "note":
			return true
		}
	}
	return false
}

func isMetadataNode(n *xmlNode) bool {
	switch n.Name.Local {
	case "metadata", "desc":
		return true
	}
	return false
}
