package vttlayout

import (
	"fmt"
	"math"
	"sort"
)

// Options controls the relayout solver.
type Options struct {
	// GapPx overrides the default inter box gap when > 0.
	GapPx float64
	// KeepWidth forces authored widths even outside the safe area.
	KeepWidth bool
}

func (o Options) gap(cv Canvas) float64 {
	if o.GapPx > 0 {
		return o.GapPx
	}
	return GapPx(cv)
}

// Box is one cue's resolved placement.
type Box struct {
	Cue         *Cue
	Rect        Rect
	Authored    Rect
	Lines       []string
	Edge        Edge
	Priority    int
	WidthPct    float64
	DeltaY      float64
	Pushed      bool
	PlacementOK bool
}

// ViolationKind enumerates detected geometry problems.
type ViolationKind string

const (
	ViolationOverlap     ViolationKind = "region_overlap"
	ViolationOutOfBounds ViolationKind = "out_of_bounds"
)

// Violation is one detected overlap or out of bounds condition.
type Violation struct {
	Kind         ViolationKind `json:"kind"`
	CueA         int           `json:"cue_a"`
	CueB         int           `json:"cue_b"`
	Intersection Rect          `json:"intersection"`
	Detail       string        `json:"detail"`
}

// Combo is a (possibly certified) unsolvable simultaneous combination.
type Combo struct {
	ID                string       `json:"id"`
	StartMS           Milliseconds `json:"start_ms"`
	EndMS             Milliseconds `json:"end_ms"`
	CueOrders         []int        `json:"cue_orders"`
	TotalHeightPx     float64      `json:"total_height_px"`
	AvailableHeightPx float64      `json:"available_height_px"`
	OverflowPx        float64      `json:"overflow_px"`
	Certified         bool         `json:"certified"`
	Reason            string       `json:"reason"`
}

// Plan is the full check + relayout result.
type Plan struct {
	Canvas     Canvas
	Options    Options
	Authored   []*AuthoredBox
	Boxes      []*Box
	ByOrder    map[int]*Box
	Before     []Violation
	After      []Violation
	Combos     []*Combo
	Candidates []*Candidate
	Feasible   bool
}

// Analyze runs the full pipeline including repair candidates.
func Analyze(doc *Document, cv Canvas, opts Options) *Plan {
	plan := Layout(doc, cv, opts)
	plan.Combos = DetectCombos(doc, cv, plan)
	plan.Feasible = len(plan.After) == 0 && len(plan.Combos) == 0
	plan.Candidates = BuildCandidates(doc, cv, opts, plan)
	return plan
}

// Layout measures authored boxes, records original violations and runs the
// priority aware collision push solver.
func Layout(doc *Document, cv Canvas, opts Options) *Plan {
	authored := BuildAuthoredBoxes(doc, cv)
	plan := &Plan{
		Canvas:   cv,
		Options:  opts,
		Authored: authored,
		ByOrder:  map[int]*Box{},
	}

	plan.Before = detectPairOverlaps(authored)
	plan.Before = append(plan.Before, detectAuthoredBounds(authored, cv)...)

	gap := opts.gap(cv)
	safe := cv.SafeRect()

	order := make([]*Cue, len(doc.Cues))
	copy(order, doc.Cues)
	sort.SliceStable(order, func(i, j int) bool {
		pi := doc.Priority(order[i])
		pj := doc.Priority(order[j])
		if pi != pj {
			return pi > pj
		}
		if order[i].StartMS != order[j].StartMS {
			return order[i].StartMS < order[j].StartMS
		}
		return order[i].Order < order[j].Order
	})

	placed := map[int]*Box{}

	for _, cue := range order {
		var ar *AuthoredBox
		for _, a := range authored {
			if a.Cue.Order == cue.Order {
				ar = a
				break
			}
		}

		w := ar.Rect.W
		if !opts.KeepWidth && w > safe.W {
			w = safe.W
		}
		if w > safe.W {
			w = safe.W
		}
		if w < 1 {
			w = 1
		}
		innerW := cv.ContentWidth(w)
		lines := WrapLines(cue.Text, innerW, DefaultMeter(cv.FontPx))
		h := cv.BlockHeight(len(lines))

		rect, edge := ResolveRect(cue, ar.Region, cv, w, h)
		// Clamp horizontally into the safe area.
		if w <= safe.W {
			rect.X = clampFloat(rect.X, safe.X, safe.Right()-w)
		}
		// Start from the in safe bounds vertical position when possible.
		if h <= safe.H {
			rect.Y = clampFloat(rect.Y, safe.Y, safe.Bottom()-h)
		}
		rect.W, rect.H = w, h

		box := &Box{
			Cue: cue, Rect: rect, Authored: ar.Rect, Lines: lines,
			Edge: edge, Priority: doc.Priority(cue), WidthPct: w / cv.Width * 100,
			PlacementOK: h <= safe.H,
		}

		var obstacles []*Box
		for _, p := range placed {
			if !OverlapTime(cue, p.Cue) {
				continue
			}
			if rect.X < p.Rect.Right() && rect.Right() > p.Rect.X {
				obstacles = append(obstacles, p)
			}
		}

		desiredY := rect.Y
		if y, ok := nearestFeasibleY(rect, desiredY, edge, obstacles, safe, gap); ok {
			rect.Y = y
		} else {
			rect.Y = bestEffortY(rect, desiredY, obstacles, safe)
			box.PlacementOK = false
		}

		box.DeltaY = rect.Y - ar.Rect.Y
		box.Pushed = absFloat(box.DeltaY) > 0.5
		box.Rect = rect
		placed[cue.Order] = box
		plan.Boxes = append(plan.Boxes, box)
		plan.ByOrder[cue.Order] = box
	}

	relaxPlacements(plan, safe, gap)

	// Keep Boxes in cue order for stable rendering/reporting.
	sort.Slice(plan.Boxes, func(i, j int) bool {
		return plan.Boxes[i].Cue.Order < plan.Boxes[j].Cue.Order
	})
	for _, b := range plan.Boxes {
		b.DeltaY = b.Rect.Y - b.Authored.Y
		b.Pushed = absFloat(b.DeltaY) > 0.5
	}

	plan.After = detectFinalViolations(plan)
	return plan
}

