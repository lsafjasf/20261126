package cuemap

// BuildMapping builds the logical-to-screen mapping for one display line.
// It is the descriptive entry point equivalent to MapLine.
func BuildMapping(logicalLine string, baseDirection Direction) Mapping {
	return MapLine(logicalLine, baseDirection)
}

// DisplayOrder returns logical cluster indexes in screen order.
func (m Mapping) DisplayOrder() []int {
	return m.ScreenOrder
}

// DisplayText returns the diagnostic screen-order text.
func (m Mapping) DisplayText() string {
	return m.Visual
}

// LayoutCue typesets one cue and returns either collision-free geometry or a
// fallback record that points back to the original cue.
func LayoutCue(cue Cue, options Options) Result {
	return Typeset(cue, options)
}

// OriginalCue returns the preserved cue and true when the result could not be
// safely typeset.
func (r Result) OriginalCue() (Cue, bool) {
	if r.Fallback == nil {
		return Cue{}, false
	}
	return r.Fallback.Cue, true
}
