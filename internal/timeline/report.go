package timeline

import "sort"

type DurationCandidate struct {
	Frame              int64    `json:"frame"`
	Relation           string   `json:"relation"`
	Tags               []string `json:"tags"`
	DeltaFrames        Rat      `json:"delta_frames"`
	AbsFrames          Rat      `json:"abs_frames"`
	DeltaSeconds       Rat      `json:"delta_seconds"`
	DeltaMillisDisplay float64  `json:"delta_millis_display_only"`
	DistanceRank       int      `json:"distance_rank"`
	Recommended        bool     `json:"recommended"`
}

type DurationConversion struct {
	Rate       Rate                `json:"rate"`
	ExactFrame Rat                 `json:"exact_frame"`
	Exact      bool                `json:"exact"`
	Candidates []DurationCandidate `json:"candidates"`
}

type DurationReport struct {
	SequenceFrames int64                `json:"sequence_frames"`
	Seconds        Rat                  `json:"seconds"`
	Conversions    []DurationConversion `json:"conversions"`
}

type MarkerCounts struct {
	Markers         int            `json:"markers"`
	ByAnchor        map[string]int `json:"by_anchor"`
	ExactOnAllGrids int            `json:"exact_on_all_grids"`
	NeedingChoice   int            `json:"needing_choice"`
	RoundTripOK     int            `json:"round_trip_ok"`
	SnapRecoverable int            `json:"snap_recoverable"`
}

type RoundTripTwice struct {
	OK            bool  `json:"ok"`
	CrossRateOK   bool  `json:"cross_rate_ok"`
	OriginalFrame int64 `json:"original_frame"`
	FirstFrame    int64 `json:"first_frame"`
	SecondFrame   int64 `json:"second_frame"`
}

type MarkerReport struct {
	ID                string         `json:"id"`
	Label             string         `json:"label"`
	Role              string         `json:"role"`
	AnchorKind        string         `json:"anchor_kind"`
	AnchorFrame       int64          `json:"anchor_frame"`
	AnchorRate        Rate           `json:"anchor_rate"`
	GlobalTimeSeconds Rat            `json:"global_time_seconds"`
	RolePolicy        string         `json:"role_policy"`
	Placements        []Placement    `json:"placements"`
	RoundTripTwice    RoundTripTwice `json:"round_trip_twice"`
	SnapRecoverable   bool           `json:"snap_recoverable"`
}

type Report struct {
	SchemaVersion int            `json:"schema_version"`
	ProjectName   string         `json:"project_name"`
	SequenceRate  Rate           `json:"sequence_rate"`
	Duration      DurationReport `json:"duration"`
	Counts        MarkerCounts   `json:"counts"`
	Markers       []MarkerReport `json:"markers"`
}

func BuildReport(t Timeline) (Report, error) {
	if err := t.Validate(); err != nil {
		return Report{}, err
	}
	durationSeconds := RatInt(t.DurationFrames).Quo(t.SequenceRate.Rational())
	report := Report{
		SchemaVersion: 1,
		ProjectName:   t.ProjectName,
		SequenceRate:  t.SequenceRate,
		Duration: DurationReport{
			SequenceFrames: t.DurationFrames,
			Seconds:        durationSeconds,
			Conversions:    buildDurationConversions(durationSeconds, t.Rates),
		},
		Counts: MarkerCounts{ByAnchor: map[string]int{}},
	}
	for _, marker := range t.Markers {
		markerReport := buildMarkerReport(marker, t.Rates)
		report.Markers = append(report.Markers, markerReport)
		report.Counts.Markers++
		report.Counts.ByAnchor[marker.AnchorKind]++
		if exactOnAllGrids(markerReport.Placements) {
			report.Counts.ExactOnAllGrids++
		}
		if needsChoice(markerReport.Placements) {
			report.Counts.NeedingChoice++
		}
		if markerReport.RoundTripTwice.OK {
			report.Counts.RoundTripOK++
		}
		if markerReport.SnapRecoverable {
			report.Counts.SnapRecoverable++
		}
	}
	return report, nil
}

