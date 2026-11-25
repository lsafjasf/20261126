package ttmlevents

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

type effectiveProp struct {
	Property
	AnimationPath string
	Interpolation string
	EndValue      string
	MergeGroup    string
}

type effectiveStyle []effectiveProp

func (es effectiveStyle) hidden() bool {
	for _, prop := range es {
		switch prop.Name {
		case "display":
			if strings.EqualFold(strings.TrimSpace(prop.Value), "none") {
				return true
			}
		case "visibility":
			value := strings.ToLower(strings.TrimSpace(prop.Value))
			if value == "hidden" || value == "collapse" || value == "collapsed" {
				return true
			}
		}
	}
	return false
}

func (es effectiveStyle) properties() []Property {
	out := make([]Property, 0, len(es))
	for _, prop := range es {
		p := prop.Property
		if prop.AnimationPath != "" {
			p.Source = "animation:" + prop.AnimationPath
			p.Inherited = false
		}
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func effectiveStyleForRun(run TextRun, at time.Duration, sliceEnd *time.Duration, tracks []AnimationTrack) effectiveStyle {
	merged := make(map[string]effectiveProp)
	staticDepth := make(map[string]int)
	for _, prop := range run.Style {
		merged[prop.Name] = effectiveProp{
			Property:      prop,
			Interpolation: "static",
			MergeGroup:    "static:" + prop.Name,
		}
		staticDepth[prop.Name] = prop.DeclarationDepth
	}

	affecting := make([]AnimationTrack, 0)
	for _, track := range tracks {
		if track.Property == "" || !animationAffectsRun(track, run) || !trackActiveAt(track, at) {
			continue
		}
		affecting = append(affecting, track)
	}
	sort.SliceStable(affecting, func(i, j int) bool {
		leftDepth := strings.Count(affecting[i].HostPath, "/")
		rightDepth := strings.Count(affecting[j].HostPath, "/")
		if leftDepth != rightDepth {
			return leftDepth < rightDepth
		}
		return affecting[i].NodeOrder < affecting[j].NodeOrder
	})

	for _, track := range affecting {
		hostDepth := strings.Count(track.HostPath, "/")
		if depth, ok := staticDepth[track.Property]; ok && hostDepth < depth {
			continue
		}
		segment, index, ok := segmentAt(track, at)
		if !ok {
			continue
		}
		value := evaluateSegment(segment, at)
		prop := Property{
			Name:      track.Property,
			Value:     value,
			Source:    "animation",
			Inherited: false,
		}
		effective := effectiveProp{
			Property:      prop,
			AnimationPath: track.Path,
			Interpolation: segment.CalcMode,
			MergeGroup:    track.Path + "#segment-" + strconv.Itoa(index),
		}
		if sliceEnd != nil {
			effective.EndValue = evaluateSegment(segment, *sliceEnd)
		}
		merged[track.Property] = effective
	}

	names := make([]string, 0, len(merged))
	for name := range merged {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make(effectiveStyle, 0, len(names))
	for _, name := range names {
		out = append(out, merged[name])
	}
	return out
}

func animationAffectsRun(track AnimationTrack, run TextRun) bool {
	if track.HostPath == "" || run.ElementPath == "" {
		return false
	}
	return run.ElementPath == track.HostPath || strings.HasPrefix(run.ElementPath, track.HostPath+"/")
}

func trackActiveAt(track AnimationTrack, at time.Duration) bool {
	if at < track.Begin {
		return false
	}
	if track.End != nil && at >= *track.End {
		return false
	}
	return true
}

func segmentAt(track AnimationTrack, at time.Duration) (AnimationSegment, int, bool) {
	for i, segment := range track.Segments {
		if at < segment.Begin {
			continue
		}
		if segment.End == nil || at < *segment.End || at == *segment.End {
			// At an end boundary, callers asking for the post-boundary state
			// should normally select the next segment. Allowing exact end makes
			// endpoint evaluation stable for EndValue.
			if segment.End != nil && at == *segment.End && i+1 < len(track.Segments) {
				next := track.Segments[i+1]
				if next.Begin == at {
					return next, i + 1, true
				}
			}
			return segment, i, true
		}
	}
	return AnimationSegment{}, -1, false
}

func evaluateSegment(segment AnimationSegment, at time.Duration) string {
	switch strings.ToLower(segment.CalcMode) {
	case "discrete", "freeze":
		if segment.Value != "" {
			return segment.Value
		}
		return segment.From
	}

	if segment.End == nil || *segment.End <= segment.Begin {
		return segment.From
	}
	if at <= segment.Begin {
		return segment.From
	}
	if at >= *segment.End {
		return segment.To
	}
	ratio := float64(at-segment.Begin) / float64(*segment.End-segment.Begin)
	if colorFrom, ok1 := parseColor(segment.From); true {
		if colorTo, ok2 := parseColor(segment.To); ok1 && ok2 {
			return interpolateColor(colorFrom, colorTo, ratio).String()
		}
	}
	if fromValue, fromUnit, ok1 := parseNumberUnit(segment.From); true {
		if toValue, toUnit, ok2 := parseNumberUnit(segment.To); ok1 && ok2 && fromUnit == toUnit {
			value := fromValue + (toValue-fromValue)*ratio
			return formatNumber(value) + fromUnit
		}
	}
	if ratio == 0 {
		return segment.From
	}
	if ratio == 1 {
		return segment.To
	}
	return fmt.Sprintf("%s→%s@%.3f", segment.From, segment.To, ratio)
}

type color struct{ R, G, B uint8 }

func (c color) String() string {
	return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B)
}

func interpolateColor(a, b color, p float64) color {
	mix := func(x, y uint8) uint8 { return uint8(float64(x) + (float64(y)-float64(x))*p + 0.5) }
	return color{mix(a.R, b.R), mix(a.G, b.G), mix(a.B, b.B)}
}

func parseColor(raw string) (color, bool) {
	s := strings.TrimSpace(strings.ToLower(raw))
	named := map[string]color{
		"red": {255, 0, 0}, "blue": {0, 0, 255}, "green": {0, 128, 0},
		"lime": {0, 255, 0}, "white": {255, 255, 255}, "black": {0, 0, 0},
		"yellow": {255, 255, 0}, "cyan": {0, 255, 255}, "magenta": {255, 0, 255},
		"gray": {128, 128, 128}, "grey": {128, 128, 128}, "orange": {255, 165, 0},
	}
	if c, ok := named[s]; ok {
		return c, true
	}
	if strings.HasPrefix(s, "#") {
		hex := s[1:]
		if len(hex) == 3 {
			hex = string([]byte{hex[0], hex[0], hex[1], hex[1], hex[2], hex[2]})
		}
		if len(hex) != 6 {
			return color{}, false
		}
		v, err := strconv.ParseUint(hex, 16, 32)
		if err != nil {
			return color{}, false
		}
		return color{uint8(v >> 16), uint8(v >> 8), uint8(v)}, true
	}
	if strings.HasPrefix(s, "rgb(") && strings.HasSuffix(s, ")") {
		parts := strings.Split(strings.TrimSuffix(strings.TrimPrefix(s, "rgb("), ")"), ",")
		if len(parts) != 3 {
			return color{}, false
		}
		var vals [3]uint8
		for i, part := range parts {
			v, err := strconv.Atoi(strings.TrimSpace(part))
			if err != nil || v < 0 || v > 255 {
				return color{}, false
			}
			vals[i] = uint8(v)
		}
		return color{vals[0], vals[1], vals[2]}, true
	}
	return color{}, false
}

func parseNumberUnit(raw string) (float64, string, bool) {
	s := strings.TrimSpace(raw)
	i := 0
	for i < len(s) && ((s[i] >= '0' && s[i] <= '9') || s[i] == '.' || s[i] == '-' || s[i] == '+') {
		i++
	}
	if i == 0 {
		return 0, "", false
	}
	value, err := strconv.ParseFloat(s[:i], 64)
	if err != nil {
		return 0, "", false
	}
	return value, strings.TrimSpace(s[i:]), true
}

func formatNumber(v float64) string {
	if v == float64(int64(v)) {
		return strconv.FormatInt(int64(v), 10)
	}
	return strconv.FormatFloat(v, 'f', 3, 64)
}
