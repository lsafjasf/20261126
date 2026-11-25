package edlframe

import "fmt"

// Analyze detects reel-level and event-level timebase conflicts without
// modifying the input document.
func Analyze(doc Document, options UnifyOptions) (*Report, error) {
	opts := options.withDefaults()
	if opts.Policy != "preserve-wall-clock" {
		return nil, fmt.Errorf("unsupported conversion policy %q", opts.Policy)
	}
	if opts.Prefer != "source-list" && opts.Prefer != "event-table" {
		return nil, fmt.Errorf("canonical preference must be source-list or event-table, got %q", opts.Prefer)
	}

	var forcedCanonical *Timebase
	if opts.CanonicalRate != "" {
		tb, err := ParseTimebase(opts.CanonicalRate)
		if err != nil {
			return nil, err
		}
		forcedCanonical = &tb
	}

	report := &Report{
		SchemaVersion: SchemaVersion,
		Policy:        opts,
		Reels:         []ReelReport{},
		Events:        []EventReport{},
	}

	reelOrder, reelDecls := collectReelIndex(doc, report)
	sourceByReel := collectSourcesByReel(doc)
	eventIndicesByReel := make(map[string][]int)
	for i := range doc.Events {
		eventIndicesByReel[doc.Events[i].ReelID] = append(eventIndicesByReel[doc.Events[i].ReelID], i)
	}

	seenEventIDs := make(map[string]int)
	for i, event := range doc.Events {
		if event.EventID != "" {
			if first, exists := seenEventIDs[event.EventID]; exists {
				report.Issues = append(report.Issues, Issue{
					Code:     "duplicate_event_id",
					Severity: SeverityError,
					Field:    "eventId",
					Message:  fmt.Sprintf("event %s duplicates event at index %d", event.EventID, first),
				})
			} else {
				seenEventIDs[event.EventID] = i
			}
		}
	}

	reelReports := make(map[string]*ReelReport, len(reelOrder))
	canonicalByReel := make(map[string]Timebase, len(reelOrder))
	canonicalOKByReel := make(map[string]bool, len(reelOrder))

	for _, reelID := range reelOrder {
		decl := reelDecls[reelID]
		rr := &ReelReport{
			ReelID:           reelID,
			SourceListRate:   decl.SourceListRate,
			EventTableRate:   decl.EventTableRate,
			AffectedEventIDs: []string{},
			ConflictEventIDs: []string{},
			InvalidEventIDs:  []string{},
		}

		sourceTB, sourceOK := resolveReelSourceTimebase(reelID, decl, sourceByReel, rr)
		eventTableTB, eventTableOK := parseOptionalReelRate(decl.EventTableRate, "eventTableRate", rr)
		if decl.EventTableRate == "" {
			eventTableTB, eventTableOK = inferEventTableTimebase(doc.Events, eventIndicesByReel[reelID], rr)
			if eventTableOK {
				rr.EventTableRate = eventTableTB.CanonicalSpec
			}
		}

		rr.RateConflict = sourceOK && eventTableOK && !sameTimebase(sourceTB, eventTableTB)
		if rr.RateConflict {
			rr.Issues = append(rr.Issues, Issue{
				Code:     "reel_timebase_conflict",
				Severity: SeverityError,
				Field:    "sourceListRate,eventTableRate",
				Message: fmt.Sprintf(
					"reel %q declares %s in source list and %s in event table",
					reelID, sourceTB.CanonicalSpec, eventTableTB.CanonicalSpec,
				),
			})
		}

		canonicalTB, canonicalOK := chooseCanonical(forcedCanonical, opts.Prefer, sourceTB, sourceOK, eventTableTB, eventTableOK)
		if canonicalOK {
			rr.CanonicalRate = canonicalTB.CanonicalSpec
			canonicalByReel[reelID] = canonicalTB
			canonicalOKByReel[reelID] = true
		} else {
			rr.Issues = append(rr.Issues, Issue{
				Code:     "missing_canonical_timebase",
				Severity: SeverityError,
				Message:  fmt.Sprintf("reel %q has no usable canonical frame rate", reelID),
			})
		}
		reelReports[reelID] = rr
	}

	for eventIndex, event := range doc.Events {
		reelID := event.ReelID
		rr := reelReports[reelID]
		if rr == nil {
			rr = &ReelReport{ReelID: reelID, AffectedEventIDs: []string{}, ConflictEventIDs: []string{}, InvalidEventIDs: []string{}}
			reelReports[reelID] = rr
			reelOrder = append(reelOrder, reelID)
		}

		eventReport := analyzeEvent(
			event,
			eventIndex,
			effectiveReelDecl(reelDecls[reelID], rr),
			sourceByReel,
			canonicalByReel[reelID],
			canonicalOKByReel[reelID],
			opts.ExpandFrames,
		)
		report.Events = append(report.Events, eventReport)

		if eventReport.RateConflict {
			rr.ConflictEventIDs = append(rr.ConflictEventIDs, eventReport.EventID)
			report.Summary.EventConflictCount++
		}
		if hasErrorIssue(eventReport.Issues) {
			rr.InvalidEventIDs = append(rr.InvalidEventIDs, eventReport.EventID)
			report.Summary.InvalidEventCount++
		}
		if eventReport.Convertible && eventReport.Source != nil && eventReport.Source.Impact != nil {
			rr.AffectedFrameCount += eventReport.Source.Impact.Canonical.FrameCount
			report.Summary.ConvertedEventCount++
		}
	}

	for _, reelID := range reelOrder {
		rr := reelReports[reelID]
		if rr == nil {
			continue
		}
		if rr.RateConflict {
			for _, eventIndex := range eventIndicesByReel[reelID] {
				rr.AffectedEventIDs = append(rr.AffectedEventIDs, eventID(doc.Events[eventIndex], eventIndex))
			}
			report.Summary.ReelConflictCount++
		}
		report.Summary.AffectedFrameCount += rr.AffectedFrameCount
		report.Reels = append(report.Reels, *rr)
	}

	report.Summary.ReelCount = len(reelOrder)
	report.Summary.EventCount = len(doc.Events)
	return report, nil
}

