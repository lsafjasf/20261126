package ttmlevents

import _ "embed"

//go:embed testdata/nested_animation.ttml
var sampleTTML string

// SampleTTML returns a compact document containing nested timed spans and
// an internal color animation.
func SampleTTML() string {
	return sampleTTML
}
