package eac3

import (
	"errors"
	"fmt"
	"strings"
)

func (g *Graph) resolve(req SwitchRequest) (*State, error) {
	if req.StateID != "" {
		if st, ok := g.States[req.StateID]; ok {
			return st, nil
		}
		return nil, fmt.Errorf("%w: id=%s", ErrStateNotFound, req.StateID)
	}
	if req.ProgramID == "" {
		return nil, fmt.Errorf("%w: empty program id", ErrStateNotFound)
	}
	mode := req.Mode
	if mode == "" {
		mode = ModeIndependent
	}
	if mode != ModeIndependent && mode != ModeAssociated && mode != ModeMixed {
		return nil, fmt.Errorf("%w: %q", ErrInvalidMode, mode)
	}

	var matches []*State
	for _, id := range g.StateOrder {
		st := g.States[id]
		if st.ProgramID != req.ProgramID || st.Mode != mode {
			continue
		}
		if req.AnchorID != nil && (st.Anchor == nil || st.Anchor.ID != *req.AnchorID) {
			continue
		}
		if len(req.DependentRefs) > 0 && !containsAllRefs(st.DecodeSubstreams, req.DependentRefs) {
			continue
		}
		matches = append(matches, st)
	}
	if len(matches) == 0 {
		return nil, fmt.Errorf("%w: program=%s mode=%s", ErrStateNotFound, req.ProgramID, mode)
	}
	if len(matches) > 1 {
		return nil, fmt.Errorf("%w: program=%s mode=%s has %d states; set state_id", ErrAmbiguousTarget, req.ProgramID, mode, len(matches))
	}
	return matches[0], nil
}

func containsAllRefs(haystack, needles []SubstreamRef) bool {
	available := make(map[string]bool, len(haystack))
	for _, ref := range haystack {
		available[refKey(ref)] = true
	}
	for _, ref := range needles {
		if !available[refKey(ref)] {
			return false
		}
	}
	return true
}

