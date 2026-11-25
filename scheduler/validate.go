package scheduler

import (
	"fmt"
	"strings"
)

const (
	defaultBeforeMS = int64(3_000)
	defaultAfterMS  = int64(8_000)
)

type preparedCue struct {
	cue    Cue
	window Window
	long   *Variant
	short  *Variant
}

type preparedPlan struct {
	timeline Timeline
	cues     []preparedCue
	options  Options
}

func prepare(input Plan) (preparedPlan, error) {
	options := input.Options
	if options.DefaultBeforeMS == 0 {
		options.DefaultBeforeMS = defaultBeforeMS
	}
	if options.DefaultAfterMS == 0 {
		options.DefaultAfterMS = defaultAfterMS
	}
	if options.DefaultBeforeMS < 0 || options.DefaultAfterMS < 0 || options.MinClearanceMS < 0 {
		return preparedPlan{}, fmt.Errorf("options: durations must be non-negative")
	}
	if input.Timeline.DurationMS < 0 {
		return preparedPlan{}, fmt.Errorf("timeline.duration_ms: must be non-negative")
	}

	cues := make([]preparedCue, 0, len(input.Cues))
	seenCueIDs := make(map[string]struct{}, len(input.Cues))
	for i, cue := range input.Cues {
		cue = cloneCue(cue)
		if strings.TrimSpace(cue.ID) == "" {
			cue.ID = fmt.Sprintf("cue-%d", i+1)
		} else {
			cue.ID = strings.TrimSpace(cue.ID)
		}
		if _, exists := seenCueIDs[cue.ID]; exists {
			return preparedPlan{}, fmt.Errorf("cues[%d].id: duplicate cue id %q", i, cue.ID)
		}
		seenCueIDs[cue.ID] = struct{}{}
		if cue.AnchorMS < 0 {
			return preparedPlan{}, fmt.Errorf("cue %q: anchor_ms must be non-negative", cue.ID)
		}
		if input.Timeline.DurationMS > 0 && cue.AnchorMS > input.Timeline.DurationMS {
			return preparedPlan{}, fmt.Errorf("cue %q: anchor_ms is outside timeline", cue.ID)
		}
		if cue.Long == nil && cue.Short == nil {
			return preparedPlan{}, fmt.Errorf("cue %q: at least one of long or short is required", cue.ID)
		}
		for _, check := range []struct {
			name    string
			variant *Variant
		}{
			{name: "long", variant: cue.Long},
			{name: "short", variant: cue.Short},
		} {
			if check.variant == nil {
				continue
			}
			if _, err := variantDuration(*check.variant); err != nil {
				return preparedPlan{}, fmt.Errorf("cue %q %s: %w", cue.ID, check.name, err)
			}
		}

		window, err := cueWindow(cue, options, input.Timeline.DurationMS)
		if err != nil {
			return preparedPlan{}, err
		}
		if input.Timeline.DurationMS > 0 && window.EndMS > input.Timeline.DurationMS {
			return preparedPlan{}, fmt.Errorf(
				"cue %q: window end_ms %d exceeds timeline duration %d",
				cue.ID, window.EndMS, input.Timeline.DurationMS,
			)
		}
		cues = append(cues, preparedCue{
			cue: cue, window: window, long: cue.Long, short: cue.Short,
		})
	}

	return preparedPlan{timeline: input.Timeline, cues: cues, options: options}, nil
}

func cloneCue(cue Cue) Cue {
	cue.Long = cloneVariant(cue.Long)
	cue.Short = cloneVariant(cue.Short)
	if cue.Window != nil {
		window := *cue.Window
		cue.Window = &window
	}
	return cue
}

func cloneVariant(variant *Variant) *Variant {
	if variant == nil {
		return nil
	}
	copied := *variant
	copied.Segments = append([]Segment(nil), variant.Segments...)
	return &copied
}

func cueWindow(cue Cue, options Options, timelineDuration int64) (Window, error) {
	if cue.Window != nil {
		if cue.Window.StartMS < 0 || cue.Window.EndMS < 0 {
			return Window{}, fmt.Errorf("cue %q: window timestamps must be non-negative", cue.ID)
		}
		if cue.Window.EndMS <= cue.Window.StartMS {
			return Window{}, fmt.Errorf("cue %q: window end_ms must be greater than start_ms", cue.ID)
		}
		return *cue.Window, nil
	}

	start := cue.AnchorMS - options.DefaultBeforeMS
	if start < 0 {
		start = 0
	}
	end := cue.AnchorMS + options.DefaultAfterMS
	if timelineDuration > 0 && end > timelineDuration {
		end = timelineDuration
	}
	if end <= start {
		end = start + 1
	}
	return Window{StartMS: start, EndMS: end}, nil
}

func variantDuration(variant Variant) (int64, error) {
	if variant.DurationMS < 0 {
		return 0, fmt.Errorf("duration_ms must be non-negative")
	}
	duration := variant.DurationMS
	if duration == 0 {
		for _, segment := range variant.Segments {
			if segment.DurationMS < 0 {
				return 0, fmt.Errorf("segment %q duration_ms must be non-negative", segment.ID)
			}
			duration += segment.DurationMS
		}
	}
	if duration <= 0 {
		return 0, fmt.Errorf("duration must be greater than zero")
	}
	return duration, nil
}
