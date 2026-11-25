package opusalign

import "fmt"

type Cue struct {
	ID          string `json:"id,omitempty"`
	PacketID    string `json:"packet_id,omitempty"`
	Sequence    *int64 `json:"sequence,omitempty"`
	StartSample int64  `json:"start_sample,omitempty"`
	EndSample   int64  `json:"end_sample,omitempty"`
	Text        string `json:"text,omitempty"`
}

type SyncedCue struct {
	ID              string      `json:"id,omitempty"`
	Status          string      `json:"status"`
	Text            string      `json:"text"`
	TextSource      string      `json:"text_source"`
	Range           SampleRange `json:"range"`
	SRTStart        string      `json:"srt_start,omitempty"`
	SRTEnd          string      `json:"srt_end,omitempty"`
	PacketIndices   []int       `json:"packet_indices,omitempty"`
	MissingSamples  int64       `json:"missing_samples,omitempty"`
	Duplicate       bool        `json:"duplicate,omitempty"`
	IllegalDuration bool        `json:"illegal_duration,omitempty"`
	Message         string      `json:"message,omitempty"`
}

func PacketCues(intervals []PacketInterval) []SyncedCue {
	cues := make([]SyncedCue, 0, len(intervals))
	for _, interval := range intervals {
		status := "aligned"
		message := ""
		if !interval.Audible.Valid() {
			status = "priming_skipped"
			message = "interval is covered by pre-skip"
		}
		if !interval.Received || interval.TextStatus == TextLost || interval.TextStatus == TextGeneratorSuppressed {
			status = "missing_text"
			if interval.TextStatus == TextGeneratorSuppressed {
				message = "generator transcript suppressed for ordinary playback"
			} else {
				message = "packet loss leaves an empty playback subtitle"
			}
		} else if interval.Text == "" {
			status = "no_observed_text"
		}
		if hasIssue(interval.Issues, IssueDuplicate) {
			status = IssueDuplicate
		}
		if hasIssue(interval.Issues, IssueIllegalDuration) || hasIssue(interval.Issues, IssueTOCMismatch) {
			status = IssueIllegalDuration
		}

		textSource := TextEmpty
		if interval.Text != "" {
			textSource = TextObserved
		} else if !interval.Received {
			textSource = TextLost
		}
		cues = append(cues, SyncedCue{
			ID:              cueID(interval),
			Status:          status,
			Text:            interval.Text,
			TextSource:      textSource,
			Range:           interval.Audible,
			SRTStart:        FormatSRTTime(interval.Audible.Start),
			SRTEnd:          FormatSRTTime(interval.Audible.End),
			PacketIndices:   []int{interval.Index},
			Duplicate:       hasIssue(interval.Issues, IssueDuplicate),
			IllegalDuration: hasIssue(interval.Issues, IssueIllegalDuration) || hasIssue(interval.Issues, IssueTOCMismatch),
			Message:         message,
		})
	}
	return cues
}

func SyncCues(cues []Cue, intervals []PacketInterval) []SyncedCue {
	report := Report{Intervals: intervals}
	return SyncCuesToReport(cues, report)
}

func SyncCuesToReport(cues []Cue, report Report) []SyncedCue {
	intervals := report.Intervals
	result := make([]SyncedCue, 0, len(cues))
	for _, cue := range cues {
		switch {
		case cue.PacketID != "":
			result = append(result, syncCueByID(cue, intervals))
		case cue.Sequence != nil:
			result = append(result, syncCueBySequence(cue, intervals))
		default:
			result = append(result, syncCueByRange(cue, report))
		}
	}
	return result
}

func syncCueByID(cue Cue, intervals []PacketInterval) SyncedCue {
	matches := make([]int, 0)
	for _, interval := range intervals {
		if interval.ID == cue.PacketID {
			matches = append(matches, interval.Index)
		}
	}
	synced := baseSyncedCue(cue)
	synced.PacketIndices = matches
	if len(matches) == 0 {
		synced.Status = IssueMissing
		synced.Message = fmt.Sprintf("packet id %q is absent", cue.PacketID)
		return synced
	}
	applyIntervalToCue(&synced, intervals[matches[0]])
	if len(matches) > 1 {
		synced.Status = IssueDuplicate
		synced.Duplicate = true
		synced.Message = fmt.Sprintf("packet id matches %d intervals", len(matches))
	}
	return synced
}

