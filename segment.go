package ac3select

type segmentKey struct {
	bitstreamMode BitstreamMode
	acmod         AudioCodingMode
	lfeOn         bool
	sampleRate    int
}

func buildSegments(frames []Frame) []ProgramSegment {
	segments := make([]ProgramSegment, 0)
	var cumulativeSamples int64
	var cumulativeMS float64

	for _, frame := range frames {
		h := frame.Header
		key := segmentKey{
			bitstreamMode: h.BitstreamMode,
			acmod:         h.ACMod,
			lfeOn:         h.LFEOn,
			sampleRate:    h.SampleRate,
		}

		idx := len(segments) - 1
		sameAsLast := false
		if idx >= 0 {
			last := segments[idx]
			sameAsLast = last.BitstreamMode == key.bitstreamMode &&
				last.ACMod == key.acmod &&
				last.LFEOn == key.lfeOn &&
				last.SampleRate == key.sampleRate
		}
		if !sameAsLast {
			idx = len(segments)
			segment := ProgramSegment{
				Index:             idx,
				StartOffset:       h.Offset,
				EndOffset:         h.Offset + int64(h.FrameSize),
				StartFrameIndex:   frame.Index,
				StartSample:       cumulativeSamples,
				StartMS:           cumulativeMS,
				BitstreamMode:     h.BitstreamMode,
				BitstreamModeName: h.BitstreamModeName,
				ServiceRole:       h.ServiceRole,
				ACMod:             h.ACMod,
				ACModName:         h.ACModName,
				Layout:            h.Layout,
				LayoutShort:       h.LayoutShort,
				FullBandChannels:  h.FullBandChannels,
				LFEOn:             h.LFEOn,
				ChannelCount:      h.ChannelCount,
				Channels:          append([]Channel(nil), h.Channels...),
				SampleRate:        h.SampleRate,
				Track:             h.BitstreamMode.trackCategory(h.ACMod, h.ChannelCount),
				Coupling: CouplingSummary{
					CoupledChannels: []string{},
				},
			}
			segments = append(segments, segment)
		}

		segment := &segments[idx]
		segment.FrameCount++
		segment.SampleCount += SamplesPerFrame
		if h.SampleRate > 0 {
			cumulativeMS += float64(SamplesPerFrame) * 1000 / float64(h.SampleRate)
		}
		segment.EndOffset = h.Offset + int64(h.FrameSize)
		segment.Length = segment.EndOffset - segment.StartOffset
		segment.EndMS = cumulativeMS
		segment.DurationMS = segment.EndMS - segment.StartMS
		updateCouplingSummary(&segment.Coupling, h.Coupling)
		cumulativeSamples += SamplesPerFrame
	}

	return segments
}

func updateCouplingSummary(sum *CouplingSummary, coupling Coupling) {
	sum.ParsedFrames++
	if !coupling.InUse {
		sum.AlwaysInUse = false
		return
	}

	sum.InUseFrames++
	sum.InUse = true
	sum.EverInUse = true
	if sum.InUseFrames == 1 || len(sum.CoupledChannels) == 0 {
		sum.CoupledChannels = append([]string(nil), coupling.CoupledChannels...)
		sum.StartSubband = coupling.StartSubband
		sum.EndSubband = coupling.EndSubband
		sum.StartFrequencyHz = coupling.StartFrequency
		sum.EndFrequencyHz = coupling.EndFrequency
		sum.BandCount = coupling.BandCount
	}
	if sum.ParsedFrames == sum.InUseFrames {
		sum.AlwaysInUse = true
	} else {
		sum.AlwaysInUse = false
	}
}

func buildSelection(segments []ProgramSegment) Selection {
	selection := Selection{
		Policy: []string{
			"classify service with bsmod before considering channel count or stream order",
			"prefer bsmod=0 complete main, then bsmod=1 music and effects",
			"prefer exact bsmod=5 commentary",
			"for repeated equal candidates choose the longest segment, then the lowest index",
			"expose VI/HI/dialogue/voice-over separately instead of relabelling them as commentary",
		},
		AnnotationTracks: []SegmentRef{},
	}

	bestMain := -1
	bestCommentary := -1
	for i := range segments {
		segment := &segments[i]
		track := segment.BitstreamMode.trackCategory(segment.ACMod, segment.ChannelCount)
		segment.Track = track

		ref := SegmentRef{
			SegmentIndex: i,
			FrameCount:   segment.FrameCount,
			Offset:       segment.StartOffset,
			Role:         track.Role,
			Label:        track.Label,
			Reason:       track.Reason,
		}

		if track.IsAnnotation {
			selection.AnnotationTracks = append(selection.AnnotationTracks, ref)
		}
		if track.IsMain && betterCandidate(segments, i, bestMain) {
			bestMain = i
		}
		if track.IsCommentary && betterCandidate(segments, i, bestCommentary) {
			bestCommentary = i
		}
	}

	if bestMain < 0 {
		fallback := fallbackMain(segments)
		if fallback >= 0 {
			bestMain = fallback
			selection.Ambiguous = true
			selection.Notes = append(selection.Notes,
				"no bsmod=0/1 main service; fell back to the longest non-associated segment")
		} else {
			selection.Notes = append(selection.Notes,
				"no main service could be determined from bsmod/acmod")
		}
	}
	if bestCommentary < 0 {
		selection.Notes = append(selection.Notes,
			"no exact bsmod=5 commentary segment; voice-over and accessibility services are listed as annotation tracks")
	}

	if bestMain >= 0 {
		segments[bestMain].Track.Chosen = true
		segments[bestMain].Track.SelectedAs = "main"
		ref := segmentRef(segments[bestMain], "main",
			"selected as main program from explicit bsmod and stable longest-segment tie-break")
		selection.Main = &ref
	}
	if bestCommentary >= 0 {
		segments[bestCommentary].Track.Chosen = true
		segments[bestCommentary].Track.SelectedAs = "commentary"
		ref := segmentRef(segments[bestCommentary], "commentary",
			"selected as commentary from bsmod=5 and stable longest-segment tie-break")
		selection.Commentary = &ref
	}

	return selection
}

func betterCandidate(segments []ProgramSegment, candidate, current int) bool {
	if current < 0 {
		return true
	}
	if segments[candidate].FrameCount != segments[current].FrameCount {
		return segments[candidate].FrameCount > segments[current].FrameCount
	}
	return candidate < current
}

func fallbackMain(segments []ProgramSegment) int {
	best := -1
	for i, segment := range segments {
		if segment.Track.IsAnnotation || segment.Track.Category == "emergency" {
			continue
		}
		if segment.ACMod == ACModDualMono {
			continue
		}
		if best < 0 || segment.FrameCount > segments[best].FrameCount ||
			(segment.FrameCount == segments[best].FrameCount && i < best) {
			best = i
		}
	}
	return best
}

func segmentRef(segment ProgramSegment, role, reason string) SegmentRef {
	return SegmentRef{
		SegmentIndex: segment.Index,
		FrameCount:   segment.FrameCount,
		Offset:       segment.StartOffset,
		Role:         role,
		Label:        segment.Track.Label,
		Reason:       reason,
	}
}
