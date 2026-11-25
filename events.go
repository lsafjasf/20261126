package eac3

// 稳定原因码。测试和上层播放器可依赖这些字符串，而不是解析中文文案。
const (
	ReasonInitialPrepare        = "INITIAL_PREPARE"
	ReasonNoChange              = "NO_CHANGE"
	ReasonUserSelection         = "USER_PROGRAM_SELECTION"
	ReasonProgramChanged        = "PROGRAM_CHANGED"
	ReasonModeChanged           = "PROGRAM_MODE_CHANGED"
	ReasonSubstreamChanged      = "SUBSTREAM_SELECTION_CHANGED"
	ReasonAnchorResolved        = "ANCHOR_SUBSTREAM_RESOLVED"
	ReasonAnchorMissing         = "ANCHOR_SUBSTREAM_MISSING"
	ReasonDependentOnly         = "DEPENDENT_PRESENTATION_REQUIRES_ANCHOR"
	ReasonSampleRateChanged     = "SAMPLE_RATE_CHANGED"
	ReasonChannelChanged        = "CHANNEL_STRUCTURE_CHANGED"
	ReasonDRCChanged            = "DRC_CONFIG_CHANGED"
	ReasonFrameCadenceChanged   = "FRAME_CADENCE_CHANGED"
	ReasonCodedBoundaryChanged  = "CODED_FRAME_BOUNDARY_CHANGED"
	ReasonBoundaryDiscontinuity = "FRAME_BOUNDARY_DISCONTINUITY"
	ReasonTopologyChanged       = "PROGRAM_TOPOLOGY_CHANGED"
	ReasonResyncRequired        = "RESYNC_REQUIRED"
)

// EventKind 表示必须重建的受控子系统。
type EventKind string

const (
	EventInitialize   EventKind = "session.initialize"
	EventBoundaryGate EventKind = "boundary.gate"
	EventFrameResync  EventKind = "frame.resync"
	EventProgramGraph EventKind = "program_graph.rebuild"
	EventDecoder      EventKind = "decoder.rebuild"
	EventMixer        EventKind = "mixer.rebuild"
	EventOutput       EventKind = "output.rebuild"
	EventDRC          EventKind = "drc.rebuild"
	EventRamp         EventKind = "output.ramp"
)

type Severity string

const (
	SeverityInfo Severity = "info"
	SeveritySoft Severity = "soft"
	SeverityHard Severity = "hard"
)

// Reason 是一次字段级或拓扑级变化的审计记录。
type Reason struct {
	Code    string `json:"code"`
	Field   string `json:"field,omitempty"`
	Message string `json:"message"`
	Old     string `json:"old,omitempty"`
	New     string `json:"new,omitempty"`
}

// RebuildEvent 是播放器执行层应按 Order 顺序执行的受控动作。
type RebuildEvent struct {
	Order    int       `json:"order"`
	Kind     EventKind `json:"kind"`
	Severity Severity  `json:"severity"`
	Reasons  []Reason  `json:"reasons"`
	Actions  []string  `json:"actions"`
	Flush    bool      `json:"flush,omitempty"`
	Drain    bool      `json:"drain,omitempty"`
	Ramp     bool      `json:"ramp,omitempty"`
}

// BoundaryDecision 说明切换应卡在哪个边界执行。
type BoundaryDecision struct {
	WaitForSyncword      bool   `json:"wait_for_syncword"`
	CompleteCurrentFrame bool   `json:"complete_current_frame"`
	ResetCadence         bool   `json:"reset_cadence"`
	ReseedFrameSize      bool   `json:"reseed_frame_size"`
	DropPartialBytes     bool   `json:"drop_partial_bytes"`
	Reason               string `json:"reason"`
}

// Switch 是状态图中的一条带标签边，也是一次切换的完整交付物。
type Switch struct {
	Sequence int              `json:"sequence"`
	FromID   string           `json:"from_id,omitempty"`
	ToID     string           `json:"to_id"`
	Request  SwitchRequest    `json:"request"`
	NoOp     bool             `json:"noop"`
	Reasons  []Reason         `json:"reasons"`
	Events   []RebuildEvent   `json:"events"`
	Boundary BoundaryDecision `json:"boundary"`
}

func reason(code, msg, oldValue, newValue string) Reason {
	return Reason{Code: code, Message: msg, Old: oldValue, New: newValue}
}