func collectReelIndex(doc Document, report *Report) ([]string, map[string]ReelDecl) {
	order := []string{}
	decls := map[string]ReelDecl{}
	seen := map[string]int{}

	addID := func(id string) {
		if _, exists := seen[id]; !exists {
			seen[id] = len(order)
			order = append(order, id)
		}
	}

	for i, reel := range doc.Reels {
		if _, exists := seen[reel.ReelID]; exists {
			report.Issues = append(report.Issues, Issue{
				Code:     "duplicate_reel_declaration",
				Severity: SeverityError,
				Field:    "reels",
				Message:  fmt.Sprintf("reel %q is declared more than once at index %d", reel.ReelID, i),
			})
			continue
		}
		addID(reel.ReelID)
		decls[reel.ReelID] = reel
	}
	for _, source := range doc.SourceList {
		addID(source.ReelID)
	}
	for _, event := range doc.Events {
		addID(event.ReelID)
	}
	return order, decls
}

func effectiveReelDecl(decl ReelDecl, rr *ReelReport) ReelDecl {
	if decl.EventTableRate == "" && rr != nil && rr.EventTableRate != "" {
		decl.EventTableRate = rr.EventTableRate
	}
	return decl
}

func collectSourcesByReel(doc Document) map[string][]SourceEntry {
	result := map[string][]SourceEntry{}
	for _, source := range doc.SourceList {
		result[source.ReelID] = append(result[source.ReelID], source)
	}
	return result
}