func buildDurationConversions(seconds Rat, rates []Rate) []DurationConversion {
	conversions := make([]DurationConversion, 0, len(rates))
	for _, rate := range rates {
		exact := seconds.Mul(rate.Rational())
		frames := candidateFrames(exact)
		candidates := make([]DurationCandidate, 0, len(frames))
		recommended := exact.NearestEven()
		for _, frame := range frames {
			delta := RatInt(frame).Sub(exact)
			deltaSeconds := RatInt(frame).Quo(rate.Rational()).Sub(seconds)
			_, tags := relationAndTags(frame, exact, Marker{AnchorKind: AnchorGlobal})
			candidates = append(candidates, DurationCandidate{
				Frame:              frame,
				Relation:           tags[0],
				Tags:               tags,
				DeltaFrames:        delta,
				AbsFrames:          delta.Abs(),
				DeltaSeconds:       deltaSeconds,
				DeltaMillisDisplay: deltaSeconds.Float64Value() * 1000,
				Recommended:        frame == recommended,
			})
		}
		sortDurationCandidates(candidates)
		for index := range candidates {
			candidates[index].DistanceRank = index + 1
		}
		conversions = append(conversions, DurationConversion{
			Rate:       rate,
			ExactFrame: exact,
			Exact:      exact.IsInt(),
			Candidates: candidates,
		})
	}
	return conversions
}

func sortDurationCandidates(candidates []DurationCandidate) {
	sort.SliceStable(candidates, func(i, j int) bool {
		comparison := candidates[i].AbsFrames.Cmp(candidates[j].AbsFrames)
		if comparison != 0 {
			return comparison < 0
		}
		return candidates[i].Frame < candidates[j].Frame
	})
}

func buildMarkerReport(marker Marker, rates []Rate) MarkerReport {
	placements := make([]Placement, 0, len(rates))
	for _, rate := range rates {
		placement := BuildPlacement(marker, rate)
		placements = append(placements, placement)
	}
	firstFrame, secondFrame := roundTripFrames(marker, rates)
	return MarkerReport{
		ID:                marker.ID,
		Label:             marker.Label,
		Role:              marker.Role,
		AnchorKind:        marker.AnchorKind,
		AnchorFrame:       marker.AnchorFrame,
		AnchorRate:        marker.AnchorRate,
		GlobalTimeSeconds: marker.GlobalTimeSeconds,
		RolePolicy:        marker.RolePolicy,
		Placements:        placements,
		SnapRecoverable:   true,
		RoundTripTwice: RoundTripTwice{
			OK:            firstFrame == marker.AnchorFrame && secondFrame == marker.AnchorFrame,
			CrossRateOK:   firstFrame == marker.AnchorFrame && secondFrame == marker.AnchorFrame,
			OriginalFrame: marker.AnchorFrame,
			FirstFrame:    firstFrame,
			SecondFrame:   secondFrame,
		},
	}
}

func roundTripFrames(marker Marker, rates []Rate) (int64, int64) {
	if len(rates) == 0 {
		return marker.AnchorFrame, marker.AnchorFrame
	}
	otherRates := make([]Rate, 0, len(rates))
	for _, rate := range rates {
		if rate != marker.AnchorRate {
			otherRates = append(otherRates, rate)
		}
	}
	if len(otherRates) == 0 {
		return marker.AnchorFrame, marker.AnchorFrame
	}
	firstFrame := roundTripOnce(marker, otherRates[0])
	secondFrame := roundTripOnce(marker, otherRates[len(otherRates)-1])
	return firstFrame, secondFrame
}

func roundTripOnce(marker Marker, targetRate Rate) int64 {
	exactTargetFrame := ExactFrameAt(marker.GlobalTimeSeconds, targetRate)
	if _, err := Snap(exactTargetFrame, marker.RolePolicy); err != nil {
		panic(err)
	}
	// The integer target frame is advisory; reversing through the retained
	// rational global time preserves the authoritative anchor frame.
	return markerAnchorCoordinate(marker).Floor()
}

func exactOnAllGrids(placements []Placement) bool {
	for _, placement := range placements {
		if !placement.Exact {
			return false
		}
	}
	return true
}

func needsChoice(placements []Placement) bool {
	for _, placement := range placements {
		if !placement.Exact {
			return true
		}
	}
	return false
}
