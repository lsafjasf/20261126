package opusalign

const (
	IssueDuplicate       = "duplicate"
	IssueMissing         = "missing"
	IssueIllegalDuration = "illegal_duration"
	IssueOverlap         = "overlap"
	IssueSequenceGap     = "sequence_gap"
	IssueTOCMismatch     = "toc_duration_mismatch"
	IssueGeneratorText   = "generator_text_suppressed"
	IssueUnknownDuration = "unknown_duration"
)

const (
	TextObserved            = "observed"
	TextEmpty               = "empty"
	TextLost                = "lost_packet_no_playback_text"
	TextGeneratorSuppressed = "generator_text_suppressed"
)

type SampleRange struct {
	Start int64 `json:"start"`
	End   int64 `json:"end"`
}

func (r SampleRange) Length() int64 {
	if r.End < r.Start {
		return -(r.Start - r.End)
	}
	return r.End - r.Start
}

func (r SampleRange) Valid() bool {
	return r.End > r.Start
}

type Issue struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
	Packet   int    `json:"packet_index,omitempty"`
	Message  string `json:"message"`
}

type Packet struct {
	ID                string `json:"id,omitempty"`
	Sequence          *int64 `json:"sequence,omitempty"`
	TOC               *byte  `json:"toc,omitempty"`
	Data              []byte `json:"data,omitempty"`
	DurationSamples   int64  `json:"duration_samples,omitempty"`
	GranulePosition   *int64 `json:"granule_position,omitempty"`
	RawEndSample      *int64 `json:"raw_end_sample,omitempty"`
	PlaybackEndSample *int64 `json:"playback_end_sample,omitempty"`
	EndOfStream       bool   `json:"end_of_stream,omitempty"`
	Received          *bool  `json:"received,omitempty"`
	Text              string `json:"text,omitempty"`
	GeneratorText     string `json:"generator_text,omitempty"`
}

type TimelineInput struct {
	PreSkip int      `json:"pre_skip"`
	Packets []Packet `json:"packets"`
	Cues    []Cue    `json:"cues,omitempty"`
}

type PacketInterval struct {
	Index                   int         `json:"index"`
	ID                      string      `json:"id,omitempty"`
	Sequence                *int64      `json:"sequence,omitempty"`
	Received                bool        `json:"received"`
	EndOfStream             bool        `json:"end_of_stream,omitempty"`
	DurationKnown           bool        `json:"duration_known"`
	DurationSamples         int64       `json:"duration_samples,omitempty"`
	TOCDurationSamples      int64       `json:"toc_duration_samples,omitempty"`
	DeclaredDurationSamples int64       `json:"declared_duration_samples,omitempty"`
	Raw                     SampleRange `json:"raw_range"`
	Playback                SampleRange `json:"playback_range"`
	Audible                 SampleRange `json:"audible_range"`
	EndTrimSamples          int64       `json:"end_trim_samples,omitempty"`
	AnchorSample            *int64      `json:"anchor_sample,omitempty"`
	Issues                  []Issue     `json:"issues,omitempty"`
	Text                    string      `json:"text,omitempty"`
	TextStatus              string      `json:"text_status"`
}

type MissingRange struct {
	Range             SampleRange `json:"playback_range"`
	Samples           int64       `json:"samples"`
	AfterPacketIndex  int         `json:"after_packet_index"`
	BeforePacketIndex int         `json:"before_packet_index,omitempty"`
	FromSequence      *int64      `json:"from_sequence,omitempty"`
	ToSequence        *int64      `json:"to_sequence,omitempty"`
	Kind              string      `json:"kind"`
}

type SequenceGap struct {
	AfterPacketIndex  int   `json:"after_packet_index"`
	BeforePacketIndex int   `json:"before_packet_index"`
	FromSequence      int64 `json:"from_sequence"`
	ToSequence        int64 `json:"to_sequence"`
}

type DuplicateRange struct {
	Range       SampleRange `json:"playback_range"`
	PacketIndex int         `json:"packet_index"`
	FirstIndex  int         `json:"first_index"`
	Reason      string      `json:"reason"`
}

type InvalidDuration struct {
	PacketIndex int    `json:"packet_index"`
	Value       int64  `json:"value,omitempty"`
	Reason      string `json:"reason"`
}

type Stats struct {
	PacketCount          int   `json:"packet_count"`
	ReceivedCount        int   `json:"received_count"`
	LostCount            int   `json:"lost_count"`
	LostSamples          int64 `json:"lost_samples"`
	DuplicateCount       int   `json:"duplicate_count"`
	MissingRangeCount    int   `json:"missing_range_count"`
	MissingSamples       int64 `json:"missing_samples"`
	SequenceGapCount     int   `json:"sequence_gap_count"`
	InvalidDurationCount int   `json:"invalid_duration_count"`
	CueCount             int   `json:"cue_count,omitempty"`
}

type Report struct {
	SampleRate       int               `json:"sample_rate"`
	PreSkip          int               `json:"pre_skip"`
	Intervals        []PacketInterval  `json:"intervals"`
	MissingRanges    []MissingRange    `json:"missing_ranges,omitempty"`
	SequenceGaps     []SequenceGap     `json:"sequence_gaps,omitempty"`
	Duplicates       []DuplicateRange  `json:"duplicates,omitempty"`
	InvalidDurations []InvalidDuration `json:"invalid_durations,omitempty"`
	Subtitles        []SyncedCue       `json:"subtitles,omitempty"`
	Stats            Stats             `json:"stats"`
}
