package vttlayout

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// FmtPct rounds a percentage to two decimals and trims trailing zeros.
func FmtPct(v float64) string {
	r := math.Round(v*100) / 100
	return strconv.FormatFloat(r, 'f', -1, 64)
}

// Serialize renders a Document back to WebVTT text.
func Serialize(doc *Document) string {
	var b strings.Builder
	b.WriteString("WEBVTT\n\n")
	for _, id := range doc.RegionOrder {
		r := doc.Regions[id]
		if r == nil {
			continue
		}
		fmt.Fprintf(&b, "REGION\nid:%s\n", r.ID)
		fmt.Fprintf(&b, "width:%s%%\n", FmtPct(r.WidthPct))
		fmt.Fprintf(&b, "lines:%d\n", r.Lines)
		fmt.Fprintf(&b, "regionanchor:%s%%,%s%%\n", FmtPct(r.RegionAnchorX), FmtPct(r.RegionAnchorY))
		fmt.Fprintf(&b, "viewportanchor:%s%%,%s%%\n", FmtPct(r.ViewportAnchorX), FmtPct(r.ViewportAnchorY))
		if r.Scroll != "" {
			fmt.Fprintf(&b, "scroll:%s\n", r.Scroll)
		}
		if r.PrioritySet {
			fmt.Fprintf(&b, "priority:%d\n", r.Priority)
		}
		b.WriteString("\n")
	}
	for _, c := range doc.Cues {
		if c.ID != "" && !isGeneratedID(c.ID) {
			b.WriteString(c.ID)
			b.WriteString("\n")
		}
		fmt.Fprintf(&b, "%s --> %s", FormatTimestamp(c.StartMS), FormatTimestamp(c.EndMS))
		if c.RawSettings != "" {
			b.WriteString(" ")
			b.WriteString(c.RawSettings)
		}
		b.WriteString("\n")
		b.WriteString(c.Text)
		b.WriteString("\n\n")
	}
	return b.String()
}

func isGeneratedID(id string) bool {
	return strings.HasPrefix(id, "L") && len(id) > 1
}

// FixedDocument builds a new Document whose regions are the solved pixel
// rectangles expressed as top left anchored percentage regions. Every cue
// gets its own region so the solved geometry round trips into a player.
func FixedDocument(plan *Plan) *Document {
	doc := &Document{Regions: map[string]*Region{}}
	cv := plan.Canvas
	for _, bx := range plan.Boxes {
		id := fmt.Sprintf("L%d", bx.Cue.Order+1)
		r := &Region{
			ID: id,
			// Slight width expansion absorbs padding/rounding in players.
			WidthPct:        bx.Rect.W / cv.Width * 100,
			Lines:           len(bx.Lines),
			RegionAnchorX:   0,
			RegionAnchorY:   0,
			ViewportAnchorX: bx.Rect.X / cv.Width * 100,
			ViewportAnchorY: bx.Rect.Y / cv.Height * 100,
			Priority:        bx.Priority,
			PrioritySet:     true,
		}
		doc.Regions[id] = r
		doc.RegionOrder = append(doc.RegionOrder, id)

		cp := *bx.Cue
		cp.Settings.Region = id
		cp.RawSettings = rewriteRegionSetting(cp.RawSettings, id)
		doc.Cues = append(doc.Cues, &cp)
	}
	return doc
}

func rewriteRegionSetting(raw, id string) string {
	toks := strings.Fields(raw)
	found := false
	for i, t := range toks {
		if strings.HasPrefix(t, "region:") {
			toks[i] = "region:" + id
			found = true
		}
	}
	if !found {
		toks = append([]string{"region:" + id}, toks...)
	}
	return strings.Join(toks, " ")
}

// WriteFixedVTT serializes the solved layout as WebVTT.
func WriteFixedVTT(plan *Plan) string {
	doc := FixedDocument(plan)
	return Serialize(doc)
}

// ApplyCandidate returns a modified Document implementing a split or time
// candidate. Canvas/font candidates affect rendering options, not VTT text.
func ApplyCandidate(doc *Document, cand *Candidate) (*Document, error) {
	var target *Cue
	for _, c := range doc.Cues {
		if c.Order == cand.TargetOrder {
			target = c
		}
	}
	if target == nil {
		return nil, fmt.Errorf("candidate targets missing cue order %d", cand.TargetOrder)
	}
	switch cand.Kind {
	case CandidateSplitCue:
		if len(cand.Proposed) != 2 {
			return nil, fmt.Errorf("split candidate has %d proposals", len(cand.Proposed))
		}
		p0, p1 := cand.Proposed[0], cand.Proposed[1]
		return withSplitCue(doc, target, p0.EndMS, p0.Text, p1.Text), nil
	case CandidateShiftTime, CandidateExtendTime:
		ns, ne := cand.NewStartMS, cand.NewEndMS
		if ns == 0 && ne == 0 && len(cand.Proposed) == 1 {
			ns, ne = cand.Proposed[0].StartMS, cand.Proposed[0].EndMS
		}
		return withMovedCue(doc, target, ns, ne), nil
	default:
		return nil, fmt.Errorf("candidate kind %q is a canvas/rendering change, not a VTT edit", cand.Kind)
	}
}