// Switch 在状态图上增加一条带原因和重建事务的边，并把目标置为当前状态。
func (g *Graph) Switch(req SwitchRequest) (Switch, error) {
	if g == nil {
		return Switch{}, errors.New("eac3: nil graph")
	}
	target, err := g.resolve(req)
	if err != nil {
		return Switch{}, err
	}

	sw := Switch{Sequence: len(g.Transitions) + 1, Request: req, ToID: target.ID}
	if current := g.Current(); current != nil {
		sw.FromID = current.ID
	}

	if current := g.Current(); current != nil && current.ID == target.ID && !req.ForceResync {
		sw.NoOp = true
		sw.Reasons = []Reason{reason(ReasonNoChange, "目标节目状态与当前状态相同，不产生重建", target.ID, target.ID)}
		sw.Boundary = BoundaryDecision{Reason: "状态相同，不执行重建，也不请求额外边界动作"}
		g.Transitions = append(g.Transitions, sw)
		return sw, nil
	}

	sw.Reasons = buildReasons(g.Current(), target, req)
	sw.Boundary = buildBoundary(g.Current(), target, req)
	sw.Events = buildEvents(g.Current(), target, req, sw.Reasons, sw.Boundary)

	g.CurrentID = target.ID
	g.Transitions = append(g.Transitions, sw)
	return sw, nil
}
func buildReasons(from *State, to *State, req SwitchRequest) []Reason {
	var reasons []Reason
	if from == nil {
		message := "首个节目状态：在第一个有效 E-AC-3 syncframe 后初始化解码前图"
		if req.Reason != "" {
			message += "；调用方说明：" + req.Reason
		}
		reasons = append(reasons, reason(ReasonInitialPrepare, message, "", to.ID))
		return reasons
	}

	userMessage := "播放器选择了新的节目/substream 呈现图"
	if req.Reason != "" {
		userMessage += "：" + req.Reason
	}
	reasons = append(reasons, reason(ReasonUserSelection, userMessage, from.ID, to.ID))

	if from.ProgramID != to.ProgramID {
		reasons = append(reasons, reason(ReasonProgramChanged,
			"跨节目切换，节目身份和可用 substream 拓扑均已改变", from.ProgramID, to.ProgramID))
	}
	if from.Mode != to.Mode {
		reasons = append(reasons, reason(ReasonModeChanged,
			"节目呈现模式变化，必须重建 substream 混音图", string(from.Mode), string(to.Mode)))
	}
	if to.Mode != ModeIndependent && to.Anchor != nil {
		reasons = append(reasons, reason(ReasonAnchorResolved,
			"关联/混合图已绑定 matching substreamid 的 independent anchor，dependent 不会被裸渲染",
			refKeyPtr(from.Anchor), refKeyPtr(to.Anchor)))
	}
	if refsKey(from.DecodeSubstreams) != refsKey(to.DecodeSubstreams) ||
		refsKey(from.RenderSubstreams) != refsKey(to.RenderSubstreams) ||
		refKeyPtr(from.Anchor) != refKeyPtr(to.Anchor) {
		reasons = append(reasons, reason(ReasonSubstreamChanged,
			"独立/关联 substream 编组或渲染路由变化",
			fmt.Sprintf("decode=[%s]; render=[%s]", refsKey(from.DecodeSubstreams), refsKey(from.RenderSubstreams)),
			fmt.Sprintf("decode=[%s]; render=[%s]", refsKey(to.DecodeSubstreams), refsKey(to.RenderSubstreams))))
	}
	if from.ProgramID == to.ProgramID && from.TopologyVersion != to.TopologyVersion && to.TopologyVersion != 0 {
		reasons = append(reasons, reason(ReasonTopologyChanged,
			"节目 substream 拓扑版本变化",
			fmt.Sprint(from.TopologyVersion), fmt.Sprint(to.TopologyVersion)))
	}
	if from.Spec.SampleRateHz != to.Spec.SampleRateHz {
		reasons = append(reasons, reason(ReasonSampleRateChanged,
			"采样率变化，旧 PCM 采样时钟和输出设备不能继续使用",
			fmt.Sprintf("%d Hz", from.Spec.SampleRateHz), fmt.Sprintf("%d Hz", to.Spec.SampleRateHz)))
	}
	if from.Spec.Channel.identity() != to.Spec.Channel.identity() {
		reasons = append(reasons, reason(ReasonChannelChanged,
			"声道结构或定制声道映射变化，混音器/输出线路必须重建",
			channelBrief(from.Spec.Channel), channelBrief(to.Spec.Channel)))
	}
	if from.Spec.DRC.identity() != to.Spec.DRC.identity() {
		reasons = append(reasons, reason(ReasonDRCChanged,
			"DRC 配置身份变化，增益曲线和平滑状态必须重建；逐 block 增益值不应触发整解码器重建",
			drcBrief(from.Spec.DRC), drcBrief(to.Spec.DRC)))
	}
	if from.Spec.Boundary.timelineIdentity() != to.Spec.Boundary.timelineIdentity() {
		reasons = append(reasons, reason(ReasonFrameCadenceChanged,
			"每帧 PCM 样本数或 audio block 数变化，帧边界栅格必须重建",
			boundaryBrief(from.Spec.Boundary), boundaryBrief(to.Spec.Boundary)))
	}
	if from.Spec.Boundary.FrameSizeBytes != to.Spec.Boundary.FrameSizeBytes {
		reasons = append(reasons, reason(ReasonCodedBoundaryChanged,
			"压缩帧 frmsiz 变化，解析器下一字节边界必须按目标帧头重新播种",
			fmt.Sprintf("%d bytes", from.Spec.Boundary.FrameSizeBytes),
			fmt.Sprintf("%d bytes", to.Spec.Boundary.FrameSizeBytes)))
	}
	if req.ForceResync {
		reasons = append(reasons, reason(ReasonResyncRequired,
			"调用方要求强制重新同步，丢弃不完整字节并从 syncword 开始", "", ""))
	}
	return reasons
}

func channelBrief(c ChannelStructure) string {
	if c.ChannelMapPresent {
		return fmt.Sprintf("%dch chanmap=0x%04x labels=[%s]", c.Channels, c.ChannelMap, strings.Join(c.Labels, ","))
	}
	return fmt.Sprintf("%dch acmod=%d lfe=%t labels=[%s]", c.Channels, c.Acmod, c.LFEOn, strings.Join(c.Labels, ","))
}

