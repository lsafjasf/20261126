package ttmlevents

import (
	"encoding/xml"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// Parse reads a TTML document from r using only the Go standard library.
func Parse(r io.Reader) (*Document, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("ttmlevents: read TTML: %w", err)
	}
	return ParseString(string(data))
}

// ParseString parses TTML source text.
func ParseString(source string) (*Document, error) {
	source = strings.TrimPrefix(source, "\ufeff")
	decoder := xml.NewDecoder(strings.NewReader(source))
	decoder.Strict = true
	decoder.Entity = xml.HTMLEntity

	doc := &Document{Styles: make(map[string]StyleRule)}
	var stack []*Node

	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("ttmlevents: parse XML: %w", err)
		}

		switch tok := token.(type) {
		case xml.StartElement:
			node := doc.newElement(tok)
			if len(stack) == 0 {
				if doc.Root != nil {
					return nil, fmt.Errorf("ttmlevents: multiple root elements")
				}
				doc.Root = node
				node.Path = "/" + node.Local + "[1]"
			} else {
				parent := stack[len(stack)-1]
				node.Parent = parent
				node.Path = childPath(parent, node)
				parent.Children = append(parent.Children, node)
			}
			stack = append(stack, node)

		case xml.CharData:
			if len(stack) == 0 {
				continue
			}
			parent := stack[len(stack)-1]
			text := string(tok)
			if text == "" {
				continue
			}
			node := doc.newText(text)
			node.Parent = parent
			node.Path = childPath(parent, node)
			parent.Children = append(parent.Children, node)

		case xml.EndElement:
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		}
	}

	if doc.Root == nil {
		return nil, fmt.Errorf("ttmlevents: empty TTML document")
	}

	postOrder := 0
	assignPostOrder(doc.Root, &postOrder)
	doc.parseDocumentParameters()
	doc.parseStyleRules()
	return doc, nil
}

func assignPostOrder(n *Node, order *int) {
	if n == nil {
		return
	}
	for _, child := range n.Children {
		assignPostOrder(child, order)
	}
	if n.Kind == ElementNode {
		*order++
		n.PostOrder = *order
	}
}

func (d *Document) nextOrder() int {
	d.orderCounter++
	return d.orderCounter
}

func (d *Document) newElement(start xml.StartElement) *Node {
	node := &Node{
		Kind:      ElementNode,
		Local:     start.Name.Local,
		Qualified: qualifiedName(start.Name),
		DocOrder:  d.nextOrder(),
	}
	for _, attr := range start.Attr {
		a := Attr{
			Namespace: attr.Name.Space,
			Name:      attr.Name.Local,
			Value:     attr.Value,
		}
		if a.Name == "" && strings.Contains(attr.Name.Space, ":") {
			a.Name = attr.Name.Space
		}
		node.Attrs = append(node.Attrs, a)
		if isXMLID(attr.Name) {
			node.ID = attr.Value
		}
	}
	if node.ID == "" {
		if value, ok := node.AttrValue("id"); ok {
			node.ID = value
		}
	}
	return node
}

func (d *Document) newText(text string) *Node {
	return &Node{Kind: TextNode, Text: text, DocOrder: d.nextOrder()}
}

func qualifiedName(name xml.Name) string {
	if name.Space == "" {
		return name.Local
	}
	if strings.HasPrefix(name.Space, "http://www.w3.org/") || strings.Contains(name.Space, "/") {
		return name.Local
	}
	return name.Space + ":" + name.Local
}

func isXMLID(name xml.Name) bool {
	return name.Local == "id" && (name.Space == "http://www.w3.org/XML/1998/namespace" || name.Space == "xml")
}

func childPath(parent, child *Node) string {
	index := 0
	for _, existing := range parent.Children {
		if child.Kind == TextNode {
			if existing.Kind == TextNode {
				index++
			}
		} else if existing.Kind == ElementNode && existing.Local == child.Local {
			index++
		}
	}
	if child.Kind == TextNode {
		return parent.Path + "/text()[" + strconv.Itoa(index+1) + "]"
	}
	return fmt.Sprintf("%s/%s[%d]", parent.Path, child.Local, index+1)
}

func (d *Document) parseDocumentParameters() {
	if d.Root == nil {
		return
	}
	if raw, ok := d.Root.AttrValue("frameRate"); ok {
		if value, err := strconv.ParseFloat(strings.TrimSpace(raw), 64); err == nil && value > 0 {
			d.FrameRate = value
		} else {
			d.Warnings = append(d.Warnings, Warning{Code: "bad-frame-rate", Path: d.Root.Path, Message: raw})
		}
	}
	if raw, ok := d.Root.AttrValue("frameRateMultiplier"); ok {
		if rate, ok := parseFrameRateMultiplier(raw); ok && d.FrameRate > 0 {
			d.FrameRate *= rate
		}
	}
	if raw, ok := d.Root.AttrValue("tickRate"); ok {
		if value, err := strconv.ParseFloat(strings.TrimSpace(raw), 64); err == nil && value > 0 {
			d.TickRate = value
		}
	}
	if d.FrameRate == 0 {
		d.FrameRate = 30
		d.DefaultFrame = d.FrameRate
	}
}

func parseFrameRateMultiplier(raw string) (float64, bool) {
	raw = strings.TrimSpace(raw)
	parts := strings.Split(raw, " ")
	if len(parts) == 1 {
		parts = strings.Split(raw, "/")
	}
	if len(parts) != 2 {
		return 0, false
	}
	n, err1 := strconv.ParseFloat(parts[0], 64)
	q, err2 := strconv.ParseFloat(parts[1], 64)
	if err1 != nil || err2 != nil || q == 0 {
		return 0, false
	}
	return n / q, true
}

func (d *Document) parseStyleRules() {
	if d.Root == nil {
		return
	}
	walkNodes(d.Root, func(n *Node) bool {
		if !n.Is("style") || hasAncestorNamed(n, "animate") {
			return true
		}
		id := n.ID
		if id == "" {
			return true
		}
		rule := StyleRule{ID: id, DocOrder: n.DocOrder}
		if refs, ok := n.AttrValue("style"); ok {
			rule.ParentRefs = splitIDRefs(refs)
		}
		for _, attr := range n.Attrs {
			if isStructuralStyleAttr(attr.Name) {
				continue
			}
			rule.Properties = append(rule.Properties, Property{
				Name:      canonicalPropertyName(attr.Name),
				Value:     strings.TrimSpace(attr.Value),
				Source:    "style#" + id,
				Inherited: isInheritableProperty(attr.Name),
			})
		}
		if _, exists := d.Styles[id]; exists {
			d.Warnings = append(d.Warnings, Warning{Code: "duplicate-style-id", Path: n.Path, Message: id})
		}
		d.Styles[id] = rule
		return true
	})
}

func isStructuralStyleAttr(name string) bool {
	switch strings.ToLower(name) {
	case "id", "style", "xml:id":
		return true
	default:
		return false
	}
}

func splitIDRefs(value string) []string {
	fields := strings.Fields(value)
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		field = strings.TrimPrefix(field, "#")
		if field != "" {
			out = append(out, field)
		}
	}
	return out
}

func walkNodes(n *Node, fn func(*Node) bool) {
	if n == nil || !fn(n) {
		return
	}
	for _, child := range n.Children {
		walkNodes(child, fn)
	}
}

func hasAncestorNamed(n *Node, names ...string) bool {
	for p := n.Parent; p != nil; p = p.Parent {
		for _, name := range names {
			if p.Is(name) {
				return true
			}
		}
	}
	return false
}