func resolveReelSourceTimebase(
	reelID string,
	decl ReelDecl,
	sourceByReel map[string][]SourceEntry,
	rr *ReelReport,
) (Timebase, bool) {
	spec := decl.SourceListRate
	if spec != "" {
		tb, err := ParseTimebase(spec)
		if err != nil {
			rr.Issues = append(rr.Issues, Issue{
				Code: "invalid_rate", Severity: SeverityError, Field: "sourceListRate", Message: err.Error(),
			})
			return Timebase{}, false
		}

		for _, source := range sourceByReel[reelID] {
			sourceTB, parseErr := ParseTimebase(source.Rate)
			if parseErr != nil {
				rr.Issues = append(rr.Issues, Issue{
					Code: "invalid_source_rate", Severity: SeverityError, Field: "sourceList.rate", Message: parseErr.Error(),
				})
				continue
			}
			if !sameTimebase(tb, sourceTB) {
				rr.Issues = append(rr.Issues, Issue{
					Code:     "source_list_declaration_mismatch",
					Severity: SeverityError,
					Field:    "sourceListRate",
					Message:  fmt.Sprintf("source %s says %s but reel sourceListRate says %s", source.SourceID, sourceTB.CanonicalSpec, tb.CanonicalSpec),
				})
			}
		}
		return tb, true
	}

	sources := sourceByReel[reelID]
	if len(sources) == 0 {
		rr.Issues = append(rr.Issues, Issue{
			Code: "missing_source_list_rate", Severity: SeverityWarning, Field: "sourceListRate",
			Message: "no reel sourceListRate or sourceList entry",
		})
		return Timebase{}, false
	}

	var canonical Timebase
	for i, source := range sources {
		tb, err := ParseTimebase(source.Rate)
		if err != nil {
			rr.Issues = append(rr.Issues, Issue{
				Code: "invalid_source_rate", Severity: SeverityError, Field: "sourceList.rate", Message: err.Error(),
			})
			continue
		}
		if i == 0 || canonical.CanonicalSpec == "" {
			canonical = tb
			continue
		}
		if !sameTimebase(canonical, tb) {
			rr.Issues = append(rr.Issues, Issue{
				Code:     "source_list_rate_mismatch",
				Severity: SeverityError,
				Field:    "sourceList.rate",
				Message:  fmt.Sprintf("source %s uses %s while %s is already declared", source.SourceID, tb.CanonicalSpec, canonical.CanonicalSpec),
			})
		}
	}
	if canonical.CanonicalSpec == "" {
		return Timebase{}, false
	}
	rr.SourceListRate = canonical.CanonicalSpec
	return canonical, true
}

func parseOptionalReelRate(spec string, field string, rr *ReelReport) (Timebase, bool) {
	if spec == "" {
		return Timebase{}, false
	}
	tb, err := ParseTimebase(spec)
	if err != nil {
		rr.Issues = append(rr.Issues, Issue{
			Code: "invalid_rate", Severity: SeverityError, Field: field, Message: err.Error(),
		})
		return Timebase{}, false
	}
	return tb, true
}

func inferEventTableTimebase(events []Event, indices []int, rr *ReelReport) (Timebase, bool) {
	var inferred Timebase
	for _, index := range indices {
		spec := events[index].Rate
		if spec == "" {
			continue
		}
		tb, err := ParseTimebase(spec)
		if err != nil {
			continue
		}
		if inferred.CanonicalSpec == "" {
			inferred = tb
			continue
		}
		if !sameTimebase(inferred, tb) {
			return Timebase{}, false
		}
	}
	if inferred.CanonicalSpec == "" {
		rr.Issues = append(rr.Issues, Issue{
			Code: "missing_event_table_rate", Severity: SeverityWarning, Field: "eventTableRate",
			Message: "no reel eventTableRate and no explicit event rate",
		})
		return Timebase{}, false
	}
	return inferred, true
}

func chooseCanonical(
	forced *Timebase,
	prefer string,
	source Timebase,
	sourceOK bool,
	eventTable Timebase,
	eventTableOK bool,
) (Timebase, bool) {
	if forced != nil {
		return *forced, true
	}
	if prefer == "event-table" {
		if eventTableOK {
			return eventTable, true
		}
		if sourceOK {
			return source, true
		}
		return Timebase{}, false
	}
	if sourceOK {
		return source, true
	}
	if eventTableOK {
		return eventTable, true
	}
	return Timebase{}, false
}

