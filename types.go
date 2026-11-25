package ac3select

// SyncWord is the big-endian AC-3 synchronization word.
const SyncWord uint16 = 0x0B77

// SamplesPerFrame is the fixed number of audio samples in one AC-3 frame.
// Each frame contains six audio blocks of 256 samples.
const SamplesPerFrame int64 = 6 * 256

// BitstreamMode is the three-bit bsmod service description.
type BitstreamMode uint8

const (
	BSModeCompleteMain       BitstreamMode = 0
	BSModeMusicAndEffects    BitstreamMode = 1
	BSModeVisuallyImpaired   BitstreamMode = 2
	BSModeHearingImpaired    BitstreamMode = 3
	BSModeDialogue           BitstreamMode = 4
	BSModeCommentary         BitstreamMode = 5
	BSModeEmergency          BitstreamMode = 6
	BSModeVoiceOverOrKaraoke BitstreamMode = 7
)

// AudioCodingMode is the three-bit acmod channel arrangement.
type AudioCodingMode uint8

const (
	ACModDualMono AudioCodingMode = 0
	ACMod1_0      AudioCodingMode = 1
	ACMod2_0      AudioCodingMode = 2
	ACMod3_0      AudioCodingMode = 3
	ACMod2_1      AudioCodingMode = 4
	ACMod3_1      AudioCodingMode = 5
	ACMod2_2      AudioCodingMode = 6
	ACMod3_2      AudioCodingMode = 7
)

// Channel is one actual full-bandwidth or LFE channel in AC-3 bitstream order.
type Channel struct {
	Index       int    `json:"index"`
	Label       string `json:"label"`
	Name        string `json:"name"`
	Kind        string `json:"kind"`
	InCoupling  bool   `json:"inCoupling"`
	CouplingBit *int   `json:"couplingBit,omitempty"`
}

// Coupling describes the first audio block's standard coupling strategy.
type Coupling struct {
	AudioBlock      int       `json:"audioBlock"`
	StrategyPresent bool      `json:"strategyPresent"`
	InUse           bool      `json:"inUse"`
	CoupledChannels []string  `json:"coupledChannels"`
	Channels        []Channel `json:"channels,omitempty"`
	PhaseFlagsInUse bool      `json:"phaseFlagsInUse,omitempty"`
	StartSubband    int       `json:"startSubband,omitempty"`
	EndSubband      int       `json:"endSubband,omitempty"`
	StartFrequency  int       `json:"startFrequencyHz,omitempty"`
	EndFrequency    int       `json:"endFrequencyHz,omitempty"`
	BandCount       int       `json:"bandCount,omitempty"`
	BandStructure   []bool    `json:"bandStructure,omitempty"`
	BandSizes       []int     `json:"bandSizes,omitempty"`
	Note            string    `json:"note,omitempty"`
}

// CRCInfo contains the optional whole-frame CRC-16/ANSI check result.
type CRCInfo struct {
	Checked  bool   `json:"checked"`
	Valid    bool   `json:"valid"`
	Residual uint16 `json:"residual,omitempty"`
}

// Header contains the AC-3 BSI fields needed for program selection.
type Header struct {
	Offset            int64           `json:"offset"`
	SyncWord          uint16          `json:"syncWord"`
	CRC1              uint16          `json:"crc1"`
	SampleRateCode    int             `json:"sampleRateCode"`
	SampleRate        int             `json:"sampleRate"`
	FrameSizeCode     int             `json:"frameSizeCode"`
	FrameSize         int             `json:"frameSizeBytes"`
	BitstreamID       int             `json:"bitstreamID"`
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
	DolbySurroundMode *int            `json:"dolbySurroundMode,omitempty"`
	Coupling          Coupling        `json:"coupling"`
	CRC               *CRCInfo        `json:"crc,omitempty"`
}

// Frame is one successfully located and metadata-parsed AC-3 frame.
type Frame struct {
	Index  int    `json:"index"`
	Header Header `json:"header"`
}

// BadFrame describes a byte range that could not be accepted as an AC-3 frame.
type BadFrame struct {
	Index        int    `json:"index,omitempty"`
	Offset       int64  `json:"offset"`
	EndOffset    int64  `json:"endOffset"`
	Length       int64  `json:"length"`
	Kind         string `json:"kind"`
	Reason       string `json:"reason"`
	DeclaredSize int    `json:"declaredSizeBytes,omitempty"`
	RecoveredAt  int64  `json:"recoveredAt,omitempty"`
}