func drcBrief(d DRCState) string {
	if d.ConfigID != "" {
		return "config=" + d.ConfigID
	}
	return fmt.Sprintf("mode=%s profile=%s dialnorm=%d target=%d epoch=%d", d.Mode, d.Profile, d.DialNormDB, d.TargetLevelDB, d.MetadataEpoch)
}

func boundaryBrief(b BoundaryState) string {
	return fmt.Sprintf("%d samples/frame (%d blocks), coded=%d bytes", b.SamplesPerFrame, b.BlocksPerFrame, b.FrameSizeBytes)
}

func hasReason(reasons []Reason, code string) bool {
	for _, r := range reasons {
		if r.Code == code {
			return true
		}
	}
	return false
}

func buildBoundary(from *State, to *State, req SwitchRequest) BoundaryDecision {
	b := BoundaryDecision{
		WaitForSyncword:      true,
		CompleteCurrentFrame: from != nil,
		Reason:               "提交点固定为完整 E-AC-3 syncframe 边界，避免在 PCM block 中间改写解码器/输出格式",
	}
	if from == nil {
		b.CompleteCurrentFrame = false
		b.ResetCadence = true
		b.ReseedFrameSize = true
		return b
	}
	if from.Spec.Boundary.timelineIdentity() != to.Spec.Boundary.timelineIdentity() || req.ForceResync {
		b.ResetCadence = true
	}
	if from.Spec.Boundary.FrameSizeBytes != to.Spec.Boundary.FrameSizeBytes || req.ForceResync {
		b.ReseedFrameSize = true
	}
	if req.ForceResync {
		b.DropPartialBytes = true
	}
	return b
}

