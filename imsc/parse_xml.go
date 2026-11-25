package imsc

import (
	"encoding/xml"
	"io"
)

const xmlnsSpace = "http://www.w3.org/2000/xmlns/"

// parseXML reads an IMSC/TTML document into a generic node tree. A syntax
// failure is returned as an ElementError attributed to the nearest element.
func parseXML(r io.Reader) (*node, error) {
	src := newLineReader(r)
	dec := xml.NewDecoder(src)
	dec.Strict = true

	var root *node
	var stack []*node
	var nsStack []map[string]string
	siblingCount := map[*node]map[string]int{}

	for {
		token, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			element := "document"
			if len(stack) > 0 {
				top := stack[len(stack)-1]
				if top.path == "" {
					top.path = top.buildPath()
				}
				element = top.path
			}
			return nil, NewElementError(element, src.lineAt(dec.InputOffset()),
				ReasonMalformedXML, err.Error())
		}

		switch t := token.(type) {
		case xml.StartElement:
			nsMap := map[string]string{}
			for _, a := range t.Attr {
				if a.Name.Space == xmlnsSpace {
					nsMap[a.Name.Local] = a.Value
				}
			}
			nsStack = append(nsStack, nsMap)

			parent := (*node)(nil)
			if len(stack) > 0 {
				parent = stack[len(stack)-1]
			}
			local := t.Name.Local
			idx := 0
			if parent != nil {
				if siblingCount[parent] == nil {
					siblingCount[parent] = map[string]int{}
				}
				siblingCount[parent][local]++
				idx = siblingCount[parent][local] - 1
			}
			cur := &node{
				Local:  local,
				Prefix: resolvePrefix(nsStack, t.Name.Space),
				Line:   src.lineAt(dec.InputOffset()),
				Attrs:  map[string]xmlAttr{},
				parent: parent,
				index:  idx,
			}
			for _, a := range t.Attr {
				if a.Name.Space == xmlnsSpace {
					continue
				}
				cur.Attrs[a.Name.Local] = xmlAttr{
					Prefix: resolvePrefix(nsStack, a.Name.Space),
					Value:  a.Value,
				}
			}
			if parent == nil {
				root = cur
			} else {
				parent.Children = append(parent.Children, cur)
			}
			cur.path = cur.buildPath()
			stack = append(stack, cur)

		case xml.CharData:
			if len(stack) > 0 {
				stack[len(stack)-1].Text += string(t)
			}

		case xml.EndElement:
			if len(stack) > 0 {
				cur := stack[len(stack)-1]
				cur.Text = trimSpace(cur.Text)
				cur.path = cur.buildPath()
				stack = stack[:len(stack)-1]
				nsStack = nsStack[:len(nsStack)-1]
			}
		}
	}

	if root == nil {
		return nil, NewElementError("document", 0, ReasonMalformedXML, "no root element found")
	}
	return root, nil
}

// resolvePrefix finds the prefix bound to uri by scanning namespace
// declarations from the innermost element outward. The empty prefix denotes
// the default namespace.
func resolvePrefix(stack []map[string]string, uri string) string {
	for i := len(stack) - 1; i >= 0; i-- {
		for prefix, ns := range stack[i] {
			if ns == uri {
				return prefix
			}
		}
	}
	return ""
}
