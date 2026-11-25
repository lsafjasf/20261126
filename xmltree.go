package fcpxml

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"strings"
)

type xmlNode struct {
	name     xml.Name
	attrs    []xml.Attr
	children []*xmlNode
	text     strings.Builder
	raw      []byte
	start    int
	parent   *xmlNode
	hasText  bool
}

func parseXMLTree(input []byte) (*xmlNode, error) {
	dec := xml.NewDecoder(bytes.NewReader(input))
	var root *xmlNode
	var stack []*xmlNode

	for {
		startOffset := int(dec.InputOffset())
		token, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("parse XML: %w", err)
		}
		endOffset := int(dec.InputOffset())

		switch t := token.(type) {
		case xml.StartElement:
			if len(stack) == 0 && root != nil {
				return nil, fmt.Errorf("parse XML: multiple document elements")
			}
			n := &xmlNode{
				name:  t.Name,
				attrs: append([]xml.Attr(nil), t.Attr...),
				start: startOffset,
			}
			if len(stack) > 0 {
				n.parent = stack[len(stack)-1]
				n.parent.children = append(n.parent.children, n)
			} else {
				if root == nil {
					root = n
				}
			}
			stack = append(stack, n)
			if endOffset > startOffset {
				n.raw = append([]byte(nil), input[startOffset:endOffset]...)
			}
		case xml.EndElement:
			if len(stack) == 0 {
				return nil, fmt.Errorf("parse XML: unexpected end element %q", t.Name.Local)
			}
			n := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			// Re-read the complete span when offsets are available. This also
			// captures comments and whitespace exactly for non-self-closing
			// parameter elements.
			if n.start >= 0 && endOffset > n.start {
				n.raw = append([]byte(nil), input[n.start:endOffset]...)
			}
		case xml.CharData:
			if len(stack) > 0 {
				n := stack[len(stack)-1]
				n.text.Write(t)
				if len(t) > 0 && !isXMLSpace(t) {
					n.hasText = true
				}
			}
		}
	}
	if len(stack) != 0 {
		return nil, fmt.Errorf("parse XML: %d unclosed element(s)", len(stack))
	}
	if root == nil {
		return nil, fmt.Errorf("parse XML: no document element")
	}
	return root, nil
}

func isXMLSpace(b []byte) bool {
	for _, c := range b {
		switch c {
		case ' ', '\t', '\r', '\n':
		default:
			return false
		}
	}
	return true
}

func (n *xmlNode) local() string {
	if n == nil {
		return ""
	}
	return n.name.Local
}

func (n *xmlNode) attr(names ...string) string {
	if n == nil {
		return ""
	}
	for _, name := range names {
		for _, a := range n.attrs {
			if a.Name.Local == name {
				return a.Value
			}
		}
	}
	return ""
}

func (n *xmlNode) childText(names ...string) string {
	if n == nil {
		return ""
	}
	values := make([]string, 0)
	for _, child := range n.children {
		for _, name := range names {
			if child.local() == name {
				values = append(values, strings.TrimSpace(child.text.String()))
			}
		}
	}
	return minNonEmpty(values)
}

func (n *xmlNode) directChildren(names ...string) []*xmlNode {
	out := make([]*xmlNode, 0)
	for _, child := range n.children {
		for _, name := range names {
			if child.local() == name {
				out = append(out, child)
				break
			}
		}
	}
	return out
}

func minNonEmpty(values []string) string {
	best := ""
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if best == "" || value < best {
			best = value
		}
	}
	return best
}

func walkXML(n *xmlNode, fn func(*xmlNode)) {
	if n == nil {
		return
	}
	fn(n)
	for _, child := range n.children {
		walkXML(child, fn)
	}
}
