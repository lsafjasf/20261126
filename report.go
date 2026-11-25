package ac3select

// TrackSelection explains how a program segment should be used.
type TrackSelection struct {
	Category     string `json:"category"`
	Role         string `json:"role"`
	Label        string `json:"label"`
	IsMain       bool   `json:"isMain"`
	IsCommentary bool   `json:"isCommentary"`
	IsAnnotation bool   `json:"isAnnotation"`
	Chosen       bool   `json:"chosen"`
	SelectedAs   string `json:"selectedAs,omitempty"`
	Reason       string `json:"reason"`
}

// CouplingSummary aggregates block-zero coupling metadata over a segment.
type CouplingSummary struct {
	ParsedFrames     int      `json:"parsedFrames"`
	InUseFrames      int      `json:"inUseFrames"`
	InUse            bool     `json:"inUse"`
	AlwaysInUse      bool     `json:"alwaysInUse"`
	EverInUse        bool     `json:"everInUse"`
	CoupledChannels  []string `json:"coupledChannels,omitempty"`
	StartSubband     int      `json:"startSubband,omitempty"`
	EndSubband       int      `json:"endSubband,omitempty"`
	StartFrequencyHz int      `json:"startFrequencyHz,omitempty"`
	EndFrequencyHz   int      `json:"endFrequencyHz,omitempty"`
	BandCount        int      `json:"bandCount,omitempty"`
}

// ProgramSegment is a contiguous run of frames with one program identity and
// unchanged selection-critical channel configuration.
type ProgramSegment struct {
	Index             int             `json:"index"`
	StartOffset       int64           `json:"startOffset"`
	EndOffset         int64           `json:"endOffset"`
	Length            int64           `json:"lengthBytes"`
	StartFrameIndex   int             `json:"startFrameIndex"`
	FrameCount        int             `json:"frameCount"`
	StartSample       int64           `json:"startSample"`
	SampleCount       int64           `json:"sampleCount"`
	StartMS           float64         `json:"startMS"`
	EndMS             float64         `json:"endMS"`
	DurationMS        float64         `json:"durationMS"`
	SampleRate        int             `json:"sampleRate"`
	BitstreamMode     BitstreamMode   `json:"bsmod"`
	BitstreamModeName string          `json:"bsmodName"`
	ServiceRole       string          `json:"serviceRole"`
	ACMod             AudioCodingMode `json:"acmod"`
	ACModName         string          `json:"acmodName"`
	Layout            string          `json:"layout"`
	LayoutShort       string          `json:"layoutShort"`
	FullBandChannels  int             `json:"fullBandChannels"`
	LFEOn             bool            `json:"lfeon"`
	ChannelCount      int             `json:"channelCount"`
	Channels          []Channel       `json:"channels"`
	Coupling          CouplingSummary `json:"coupling"`
	Track             TrackSelection  `json:"track"`
}

// SegmentRef points to one selected program segment.
type SegmentRef struct {
	SegmentIndex int    `json:"segmentIndex"`
	FrameCount   int    `json:"frameCount"`
	Offset       int64  `json:"offset"`
	Role         string `json:"role"`
	Label        string `json:"label"`
	Reason       string `json:"reason"`
}

// Selection is the deterministic main/commentary choice for a stream.
type Selection struct {
	Policy           []string     `json:"policy"`
	Main             *SegmentRef  `json:"main,omitempty"`
	Commentary       *SegmentRef  `json:"commentary,omitempty"`
	AnnotationTracks []SegmentRef `json:"annotationTracks"`
	Ambiguous        bool         `json:"ambiguous"`
	Notes            []string     `json:"notes,omitempty"`
}

// Report is the complete result of scanning an AC-3 byte stream.
type Report struct {
	Bytes       int64            `json:"bytes"`
	FrameCount  int              `json:"frameCount"`
	BadCount    int              `json:"badFrameCount"`
	BadBytes    int64            `json:"badBytes"`
	SampleCount int64            `json:"sampleCount"`
	DurationMS  float64          `json:"durationMS"`
	Segments    []ProgramSegment `json:"segments"`
	BadFrames   []BadFrame       `json:"badFrames"`
	Frames      []Frame          `json:"frames,omitempty"`
	Selection   Selection        `json:"selection"`
}

// Programs returns the program segments. It is a semantic alias for Segments.
func (r Report) Programs() []ProgramSegment {
	return r.Segments
}

// TrackSelectionFor returns the segment's selection explanation.
func (s ProgramSegment) TrackSelectionFor() TrackSelection {
	return s.Track
}
