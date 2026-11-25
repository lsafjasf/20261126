package ttmlevents

import (
	"strconv"
	"strings"
	"time"
)

// AnimationSegment is a piecewise-constant or linear keyframe span.
type AnimationSegment struct {
	Begin        time.Duration  `json:"beginNanos"`
	BeginSeconds float64        `json:"beginSeconds"`
	End          *time.Duration `json:"endNanos,omitempty"`
	EndSeconds   *float64       `json:"endSeconds,omitempty"`
	Value        string         `json:"value,omitempty"`
	From         string         `json:"from,omitempty"`
	To           string         `json:"to,omitempty"`
	CalcMode     string         `json:"calcMode"`
}

// AnimationTrack is one child animate/set element resolved on the timeline.
type AnimationTrack struct {
	NodeOrder     int                `json:"nodeOrder"`
	HostOrder     int                `json:"hostOrder"`
	ID            string             `json:"id,omitempty"`
	Path          string             `json:"path"`
	HostPath      string             `json:"hostPath"`
	Kind          string             `json:"kind"`
	AttributeName string             `json:"attributeName"`
	Property      string             `json:"property"`
	Begin         time.Duration      `json:"beginNanos"`
	BeginSeconds  float64            `json:"beginSeconds"`
	ActiveEnd     *time.Duration     `json:"activeEndNanos,omitempty"`
	End           *time.Duration     `json:"endNanos,omitempty"`
	EndSeconds    *float64           `json:"endSeconds,omitempty"`
	Fill          string             `json:"fill,omitempty"`
	CalcMode      string             `json:"calcMode"`
	Values        []string           `json:"values,omitempty"`
	KeyTimes      []float64          `json:"keyTimes,omitempty"`
	Segments      []AnimationSegment `json:"segments,omitempty"`
}

func collectAnimations(doc *Document, tl *Timeline) []AnimationTrack {
	tracks := make([]AnimationTrack, 0)
	if doc == nil || doc.Root == nil || tl == nil {
		return tracks
	}

	var nodes []*Node
	walkNodes(doc.Root, func(n *Node) bool {
		if n.Kind == ElementNode && n.Is("animate", "set") {
			nodes = append(nodes, n)
		}
		return true
	})

	for _, n := range nodes {
		timing := tl.Get(n.DocOrder)
		if timing == nil {
			continue
		}
		host := n.Parent
		hostTiming := (*NodeTiming)(nil)
		if host != nil {
			hostTiming = tl.Get(host.DocOrder)
		}

		attrName, _ := n.AttrValue("attributeName")
		track := AnimationTrack{
			NodeOrder:     n.DocOrder,
			ID:            n.ID,
			Path:          n.Path,
			Kind:          n.Local,
			AttributeName: attrName,
			Property:      canonicalPropertyName(attrName),
			Fill:          timing.Raw.Fill,
		}
		if host != nil {
			track.HostOrder = host.DocOrder
			track.HostPath = host.Path
		}
		track.Begin = timing.Nominal.Begin
		track.BeginSeconds = durationSeconds(track.Begin)
		track.ActiveEnd = cloneDurationPtr(timing.Nominal.End)
		track.CalcMode = strings.ToLower(attrOr(n, "calcMode", defaultCalcMode(n.Local)))

		values := animationValues(n)
		track.Values = values
		track.KeyTimes = parseOptionalKeyTimes(attrOr(n, "keyTimes", ""), len(values))

		hostEnd := (*time.Duration)(nil)
		if hostTiming != nil {
			hostEnd = hostTiming.Effective.End
		}
		activeInterval := Interval{Begin: track.Begin, End: cloneDurationPtr(track.ActiveEnd)}
		if track.ActiveEnd == nil && hostEnd != nil {
			activeInterval.End = cloneDurationPtr(hostEnd)
			track.ActiveEnd = cloneDurationPtr(hostEnd)
		}
		if hostTiming != nil {
			if clipped, ok := activeInterval.Clip(hostTiming.Effective); ok {
				activeInterval = clipped
			}
		}

		track.Segments = buildAnimationSegments(doc, n, track, activeInterval, hostEnd)
		if len(track.Segments) > 0 {
			last := track.Segments[len(track.Segments)-1]
			track.End = cloneDurationPtr(last.End)
			if track.End != nil {
				sec := durationSeconds(*track.End)
				track.EndSeconds = &sec
			}
		}
		tracks = append(tracks, track)
	}
	return tracks
}

func attrOr(n *Node, name, fallback string) string {
	if v, ok := n.AttrValue(name); ok && strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	return fallback
}

func defaultCalcMode(elementName string) string {
	if equalFoldName(elementName, "set") {
		return "discrete"
	}
	return "linear"
}

