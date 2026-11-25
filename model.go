package main

import (
	"encoding/json"
	"fmt"
	"sort"
)

// Value is a decoded AAF property value. Exactly one field is set according
// to Kind; scalar values are kept in Scalar.
type Value struct {
	Kind    ValueKind
	Scalar  any      // string, float64, bool
	Ref     string   // strong reference target object ID
	WeakRef *WeakRef // weak reference resolved through the dictionary
	List    []Value  // ordered list of values
	Opaque  *Opaque  // raw data the minimal model cannot express
}

type ValueKind int

const (
	KindScalar ValueKind = iota
	KindRef
	KindWeakRef
	KindList
	KindOpaque
)

type WeakRef struct {
	Target      string `json:"target"`
	TargetClass string `json:"targetClass"`
	Path        string `json:"path,omitempty"`
}

type Opaque struct {
	Type string `json:"type"`
	Data string `json:"data"` // base64 payload, never interpreted
}

// Object is one AAF object (a "node" in the reference graph).
type Object struct {
	ID         string           `json:"id"`
	Class      string           `json:"class"`
	Properties map[string]Value `json:"-"`
}

// Project is the decoded dump of an AAF file.
type Project struct {
	Objects []*Object
	byID    map[string]*Object
}

func (p *Project) lookup(id string) *Object { return p.byID[id] }

// objectJSON mirrors Object for wire decoding; properties go through
// decodeValue so references stay typed instead of becoming map[string]any.
type objectJSON struct {
	ID         string                     `json:"id"`
	Class      string                     `json:"class"`
	Properties map[string]json.RawMessage `json:"properties"`
}

type projectJSON struct {
	Objects []objectJSON `json:"objects"`
}

func decodeValue(raw json.RawMessage) (Value, error) {
	var probe any
	if err := json.Unmarshal(raw, &probe); err != nil {
		return Value{}, err
	}
	switch t := probe.(type) {
	case map[string]any:
		if _, ok := t["ref"]; ok {
			var r struct {
				Ref string `json:"ref"`
			}
			if err := json.Unmarshal(raw, &r); err != nil {
				return Value{}, err
			}
			return Value{Kind: KindRef, Ref: r.Ref}, nil
		}
		if _, ok := t["weakref"]; ok {
			var w struct {
				WeakRef     string `json:"weakref"`
				TargetClass string `json:"targetClass"`
				Path        string `json:"path"`
			}
			if err := json.Unmarshal(raw, &w); err != nil {
				return Value{}, err
			}
			return Value{Kind: KindWeakRef, WeakRef: &WeakRef{
				Target: w.WeakRef, TargetClass: w.TargetClass, Path: w.Path,
			}}, nil
		}
		if _, ok := t["opaque"]; ok {
			var o Opaque
			if err := json.Unmarshal(raw, &o); err != nil {
				return Value{}, err
			}
			return Value{Kind: KindOpaque, Opaque: &o}, nil
		}
		return Value{}, fmt.Errorf("unknown object value shape: %s", string(raw))
	case []any:
		var raws []json.RawMessage
		if err := json.Unmarshal(raw, &raws); err != nil {
			return Value{}, err
		}
		list := make([]Value, 0, len(raws))
		for _, item := range raws {
			v, err := decodeValue(item)
			if err != nil {
				return Value{}, err
			}
			list = append(list, v)
		}
		return Value{Kind: KindList, List: list}, nil
	default:
		return Value{Kind: KindScalar, Scalar: t}, nil
	}
}

// LoadProject parses a dump and indexes objects by ID. Duplicate IDs are
// rejected up front because they make the reference graph ambiguous.
func LoadProject(data []byte) (*Project, error) {
	var wire projectJSON
	if err := json.Unmarshal(data, &wire); err != nil {
		return nil, fmt.Errorf("parse project: %w", err)
	}
	p := &Project{byID: map[string]*Object{}}
	for i, wo := range wire.Objects {
		if wo.ID == "" {
			return nil, fmt.Errorf("objects[%d]: missing id", i)
		}
		if wo.Class == "" {
			return nil, fmt.Errorf("objects[%d] (%s): missing class", i, wo.ID)
		}
		if _, dup := p.byID[wo.ID]; dup {
			return nil, fmt.Errorf("duplicate object id %q", wo.ID)
		}
		obj := &Object{ID: wo.ID, Class: wo.Class, Properties: map[string]Value{}}
		for name, raw := range wo.Properties {
			v, err := decodeValue(raw)
			if err != nil {
				return nil, fmt.Errorf("object %s property %s: %w", wo.ID, name, err)
			}
			obj.Properties[name] = v
		}
		p.byID[obj.ID] = obj
		p.Objects = append(p.Objects, obj)
	}
	return p, nil
}

// sortedPropNames gives deterministic output ordering.
func (o *Object) sortedPropNames() []string {
	names := make([]string, 0, len(o.Properties))
	for n := range o.Properties {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}