func analyzeEvent(
	event Event,
	index int,
	decl ReelDecl,
	sourceByReel map[string][]SourceEntry,
	canonical Timebase,
	canonicalOK bool,
	expandFrames bool,
) EventReport {
	id := eventID(event, index)
	result := EventReport{
		Index:    index,
		EventID:  id,
		Number:   event.Number,
		ReelID:   event.ReelID,
		SourceID: event.SourceID,
	}

	if event.EventID == "" {
		result.Issues = append(result.Issues, Issue{
			Code: "missing_event_id", Severity: SeverityWarning, Field: "eventId",
			Message: fmt.Sprintf("event at index %d has no eventId; using %s", index, id),
		})
	}
	if event.ReelID == "" {
		result.Issues = append(result.Issues, Issue{
			Code: "missing_reel_id", Severity: SeverityError, Field: "reelId", Message: "event has no reelId",
		})
	}

	declaredSpec := event.Rate
	if declaredSpec == "" {
		declaredSpec = decl.EventTableRate
	}
	result.DeclaredRate = declaredSpec

	declaredTB, rateErr := ParseTimebase(declaredSpec)
	if rateErr != nil {
		result.Issues = append(result.Issues, Issue{
			Code: "invalid_event_rate", Severity: SeverityError, Field: "rate", Message: rateErr.Error(),
		})
		return result
	}

	if canonicalOK {
		result.CanonicalRate = canonical.CanonicalSpec
		result.RateConflict = !sameTimebase(declaredTB, canonical)
	}

	if event.SourceID != "" {
		entry := findSourceEntry(sourceByReel[event.ReelID], event.SourceID)
		if entry == nil {
			result.Issues = append(result.Issues, Issue{
				Code: "unknown_source_id", Severity: SeverityWarning, Field: "sourceId",
				Message: fmt.Sprintf("sourceId %q is not in sourceList for reel %q", event.SourceID, event.ReelID),
			})
		} else if sourceTB, err := ParseTimebase(entry.Rate); err == nil && !sameTimebase(sourceTB, declaredTB) && !result.RateConflict {
			result.RateConflict = true
			result.Issues = append(result.Issues, Issue{
				Code: "source_event_rate_mismatch", Severity: SeverityError, Field: "rate",
				Message: "event rate differs from referenced sourceList rate",
			})
		}
	}

	if event.SourceIn == "" || event.SourceOut == "" {
		result.Issues = append(result.Issues, Issue{
			Code: "missing_source_interval", Severity: SeverityError, Field: "sourceIn/sourceOut",
			Message: "sourceIn and sourceOut are required",
		})
		return result
	}

	var canonicalForInterval Timebase
	if canonicalOK {
		canonicalForInterval = canonical
	} else {
		canonicalForInterval = declaredTB
	}
	result.Source = analyzeInterval(
		"source", event.SourceIn, event.SourceOut, event.CrossesMidnight,
		declaredTB, canonicalForInterval, canonicalOK, expandFrames,
	)
	result.Issues = append(result.Issues, result.Source.Issues...)
	if result.Source.Impact != nil {
		frames := result.Source.Impact.AffectedFrames
		result.AffectedFrames = &frames
	}
	if event.DurationFrames != nil {
		switch duration := *event.DurationFrames; {
		case duration < 0:
			result.Issues = append(result.Issues, Issue{
				Code:     "negative_duration",
				Severity: SeverityError,
				Field:    "durationFrames",
				Message:  fmt.Sprintf("durationFrames is negative: %d", duration),
			})
		case result.Source.Valid && duration != result.Source.DurationFrames:
			result.Issues = append(result.Issues, Issue{
				Code:     "duration_mismatch",
				Severity: SeverityError,
				Field:    "durationFrames",
				Message:  fmt.Sprintf("durationFrames %d does not match timecode duration %d", duration, result.Source.DurationFrames),
			})
		}
	}

	if (event.RecordIn == "") != (event.RecordOut == "") {
		result.Issues = append(result.Issues, Issue{
			Code: "incomplete_record_interval", Severity: SeverityError, Field: "recordIn/recordOut",
			Message: "recordIn and recordOut must both be present",
		})
	} else if event.RecordIn != "" && event.RecordOut != "" {
		result.Record = analyzeInterval(
			"record", event.RecordIn, event.RecordOut, event.RecordCrossesMidnight,
			declaredTB, canonicalForInterval, canonicalOK, expandFrames,
		)
		result.Issues = append(result.Issues, result.Record.Issues...)
	}

	sourceValid := result.Source != nil && result.Source.Valid
	recordValid := result.Record == nil || result.Record.Valid
	result.Convertible = result.RateConflict && canonicalOK && sourceValid && recordValid && !hasErrorIssue(result.Issues)
	return result
}

func findSourceEntry(entries []SourceEntry, sourceID string) *SourceEntry {
	for i := range entries {
		if entries[i].SourceID == sourceID {
			return &entries[i]
		}
	}
	return nil
}

func eventID(event Event, index int) string {
	if event.EventID != "" {
		return event.EventID
	}
	if event.Number != 0 {
		return fmt.Sprintf("%03d", event.Number)
	}
	return fmt.Sprintf("#%d", index+1)
}

func hasErrorIssue(issues []Issue) bool {
	for _, issue := range issues {
		if issue.Severity == SeverityError {
			return true
		}
	}
	return false
}