func obstaclesFor(b *Box, all []*Box) []*Box {
	var obs []*Box
	for _, o := range all {
		if o == b {
			continue
		}
		if !OverlapTime(b.Cue, o.Cue) {
			continue
		}
		if b.Rect.X < o.Rect.Right() && b.Rect.Right() > o.Rect.X {
			obs = append(obs, o)
		}
	}
	return obs
}

// relaxPlacements runs coordinate descent over box Y positions, lowest
// priority first. Every accepted move is collision free against all other
// boxes, so the violation count never increases; this repairs near misses
// where the priority pass strands a cue by a few pixels.
func relaxPlacements(plan *Plan, safe Rect, gap float64) {
	order := append([]*Box(nil), plan.Boxes...)
	sort.SliceStable(order, func(i, j int) bool {
		if order[i].Priority != order[j].Priority {
			return order[i].Priority < order[j].Priority
		}
		return order[i].Cue.Order > order[j].Cue.Order
	})

	for round := 0; round < 6; round++ {
		changed := false
		for _, b := range order {
			obs := obstaclesFor(b, plan.Boxes)
			try := b.Rect
			y, ok := nearestFeasibleY(try, try.Y, b.Edge, obs, safe, gap)
			if !ok || absFloat(y-try.Y) <= 0.5 {
				continue
			}
			try.Y = y
			b.Rect = try
			b.PlacementOK = true
			changed = true
		}
		if !changed {
			break
		}
	}

	const eps = 0.5
	for _, b := range plan.Boxes {
		b.PlacementOK = b.Rect.Y >= safe.Y-eps &&
			b.Rect.Bottom() <= safe.Bottom()+eps
		if b.PlacementOK {
			for _, o := range obstaclesFor(b, plan.Boxes) {
				if b.Rect.Intersects(o.Rect) {
					b.PlacementOK = false
					break
				}
			}
		}
	}
}

// bestEffortY picks the in safe bounds Y that minimizes intersection area
// when no collision free position exists.
func bestEffortY(rect Rect, desiredY float64, obstacles []*Box, safe Rect) float64 {
	h := rect.H
	ys := []float64{
		clampFloat(desiredY, safe.Y, safe.Bottom()-h),
		safe.Y,
		safe.Bottom() - h,
	}
	bestY := ys[0]
	bestPenalty := math.MaxFloat64
	for _, y := range ys {
		try := Rect{X: rect.X, Y: y, W: rect.W, H: h}
		penalty := 0.0
		for _, o := range obstacles {
			in := try.Intersection(o.Rect)
			if in.W > 0 && in.H > 0 {
				penalty += in.W * in.H
			}
		}
		if penalty < bestPenalty {
			bestPenalty = penalty
			bestY = y
		}
	}
	return bestY
}

