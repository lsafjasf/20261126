package audioedit

import (
	"fmt"
	"math"
	"math/bits"
)

type sourcePlan struct {
	source     Source
	delay      int64
	frame      int64
	encodedEnd int64
	editStart  int64
	editEnd    int64
}

// Compile derives a complete sample-position table from format declarations
// and edit declarations. It never reuses caller-provided timeline offsets:
// every clip is laid out again from the start.
func Compile(project Project) (Table, error) {
	if err := project.Params.validate(); err != nil {
		return Table{}, err
	}
	sources, err := planSources(project.Sources)
	if err != nil {
		return Table{}, err
	}

	table := Table{
		SampleRate: project.Params.SampleRate,
		GapSamples: project.Params.GapSamples,
		Segments:   make([]Position, 0, len(project.Clips)),
	}
	cursor := int64(0)
	seenClips := make(map[string]struct{}, len(project.Clips))

	for i, clip := range project.Clips {
		if clip.ID == "" {
			clip.ID = fmt.Sprintf("clip-%d", i+1)
		}
		if _, exists := seenClips[clip.ID]; exists {
			return Table{}, fmt.Errorf("duplicate clip id %q", clip.ID)
		}
		seenClips[clip.ID] = struct{}{}

		src, ok := sources[clip.SourceID]
		if !ok {
			return Table{}, fmt.Errorf("clip %q references unknown source %q", clip.ID, clip.SourceID)
		}
		seg, err := compileClip(project.Params, src, clip)
		if err != nil {
			return Table{}, err
		}

		if i > 0 {
			if cursor > math.MaxInt64-project.Params.GapSamples {
				return Table{}, fmt.Errorf("timeline overflows at clip %q", clip.ID)
			}
			cursor += project.Params.GapSamples
		}
		seg.TimelineStart = cursor
		if cursor > math.MaxInt64-seg.OutputSamples {
			return Table{}, fmt.Errorf("timeline overflows at clip %q", clip.ID)
		}
		seg.TimelineEnd = cursor + seg.OutputSamples
		if err := seg.validate(); err != nil {
			return Table{}, err
		}

		table.Segments = append(table.Segments, seg)
		if table.TotalPrerollSamples > math.MaxInt64-seg.PrerollSamples ||
			table.TotalTailTrimSamples > math.MaxInt64-seg.TailTrimSamples {
			return Table{}, fmt.Errorf("sample totals overflow at clip %q", clip.ID)
		}
		table.TotalPrerollSamples += seg.PrerollSamples
		table.TotalTailTrimSamples += seg.TailTrimSamples
		cursor = seg.TimelineEnd
	}
	table.TotalOutputSamples = cursor
	return table, nil
}

func planSources(sources []Source) (map[string]sourcePlan, error) {
	plans := make(map[string]sourcePlan, len(sources))
	for _, src := range sources {
		if src.ID == "" {
			return nil, fmt.Errorf("source id must not be empty")
		}
		if _, exists := plans[src.ID]; exists {
			return nil, fmt.Errorf("duplicate source id %q", src.ID)
		}
		plan, err := planSource(src)
		if err != nil {
			return nil, fmt.Errorf("source %q: %w", src.ID, err)
		}
		plans[src.ID] = plan
	}
	return plans, nil
}

func planSource(src Source) (sourcePlan, error) {
	if src.SampleRate <= 0 {
		return sourcePlan{}, fmt.Errorf("sample_rate must be positive")
	}
	spec, err := resolveFormat(src.Format)
	if err != nil {
		return sourcePlan{}, err
	}
	if spec.NativeRate != 0 && src.SampleRate != spec.NativeRate {
		return sourcePlan{}, fmt.Errorf("%s decoded sample rate must be %d, got %d",
			src.Format, spec.NativeRate, src.SampleRate)
	}

	delay := spec.Delay
	if src.CodecDelay != nil {
		delay = *src.CodecDelay
	}
	if delay < 0 {
		return sourcePlan{}, fmt.Errorf("codec_delay must not be negative")
	}
	frame := spec.FrameSamples
	if src.FrameSamples != nil {
		frame = *src.FrameSamples
	}
	if frame <= 0 {
		return sourcePlan{}, fmt.Errorf("frame_samples must be positive")
	}
	if src.Edit.MediaStart < 0 {
		return sourcePlan{}, fmt.Errorf("edit.media_start must not be negative")
	}
	if src.Edit.MediaEnd != 0 && src.Edit.MediaEnd < src.Edit.MediaStart {
		return sourcePlan{}, fmt.Errorf("edit.media_end must be after edit.media_start")
	}

	encodedEnd, err := resolveEncodedEnd(src, frame)
	if err != nil {
		return sourcePlan{}, err
	}
	editEnd := src.Edit.MediaEnd
	if encodedEnd >= 0 && editEnd != 0 {
		if editEnd > encodedEnd-delay {
			return sourcePlan{}, fmt.Errorf(
				"edit.media_end %d exceeds available presentation samples %d",
				editEnd, encodedEnd-delay)
		}
	}
	return sourcePlan{
		source:     src,
		delay:      delay,
		frame:      frame,
		encodedEnd: encodedEnd,
		editStart:  src.Edit.MediaStart,
		editEnd:    editEnd,
	}, nil
}

