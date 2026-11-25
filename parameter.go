package fcpxml

import (
	"sort"
	"strings"
)

func (b *builder) addParameter(n, anchor *xmlNode) {
	kind, effectType := b.effectIdentity(anchor)
	name, key, parameterID := parameterIdentity(n)
	source := parameterSource(n, anchor)
	p := Parameter{
		EffectKind:   kind,
		EffectType:   effectType,
		Source:       source,
		Name:         name,
		Key:          key,
		ParameterID:  parameterID,
		Mapped:       true,
		Raw:          string(n.raw),
		CanonicalXML: canonicalXMLNode(n),
	}
	p.Value, p.Issues = mapParameterValue(n)
	p.Shape = p.Value.Kind
	p.NamespaceID = p.Namespace()
	if len(p.Issues) > 0 {
		p.Mapped = false
		for i := range p.Issues {
			p.Issues[i].Parameter = name
			p.Issues[i].Namespace = p.Namespace()
			b.issues = append(b.issues, p.Issues[i])
		}
	}
	if name == "" && key == "" && parameterID == "" {
		issue := MappingIssue{
			Code:      IssueMissingIdentity,
			Parameter: name,
			Namespace: p.Namespace(),
			Detail:    "parameter has no name, key, or parameter ID",
		}
		p.Mapped = false
		p.Issues = append(p.Issues, issue)
		b.issues = append(b.issues, issue)
	}
	b.parameters = append(b.parameters, p)
}

func (b *builder) effectIdentity(anchor *xmlNode) (string, string) {
	kind := anchor.local()
	ref := anchor.attr("ref")
	if ref != "" {
		if resource, ok := b.resources[ref]; ok {
			return kind, resourceType(resource)
		}
	}
	direct := firstNonEmpty(
		minNonEmpty([]string{anchor.attr("effectID", "effectId"), anchor.childText("effectid")}),
		minNonEmpty([]string{anchor.attr("effectType", "effecttype", "type"), anchor.childText("effecttype")}),
		minNonEmpty([]string{anchor.attr("uid"), anchor.childText("uid")}),
		minNonEmpty([]string{anchor.attr("name"), anchor.childText("name")}),
	)
	if direct == "" {
		direct = ref
	}
	if direct == "" {
		direct = kind
	}
	return kind, direct
}