func buildEvents(from *State, to *State, req SwitchRequest, reasons []Reason, boundary BoundaryDecision) []RebuildEvent {
	var events []RebuildEvent
	add := func(kind EventKind, severity Severity, eventReasons []Reason, actions ...string) {
		if len(eventReasons) == 0 {
			return
		}
		events = append(events, RebuildEvent{
			Order:    len(events) + 1,
			Kind:     kind,
			Severity: severity,
			Reasons:  append([]Reason(nil), eventReasons...),
			Actions:  actions,
		})
	}
	only := func(codes ...string) []Reason {
		var out []Reason
		for _, r := range reasons {
			for _, code := range codes {
				if r.Code == code {
					out = append(out, r)
					break
				}
			}
		}
		return out
	}

	hardReasons := only(ReasonSampleRateChanged, ReasonChannelChanged, ReasonFrameCadenceChanged, ReasonModeChanged, ReasonSubstreamChanged)
	if from == nil {
		add(EventInitialize, SeverityHard, reasons,
			"创建受控 E-AC-3 节目会话",
			"初始化帧解析器、节目拓扑、解码器占位、DRC 状态和输出格式约束")
	}
	if from != nil && len(hardReasons) > 0 {
		add(EventRamp, SeveritySoft, hardReasons,
			"在当前帧末尾执行 2-5 ms 指数淡出，避免格式切换边缘产生阶跃爆音")
	}

	gateReasons := only(ReasonInitialPrepare, ReasonUserSelection)
	add(EventBoundaryGate, SeverityInfo, gateReasons,
		"停止向旧图提交新的访问单元",
		"完成当前 syncframe 内的 audio block 渲染",
		"仅在下一个 0x0B77 syncword 处提交切换")

	resyncReasons := only(ReasonFrameCadenceChanged, ReasonCodedBoundaryChanged, ReasonResyncRequired)
	if from == nil {
		resyncReasons = only(ReasonInitialPrepare)
	}
	if len(resyncReasons) > 0 {
		actions := []string{"从目标 syncword 重新建立帧解析器"}
		if boundary.ReseedFrameSize {
			actions = append(actions, "使用目标 frmsiz 重新计算下一字节边界")
		}
		if boundary.ResetCadence {
			actions = append(actions, fmt.Sprintf("把 PCM 帧栅格重置为 %d samples/frame", to.Spec.Boundary.SamplesPerFrame))
		}
		if boundary.DropPartialBytes {
			actions = append(actions, "丢弃 syncword 前的不完整残留字节")
		}
		resyncSeverity := SeveritySoft
		if hasReason(reasons, ReasonFrameCadenceChanged) || req.ForceResync || from == nil {
			resyncSeverity = SeverityHard
		}
		add(EventFrameResync, resyncSeverity, resyncReasons, actions...)
	}

	graphReasons := only(ReasonProgramChanged, ReasonModeChanged, ReasonSubstreamChanged, ReasonAnchorResolved, ReasonTopologyChanged, ReasonInitialPrepare)
	if from == nil {
		graphReasons = only(ReasonInitialPrepare)
	}
	if len(graphReasons) > 0 {
		actions := graphActions(to)
		add(EventProgramGraph, SeverityHard, graphReasons, actions...)
	}

	sampleReasons := only(ReasonSampleRateChanged, ReasonInitialPrepare)
	if from == nil {
		sampleReasons = only(ReasonInitialPrepare)
	}
	if len(sampleReasons) > 0 {
		add(EventDecoder, SeverityHard, sampleReasons,
			"flush 旧解码器延迟窗口和残留 PCM",
			fmt.Sprintf("按 %d Hz 目标帧头重建 E-AC-3 解码器", to.Spec.SampleRateHz))
	}

	mixerReasons := only(ReasonChannelChanged, ReasonModeChanged, ReasonSubstreamChanged, ReasonInitialPrepare)
	if from == nil {
		mixerReasons = only(ReasonInitialPrepare)
	}
	if len(mixerReasons) > 0 {
		add(EventMixer, SeverityHard, mixerReasons,
			"重建输入 substream 声道映射、替代声道规则和输出 downmix 矩阵",
			fmt.Sprintf("目标结构：%s", channelBrief(to.Spec.Channel)))
	}

	drcReasons := only(ReasonDRCChanged)
	if len(drcReasons) > 0 {
		add(EventDRC, SeveritySoft, drcReasons,
			"替换 DRC 配置并将增益平滑器置为中性/目标初始值",
			"保留 PCM 输出设备；逐 block dynrng 更新继续走无重建快路径")
	}

	outputReasons := only(ReasonSampleRateChanged, ReasonChannelChanged, ReasonInitialPrepare)
	if from == nil {
		outputReasons = only(ReasonInitialPrepare)
	}
	if len(outputReasons) > 0 {
		ev := RebuildEvent{
			Kind:     EventOutput,
			Severity: SeverityHard,
			Reasons:  append([]Reason(nil), outputReasons...),
			Actions: []string{
				"在帧边界 pause 输出",
				"排空或丢弃旧采样率/旧声道布局的 residual PCM",
				fmt.Sprintf("重建输出格式：%d Hz, %d ch", to.Spec.SampleRateHz, to.Spec.Channel.Channels),
				"preroll 目标首帧后再 start",
			},
			Flush: true,
			Drain: true,
		}
		events = append(events, ev)
	}

	if from != nil && len(hardReasons) > 0 {
		events = append(events, RebuildEvent{
			Kind:     EventRamp,
			Severity: SeveritySoft,
			Reasons:  append([]Reason(nil), hardReasons...),
			Actions:  []string{"目标首帧 preroll 完成后执行短淡入，再恢复正常增益"},
			Ramp:     true,
		})
	}

	for i := range events {
		events[i].Order = i + 1
	}
	return events
}

func graphActions(to *State) []string {
	actions := []string{fmt.Sprintf("进入 %s 节目图：%s", to.Mode, to.ProgramID)}
	if to.Anchor != nil {
		actions = append(actions, "以 independent substream "+refKey(*to.Anchor)+" 作为可独立呈现/解码锚点")
	}
	switch to.Mode {
	case ModeIndependent:
		actions = append(actions, "只渲染 independent anchor，跳过其他 substream")
	case ModeAssociated:
		actions = append(actions, "保留 anchor 解码以满足 dependent 的呈现依赖，渲染路由切到关联节目")
	case ModeMixed:
		actions = append(actions, "按 BSI/chanmap 合并 anchor 与 dependent，再实例化混音/声道扩展图")
	}
	if len(to.DecodeSubstreams) > 0 {
		actions = append(actions, "解码引用："+refsKey(to.DecodeSubstreams))
	}
	if len(to.RenderSubstreams) > 0 {
		actions = append(actions, "渲染引用："+refsKey(to.RenderSubstreams))
	}
	return actions
}
