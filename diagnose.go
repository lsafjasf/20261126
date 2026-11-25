package main

import (
	"fmt"
	"sort"
)

const (
	SevFatal   = "fatal"
	SevWarning = "warning"
	SevInfo    = "info"
)

// Diag is one diagnostic record about the project.
type Diag struct {
	Severity string `json:"severity"`
	Code     string `json:"code"`
	ObjectID string `json:"object,omitempty"`
	Property string `json:"property,omitempty"`
	Message  string `json:"message"`
}

// Report is the full diagnostic output for a project.
type Report struct {
	Input    string `json:"input"`
	OK       bool   `json:"ok"`
	Fatals   int    `json:"fatals"`
	Warnings int    `json:"warnings"`
	Diags    []Diag `json:"diagnostics"`
}

func (r *Report) add(d Diag) {
	r.Diags = append(r.Diags, d)
	switch d.Severity {
	case SevFatal:
		r.Fatals++
	case SevWarning:
		r.Warnings++
	}
}

func (r *Report) sortDiags() {
	sort.SliceStable(r.Diags, func(i, j int) bool {
		if r.Diags[i].Severity != r.Diags[j].Severity {
			return sevRank(r.Diags[i].Severity) < sevRank(r.Diags[j].Severity)
		}
		if r.Diags[i].ObjectID != r.Diags[j].ObjectID {
			return r.Diags[i].ObjectID < r.Diags[j].ObjectID
		}
		return r.Diags[i].Property < r.Diags[j].Property
	})
}

func sevRank(s string) int {
	switch s {
	case SevFatal:
		return 0
	case SevWarning:
		return 1
	default:
		return 2
	}
}

// missingEdgePolicy classifies a dangling reference from a reachable object.
// It returns the diagnostic code, severity and a human-readable explanation.
func missingEdgePolicy(e Edge) (code, sev, why string) {
	switch {
	case e.FromCls == "OperationGroup" && e.Prop == "Operation":
		return "MISSING_EFFECT", SevFatal,
			"operation group references an effect definition that is absent; " +
				"treating it as no-effect would silently change the timeline"
	case e.FromCls == "OperationGroup" && e.Prop == "InputSegments":
		return "MISSING_EFFECT_INPUT", SevFatal,
			"effect input segment is absent; the effect cannot be evaluated"
	case e.FromCls == "Parameter" && e.Prop == "ParameterDefinition":
		return "MISSING_EFFECT_PARAM", SevFatal,
			"effect parameter references an absent parameter definition"
	case e.Prop == "SourceID":
		return "MISSING_SOURCE", SevWarning,
			"source clip points at an absent source mob; a Filler of equal " +
				"length is substituted so the timeline duration stays intact"
	case e.Prop == "EssenceDescriptor":
		return "MISSING_ESSENCE_DESC", SevWarning,
			"essence descriptor is absent; the source mob is kept without it"
	case e.Prop == "DescriptiveMetadata":
		return "MISSING_METADATA", SevWarning,
			"descriptive metadata framework is absent; the attachment is dropped"
	case e.Prop == "DataDefinition":
		return "MISSING_DATA_DEF", SevFatal,
			"component has no resolvable data definition; its essence kind is unknown"
	case e.Prop == "Slots" || e.Prop == "Segment" || e.Prop == "Components" ||
		e.Prop == "Mobs" || e.Prop == "ContentStorage":
		return "MISSING_STRUCTURAL", SevFatal,
			"structural reference is broken; the object tree cannot be reassembled"
	default:
		return "MISSING_NODE", SevFatal,
			"referenced object is absent and no safe repair is defined for this property"
	}
}

// Diagnose runs all checks over the graph and returns the report.
func Diagnose(p *Project, g *Graph, inputName string) *Report {
	rep := &Report{Input: inputName}

	for _, e := range g.MissingEdges() {
		if e.Weak {
			rep.add(Diag{
				Severity: SevWarning,
				Code:     "DANGLING_WEAKREF",
				ObjectID: e.FromID,
				Property: e.Prop,
				Message: fmt.Sprintf("weak reference to absent %s is dropped "+
					"(%s.%s -> %s)", e.ToID, e.FromCls, e.Prop, e.ToID),
			})
			continue
		}
		code, sev, why := missingEdgePolicy(e)
		rep.add(Diag{
			Severity: sev,
			Code:     code,
			ObjectID: e.FromID,
			Property: e.Prop,
			Message:  fmt.Sprintf("%s (%s.%s -> %s)", why, e.FromCls, e.Prop, e.ToID),
		})
	}

	for _, obj := range p.Objects {
		if !g.Reachable[obj.ID] {
			continue
		}
		rule, known := minimalSchema[obj.Class]
		if !known {
			rep.add(Diag{
				Severity: SevFatal,
				Code:     "INEXPRESSIBLE_CLASS",
				ObjectID: obj.ID,
				Message: fmt.Sprintf("class %s is reachable but not part of the "+
					"minimal re-importable model", obj.Class),
			})
			continue
		}
		for _, name := range obj.sortedPropNames() {
			v := obj.Properties[name]
			pr, knownProp := rule.props[name]
			switch {
			case !knownProp:
				rep.add(Diag{
					Severity: SevWarning,
					Code:     "INEXPRESSIBLE_PROP",
					ObjectID: obj.ID,
					Property: name,
					Message: fmt.Sprintf("property %s.%s has no minimal-model "+
						"counterpart and is dropped", obj.Class, name),
				})
			case v.Kind == KindOpaque:
				sev := SevWarning
				if pr.required {
					sev = SevFatal
				}
				rep.add(Diag{
					Severity: sev,
					Code:     "INEXPRESSIBLE_VALUE",
					ObjectID: obj.ID,
					Property: name,
					Message: fmt.Sprintf("property %s.%s carries opaque data of "+
						"type %s that the minimal model cannot express",
						obj.Class, name, v.Opaque.Type),
				})
			}
		}
	}

	for _, id := range g.Orphans(p) {
		obj := p.lookup(id)
		rep.add(Diag{
			Severity: SevInfo,
			Code:     "ORPHAN_PRUNED",
			ObjectID: id,
			Message: fmt.Sprintf("object %s (%s) is unreachable from the header "+
				"and is pruned from the minimal project", id, obj.Class),
		})
	}

	rep.OK = rep.Fatals == 0
	rep.sortDiags()
	return rep
}
