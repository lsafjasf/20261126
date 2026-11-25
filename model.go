package fcpxml

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
)

var ErrLossySave = errors.New("fcpxml: lossy save blocked")

type ValueKind string

const (
	ValueEmpty      ValueKind = "empty"
	ValueScalar     ValueKind = "scalar"
	ValueStructured ValueKind = "structured"
	ValueRaw        ValueKind = "raw"
)

// ParameterValue is the normalized, lossless representation used for a
// mapped parameter. Structured values keep their field order in Fields.
type ParameterValue struct {
	Kind   ValueKind        `json:"kind"`
	Text   string           `json:"text,omitempty"`
	Fields []ParameterField `json:"fields,omitempty"`
}

type ParameterField struct {
	Name  string         `json:"name"`
	Key   string         `json:"key,omitempty"`
	Value ParameterValue `json:"value"`
}

type IssueCode string

const (
	IssueUnsupportedValue IssueCode = "unsupported_value"
	IssueMissingIdentity  IssueCode = "missing_parameter_identity"
)

type MappingIssue struct {
	Code      IssueCode `json:"code"`
	Parameter string    `json:"parameter"`
	Namespace string    `json:"namespace"`
	Detail    string    `json:"detail"`
}

type Parameter struct {
	EffectKind  string    `json:"effect_kind"`
	EffectType  string    `json:"effect_type"`
	Source      string    `json:"source"`
	NamespaceID string    `json:"namespace"`
	Shape       ValueKind `json:"shape"`
	Name        string    `json:"name"`
	Key         string    `json:"key,omitempty"`
	ParameterID string    `json:"parameter_id,omitempty"`

	Value  ParameterValue `json:"value"`
	Mapped bool           `json:"mapped"`

	// Raw is the exact XML subtree of the parameter as imported. It is kept
	// even when Value cannot be mapped so the original data is never lost.
	Raw string `json:"-"`

	// CanonicalXML is a stable semantic form used only for sorting and identity
	// comparisons; Raw remains the exact imported representation.
	CanonicalXML string `json:"-"`

	Issues []MappingIssue `json:"issues,omitempty"`
}

func (p Parameter) Namespace() string {
	return NamespaceKey(p.EffectKind, p.EffectType, p.Source, p.Name,
		string(p.Shape), p.Key, p.ParameterID)
}

// NamespaceKey is separator-safe and unambiguous even when any component
// contains a colon, slash, or NUL. Lengths are included before each value.
func NamespaceKey(effectKind, effectType, source, name string, discriminators ...string) string {
	parts := []string{effectKind, effectType, source, name}
	parts = append(parts, discriminators...)
	var b strings.Builder
	for _, part := range parts {
		b.WriteString(lengthPart(part))
	}
	return b.String()
}

func lengthPart(s string) string {
	return fmt.Sprintf("%d:%s", len(s), s)
}

type Snapshot struct {
	Version    string         `json:"version"`
	Parameters []Parameter    `json:"parameters"`
	Lossless   bool           `json:"lossless"`
	Issues     []MappingIssue `json:"issues"`
	Digest     string         `json:"digest"`
}

func (p Parameter) canonicalValue() string {
	var b strings.Builder
	appendValue(&b, p.Value)
	return b.String()
}

func appendValue(b *strings.Builder, v ParameterValue) {
	b.WriteString(string(v.Kind))
	b.WriteByte('\x1f')
	b.WriteString(v.Text)
	b.WriteByte('\x1e')
	for _, f := range v.Fields {
		b.WriteString(f.Name)
		b.WriteByte('\x1d')
		b.WriteString(f.Key)
		b.WriteByte('\x1d')
		appendValue(b, f.Value)
	}
}

func canonicalParameterLess(a, b Parameter) bool {
	if a.Namespace() != b.Namespace() {
		return a.Namespace() < b.Namespace()
	}
	av, bv := a.canonicalValue(), b.canonicalValue()
	if av != bv {
		return av < bv
	}
	if a.CanonicalXML != b.CanonicalXML {
		return a.CanonicalXML < b.CanonicalXML
	}
	if a.Raw != b.Raw {
		return a.Raw < b.Raw
	}
	return a.Key < b.Key
}

func sortParameters(ps []Parameter) {
	sort.SliceStable(ps, func(i, j int) bool {
		return canonicalParameterLess(ps[i], ps[j])
	})
}

func digestSnapshot(s Snapshot) string {
	h := sha256.New()
	h.Write([]byte(s.Version))
	for _, p := range s.Parameters {
		h.Write([]byte(p.Namespace()))
		h.Write([]byte{0})
		h.Write([]byte(p.canonicalValue()))
		h.Write([]byte{0})
		h.Write([]byte(p.CanonicalXML))
		h.Write([]byte{0})
	}
	for _, issue := range s.Issues {
		h.Write([]byte(issue.Code))
		h.Write([]byte{0})
		h.Write([]byte(issue.Namespace))
		h.Write([]byte{0})
		h.Write([]byte(issue.Detail))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

type LossySaveError struct {
	Issues []MappingIssue
}

func (e *LossySaveError) Error() string {
	if len(e.Issues) == 0 {
		return ErrLossySave.Error()
	}
	names := make([]string, 0, len(e.Issues))
	for _, issue := range e.Issues {
		name := issue.Parameter
		if name == "" {
			name = issue.Namespace
		}
		names = append(names, fmt.Sprintf("%s (%s)", name, issue.Code))
	}
	return fmt.Sprintf("%s: %s", ErrLossySave, strings.Join(names, ", "))
}

func (e *LossySaveError) Is(target error) bool {
	return target == ErrLossySave
}