func detectPairOverlaps(boxes []*AuthoredBox) []Violation {
	var out []Violation
	for i := 0; i < len(boxes); i++ {
		for j := i + 1; j < len(boxes); j++ {
			a, b := boxes[i], boxes[j]
			if !OverlapTime(a.Cue, b.Cue) {
				continue
			}
			inter := a.Rect.Intersection(b.Rect)
			if inter.W > 0.5 && inter.H > 0.5 {
				out = append(out, Violation{
					Kind: ViolationOverlap, CueA: a.Cue.Order, CueB: b.Cue.Order,
					Intersection: inter,
					Detail:       fmt.Sprintf("regions overlap by %.0fx%.0fpx", inter.W, inter.H),
				})
			}
		}
	}
	return out
}

func detectAuthoredBounds(boxes []*AuthoredBox, cv Canvas) []Violation {
	var out []Violation
	for _, b := range boxes {
		if oob := outOfBoundsRect(b.Rect, cv); oob != "" {
			out = append(out, Violation{
				Kind: ViolationOutOfBounds, CueA: b.Cue.Order, CueB: -1,
				Detail: oob,
			})
		}
	}
	return out
}

func outOfBoundsRect(r Rect, cv Canvas) string {
	const eps = 0.5
	var sides []string
	if r.X < -eps {
		sides = append(sides, fmt.Sprintf("left %.0fpx", -r.X))
	}
	if r.Y < -eps {
		sides = append(sides, fmt.Sprintf("top %.0fpx", -r.Y))
	}
	if r.Right() > cv.Width+eps {
		sides = append(sides, fmt.Sprintf("right %.0fpx", r.Right()-cv.Width))
	}
	if r.Bottom() > cv.Height+eps {
		sides = append(sides, fmt.Sprintf("bottom %.0fpx", r.Bottom()-cv.Height))
	}
	if len(sides) == 0 {
		return ""
	}
	return "box outside canvas at " + joinWords(sides)
}

func detectFinalViolations(plan *Plan) []Violation {
	var out []Violation
	boxes := plan.Boxes
	for i := 0; i < len(boxes); i++ {
		if !boxes[i].PlacementOK {
			out = append(out, Violation{
				Kind: ViolationOutOfBounds, CueA: boxes[i].Cue.Order, CueB: -1,
				Detail: "cannot fit inside safe canvas while respecting priority",
			})
		}
		for j := i + 1; j < len(boxes); j++ {
			a, b := boxes[i], boxes[j]
			if !OverlapTime(a.Cue, b.Cue) {
				continue
			}
			inter := a.Rect.Intersection(b.Rect)
			if inter.W > 0.5 && inter.H > 0.5 {
				out = append(out, Violation{
					Kind: ViolationOverlap, CueA: a.Cue.Order, CueB: b.Cue.Order,
					Intersection: inter,
					Detail:       fmt.Sprintf("re layout still overlaps by %.0fx%.0fpx", inter.W, inter.H),
				})
			}
		}
	}
	return out
}

// DetectCombos finds certified unsolvable simultaneous combinations using an
// x axis sweep: at any horizontal coordinate, every box covering it must be
// vertically disjoint, so summed heights plus gaps cannot exceed the safe
// height. Interval graphs make this peak set a real horizontal clique.
func DetectCombos(doc *Document, cv Canvas, plan *Plan) []*Combo {
	safe := cv.SafeRect()
	gap := plan.Options.gap(cv)
	stages := BuildStages(doc.Cues)

	var combos []*Combo
	seen := map[string]bool{}

	for _, stage := range stages {
		peakOrders, totalH := horizontalPeak(stage.Cues, plan.ByOrder, gap)
		if totalH <= safe.H+0.5 {
			continue
		}
		key := ordersKey(peakOrders)
		if seen[key] {
			continue
		}
		seen[key] = true
		overflow := totalH - safe.H
		combos = append(combos, &Combo{
			ID:                fmt.Sprintf("combo-%d", len(combos)+1),
			StartMS:           stage.StartMS,
			EndMS:             stage.EndMS,
			CueOrders:         peakOrders,
			TotalHeightPx:     totalH,
			AvailableHeightPx: safe.H,
			OverflowPx:        overflow,
			Certified:         true,
			Reason: fmt.Sprintf(
				"%d horizontally overlapping cue blocks need %.0fpx but safe canvas offers %.0fpx (overflow %.0fpx)",
				len(peakOrders), totalH, safe.H, overflow),
		})
	}

	// Surface solver failures that the height bound did not certify.
	for _, b := range plan.Boxes {
		if b.PlacementOK {
			continue
		}
		active := simultaneousOrders(doc, b.Cue)
		key := ordersKey(active)
		if seen[key] {
			continue
		}
		seen[key] = true
		combos = append(combos, &Combo{
			ID:                fmt.Sprintf("combo-%d", len(combos)+1),
			StartMS:           b.Cue.StartMS,
			EndMS:             b.Cue.EndMS,
			CueOrders:         active,
			AvailableHeightPx: safe.H,
			Certified:         false,
			Reason:            "priority aware packing could not place every simultaneous cue",
		})
	}

	sort.Slice(combos, func(i, j int) bool { return combos[i].StartMS < combos[j].StartMS })
	for i, c := range combos {
		c.ID = fmt.Sprintf("combo-%d", i+1)
	}
	return combos
}

