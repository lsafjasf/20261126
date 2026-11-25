package main

import "sort"

// Edge is one typed reference from a property of a source object to a
// target object ID (which may or may not exist in the dump).
type Edge struct {
	FromID   string `json:"from"`
	FromCls  string `json:"fromClass"`
	Prop     string `json:"property"`
	ToID     string `json:"to"`
	Weak     bool   `json:"weak,omitempty"`
	ToExists bool   `json:"toExists"`
}

// Graph is the object reference graph of a project.
type Graph struct {
	Edges     []Edge
	Out       map[string][]Edge // edges by source object ID
	Reachable map[string]bool   // objects reachable from the Header
	Roots     []string          // root object IDs (Header, or all mobs if no Header)
}

// walkRefs collects reference targets from a value tree.
func walkRefs(v Value, fn func(target string, weak bool)) {
	switch v.Kind {
	case KindRef:
		fn(v.Ref, false)
	case KindWeakRef:
		fn(v.WeakRef.Target, true)
	case KindList:
		for _, item := range v.List {
			walkRefs(item, fn)
		}
	}
}

// BuildGraph extracts every reference edge and computes reachability from
// the Header (or, for header-less dumps, from every mob).
func BuildGraph(p *Project) *Graph {
	g := &Graph{
		Out:       map[string][]Edge{},
		Reachable: map[string]bool{},
	}
	for _, obj := range p.Objects {
		for _, name := range obj.sortedPropNames() {
			walkRefs(obj.Properties[name], func(target string, weak bool) {
				e := Edge{
					FromID:   obj.ID,
					FromCls:  obj.Class,
					Prop:     name,
					ToID:     target,
					Weak:     weak,
					ToExists: p.lookup(target) != nil,
				}
				g.Edges = append(g.Edges, e)
				g.Out[obj.ID] = append(g.Out[obj.ID], e)
			})
		}
	}
	for _, obj := range p.Objects {
		if obj.Class == "Header" {
			g.Roots = append(g.Roots, obj.ID)
		}
	}
	if len(g.Roots) == 0 {
		for _, obj := range p.Objects {
			switch obj.Class {
			case "CompositionMob", "MasterMob", "SourceMob":
				g.Roots = append(g.Roots, obj.ID)
			}
		}
	}
	queue := append([]string(nil), g.Roots...)
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		if g.Reachable[id] {
			continue
		}
		g.Reachable[id] = true
		for _, e := range g.Out[id] {
			if e.ToExists && !g.Reachable[e.ToID] {
				queue = append(queue, e.ToID)
			}
		}
	}
	return g
}

// MissingEdges returns edges whose target object is absent from the dump,
// restricted to edges originating from reachable objects.
func (g *Graph) MissingEdges() []Edge {
	var out []Edge
	for _, e := range g.Edges {
		if !e.ToExists && g.Reachable[e.FromID] {
			out = append(out, e)
		}
	}
	return out
}

// Orphans returns IDs of objects not reachable from any root.
func (g *Graph) Orphans(p *Project) []string {
	var out []string
	for _, obj := range p.Objects {
		if !g.Reachable[obj.ID] {
			out = append(out, obj.ID)
		}
	}
	sort.Strings(out)
	return out
}
