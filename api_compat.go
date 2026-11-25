package ac3select

import "io"

// Analyze is a semantic alias for ParseBytes.
func Analyze(data []byte) Report {
	return ParseBytes(data)
}

// AnalyzeWithOptions is a semantic alias for ParseBytesWithOptions.
func AnalyzeWithOptions(data []byte, options Options) Report {
	return ParseBytesWithOptions(data, options)
}

// AnalyzeReader is a semantic alias for Read.
func AnalyzeReader(r io.Reader) (Report, error) {
	return Read(r)
}

// Scan is a short alias for ParseBytes.
func Scan(data []byte) Report {
	return ParseBytes(data)
}

// MainSegment returns the selected main segment, if any.
func (r Report) MainSegment() *ProgramSegment {
	if r.Selection.Main == nil {
		return nil
	}
	index := r.Selection.Main.SegmentIndex
	if index < 0 || index >= len(r.Segments) {
		return nil
	}
	return &r.Segments[index]
}

// CommentarySegment returns the selected commentary segment, if any.
func (r Report) CommentarySegment() *ProgramSegment {
	if r.Selection.Commentary == nil {
		return nil
	}
	index := r.Selection.Commentary.SegmentIndex
	if index < 0 || index >= len(r.Segments) {
		return nil
	}
	return &r.Segments[index]
}

// IsCommentary reports whether the segment is an explicit commentary service.
func (s ProgramSegment) IsCommentary() bool {
	return s.Track.IsCommentary
}

// IsMain reports whether the segment is a main service candidate.
func (s ProgramSegment) IsMain() bool {
	return s.Track.IsMain
}

// ChannelLabels returns actual channel labels in AC-3 bitstream order.
func (s ProgramSegment) ChannelLabels() []string {
	labels := make([]string, 0, len(s.Channels))
	for _, channel := range s.Channels {
		labels = append(labels, channel.Label)
	}
	return labels
}
