package fcpxml

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
)

type Document struct {
	input      []byte
	root       *xmlNode
	version    string
	parameters []Parameter
	issues     []MappingIssue
}

type resourceEffect struct {
	ID         string
	EffectID   string
	EffectType string
	UID        string
	Name       string
}

func Parse(r io.Reader) (*Document, error) {
	input, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("read FCPXML: %w", err)
	}
	return ParseBytes(input)
}

func ParseFile(path string) (*Document, error) {
	input, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read FCPXML %q: %w", path, err)
	}
	return ParseBytes(input)
}

func ParseBytes(input []byte) (*Document, error) {
	root, err := parseXMLTree(input)
	if err != nil {
		return nil, err
	}
	if root.local() != "fcpxml" {
		return nil, fmt.Errorf("parse FCPXML: root element is <%s>, want <fcpxml>", root.local())
	}
	resources := buildResourceIndex(root)
	b := &builder{resources: resources}
	b.visit(root, nil)
	doc := &Document{
		input:      append([]byte(nil), input...),
		root:       root,
		version:    root.attr("version"),
		parameters: b.parameters,
		issues:     b.issues,
	}
	doc.finish()
	return doc, nil
}

func (d *Document) Version() string {
	return d.version
}

func (d *Document) Parameters() []Parameter {
	out := make([]Parameter, len(d.parameters))
	copy(out, d.parameters)
	for i := range out {
		out[i].Issues = append([]MappingIssue(nil), out[i].Issues...)
		out[i].Value = cloneValue(out[i].Value)
	}
	return out
}

func (d *Document) Issues() []MappingIssue {
	out := make([]MappingIssue, len(d.issues))
	copy(out, d.issues)
	return out
}

func (d *Document) Snapshot() Snapshot {
	parameters := d.Parameters()
	for i := range parameters {
		parameters[i].Raw = ""
	}
	sortParameters(parameters)
	issues := make([]MappingIssue, len(d.issues))
	copy(issues, d.issues)
	sortIssues(issues)
	s := Snapshot{
		Version:    d.version,
		Parameters: parameters,
		Lossless:   len(issues) == 0,
		Issues:     issues,
	}
	s.Digest = digestSnapshot(s)
	return s
}

func (d *Document) CanonicalJSON() ([]byte, error) {
	return json.MarshalIndent(d.Snapshot(), "", "  ")
}

func (d *Document) finish() {
	sortParameters(d.parameters)
	issues := make([]MappingIssue, 0, len(d.issues))
	for _, issue := range d.issues {
		issues = append(issues, issue)
	}
	sortIssues(issues)
	d.issues = issues
}

func sortIssues(issues []MappingIssue) {
	sort.SliceStable(issues, func(i, j int) bool {
		if issues[i].Code != issues[j].Code {
			return issues[i].Code < issues[j].Code
		}
		if issues[i].Namespace != issues[j].Namespace {
			return issues[i].Namespace < issues[j].Namespace
		}
		if issues[i].Parameter != issues[j].Parameter {
			return issues[i].Parameter < issues[j].Parameter
		}
		return issues[i].Detail < issues[j].Detail
	})
}

type builder struct {
	resources  map[string]resourceEffect
	parameters []Parameter
	issues     []MappingIssue
}

func buildResourceIndex(root *xmlNode) map[string]resourceEffect {
	index := make(map[string]resourceEffect)
	walkXML(root, func(n *xmlNode) {
		if n.local() != "effect" {
			return
		}
		id := n.attr("id")
		if id == "" {
			return
		}
		candidate := resourceFromNode(n)
		if old, ok := index[id]; !ok || resourceLess(candidate, old) {
			index[id] = candidate
		}
	})
	return index
}

func resourceFromNode(n *xmlNode) resourceEffect {
	return resourceEffect{
		ID:         n.attr("id"),
		EffectID:   minNonEmpty([]string{n.attr("effectID", "effectId"), n.childText("effectid")}),
		EffectType: minNonEmpty([]string{n.attr("effectType", "effecttype", "type"), n.childText("effecttype")}),
		UID:        minNonEmpty([]string{n.attr("uid"), n.childText("uid")}),
		Name:       minNonEmpty([]string{n.attr("name"), n.childText("name")}),
	}
}

func resourceLess(a, b resourceEffect) bool {
	if scoreA, scoreB := resourceScore(a), resourceScore(b); scoreA != scoreB {
		return scoreA > scoreB
	}
	return resourceIdentity(a) < resourceIdentity(b)
}

func resourceScore(r resourceEffect) int {
	score := 0
	for _, value := range []string{r.EffectID, r.EffectType, r.UID, r.Name, r.ID} {
		if value != "" {
			score++
		}
	}
	return score
}

func resourceIdentity(r resourceEffect) string {
	return strings.Join([]string{r.EffectID, r.UID, r.EffectType, r.Name, r.ID}, "\x1f")
}

func (b *builder) visit(n *xmlNode, anchor *xmlNode) {
	if isEffectAnchor(n) {
		anchor = n
	}
	if isParameterNode(n, anchor) {
		b.addParameter(n, anchor)
	}
	for _, child := range n.children {
		b.visit(child, anchor)
	}
}

func isEffectAnchor(n *xmlNode) bool {
	name := n.local()
	switch name {
	case "filter-video", "filter-audio", "filter-video-mask", "filter-audio-mask",
		"effect", "title", "video", "clip", "clipitem", "transition", "transitionitem",
		"generatoritem", "generator", "motion", "mask-isolation":
		return true
	}
	return strings.HasPrefix(name, "filter-") || strings.HasPrefix(name, "adjust-")
}

func isParameterNode(n, anchor *xmlNode) bool {
	if anchor == nil {
		return false
	}
	switch n.local() {
	case "param", "parameter":
		return true
	case "data":
		if n.attr("key") != "effectData" && n.attr("key") != "effectConfig" && n.attr("key") != "effectState" {
			return false
		}
		for current := n.parent; current != nil && current != anchor; current = current.parent {
			if current.local() == "param" || current.local() == "parameter" {
				return false
			}
		}
		return true
	default:
		return false
	}
}