func resourceType(r resourceEffect) string {
	if value := firstNonEmpty(r.EffectID, r.UID, r.EffectType, r.Name, r.ID); value != "" {
		return value
	}
	return "effect"
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func parameterIdentity(n *xmlNode) (name, key, parameterID string) {
	name = firstNonEmpty(n.attr("name"), n.childText("name"))
	key = firstNonEmpty(n.attr("key"), n.childText("key"))
	parameterID = firstNonEmpty(n.attr("parameterid", "parameterID"), n.childText("parameterid"))
	if n.local() == "data" {
		name = firstNonEmpty(name, n.attr("key"))
		key = firstNonEmpty(key, n.attr("key"))
	}
	return name, key, parameterID
}

func parameterSource(n, anchor *xmlNode) string {
	chain := make([]*xmlNode, 0, 6)
	for current := n; current != nil && current != anchor; current = current.parent {
		chain = append(chain, current)
	}
	segments := make([]string, 0, len(chain))
	for i := len(chain) - 1; i >= 0; i-- {
		current := chain[i]
		segment := current.local()
		if current.local() == "param" || current.local() == "parameter" {
			if name, _, _ := parameterIdentity(current); name != "" {
				segment += "[" + name + "]"
			}
		}
		segments = append(segments, segment)
	}
	if len(segments) == 0 {
		return n.local()
	}
	return strings.Join(segments, "/")
}

func mapParameterValue(n *xmlNode) (ParameterValue, []MappingIssue) {
	if n.local() == "data" {
		return rawValue(n), []MappingIssue{unsupportedIssue("binary effect data cannot be normalized")}
	}
	if n.local() == "parameter" {
		return mapLegacyParameter(n)
	}
	return mapModernParameter(n)
}

func mapModernParameter(n *xmlNode) (ParameterValue, []MappingIssue) {
	if value := n.attr("value"); value != "" {
		if len(n.children) > 0 || n.hasText {
			return rawValue(n), []MappingIssue{unsupportedIssue("parameter combines value attribute with child data")}
		}
		return ParameterValue{Kind: ValueScalar, Text: value}, nil
	}
	if len(n.children) == 0 {
		if n.hasText {
			return rawValue(n), []MappingIssue{unsupportedIssue("parameter has text outside a value element")}
		}
		return ParameterValue{Kind: ValueEmpty}, nil
	}
	valueNodes := n.directChildren("value")
	nested := n.directChildren("param")
	other := make([]*xmlNode, 0)
	for _, child := range n.children {
		switch child.local() {
		case "value", "param":
		default:
			other = append(other, child)
		}
	}
	if len(valueNodes) == 1 && len(nested) == 0 && len(other) == 0 {
		return mapValueNode(valueNodes[0])
	}
	if len(nested) > 0 && len(valueNodes) == 0 && len(other) == 0 {
		if n.hasText {
			return rawValue(n), []MappingIssue{unsupportedIssue("structured parameter has mixed text")}
		}
		return structuredParams(nested)
	}
	return rawValue(n), []MappingIssue{unsupportedIssue("parameter child elements use an unsupported shape")}
}

func mapLegacyParameter(n *xmlNode) (ParameterValue, []MappingIssue) {
	for _, child := range n.children {
		switch child.local() {
		case "name", "parameterid", "parameterspecifier", "valuemin", "valuemax":
		case "value":
		default:
			return rawValue(n), []MappingIssue{unsupportedIssue("legacy parameter contains animated or unsupported data")}
		}
	}
	valueNodes := n.directChildren("value")
	if len(valueNodes) == 1 {
		return mapValueNode(valueNodes[0])
	}
	if len(valueNodes) == 0 {
		if n.hasText {
			return rawValue(n), []MappingIssue{unsupportedIssue("legacy parameter has text outside a value element")}
		}
		return ParameterValue{Kind: ValueEmpty}, nil
	}
	return rawValue(n), []MappingIssue{unsupportedIssue("legacy parameter has multiple value elements")}
}

func mapValueNode(n *xmlNode) (ParameterValue, []MappingIssue) {
	for _, child := range n.children {
		if child.local() == "keyframe" || child.local() == "data" {
			return rawValue(n), []MappingIssue{unsupportedIssue("value contains animated or binary data")}
		}
	}
	for _, attr := range n.attrs {
		switch attr.Name.Local {
		case "key", "value":
		default:
			return rawValue(n), []MappingIssue{unsupportedIssue("value node has attributes outside the supported schema")}
		}
	}
	if len(n.children) == 0 {
		if n.hasText {
			return ParameterValue{Kind: ValueScalar, Text: strings.TrimSpace(n.text.String())}, nil
		}
		if value := n.attr("value"); value != "" {
			return ParameterValue{Kind: ValueScalar, Text: value}, nil
		}
		return ParameterValue{Kind: ValueEmpty}, nil
	}
	if n.hasText {
		return rawValue(n), []MappingIssue{unsupportedIssue("value has mixed text and child elements")}
	}
	fields := make([]ParameterField, 0, len(n.children))
	for _, child := range n.children {
		value, issues := mapValueNode(child)
		if len(issues) > 0 {
			return rawValue(n), issues
		}
		fields = append(fields, ParameterField{
			Name:  child.local(),
			Key:   child.attr("key"),
			Value: value,
		})
	}
	sortFields(fields)
	return ParameterValue{Kind: ValueStructured, Fields: fields}, nil
}

func structuredParams(params []*xmlNode) (ParameterValue, []MappingIssue) {
	fields := make([]ParameterField, 0, len(params))
	for _, param := range params {
		name, key, _ := parameterIdentity(param)
		value, issues := mapParameterValue(param)
		if len(issues) > 0 {
			return rawValue(param.parent), issues
		}
		fields = append(fields, ParameterField{Name: name, Key: key, Value: value})
	}
	sortFields(fields)
	return ParameterValue{Kind: ValueStructured, Fields: fields}, nil
}

func sortFields(fields []ParameterField) {
	sort.SliceStable(fields, func(i, j int) bool {
		if fields[i].Name != fields[j].Name {
			return fields[i].Name < fields[j].Name
		}
		if fields[i].Key != fields[j].Key {
			return fields[i].Key < fields[j].Key
		}
		return fieldValueString(fields[i].Value) < fieldValueString(fields[j].Value)
	})
}

func fieldValueString(v ParameterValue) string {
	var b strings.Builder
	appendValue(&b, v)
	return b.String()
}

func rawValue(n *xmlNode) ParameterValue {
	return ParameterValue{Kind: ValueRaw, Text: canonicalXMLNode(n)}
}

func unsupportedIssue(detail string) MappingIssue {
	return MappingIssue{Code: IssueUnsupportedValue, Detail: detail}
}

func cloneValue(v ParameterValue) ParameterValue {
	out := v
	out.Fields = append([]ParameterField(nil), v.Fields...)
	for i := range out.Fields {
		out.Fields[i].Value = cloneValue(out.Fields[i].Value)
	}
	return out
}
