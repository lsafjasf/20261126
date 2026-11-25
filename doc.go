// Package ttmlevents parses TTML documents and flattens nested timed spans,
// style references, and child animation elements into deterministic timeline
// slices and causal event tables.
//
// The zero value is not usable; use Parse or ParseString and then Build.
package ttmlevents
