package ttmlevents

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

func buildEvents(doc *Document, tl *Timeline, tracks []AnimationTrack, slices []Slice) []Event {
	events := make([]Event, 0)

	for _, info := range tl.Timings {
		n := info.node
		if n == nil || !isEventElement(n) {
			continue
		}
		depth := pathDepth(info.Path)
		target := targetRef(info.ID, info.Path)
		parentRef := ""
		if n.Parent != nil && n.Parent.Kind == ElementNode {
			parentRef = targetRef(n.Parent.ID, n.Parent.Path)
		}

		if info.Visible && !info.Effective.Empty() {
			seconds, label := eventTimeFields(info.Effective.Begin)
			events = append(events, Event{
				Time: info.Effective.Begin, TimeSeconds: seconds, Timecode: label,
				Phase: PhaseBegin, Kind: info.Name, Target: target, NodePath: info.Path,
				Title:    info.Name + " 开始",
				Detail:   intervalDetail(info),
				CausedBy: parentCausedBegin(parentRef),
				depth:    depth, docOrder: info.DocOrder, postOrder: n.PostOrder, tie: 0,
			})
			if info.Effective.End != nil {
				seconds, label := eventTimeFields(*info.Effective.End)
				detail := intervalDetail(info)
				cause := "半开区间 [begin,end) 右端点：先摘除样式/文本，再应用同刻开始"
				if info.Clipped {
					cause = "后代先结束；实际结束被祖先活动区间裁剪"
				}
				events = append(events, Event{
					Time: *info.Effective.End, TimeSeconds: seconds, Timecode: label,
					Phase: PhaseEnd, Kind: info.Name, Target: target, NodePath: info.Path,
					Title:    info.Name + " 结束",
					Detail:   detail,
					CausedBy: cause,
					depth:    depth, docOrder: info.DocOrder, postOrder: n.PostOrder, tie: 1,
				})
			}
		} else {
			doc.Warnings = append(doc.Warnings, Warning{
				Code: "timed-element-not-visible",
				Path: info.Path,
				Message: fmt.Sprintf("nominal=%s..%s is outside active ancestor interval",
					formatDuration(info.Nominal.Begin), endLabel(info.Nominal.End)),
			})
		}
	}

	for _, track := range tracks {
		for i, segment := range track.Segments {
			seconds, label := eventTimeFields(segment.Begin)
			title := "关键帧"
			detail := fmt.Sprintf("%s: %s -> %s (%s)", track.Property, segment.From, segment.To, segment.CalcMode)
			if segment.CalcMode == "freeze" {
				title = "fill=freeze 保持末值"
				detail = track.Property + "=" + segment.Value
			} else if i == 0 {
				title = "动画生效"
			}
			events = append(events, Event{
				Time: segment.Begin, TimeSeconds: seconds, Timecode: label,
				Phase: PhaseChange, Kind: "keyframe", Target: targetRef(track.ID, track.Path),
				NodePath: track.Path, Title: title, Detail: detail,
				CausedBy: fmt.Sprintf("内部动画 %s，宿主 %s", track.Path, track.HostPath),
				depth:    pathDepth(track.Path), docOrder: track.NodeOrder, tie: 100 + i,
			})
		}
	}

	sortEvents(events)
	for i := range events {
		events[i].Sequence = i + 1
		events[i].OrderKey = fmt.Sprintf(
			"%03d|%s|%s|depth=%d|doc=%d|tie=%d",
			events[i].Sequence, formatDuration(events[i].Time), events[i].Phase,
			events[i].depth, events[i].docOrder, events[i].tie,
		)
	}
	assignCauseSequences(events, tl)
	return events
}

func assignCauseSequences(events []Event, tl *Timeline) {
	beginSequence := make(map[int]int)
	exactSequence := make(map[string]int)
	for _, event := range events {
		exactSequence[eventCausalKey(event.docOrder, event.Phase, event.tie)] = event.Sequence
		if event.Phase == PhaseBegin {
			beginSequence[event.docOrder] = event.Sequence
		}
	}

	for i := range events {
		event := &events[i]
		switch event.Phase {
		case PhaseBegin:
			info := tl.Get(event.docOrder)
			if info != nil && info.node != nil && info.node.Parent != nil && info.node.Parent.Kind == ElementNode {
				event.CauseSequence = beginSequence[info.node.Parent.DocOrder]
			}
		case PhaseEnd:
			event.CauseSequence = beginSequence[event.docOrder]
		case PhaseChange:
			if event.tie == 100 {
				event.CauseSequence = beginSequence[event.docOrder]
			} else if event.tie > 100 {
				event.CauseSequence = exactSequence[eventCausalKey(event.docOrder, event.Phase, event.tie-1)]
			}
		}
	}
}

func eventCausalKey(docOrder int, phase string, tie int) string {
	return fmt.Sprintf("%d/%s/%d", docOrder, phase, tie)
}

func sortEvents(events []Event) {
	phaseRank := map[string]int{PhaseEnd: 0, PhaseBegin: 1, PhaseChange: 2}
	sort.SliceStable(events, func(i, j int) bool {
		a, b := events[i], events[j]
		if a.Time != b.Time {
			return a.Time < b.Time
		}
		if phaseRank[a.Phase] != phaseRank[b.Phase] {
			return phaseRank[a.Phase] < phaseRank[b.Phase]
		}
		if a.Phase == PhaseEnd {
			if a.postOrder != b.postOrder {
				return a.postOrder < b.postOrder
			}
		}
		if a.docOrder != b.docOrder {
			return a.docOrder < b.docOrder
		}
		if a.tie != b.tie {
			return a.tie < b.tie
		}
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		return a.Target < b.Target
	})
}

func isEventElement(n *Node) bool {
	if n == nil || n.Kind != ElementNode {
		return false
	}
	if n.Is("animate", "set") {
		return true
	}
	if n.Is("body", "div", "p", "span", "br", "region") {
		return true
	}
	if hasAncestorNamed(n, "body") && !hasAncestorNamed(n, "metadata") {
		return !isSkippedContentElement(n)
	}
	return false
}

func pathDepth(path string) int {
	return strings.Count(path, "/")
}

func parentCausedBegin(parentRef string) string {
	if parentRef == "" {
		return "时间容器起点"
	}
	return "父时间容器 " + parentRef + " 已先建立"
}

func intervalDetail(info *NodeTiming) string {
	effective := formatDuration(info.Effective.Begin) + ".." + endLabel(info.Effective.End)
	nominal := formatDuration(info.Nominal.Begin) + ".." + endLabel(info.Nominal.End)
	if nominal == effective {
		return "absolute interval " + effective
	}
	return "nominal " + nominal + "; effective " + effective + " (clipped by ancestor)"
}

func endLabel(end *time.Duration) string {
	if end == nil {
		return "∞"
	}
	return formatDuration(*end)
}
