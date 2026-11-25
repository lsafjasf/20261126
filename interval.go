package edlframe

const frameListExpansionLimit = 100_000

func sameTimebase(a, b Timebase) bool {
	return a.Rate.Equal(b.Rate) && a.TimecodeBase == b.TimecodeBase && a.DropFrame == b.DropFrame
}

func analyzeInterval(
	field string,
	rawIn string,
	rawOut string,
	crossesMidnight bool,
	declared Timebase,
	canonical Timebase,
	hasCanonical bool,
	expandFrames bool,
) *IntervalReport {
	report := &IntervalReport{
		Field:           field,
		RawIn:           rawIn,
		RawOut:          rawOut,
		Timebase:        declared.CanonicalSpec,
		CrossesMidnight: crossesMidnight,
		Valid:           true,
	}

	inTC, inErr := ParseTimecode(rawIn, declared)
	if inErr != nil {
		report.Valid = false
		report.Issues = append(report.Issues, Issue{
			Code:     "invalid_timecode",
			Severity: SeverityError,
			Field:    field + "In",
			Message:  inErr.Error(),
		})
	}
	outTC, outErr := ParseTimecode(rawOut, declared)
	if outErr != nil {
		report.Valid = false
		report.Issues = append(report.Issues, Issue{
			Code:     "invalid_timecode",
			Severity: SeverityError,
			Field:    field + "Out",
			Message:  outErr.Error(),
		})
	}
	if !report.Valid {
		return report
	}

	start := inTC.Linear
	end := outTC.Linear
	dayFrames := declared.FramesPerDay()

	inferredMidnightWrap := false
	if !crossesMidnight && end < start && inTC.Hour == 23 && outTC.Hour == 0 {
		crossesMidnight = true
		inferredMidnightWrap = true
	}

	if crossesMidnight {
		switch {
		case end < start:
			end += dayFrames
		case end == start:
			end += dayFrames
		case end > start:
			report.Issues = append(report.Issues, Issue{
				Code:     "invalid_midnight_wrap",
				Severity: SeverityError,
				Field:    "crossesMidnight",
				Message:  "crossesMidnight is set but out is later than in",
			})
			report.Valid = false
		}
	} else if end < start {
		report.Issues = append(report.Issues, Issue{
			Code:     "negative_duration",
			Severity: SeverityError,
			Field:    field + "Out",
			Message:  "out precedes in without a declared midnight wrap",
		})
		report.Valid = false
	}

	duration := end - start
	report.StartFrame = start
	report.EndFrame = end
	report.DurationFrames = duration
	report.EndDayOffset = end / dayFrames
	if end >= dayFrames {
		report.CrossesMidnight = true
	}
	if inferredMidnightWrap && report.Valid {
		report.Issues = append(report.Issues, Issue{
			Code:     "midnight_wrap_inferred",
			Severity: SeverityInfo,
			Field:    "crossesMidnight",
			Message:  "backward timecode from hour 23 to hour 00 was interpreted as midnight wrap",
		})
	}

	if report.Valid && hasCanonical && !sameTimebase(declared, canonical) {
		report.Impact = buildFrameImpact(start, end, duration, declared, canonical, expandFrames)
	}
	return report
}

func buildFrameImpact(
	start int64,
	end int64,
	duration int64,
	declared Timebase,
	canonical Timebase,
	expandFrames bool,
) *FrameImpact {
	canonicalStart := ScaleFrames(start, declared, canonical)
	canonicalDuration := ScaleFrames(duration, declared, canonical)
	canonicalEnd := canonicalStart + canonicalDuration

	declaredSpan := FrameSpan{
		StartFrame:    start,
		EndFrame:      end,
		FirstFrame:    start,
		LastFrame:     end - 1,
		FrameCount:    duration,
		StartTimecode: FormatTimecode(start, declared),
		EndTimecode:   FormatTimecode(end, declared),
		EndDayOffset:  end / declared.FramesPerDay(),
	}
	canonicalSpan := FrameSpan{
		StartFrame:    canonicalStart,
		EndFrame:      canonicalEnd,
		FirstFrame:    canonicalStart,
		LastFrame:     canonicalEnd - 1,
		FrameCount:    canonicalDuration,
		StartTimecode: FormatTimecode(canonicalStart, canonical),
		EndTimecode:   FormatTimecode(canonicalEnd, canonical),
		EndDayOffset:  canonicalEnd / canonical.FramesPerDay(),
	}

	affected := FrameSet{
		First: canonicalStart,
		Last:  canonicalEnd - 1,
		Count: canonicalDuration,
	}
	if expandFrames && canonicalDuration > 0 && canonicalDuration <= frameListExpansionLimit {
		affected.Frames = make([]int64, canonicalDuration)
		for i := int64(0); i < canonicalDuration; i++ {
			affected.Frames[i] = canonicalStart + i
		}
	} else if expandFrames && canonicalDuration > frameListExpansionLimit {
		affected.Truncated = true
	}

	return &FrameImpact{
		Policy:             "preserve-wall-clock",
		Declared:           declaredSpan,
		Canonical:          canonicalSpan,
		StartFrameDelta:    canonicalStart - start,
		EndFrameDelta:      canonicalEnd - end,
		DurationFrameDelta: canonicalDuration - duration,
		AffectedFrames:     affected,
	}
}
