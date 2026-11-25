package aache

import (
	"errors"
	"time"
)

const (
	// DefaultCoreSamplesPerFrame is the normal AAC core frame size.
	DefaultCoreSamplesPerFrame int64 = 1024

	// DefaultSBRFactor is the usual SBR output ratio: 2x the core rate.
	DefaultSBRFactor = 2
)

var (
	// ErrCoreRate means a stream or frame has no positive core sample rate.
	ErrCoreRate = errors.New("aache: core sample rate must be positive")
	// ErrCoreSamples means a frame has a non-positive core sample count.
	ErrCoreSamples = errors.New("aache: core sample count must be positive")
	// ErrOutputRate means an observed output rate is not positive.
	ErrOutputRate = errors.New("aache: output sample rate must be positive")
	// ErrSBRRate means an SBR output rate is not greater than the core rate.
	ErrSBRRate = errors.New("aache: SBR output rate must exceed core rate")
	// ErrPSWithoutSBR means PS was enabled while SBR is not effective.
	ErrPSWithoutSBR = errors.New("aache: PS requires effective SBR")
	// ErrUnevenRatio means the requested output rate cannot be derived from the
	// core frame size as a whole number of samples.
	ErrUnevenRatio = errors.New("aache: output rate and core rate do not yield whole samples")
	// ErrObservedSamples means an observed sample count is negative.
	ErrObservedSamples = errors.New("aache: observed output sample count must not be negative")
)

// SBRConfig is an SBR header update carried by an access unit. OutputRate is
// the SBR output sample rate; zero selects DefaultSBRFactor times the core rate.
type SBRConfig struct {
	Enabled    bool   `json:"enabled"`
	OutputRate uint32 `json:"outputRate,omitempty"`
}

// PSConfig is a parametric-stereo header update carried by an access unit.
type PSConfig struct {
	Enabled bool `json:"enabled"`
}

// ExtensionUpdate contains the extension headers present in one access unit.
// A nil SBR or PS member means "no update for that extension"; it never means
// "disable". Use a non-nil member with Enabled=false for an explicit reset.
type ExtensionUpdate struct {
	SBR *SBRConfig `json:"sbr,omitempty"`
	PS  *PSConfig  `json:"ps,omitempty"`
}

// SBRState is the effective SBR state at a frame. SourceFrame and
// EffectiveFrame are global frame indexes; -1 denotes stream/ASC initial state.
type SBRState struct {
	Enabled        bool   `json:"enabled"`
	OutputRate     uint32 `json:"outputRate,omitempty"`
	SourceFrame    int    `json:"sourceFrame"`
	EffectiveFrame int    `json:"effectiveFrame"`
}

// PSState is the effective PS state at a frame.
type PSState struct {
	Enabled        bool `json:"enabled"`
	SourceFrame    int  `json:"sourceFrame"`
	EffectiveFrame int  `json:"effectiveFrame"`
}

// ExtensionState is the effective SBR/PS state. It is updated only by frames
// that carry an extension header and otherwise persists unchanged.
type ExtensionState struct {
	SBR SBRState `json:"sbr"`
	PS  PSState  `json:"ps"`
}

// Active reports whether any extension affects the output.
func (s ExtensionState) Active() bool {
	return s.SBR.Enabled || s.PS.Enabled
}

// Frame is one AAC access unit. Zero CoreSamples and CoreRate inherit the
// stream defaults.
//
// Config is the in-band extension update carried by this frame, if any. Nil
// means that this frame has no SBR/PS header; it does not clear the previously
// effective configuration.
//
// OutputSamples and OutputRate are observations from a decoder or demuxer. A
// positive OutputSamples value is authoritative for the actual sample count and
// allows the timeline to expose a frame where the decoder did not apply the
// persisted configuration.
type Frame struct {
	CoreSamples   int64            `json:"coreSamples,omitempty"`
	CoreRate      uint32           `json:"coreRate,omitempty"`
	Config        *ExtensionUpdate `json:"config,omitempty"`
	OutputSamples int64            `json:"outputSamples,omitempty"`
	OutputRate    uint32           `json:"outputRate,omitempty"`
}

// Stream is a contiguous run of access units with one core time base.
type Stream struct {
	Name string `json:"name,omitempty"`

	CoreRate    uint32 `json:"coreRate"`
	CoreSamples int64  `json:"coreSamples,omitempty"`

	// Initial is configuration known before the first frame, typically from an
	// AudioSpecificConfig. It is effective at the first frame.
	Initial *ExtensionUpdate `json:"initial,omitempty"`

	// Inherit carries the previous stream's effective extension state across a
	// boundary. Set it when concatenating access units from the same encoded
	// stream. Independent sources should leave it false and supply Initial.
	Inherit bool `json:"inherit,omitempty"`

	// DeclaredOutputRate is an optional container-level output rate. It is used
	// for the frame-local comparison so that losing SBR shows up as a duration
	// jump even when the container rate itself does not change.
	DeclaredOutputRate uint32 `json:"declaredOutputRate,omitempty"`

	Frames []Frame `json:"frames"`
}

