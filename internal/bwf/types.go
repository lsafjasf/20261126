package bwf

import "time"

type Chunk struct {
	ID           string `json:"id"`
	Offset       int64  `json:"offset"`
	DataOffset   int64  `json:"dataOffset"`
	DeclaredSize uint64 `json:"declaredSize"`
	Size         uint64 `json:"size"`
	Placeholder  bool   `json:"placeholder,omitempty"`
	Truncated    bool   `json:"truncated,omitempty"`
}

type Container struct {
	Kind         string  `json:"kind"`
	Form         string  `json:"form"`
	DeclaredSize uint64  `json:"declaredSize"`
	FileSize     int64   `json:"fileSize"`
	Chunks       []Chunk `json:"chunks"`
}

type Fmt struct {
	FormatTag     uint16 `json:"formatTag"`
	Channels      uint16 `json:"channels"`
	SampleRate    uint32 `json:"sampleRate"`
	ByteRate      uint32 `json:"byteRate"`
	BlockAlign    uint16 `json:"blockAlign"`
	BitsPerSample uint16 `json:"bitsPerSample"`
	ValidBits     uint16 `json:"validBits,omitempty"`
	SubFormatTag  uint16 `json:"subFormatTag,omitempty"`
	Extensible    bool   `json:"extensible,omitempty"`
	PayloadSize   uint64 `json:"payloadSize"`
	ChannelMask   uint32 `json:"channelMask,omitempty"`
	Compressed    bool   `json:"compressed,omitempty"`
}

type BEXT struct {
	Description       string `json:"description"`
	Originator        string `json:"originator"`
	OriginatorRef     string `json:"originatorReference"`
	OriginationDate   string `json:"originationDate"`
	OriginationTime   string `json:"originationTime"`
	TimeReferenceLow  uint32 `json:"timeReferenceLow"`
	TimeReferenceHigh uint32 `json:"timeReferenceHigh"`
	TimeReference     uint64 `json:"timeReference"`
	Version           uint16 `json:"version"`
	UMID              string `json:"umid,omitempty"`
	PayloadSize       uint64 `json:"payloadSize"`
	Short             bool   `json:"short,omitempty"`
}

type IXMLField struct {
	Path  string `json:"path"`
	Name  string `json:"name"`
	Value string `json:"value"`
}

type Timecode struct {
	Text      string `json:"text"`
	Hours     int64  `json:"hours"`
	Minutes   int64  `json:"minutes"`
	Seconds   int64  `json:"seconds"`
	Frames    int64  `json:"frames"`
	DropFrame bool   `json:"dropFrame"`
}

type Rate struct {
	Num  int64  `json:"num"`
	Den  int64  `json:"den"`
	Text string `json:"text"`
}

type IXML struct {
	Root                string      `json:"root"`
	PayloadSize         uint64      `json:"payloadSize"`
	Fields              []IXMLField `json:"fields"`
	Project             string      `json:"project,omitempty"`
	Tape                string      `json:"tape,omitempty"`
	Scene               string      `json:"scene,omitempty"`
	Take                string      `json:"take,omitempty"`
	Note                string      `json:"note,omitempty"`
	TimecodeRate        string      `json:"timecodeRate,omitempty"`
	TimecodeFlag        string      `json:"timecodeFlag,omitempty"`
	TimecodeSampleRate  uint32      `json:"timecodeSampleRate,omitempty"`
	StartTime           *Timecode   `json:"startTime,omitempty"`
	DirectTimeReference *uint64     `json:"directTimeReference,omitempty"`
	OmittedFile         string      `json:"omittedFile,omitempty"`
}

type ProjectMeta struct {
	Path             string   `json:"path"`
	Name             string   `json:"name,omitempty"`
	SampleRate       uint32   `json:"sampleRate,omitempty"`
	TimecodeRate     string   `json:"timecodeRate,omitempty"`
	StartTimecode    string   `json:"startTimecode,omitempty"`
	DropFrame        *bool    `json:"dropFrame,omitempty"`
	StartSample      *int64   `json:"startSample,omitempty"`
	TimeReference    *uint64  `json:"timeReference,omitempty"`
	DurationSamples  *int64   `json:"durationSamples,omitempty"`
	DurationSeconds  *float64 `json:"durationSeconds,omitempty"`
	ExpectedFileName string   `json:"expectedFileName,omitempty"`
	Notes            string   `json:"notes,omitempty"`
}

type Audio struct {
	DataChunks      []string `json:"dataChunks"`
	DataBytes       uint64   `json:"dataBytes"`
	Frames          uint64   `json:"frames"`
	PartialBytes    uint64   `json:"partialBytes,omitempty"`
	DurationSeconds float64  `json:"durationSeconds"`
	DurationSamples *uint64  `json:"durationSamples,omitempty"`
}

type Finding struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
}

type StartSource struct {
	ID         string `json:"id"`
	Kind       string `json:"kind"`
	Start      int64  `json:"startSample"`
	SampleRate uint32 `json:"sampleRate"`
	Rate       string `json:"rate"`
	Confidence string `json:"confidence"`
	Evidence   string `json:"evidence"`
}

type Comparison struct {
	A              string  `json:"a"`
	B              string  `json:"b"`
	DeltaSamples   int64   `json:"deltaSamples"`
	DeltaSeconds   float64 `json:"deltaSeconds"`
	FrameSamples   uint64  `json:"frameSamples,omitempty"`
	WithinOneFrame bool    `json:"withinOneFrame"`
	Meaning        string  `json:"meaning"`
}

type Hypothesis struct {
	Rank       int      `json:"rank"`
	Confidence string   `json:"confidence"`
	Statement  string   `json:"statement"`
	Evidence   []string `json:"evidence,omitempty"`
}

type Report struct {
	File        string        `json:"file"`
	GeneratedAt time.Time     `json:"generatedAt"`
	Container   Container     `json:"container"`
	Fmt         *Fmt          `json:"fmt,omitempty"`
	Audio       Audio         `json:"audio"`
	BEXT        *BEXT         `json:"bext,omitempty"`
	IXML        *IXML         `json:"ixml,omitempty"`
	Project     *ProjectMeta  `json:"project,omitempty"`
	Starts      []StartSource `json:"starts"`
	Comparisons []Comparison  `json:"comparisons"`
	Findings    []Finding     `json:"findings"`
	Semantics   []string      `json:"semanticNotes"`
	Hypotheses  []Hypothesis  `json:"hypotheses"`
	Undecidable []string      `json:"undecidable"`
}