func animationValues(n *Node) []string {
	if raw, ok := n.AttrValue("values"); ok && strings.TrimSpace(raw) != "" {
		parts := strings.Split(raw, ";")
		out := make([]string, 0, len(parts))
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part != "" {
				out = append(out, part)
			}
		}
		if len(out) > 0 {
			return out
		}
	}

	from, hasFrom := n.AttrValue("from")
	to, hasTo := n.AttrValue("to")
	by, hasBy := n.AttrValue("by")
	if hasFrom && hasTo {
		return []string{strings.TrimSpace(from), strings.TrimSpace(to)}
	}
	if hasTo {
		return []string{strings.TrimSpace(to)}
	}
	if hasFrom && hasBy {
		return []string{strings.TrimSpace(from), strings.TrimSpace(from) + " + " + strings.TrimSpace(by)}
	}
	if hasFrom {
		return []string{strings.TrimSpace(from)}
	}
	return nil
}

func parseOptionalKeyTimes(raw string, count int) []float64 {
	if strings.TrimSpace(raw) == "" || count <= 0 {
		return uniformKeyTimes(count)
	}
	parts := strings.Split(raw, ";")
	out := make([]float64, 0, len(parts))
	for _, part := range parts {
		var value float64
		if _, err := parseFloat(strings.TrimSpace(part), &value); err != nil {
			return uniformKeyTimes(count)
		}
		out = append(out, value)
	}
	if len(out) != count {
		return uniformKeyTimes(count)
	}
	for i := range out {
		if i > 0 && out[i] < out[i-1] {
			return uniformKeyTimes(count)
		}
	}
	return out
}

func uniformKeyTimes(count int) []float64 {
	if count <= 0 {
		return nil
	}
	out := make([]float64, count)
	if count == 1 {
		out[0] = 0
		return out
	}
	for i := 0; i < count; i++ {
		out[i] = float64(i) / float64(count-1)
	}
	return out
}

func buildAnimationSegments(
	doc *Document,
	n *Node,
	track AnimationTrack,
	active Interval,
	hostEnd *time.Duration,
) []AnimationSegment {
	values := track.Values
	if len(values) == 0 {
		doc.Warnings = append(doc.Warnings, Warning{Code: "animation-without-values", Path: n.Path, Message: track.Property})
		return nil
	}

	if track.Kind == "set" || len(values) == 1 {
		seg := AnimationSegment{
			Begin:    active.Begin,
			Value:    values[len(values)-1],
			From:     values[len(values)-1],
			To:       values[len(values)-1],
			CalcMode: "discrete",
		}
		if active.End != nil {
			e := *active.End
			seg.End = &e
			sec := durationSeconds(e)
			seg.EndSeconds = &sec
		}
		seg.BeginSeconds = durationSeconds(seg.Begin)
		return appendFreezeSegment(n, track, []AnimationSegment{seg}, active.End, hostEnd, values[len(values)-1])
	}

	if active.End == nil || *active.End <= active.Begin {
		doc.Warnings = append(doc.Warnings, Warning{Code: "animation-without-finite-dur", Path: n.Path, Message: track.Property})
		return nil
	}
	duration := *active.End - active.Begin
	keyTimes := track.KeyTimes
	if len(keyTimes) != len(values) {
		keyTimes = uniformKeyTimes(len(values))
	}

	segments := make([]AnimationSegment, 0, len(values)-1)
	for i := 0; i+1 < len(values); i++ {
		begin := active.Begin + time.Duration(float64(duration)*keyTimes[i])
		end := active.Begin + time.Duration(float64(duration)*keyTimes[i+1])
		if end < begin {
			end = begin
		}
		e := end
		seg := AnimationSegment{
			Begin:        begin,
			BeginSeconds: durationSeconds(begin),
			End:          &e,
			EndSeconds:   floatPtr(durationSeconds(end)),
			From:         values[i],
			To:           values[i+1],
			Value:        values[i],
			CalcMode:     track.CalcMode,
		}
		if track.CalcMode == "discrete" {
			seg.Value = values[i]
		}
		segments = append(segments, seg)
	}

	finalValue := values[len(values)-1]
	return appendFreezeSegment(n, track, segments, active.End, hostEnd, finalValue)
}

func appendFreezeSegment(
	n *Node,
	track AnimationTrack,
	segments []AnimationSegment,
	activeEnd *time.Duration,
	hostEnd *time.Duration,
	finalValue string,
) []AnimationSegment {
	if !strings.EqualFold(track.Fill, "freeze") || activeEnd == nil || hostEnd == nil || *hostEnd <= *activeEnd {
		return segments
	}
	begin := *activeEnd
	end := *hostEnd
	seg := AnimationSegment{
		Begin:        begin,
		BeginSeconds: durationSeconds(begin),
		End:          &end,
		EndSeconds:   floatPtr(durationSeconds(end)),
		From:         finalValue,
		To:           finalValue,
		Value:        finalValue,
		CalcMode:     "freeze",
	}
	return append(segments, seg)
}

func floatPtr(v float64) *float64 { return &v }

func parseFloat(s string, v *float64) (float64, error) {
	value, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, err
	}
	*v = value
	return value, nil
}
