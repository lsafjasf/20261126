package ttmlevents

import (
	"encoding/xml"
	"fmt"
	"strings"
)

// NodeKind identifies the small XML node subset used by TTML.
type NodeKind int

const (
	ElementNode NodeKind = iota + 1
	TextNode
)

// Attr is a namespace-preserving XML attribute.
type Attr struct {
	Namespace string `json:"namespace,omitempty"`
	Name      string `json:"name"`
	Value     string `json:"value"`
}

// Node is a positional XML node. Parent pointers are intentionally omitted
// from JSON; Path and DocOrder provide stable cross-references.
type Node struct {
	Kind      NodeKind `json:"kind"`
	Local     string   `json:"local,omitempty"`
	Qualified string   `json:"qualified,omitempty"`
	ID        string   `json:"id,omitempty"`
	Attrs     []Attr   `json:"attrs,omitempty"`
	Text      string   `json:"text,omitempty"`
	DocOrder  int      `json:"docOrder"`
	PostOrder int      `json:"postOrder,omitempty"`
	Path      string   `json:"path"`
	Parent    *Node    `json:"-"`
	Children  []*Node  `json:"children,omitempty"`
}

// Is reports whether the node is an element with one of the local names.
func (n *Node) Is(names ...string) bool {
	if n == nil || n.Kind != ElementNode {
		return false
	}
	for _, name := range names {
		if equalFoldName(n.Local, name) {
			return true
		}
	}
	return false
}

// AttrValue returns the first attribute matching a local name. TTML permits
// both begin and tts:begin-style spellings in different profiles.
func (n *Node) AttrValue(local string) (string, bool) {
	if n == nil {
		return "", false
	}
	for _, attr := range n.Attrs {
		if equalFoldName(attr.Name, local) {
			return attr.Value, true
		}
	}
	return "", false
}

// NamespacedAttrValue matches a property by local name and, when supplied,
// its conventional prefix/namespace suffix.
func (n *Node) NamespacedAttrValue(nsSuffix, local string) (string, bool) {
	if n == nil {
		return "", false
	}
	for _, attr := range n.Attrs {
		if !equalFoldName(attr.Name, local) {
			continue
		}
		if nsSuffix == "" || strings.HasSuffix(attr.Namespace, nsSuffix) ||
			strings.EqualFold(attr.Namespace, nsSuffix) {
			return attr.Value, true
		}
	}
	return n.AttrValue(local)
}

// Property is one deterministic style declaration.
type Property struct {
	Name             string `json:"name"`
	Value            string `json:"value"`
	Inherited        bool   `json:"inherited,omitempty"`
	Source           string `json:"source,omitempty"`
	DeclarationDepth int    `json:"-"`
}

// StyleRule is a parsed TTML head/style definition.
type StyleRule struct {
	ID         string     `json:"id"`
	ParentRefs []string   `json:"parentRefs,omitempty"`
	Properties []Property `json:"properties,omitempty"`
	DocOrder   int        `json:"docOrder"`
}

// Warning describes recoverable input that was interpreted conservatively.
type Warning struct {
	Code    string `json:"code"`
	Path    string `json:"path,omitempty"`
	Message string `json:"message"`
}

func (w Warning) Error() string {
	if w.Path == "" {
		return fmt.Sprintf("%s: %s", w.Code, w.Message)
	}
	return fmt.Sprintf("%s at %s: %s", w.Code, w.Path, w.Message)
}

// Document is the parsed TTML source before timeline expansion.
type Document struct {
	XMLName      xml.Name             `json:"-"`
	Root         *Node                `json:"root"`
	Styles       map[string]StyleRule `json:"styles,omitempty"`
	FrameRate    float64              `json:"frameRate,omitempty"`
	TickRate     float64              `json:"tickRate,omitempty"`
	DefaultFrame float64              `json:"defaultFrame,omitempty"`
	Warnings     []Warning            `json:"warnings,omitempty"`
	orderCounter int
}

func equalFoldName(a, b string) bool {
	a = strings.TrimSpace(a)
	b = strings.TrimSpace(b)
	if strings.EqualFold(a, b) {
		return true
	}
	if i := strings.LastIndex(a, ":"); i >= 0 {
		a = a[i+1:]
	}
	if i := strings.LastIndex(b, ":"); i >= 0 {
		b = b[i+1:]
	}
	return strings.EqualFold(a, b)
}
