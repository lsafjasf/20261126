package eac3

import (
	"fmt"
	"sort"
	"strings"
)

// SubstreamType 对应 E-AC-3 syncframe 中的 strmtyp。
type SubstreamType uint8

const (
	SubstreamIndependent SubstreamType = 0
	SubstreamDependent   SubstreamType = 1
	// SubstreamAC3Converted 表示可走 AC-3 兼容路径的帧。
	SubstreamAC3Converted SubstreamType = 2
	SubstreamReserved     SubstreamType = 3
)

func (t SubstreamType) String() string {
	switch t {
	case SubstreamIndependent:
		return "independent"
	case SubstreamDependent:
		return "dependent"
	case SubstreamAC3Converted:
		return "ac3-converted"
	default:
		return "reserved"
	}
}

// ProgramMode 是播放器最终选择的节目呈现图，而不是线上 strmtyp。
type ProgramMode string

const (
	ModeIndependent ProgramMode = "independent"
	ModeAssociated  ProgramMode = "associated"
	ModeMixed       ProgramMode = "mixed"
)

// ChannelStructure 只描述解码前必须固定的声道拓扑。
// ChannelMap 存在时优先使用定制 chanmap；否则用 Acmod+LFE。
type ChannelStructure struct {
	Acmod             uint8    `json:"acmod"`
	LFEOn             bool     `json:"lfe_on"`
	ChannelMapPresent bool     `json:"channel_map_present,omitempty"`
	ChannelMap        uint16   `json:"channel_map,omitempty"`
	Channels          int      `json:"channels"`
	Labels            []string `json:"labels,omitempty"`
}

func (c ChannelStructure) identity() string {
	if c.ChannelMapPresent {
		return fmt.Sprintf("map:%04x:ch:%d:%s", c.ChannelMap, c.Channels, strings.Join(c.Labels, ","))
	}
	return fmt.Sprintf("acmod:%d:lfe:%t:ch:%d:%s", c.Acmod, c.LFEOn, c.Channels, strings.Join(c.Labels, ","))
}

// DRCState 是稳定的 DRC 配置身份。逐 block 变化的 dynrng 增益值不应
// 直接触发解码器重建，只应更新运行时增益平滑器。
type DRCState struct {
	Present       bool   `json:"present"`
	ConfigID      string `json:"config_id,omitempty"`
	Mode          string `json:"mode,omitempty"`
	Profile       string `json:"profile,omitempty"`
	DialNormDB    int    `json:"dialnorm_db,omitempty"`
	TargetLevelDB int    `json:"target_level_db,omitempty"`
	MetadataEpoch uint64 `json:"metadata_epoch,omitempty"`
}

func (d DRCState) identity() string {
	if d.ConfigID != "" {
		return "id:" + d.ConfigID
	}
	return fmt.Sprintf("present:%t/mode:%s/profile:%s/dial:%d/target:%d/epoch:%d",
		d.Present, d.Mode, d.Profile, d.DialNormDB, d.TargetLevelDB, d.MetadataEpoch)
}

// BoundaryState 描述 PCM 帧边界节奏和压缩帧字节边界。
type BoundaryState struct {
	BlocksPerFrame  uint8 `json:"blocks_per_frame"`
	SamplesPerFrame int   `json:"samples_per_frame"`
	FrameSizeBytes  int   `json:"frame_size_bytes,omitempty"`
}

func (b BoundaryState) identity() string {
	return fmt.Sprintf("blocks:%d/samples:%d/coded_bytes:%d", b.BlocksPerFrame, b.SamplesPerFrame, b.FrameSizeBytes)
}

func (b BoundaryState) timelineIdentity() string {
	return fmt.Sprintf("blocks:%d/samples:%d", b.BlocksPerFrame, b.SamplesPerFrame)
}

// ProgramSpec 是构造解码器、输出设备、混音器和 DRC 链所需的最小状态。
type ProgramSpec struct {
	SampleRateHz int              `json:"sample_rate_hz"`
	Channel      ChannelStructure `json:"channel"`
	DRC          DRCState         `json:"drc"`
	Boundary     BoundaryState    `json:"boundary"`
}

func (s ProgramSpec) identity() string {
	return strings.Join([]string{
		fmt.Sprintf("sr:%d", s.SampleRateHz),
		"ch:" + s.Channel.identity(),
		"drc:" + s.DRC.identity(),
		"boundary:" + s.Boundary.identity(),
	}, "|")
}

// FrameInfo 是解码前从 syncframe/BSI 或外部节目描述得到的一帧信息。
type FrameInfo struct {
	ProgramID   string        `json:"program_id,omitempty"`
	Type        SubstreamType `json:"type"`
	SubstreamID uint8         `json:"substream_id"`
	Spec        ProgramSpec   `json:"spec"`
	ByteOffset  int64         `json:"byte_offset,omitempty"`
}

// SwitchRequest 指定要进入的节目图。Associated/Mixed 不需要调用方手工
// 展开 anchor；状态图会根据 substreamid 找到独立主 substream。
type SwitchRequest struct {
	StateID       string         `json:"state_id,omitempty"`
	ProgramID     string         `json:"program_id"`
	Mode          ProgramMode    `json:"mode"`
	AnchorID      *uint8         `json:"anchor_id,omitempty"`
	DependentRefs []SubstreamRef `json:"dependent_refs,omitempty"`
	Reason        string         `json:"reason,omitempty"`
	ForceResync   bool           `json:"force_resync,omitempty"`
}

// SubstreamRef 用 strmtyp + substreamid 唯一标识图中的一个 substream 角色。
// independent 0 与关联它的 dependent 0 可以有相同 ID，但 Type 不同。
type SubstreamRef struct {
	Type SubstreamType `json:"type"`
	ID   uint8         `json:"id"`
	Slot uint8         `json:"slot,omitempty"`
}

// State 是状态图中的一个不可变节目状态节点。
type State struct {
	ID               string         `json:"id"`
	ProgramID        string         `json:"program_id"`
	Mode             ProgramMode    `json:"mode"`
	Anchor           *SubstreamRef  `json:"anchor,omitempty"`
	DecodeSubstreams []SubstreamRef `json:"decode_substreams"`
	RenderSubstreams []SubstreamRef `json:"render_substreams"`
	Spec             ProgramSpec    `json:"spec"`
	TopologyVersion  uint64         `json:"topology_version"`
}

func (s State) key() string {
	return strings.Join([]string{
		s.ProgramID,
		string(s.Mode),
		"anchor:" + refKeyPtr(s.Anchor),
		"decode:" + refsKey(s.DecodeSubstreams),
		"render:" + refsKey(s.RenderSubstreams),
		s.Spec.identity(),
	}, "|")
}

func refKey(r SubstreamRef) string {
	return fmt.Sprintf("%d:%d:%d", r.Type, r.ID, r.Slot)
}

func refKeyPtr(r *SubstreamRef) string {
	if r == nil {
		return "<none>"
	}
	return refKey(*r)
}

func refsKey(refs []SubstreamRef) string {
	keys := make([]string, 0, len(refs))
	for _, ref := range refs {
		keys = append(keys, refKey(ref))
	}
	sort.Strings(keys)
	return strings.Join(keys, ",")
}
