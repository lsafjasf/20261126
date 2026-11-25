package ttmlevents

import (
	"strings"
	"time"
)

// RawTiming contains timing attributes exactly as expressed on one node.
// Begin is the parsed offset; End is relative to the nearest time container.
type RawTiming struct {
	Begin         time.Duration
	BeginSet      bool
	End           *time.Duration
	EndUnbounded  bool
	Dur           *time.Duration
	TimeContainer string
	Fill          string
	RepeatCount   string
}

// NodeTiming is the resolved timing information for one XML element.
type NodeTiming struct {
	DocOrder      int       `json:"docOrder"`
	Path          string    `json:"path"`
	ID            string    `json:"id,omitempty"`
	Name          string    `json:"name"`
	TimeContainer string    `json:"timeContainer"`
	Fill          string    `json:"fill,omitempty"`
	Nominal       Interval  `json:"nominal"`
	Effective     Interval  `json:"effective"`
	Clipped       bool      `json:"clipped"`
	Visible       bool      `json:"visible"`
	Raw           RawTiming `json:"-"`
	node          *Node
}

// Timeline contains absolute, ancestor-resolved timing for all elements.
type Timeline struct {
	Timings []*NodeTiming `json:"timings"`
	byOrder map[int]*NodeTiming
}

// TimingOptions controls how literal begin/end/dur attributes are interpreted.
type TimingOptions struct {
	// AbsoluteTimes treats every begin/end expression as document media time.
	// The TTML/SMIL default is false: child offsets are resolved against the
	// nearest par/seq time container before ancestor clipping.
	AbsoluteTimes bool
}

// Get returns resolved timing by source document order.
func (t *Timeline) Get(docOrder int) *NodeTiming {
	if t == nil || t.byOrder == nil {
		return nil
	}
	return t.byOrder[docOrder]
}

func extractRawTiming(doc *Document, n *Node) RawTiming {
	raw := RawTiming{TimeContainer: "par"}
	if n == nil || n.Kind != ElementNode {
		return raw
	}
	if value, ok := n.AttrValue("timeContainer"); ok && strings.TrimSpace(value) != "" {
		raw.TimeContainer = strings.ToLower(strings.TrimSpace(value))
	}
	if value, ok := n.AttrValue("fill"); ok {
		raw.Fill = strings.ToLower(strings.TrimSpace(value))
	}
	if value, ok := n.AttrValue("repeatCount"); ok {
		raw.RepeatCount = strings.TrimSpace(value)
	}

	if value, ok := n.AttrValue("begin"); ok {
		d, unbounded, err := doc.parseTime(value)
		if err == nil && !unbounded {
			raw.Begin = d
			raw.BeginSet = true
		} else if err != nil {
			doc.Warnings = append(doc.Warnings, Warning{Code: "bad-begin", Path: n.Path, Message: value})
		}
	}
	if value, ok := n.AttrValue("end"); ok {
		d, unbounded, err := doc.parseTime(value)
		switch {
		case unbounded:
			raw.EndUnbounded = true
		case err == nil:
			raw.End = &d
		default:
			doc.Warnings = append(doc.Warnings, Warning{Code: "bad-end", Path: n.Path, Message: value})
		}
	}
	if value, ok := n.AttrValue("dur"); ok {
		d, unbounded, err := doc.parseTime(value)
		if err == nil && !unbounded && d >= 0 {
			raw.Dur = &d
		} else {
			doc.Warnings = append(doc.Warnings, Warning{Code: "bad-dur", Path: n.Path, Message: value})
		}
	}
	return raw
}

// ResolveTiming computes absolute intervals for every element. Child begin
// and end offsets are resolved against the nearest par/seq time container,
// then clipped by the parent's effective interval.
func ResolveTiming(doc *Document) *Timeline {
	return ResolveTimingWithOptions(doc, TimingOptions{})
}

// ResolveTimingWithOptions resolves timing with explicit compatibility options.
func ResolveTimingWithOptions(doc *Document, options TimingOptions) *Timeline {
	tl := &Timeline{byOrder: make(map[int]*NodeTiming)}
	if doc == nil || doc.Root == nil {
		return tl
	}
	resolveTimingNode(doc, tl, doc.Root, 0, 0, UnboundedInterval(0), UnboundedInterval(0), options)
	return tl
}

func resolveTimingNode(
	doc *Document,
	tl *Timeline,
	n *Node,
	containerOrigin time.Duration,
	scheduledBegin time.Duration,
	parentEffective Interval,
	parentNominal Interval,
	options TimingOptions,
) *NodeTiming {
	raw := extractRawTiming(doc, n)
	if options.AbsoluteTimes {
		containerOrigin = 0
		scheduledBegin = raw.Begin
	}
	begin := scheduledBegin

	var nominalEnd *time.Duration
	if raw.End != nil {
		end := *raw.End
		if !options.AbsoluteTimes {
			end = containerOrigin + end
		}
		nominalEnd = &end
	}
	if raw.Dur != nil {
		durEnd := begin + *raw.Dur
		nominalEnd = minDurationPtr(nominalEnd, &durEnd)
	}
	nominal := Interval{Begin: begin, End: cloneDurationPtr(nominalEnd)}

	effective, overlap := nominal.Clip(parentEffective)
	info := &NodeTiming{
		DocOrder:      n.DocOrder,
		Path:          n.Path,
		ID:            n.ID,
		Name:          n.Local,
		TimeContainer: raw.TimeContainer,
		Fill:          raw.Fill,
		Nominal:       nominal,
		Effective:     effective,
		Raw:           raw,
		node:          n,
	}
	if !overlap {
		info.Clipped = true
		info.Visible = false
		zero := parentEffective.Begin
		info.Effective = BoundedInterval(zero, zero)
	} else {
		info.Clipped = nominalEnd != nil && effective.End != nil && *effective.End < *nominalEnd
		info.Visible = !effective.Empty()
	}
	tl.Timings = append(tl.Timings, info)
	tl.byOrder[n.DocOrder] = info

	childOrigin := nominal.Begin
	container := raw.TimeContainer
	if container != "seq" {
		container = "par"
	}

	var cursor time.Duration
	if container == "seq" {
		cursor = nominal.Begin
	}

	for _, child := range n.Children {
		if child.Kind != ElementNode {
			continue
		}
		childRaw := extractRawTiming(doc, child)
		var childBegin time.Duration
		switch {
		case options.AbsoluteTimes:
			childBegin = childRaw.Begin
		case container == "seq":
			childBegin = cursor + childRaw.Begin
		default:
			childBegin = nominal.Begin + childRaw.Begin
		}
		childInfo := resolveTimingNode(doc, tl, child, childOrigin, childBegin, info.Effective, nominal, options)
		if container == "seq" && childInfo.Nominal.End != nil {
			cursor = *childInfo.Nominal.End
		}
	}
	return info
}
