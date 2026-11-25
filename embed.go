package vttlayout

import _ "embed"

// ExampleOverlapVTT is the resolvable portrait overlap sample.
//
//go:embed examples/portrait_overlap.vtt
var ExampleOverlapVTT string

// ExampleInfeasibleVTT is the no solution stack with repair candidates.
//
//go:embed examples/infeasible_stack.vtt
var ExampleInfeasibleVTT string