func syncCueBySequence(cue Cue, intervals []PacketInterval) SyncedCue {
	matches := make([]int, 0)
	for _, interval := range intervals {
		if interval.Sequence != nil && *interval.Sequence == *cue.Sequence {
			matches = append(matches, interval.Index)
		}
	}
	synced := baseSyncedCue(cue)
	synced.PacketIndices = matches
	if len(matches) == 0 {
		synced.Status = IssueMissing
		synced.Message = fmt.Sprintf("sequence %d is absent", *cue.Sequence)
		return synced
	}
	applyIntervalToCue(&synced, intervals[matches[0]])
	if len(matches) > 1 {
		synced.Status = IssueDuplicate
		synced.Duplicate = true
		synced.Message = fmt.Sprintf("sequence matches %d intervals", len(matches))
	}
	return synced
}

func syncCueByRange(cue Cue, report Report) SyncedCue {
	synced := baseSyncedCue(cue)
	if cue.EndSample <= cue.StartSample {
		synced.Status = IssueIllegalDuration
		synced.IllegalDuration = true
		synced.Message = "cue end must be greater than start"
		return synced
	}
	synced.Range = SampleRange{Start: cue.StartSample, End: cue.EndSample}
	for _, interval := range report.Intervals {
		overlap := intersection(synced.Range, interval.Audible)
		if !overlap.Valid() {
			continue
		}
		synced.PacketIndices = append(synced.PacketIndices, interval.Index)
		if !interval.Received {
			synced.MissingSamples += overlap.Length()
			synced.Status = IssueMissing
		}
		if hasIssue(interval.Issues, IssueDuplicate) {
			synced.Duplicate = true
		}
		if hasIssue(interval.Issues, IssueIllegalDuration) {
			synced.IllegalDuration = true
		}
	}
	for _, missing := range report.MissingRanges {
		overlap := intersection(synced.Range, missing.Range)
		if overlap.Valid() {
			synced.MissingSamples += overlap.Length()
		}
	}
	switch {
	case synced.MissingSamples >= synced.Range.Length() && synced.Range.Valid():
		synced.Status = IssueMissing
	case synced.MissingSamples > 0:
		synced.Status = "partial_missing"
	case synced.Status == "":
		synced.Status = "aligned"
	}
	synced.SRTStart = FormatSRTTime(cue.StartSample)
	synced.SRTEnd = FormatSRTTime(cue.EndSample)
	return synced
}

func baseSyncedCue(cue Cue) SyncedCue {
	textSource := TextEmpty
	if cue.Text != "" {
		textSource = TextObserved
	}
	return SyncedCue{ID: cue.ID, Text: cue.Text, TextSource: textSource, Status: "aligned"}
}

func applyIntervalToCue(synced *SyncedCue, interval PacketInterval) {
	synced.Range = interval.Audible
	synced.SRTStart = FormatSRTTime(interval.Audible.Start)
	synced.SRTEnd = FormatSRTTime(interval.Audible.End)
	if !interval.Received {
		synced.Status = IssueMissing
		synced.Text = ""
		synced.TextSource = TextLost
		synced.MissingSamples = interval.Audible.Length()
		synced.Message = "referenced packet was lost; source transcript was not substituted"
	}
	if hasIssue(interval.Issues, IssueDuplicate) {
		synced.Duplicate = true
	}
	if hasIssue(interval.Issues, IssueIllegalDuration) || hasIssue(interval.Issues, IssueTOCMismatch) {
		synced.IllegalDuration = true
	}
}

func intersection(a, b SampleRange) SampleRange {
	start := max64(a.Start, b.Start)
	end := min64(a.End, b.End)
	return SampleRange{Start: start, End: end}
}

func min64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

func cueID(interval PacketInterval) string {
	if interval.ID != "" {
		return interval.ID
	}
	return fmt.Sprintf("packet-%d", interval.Index)
}

func FormatSRTTime(sample int64) string {
	if sample < 0 {
		sample = 0
	}
	totalMS := (sample + 24) / 48
	hours := totalMS / 3_600_000
	minutes := (totalMS % 3_600_000) / 60_000
	seconds := (totalMS % 60_000) / 1000
	millis := totalMS % 1000
	return fmt.Sprintf("%02d:%02d:%02d,%03d", hours, minutes, seconds, millis)
}
