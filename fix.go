package edlframe

// Unify analyzes the document and returns a framebase-unified copy.
//
// Every convertible event is processed. Events with illegal intervals such as
// negative durations are intentionally left on their original explicit rate so
// an automatic rewrite cannot hide a damaged EDL row.
func Unify(doc Document, options UnifyOptions) (*Document, *Report, error) {
	report, err := Analyze(doc, options)
	if err != nil {
		return nil, nil, err
	}

	fixed := doc
	if fixed.SchemaVersion == 0 {
		fixed.SchemaVersion = SchemaVersion
	}
	fixed.Reels = append([]ReelDecl(nil), doc.Reels...)
	fixed.SourceList = append([]SourceEntry(nil), doc.SourceList...)
	fixed.Events = append([]Event(nil), doc.Events...)

	for _, reelReport := range report.Reels {
		if reelReport.CanonicalRate == "" {
			continue
		}

		found := false
		for i := range fixed.Reels {
			if fixed.Reels[i].ReelID == reelReport.ReelID {
				found = true
				fixed.Reels[i].SourceListRate = reelReport.CanonicalRate
				fixed.Reels[i].EventTableRate = reelReport.CanonicalRate
			}
		}
		if !found {
			fixed.Reels = append(fixed.Reels, ReelDecl{
				ReelID:         reelReport.ReelID,
				SourceListRate: reelReport.CanonicalRate,
				EventTableRate: reelReport.CanonicalRate,
			})
		}

		for i := range fixed.SourceList {
			if fixed.SourceList[i].ReelID == reelReport.ReelID {
				fixed.SourceList[i].Rate = reelReport.CanonicalRate
			}
		}
	}

	for _, eventReport := range report.Events {
		if eventReport.Index < 0 || eventReport.Index >= len(fixed.Events) {
			continue
		}
		event := &fixed.Events[eventReport.Index]

		if !eventReport.Convertible {
			if eventReport.DeclaredRate != "" {
				event.Rate = eventReport.DeclaredRate
			}
			continue
		}

		event.Rate = eventReport.CanonicalRate
		if eventReport.Source != nil && eventReport.Source.Impact != nil {
			canonical := eventReport.Source.Impact.Canonical
			event.SourceIn = canonical.StartTimecode
			event.SourceOut = canonical.EndTimecode
			if event.DurationFrames != nil {
				duration := canonical.FrameCount
				event.DurationFrames = &duration
			}
		}
		if eventReport.Record != nil && eventReport.Record.Impact != nil {
			canonical := eventReport.Record.Impact.Canonical
			event.RecordIn = canonical.StartTimecode
			event.RecordOut = canonical.EndTimecode
		}
	}

	return &fixed, report, nil
}