// FrameTiming is the repaired mapping for one access unit.
type FrameTiming struct {
	Index         int `json:"index"`
	StreamIndex   int `json:"streamIndex"`
	FrameInStream int `json:"frameInStream"`

	CoreStart    int64         `json:"coreStart"`
	CoreSamples  int64         `json:"coreSamples"`
	CoreRate     uint32        `json:"coreRate"`
	CoreDuration time.Duration `json:"coreDuration"`

	Config            ExtensionState `json:"config"`
	ConfigChanged     bool           `json:"configChanged"`
	ConfigSource      int            `json:"configSource"`
	ConfigEffectiveAt int            `json:"configEffectiveAt"`

	ExpectedOutputSamples int64  `json:"expectedOutputSamples"`
	ExpectedOutputRate    uint32 `json:"expectedOutputRate"`
	OutputSamples         int64  `json:"outputSamples"`
	OutputRate            uint32 `json:"outputRate"`
	OutputStart           int64  `json:"outputStart"`
	ObservedSamples       bool   `json:"observedSamples"`
	OutputMismatch        bool   `json:"outputMismatch"`

	// PresentationDuration is the exact frame duration rounded to nanoseconds
	// once. Start and End are cumulative rational positions rounded separately,
	// so their subtraction can differ by one nanosecond at fractional rates.
	PresentationDuration time.Duration `json:"duration"`
	Start                time.Duration `json:"start"`
	End                  time.Duration `json:"end"`
}

// Duration returns the presentation duration of the frame.
func (f FrameTiming) Duration() time.Duration {
	if f.PresentationDuration != 0 {
		return f.PresentationDuration
	}
	return f.End - f.Start
}

// StreamTiming summarises one input stream after mapping.
type StreamTiming struct {
	Name            string        `json:"name"`
	StartFrame      int           `json:"startFrame"`
	EndFrame        int           `json:"endFrame"`
	Start           time.Duration `json:"start"`
	End             time.Duration `json:"end"`
	OutputStart     int64         `json:"outputStart"`
	OutputSamples   int64         `json:"outputSamples"`
	DeclaredOutRate uint32        `json:"declaredOutputRate,omitempty"`
}

// ConfigGap is the first frame whose per-frame configuration lookup is empty
// even though a previously received in-band SBR/PS header is still effective.
// It is the first place a frame-local mapper "loses" configuration.
type ConfigGap struct {
	Frame         int            `json:"frame"`
	StreamIndex   int            `json:"streamIndex"`
	FrameInStream int            `json:"frameInStream"`
	LastConfig    int            `json:"lastConfigFrame"`
	Effective     ExtensionState `json:"effective"`
	Reason        string         `json:"reason"`
}

// OutputMismatch records the first frame where observed output disagrees with
// the persisted configuration. This is evidence of an actual decoder or editor
// configuration loss rather than merely a missing in-band header.
type OutputMismatch struct {
	Frame           int            `json:"frame"`
	StreamIndex     int            `json:"streamIndex"`
	FrameInStream   int            `json:"frameInStream"`
	Effective       ExtensionState `json:"effective"`
	ExpectedSamples int64          `json:"expectedSamples"`
	ObservedSamples int64          `json:"observedSamples"`
	ExpectedRate    uint32         `json:"expectedRate"`
	ObservedRate    uint32         `json:"observedRate"`
	Reason          string         `json:"reason"`
}

// Timeline is a complete, monotonic presentation map.
type Timeline struct {
	Policy              ExtensionPolicy `json:"policy"`
	Streams             []StreamTiming  `json:"streams"`
	Frames              []FrameTiming   `json:"frames"`
	FirstConfigGap      *ConfigGap      `json:"firstConfigGap,omitempty"`
	FirstOutputMismatch *OutputMismatch `json:"firstOutputMismatch,omitempty"`
	CoreSamples         int64           `json:"coreSamples"`
	OutputSamples       int64           `json:"outputSamples"`
	Duration            time.Duration   `json:"duration"`
}

// ExtensionPolicy selects how missing per-frame extension headers are handled.
type ExtensionPolicy string

const (
	// PersistExtensions carries the last SBR/PS header forward. This is the
	// correct AAC-HE interpretation.
	PersistExtensions ExtensionPolicy = "persist"
	// FrameLocalExtensions discards extension state on frames without an update.
	// It exists to reproduce and locate the editor bug.
	FrameLocalExtensions ExtensionPolicy = "frame-local"
)
