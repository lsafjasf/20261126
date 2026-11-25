package main

import (
	"encoding/json"
	"fmt"
)

// encodeValue converts a Value back to its wire form.
func encodeValue(v Value) (any, error) {
	switch v.Kind {
	case KindScalar:
		return v.Scalar, nil
	case KindRef:
		return map[string]any{"ref": v.Ref}, nil
	case KindWeakRef:
		return map[string]any{
			"weakref":     v.WeakRef.Target,
			"targetClass": v.WeakRef.TargetClass,
			"path":        v.WeakRef.Path,
		}, nil
	case KindOpaque:
		return map[string]any{"type": v.Opaque.Type, "opaque": v.Opaque.Data}, nil
	case KindList:
		out := make([]any, 0, len(v.List))
		for _, item := range v.List {
			enc, err := encodeValue(item)
			if err != nil {
				return nil, err
			}
			out = append(out, enc)
		}
		return out, nil
	}
	return nil, fmt.Errorf("unknown value kind %d", v.Kind)
}

// EmitMinimal produces a self-contained, re-importable project containing
// only reachable objects, expressible properties and repaired references.
// It must only be called when Diagnose reported no fatals.
func EmitMinimal(p *Project, g *Graph) (*Project, error) {
	out := &Project{byID: map[string]*Object{}}
	for _, obj := range p.Objects {
		if !g.Reachable[obj.ID] {
			continue
		}
		rule := minimalSchema[obj.Class] // reachability implies known class here
		clone := &Object{ID: obj.ID, Class: obj.Class, Properties: map[string]Value{}}
		for _, name := range obj.sortedPropNames() {
			pr, ok := rule.props[name]
			if !ok {
				continue // inexpressible property: dropped
			}
			v := obj.Properties[name]
			if v.Kind == KindOpaque {
				if pr.required {
					return nil, fmt.Errorf("object %s: required property %s is opaque",
						obj.ID, name)
				}
				continue
			}
			repaired, drop, err := repairValue(p, g, obj, name, v)
			if err != nil {
				return nil, err
			}
			if drop {
				continue
			}
			clone.Properties[name] = repaired
		}
		out.byID[clone.ID] = clone
		out.Objects = append(out.Objects, clone)
	}
	return out, nil
}

// repairValue rewrites references whose targets are missing. The returned
// drop flag removes the whole property (used for metadata attachments).
func repairValue(p *Project, g *Graph, obj *Object, prop string, v Value) (Value, bool, error) {
	switch v.Kind {
	case KindRef, KindWeakRef:
		target, weak := v.Ref, false
		if v.Kind == KindWeakRef {
			target, weak = v.WeakRef.Target, true
		}
		if p.lookup(target) != nil && g.Reachable[target] {
			return v, false, nil
		}
		switch prop {
		case "DescriptiveMetadata", "EssenceDescriptor":
			return Value{}, true, nil // detach: property removed entirely
		case "SourceID":
			// Handled at the object level in substituteFiller; a bare
			// SourceID without context cannot be repaired here.
			return v, false, nil
		default:
			if weak {
				return Value{}, true, nil // dangling weak refs are safe to drop
			}
			return Value{}, false, fmt.Errorf(
				"object %s property %s: unrepairable reference to %s",
				obj.ID, prop, target)
		}
	case KindList:
		out := make([]Value, 0, len(v.List))
		for _, item := range v.List {
			repaired, drop, err := repairValue(p, g, obj, prop, item)
			if err != nil {
				return Value{}, false, err
			}
			if drop {
				continue
			}
			out = append(out, repaired)
		}
		return Value{Kind: KindList, List: out}, false, nil
	default:
		return v, false, nil
	}
}

// SubstituteFillers replaces SourceClips whose SourceID dangles with Filler
// objects of equal length, preserving timeline duration. It returns the
// substituted object IDs for reporting.
func SubstituteFillers(p *Project, g *Graph) []string {
	var substituted []string
	for _, obj := range p.Objects {
		if obj.Class != "SourceClip" || !g.Reachable[obj.ID] {
			continue
		}
		src, ok := obj.Properties["SourceID"]
		if !ok || src.Kind != KindRef {
			continue
		}
		if p.lookup(src.Ref) != nil {
			continue
		}
		obj.Class = "Filler"
		delete(obj.Properties, "SourceID")
		delete(obj.Properties, "SourceMobSlot")
		delete(obj.Properties, "StartTime")
		substituted = append(substituted, obj.ID)
	}
	return substituted
}

// MarshalProject serializes a project back to the dump wire format.
func MarshalProject(p *Project) ([]byte, error) {
	wire := projectJSON{}
	for _, obj := range p.Objects {
		wo := objectJSON{
			ID:         obj.ID,
			Class:      obj.Class,
			Properties: map[string]json.RawMessage{},
		}
		for _, name := range obj.sortedPropNames() {
			enc, err := encodeValue(obj.Properties[name])
			if err != nil {
				return nil, fmt.Errorf("object %s property %s: %w", obj.ID, name, err)
			}
			raw, err := json.Marshal(enc)
			if err != nil {
				return nil, err
			}
			wo.Properties[name] = raw
		}
		wire.Objects = append(wire.Objects, wo)
	}
	return json.MarshalIndent(wire, "", "  ")
}
