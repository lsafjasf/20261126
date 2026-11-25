package httpapi

import "subselect"

type switchRequest struct {
	Language        string             `json:"language"`
	TrackID         string             `json:"track_id"`
	Version         string             `json:"version"`
	Kind            subselect.Kind     `json:"kind"`
	Forced          subselect.TriState `json:"forced"`
	HearingImpaired subselect.TriState `json:"hearing_impaired"`
}

type seekRequest struct {
	PositionMS int64 `json:"position_ms"`
}

type timeBaseDTO struct {
	Timescale uint32 `json:"timescale"`
	OffsetMS  int64  `json:"offset_ms"`
}

type cueDTO struct {
	ID      string `json:"id"`
	StartMS int64  `json:"start_ms"`
	EndMS   int64  `json:"end_ms"`
	Text    string `json:"text"`
}

type stateDTO struct {
	SessionID  string `json:"session_id"`
	Generation uint64 `json:"generation"`
	Status     string `json:"status"`
	PositionMS int64  `json:"position_ms"`
	Off        bool   `json:"off"`
	Stale      bool   `json:"stale,omitempty"`

	TrackID  string `json:"track_id,omitempty"`
	Language string `json:"language,omitempty"`
	Version  string `json:"version,omitempty"`
	Reason   string `json:"reason,omitempty"`

	TimeBase   *timeBaseDTO `json:"time_base,omitempty"`
	Candidates []string     `json:"candidates,omitempty"`
	Variants   []string     `json:"variants,omitempty"`

	LanguageMissing       bool `json:"language_missing,omitempty"`
	VersionMissing        bool `json:"version_missing,omitempty"`
	RequestedTrackMissing bool `json:"requested_track_missing,omitempty"`

	Cue        *cueDTO        `json:"cue,omitempty"`
	LoadErrors []loadErrorDTO `json:"load_errors,omitempty"`
}

type loadErrorDTO struct {
	TrackID string `json:"track_id"`
	Error   string `json:"error"`
}

type sessionCreatedDTO struct {
	SessionID string `json:"session_id"`
}

func toStateDTO(v *subselect.View) stateDTO {
	if v == nil {
		return stateDTO{}
	}
	dto := stateDTO{
		SessionID:             v.SessionID,
		Generation:            v.Generation,
		Status:                string(v.Status),
		PositionMS:            v.PositionMS(),
		Off:                   v.Off,
		Reason:                string(v.Decision.Reason),
		Candidates:            v.Decision.Candidates,
		Variants:              v.Decision.Variants,
		LanguageMissing:       v.Decision.LanguageMissing,
		VersionMissing:        v.Decision.VersionMissing,
		RequestedTrackMissing: v.Decision.RequestedTrackMissing,
	}
	if t := v.Decision.Track; t != nil {
		dto.TrackID = t.ID
		dto.Language = t.Language
		dto.Version = t.Version
		dto.TimeBase = &timeBaseDTO{
			Timescale: t.TimeBase.Timescale,
			OffsetMS:  t.TimeBase.Offset.Milliseconds(),
		}
	}
	if v.Cue != nil {
		dto.Cue = &cueDTO{
			ID:      v.Cue.ID,
			StartMS: v.Cue.StartMS(),
			EndMS:   v.Cue.EndMS(),
			Text:    v.Cue.Text,
		}
	}
	return dto
}