func resolveEncodedEnd(src Source, frame int64) (int64, error) {
	var packetEnd int64 = -1
	if src.Packets != nil {
		if *src.Packets <= 0 {
			return -1, fmt.Errorf("packets must be positive")
		}
		hi, lo := bits.Mul64(uint64(*src.Packets), uint64(frame))
		if hi != 0 || lo > math.MaxInt64 {
			return -1, fmt.Errorf("packets * frame_samples overflows")
		}
		packetEnd = int64(lo)
	}

	if src.EncodedSamples == nil {
		return packetEnd, nil
	}
	if *src.EncodedSamples <= 0 {
		return -1, fmt.Errorf("encoded_samples must be positive")
	}
	encodedEnd := *src.EncodedSamples
	if packetEnd >= 0 && encodedEnd > packetEnd {
		return -1, fmt.Errorf("encoded_samples %d exceeds packet capacity %d",
			encodedEnd, packetEnd)
	}
	return encodedEnd, nil
}

func compileClip(params Params, src sourcePlan, clip Clip) (Position, error) {
	start, end, err := resolveClipRange(src, clip)
	if err != nil {
		return Position{}, fmt.Errorf("clip %q: %w", clip.ID, err)
	}

	contentStart, ok := addInt64(src.delay, start)
	if !ok {
		return Position{}, fmt.Errorf("clip %q: decoded start overflows", clip.ID)
	}
	contentEnd, ok := addInt64(src.delay, end)
	if !ok {
		return Position{}, fmt.Errorf("clip %q: decoded end overflows", clip.ID)
	}
	decodeStart := contentStart / src.frame * src.frame
	// At the very start of a stream, decoding begins at packet zero even when
	// the container's priming declaration spans several coded frames. The
	// whole priming interval is therefore part of preroll/discard.
	if start == 0 {
		decodeStart = 0
	}
	frameEnd, ok := ceilMul(contentEnd, src.frame)
	if !ok {
		return Position{}, fmt.Errorf("clip %q: decoded end overflows", clip.ID)
	}
	decodeEnd := frameEnd

	if src.encodedEnd >= 0 {
		if contentEnd > src.encodedEnd {
			return Position{}, fmt.Errorf(
				"source range ends at %d, beyond encoded samples %d",
				end, src.encodedEnd-src.delay)
		}
		// At the edit's tail, include the declared stream trailer so the
		// table reports every sample that must be discarded. In the middle
		// of a source only the final coded frame is decoded.
		if src.editEnd != 0 && end == src.editEnd {
			decodeEnd = src.encodedEnd
		}
	}

	output, err := ScaleSamples(end-start, src.source.SampleRate, params.SampleRate)
	if err != nil {
		return Position{}, fmt.Errorf("clip %q: %w", clip.ID, err)
	}
	if output <= 0 {
		return Position{}, fmt.Errorf("clip %q produces no output samples", clip.ID)
	}
	return Position{
		SegmentID:          clip.ID,
		SourceID:           src.source.ID,
		Format:             src.source.Format,
		SourceSampleRate:   src.source.SampleRate,
		SourceStart:        start,
		SourceEnd:          end,
		CodecDelay:         src.delay,
		ContentDecodeStart: contentStart,
		ContentDecodeEnd:   contentEnd,
		DecodeStart:        decodeStart,
		DecodeEnd:          decodeEnd,
		PrerollSamples:     contentStart - decodeStart,
		TailTrimSamples:    decodeEnd - contentEnd,
		OutputSamples:      output,
	}, nil
}

func resolveClipRange(src sourcePlan, clip Clip) (int64, int64, error) {
	start := clip.SourceStart
	end := clip.SourceEnd
	if end == 0 {
		if src.editEnd == 0 {
			return 0, 0, fmt.Errorf(
				"source_end is required when the source has no edit.media_end")
		}
		end = src.editEnd
	}
	if start < src.editStart {
		return 0, 0, fmt.Errorf("source_start %d precedes edit.media_start %d",
			start, src.editStart)
	}
	if end <= start {
		return 0, 0, fmt.Errorf("source_end %d must be after source_start %d", end, start)
	}
	if src.editEnd != 0 && end > src.editEnd {
		return 0, 0, fmt.Errorf("source_end %d exceeds edit.media_end %d", end, src.editEnd)
	}
	if src.encodedEnd >= 0 {
		if _, ok := addInt64(src.delay, end); !ok || src.delay+end > src.encodedEnd {
			return 0, 0, fmt.Errorf(
				"source range ends at %d, beyond encoded samples %d",
				end, src.encodedEnd-src.delay)
		}
	}
	return start, end, nil
}

// ScaleSamples maps a sample count between rates, rounding to nearest with
// ties rounded up.
func ScaleSamples(samples, fromRate, toRate int64) (int64, error) {
	if samples < 0 {
		return 0, fmt.Errorf("sample count must not be negative")
	}
	if fromRate <= 0 || toRate <= 0 {
		return 0, fmt.Errorf("sample rates must be positive")
	}
	if samples == 0 || fromRate == toRate {
		return samples, nil
	}

	hi, lo := bits.Mul64(uint64(samples), uint64(toRate))
	if hi >= uint64(fromRate) {
		return 0, fmt.Errorf("scaled sample count overflows int64")
	}
	q, r := bits.Div64(hi, lo, uint64(fromRate))
	half := uint64(fromRate/2) + uint64(fromRate%2)
	if r >= half {
		q++
	}
	if q > math.MaxInt64 {
		return 0, fmt.Errorf("scaled sample count overflows int64")
	}
	return int64(q), nil
}

func addInt64(a, b int64) (int64, bool) {
	if b > 0 && a > math.MaxInt64-b {
		return 0, false
	}
	if b < 0 && a < math.MinInt64-b {
		return 0, false
	}
	return a + b, true
}

func ceilMul(value, unit int64) (int64, bool) {
	if value == 0 {
		return 0, true
	}
	q := value / unit
	if value%unit != 0 {
		q++
	}
	if q > math.MaxInt64/unit {
		return 0, false
	}
	return q * unit, true
}