// horizontalPeak returns the cue orders covering the x coordinate with the
// largest summed block height.
func horizontalPeak(cues []*Cue, byOrder map[int]*Box, gap float64) ([]int, float64) {
	type xev struct {
		x     float64
		order int
		start bool
	}
	var events []xev
	for _, c := range cues {
		b := byOrder[c.Order]
		if b == nil {
			continue
		}
		events = append(events, xev{b.Rect.X, c.Order, true}, xev{b.Rect.Right(), c.Order, false})
	}
	sort.Slice(events, func(i, j int) bool {
		if events[i].x != events[j].x {
			return events[i].x < events[j].x
		}
		// Ends before starts at the same coordinate (zero width touch is disjoint).
		if events[i].start != events[j].start {
			return !events[i].start
		}
		return events[i].order < events[j].order
	})

	active := map[int]bool{}
	var best []int
	bestH := 0.0
	curH := 0.0
	for _, ev := range events {
		if ev.start {
			active[ev.order] = true
		} else {
			delete(active, ev.order)
		}
		orders := make([]int, 0, len(active))
		for o := range active {
			orders = append(orders, o)
		}
		sort.Ints(orders)
		curH = 0
		for i, o := range orders {
			curH += byOrder[o].Rect.H
			if i > 0 {
				curH += gap
			}
		}
		if curH > bestH {
			bestH = curH
			best = append([]int(nil), orders...)
		}
	}
	return best, bestH
}

func simultaneousOrders(doc *Document, cue *Cue) []int {
	var out []int
	for _, c := range doc.Cues {
		if OverlapTime(cue, c) {
			out = append(out, c.Order)
		}
	}
	sort.Ints(out)
	return out
}

func ordersKey(orders []int) string {
	s := append([]int(nil), orders...)
	sort.Ints(s)
	out := ""
	for i, o := range s {
		if i > 0 {
			out += "-"
		}
		out += itoa(o)
	}
	return out
}

func clampFloat(v, lo, hi float64) float64 {
	if hi < lo {
		return lo
	}
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// nearestFeasibleY searches the finite set of vertical boundary positions
// (desired position, safe edges, above/below every obstacle) and returns the
// in safe bounds, non intersecting Y closest to desiredY. The edge only
// breaks exact distance ties toward the anchored edge.
func nearestFeasibleY(rect Rect, desiredY float64, edge Edge, obstacles []*Box, safe Rect, gap float64) (float64, bool) {
	h := rect.H
	candidates := []float64{desiredY, safe.Y, safe.Bottom() - h}
	for _, p := range obstacles {
		candidates = append(candidates, p.Rect.Y-h-gap, p.Rect.Bottom()+gap)
	}

	const eps = 0.5
	bestY := 0.0
	bestDist := 0.0
	found := false
	for _, y := range candidates {
		if y < safe.Y-eps || y+h > safe.Bottom()+eps {
			continue
		}
		try := Rect{X: rect.X, Y: y, W: rect.W, H: h}
		clear := true
		for _, p := range obstacles {
			if try.Intersects(p.Rect) {
				clear = false
				break
			}
		}
		if !clear {
			continue
		}
		dist := absFloat(y - desiredY)
		if !found || dist < bestDist-eps ||
			(absFloat(dist-bestDist) <= eps && tiePrefers(y, bestY, edge)) {
			found = true
			bestY = y
			bestDist = dist
		}
	}
	return bestY, found
}

func tiePrefers(y, bestY float64, edge Edge) bool {
	if edge == EdgeBottom {
		return y > bestY // lower on screen wins
	}
	return y < bestY // higher on screen wins
}

func absFloat(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

func joinWords(parts []string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += ", "
		}
		out += p
	}
	return out
}
