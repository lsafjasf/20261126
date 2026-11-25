package ttmlevents

import (
	"io"
	"sort"
	"strings"
	"time"
)

// Analyze parses and builds the deterministic TTML event table.
func Analyze(r io.Reader) (*EventTable, error) {
	doc, err := Parse(r)
	if err != nil {
		return nil, err
	}
	return BuildEventTable(doc), nil
}

// AnalyzeString parses source and builds the deterministic event table.
func AnalyzeString(source string) (*EventTable, error) {
	doc, err := ParseString(source)
	if err != nil {
		return nil, err
	}
	return BuildEventTable(doc), nil
}

// AnalyzeWithOptions parses and builds a table using timing compatibility options.
func AnalyzeWithOptions(r io.Reader, options TimingOptions) (*EventTable, error) {
	doc, err := Parse(r)
	if err != nil {
		return nil, err
	}
	return BuildEventTableWithOptions(doc, options), nil
}

// AnalyzeStringWithOptions parses source and builds a table with options.
func AnalyzeStringWithOptions(source string, options TimingOptions) (*EventTable, error) {
	doc, err := ParseString(source)
	if err != nil {
		return nil, err
	}
	return BuildEventTableWithOptions(doc, options), nil
}

// Build is a short alias for BuildEventTable.
func Build(doc *Document) *EventTable { return BuildEventTable(doc) }

// BuildEventTable resolves timing, flattens text/styles/animations, and emits
// sorted slices plus causally ordered events.
func BuildEventTable(doc *Document) *EventTable {
	return BuildEventTableWithOptions(doc, TimingOptions{})
}

// BuildEventTableWithOptions builds an event table with timing options.
func BuildEventTableWithOptions(doc *Document, options TimingOptions) *EventTable {
	table := &EventTable{
		OrderingRules: []string{
			"time ascending",
			"at equal time: end/retire, then begin/enter, then style/keyframe change",
			"begin order: XML pre-order traversal (ancestor before descendant)",
			"end order: XML post-order traversal (descendant before ancestor)",
			"all ties use stable source order; Go maps are never used for output ordering",
			"active intervals are half-open [begin,end)",
		},
	}
	if doc == nil {
		table.Warnings = []Warning{{Code: "nil-document", Message: "BuildEventTable received nil document"}}
		return table
	}

	tl := ResolveTimingWithOptions(doc, options)
	resolver := newStyleResolver(doc)
	runs := collectTextRuns(doc, tl, resolver)
	animations := collectAnimations(doc, tl)

	table.Timeline = tl
	table.Animations = animations
	table.Runs = runs

	boundaries := collectBoundaries(tl, runs, animations)
	table.Slices = buildSlices(runs, animations, boundaries)
	table.StyleIntervals = buildStyleIntervals(runs, table.Slices, animations)
	table.Events = buildEvents(doc, tl, animations, table.Slices)
	table.Groups = groupEvents(table.Events, table.Slices)
	table.UniqueTimestamps = len(table.Groups)
	table.Warnings = append([]Warning(nil), doc.Warnings...)
	return table
}

func collectBoundaries(tl *Timeline, runs []TextRun, tracks []AnimationTrack) []time.Duration {
	set := make(map[time.Duration]struct{})
	add := func(d time.Duration) { set[d] = struct{}{} }

	if tl != nil {
		for _, info := range tl.Timings {
			if info == nil || !info.Visible {
				continue
			}
			add(info.Effective.Begin)
			if info.Effective.End != nil {
				add(*info.Effective.End)
			}
		}
	}
	for _, run := range runs {
		if !run.TimingVisible {
			continue
		}
		add(run.Begin)
		if run.End != nil {
			add(*run.End)
		}
	}
	for _, track := range tracks {
		add(track.Begin)
		if track.ActiveEnd != nil {
			add(*track.ActiveEnd)
		}
		for _, segment := range track.Segments {
			add(segment.Begin)
			if segment.End != nil {
				add(*segment.End)
			}
		}
	}

	out := make([]time.Duration, 0, len(set))
	for d := range set {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func buildSlices(runs []TextRun, tracks []AnimationTrack, boundaries []time.Duration) []Slice {
	slices := make([]Slice, 0, len(boundaries))
	for i := 0; i+1 < len(boundaries); i++ {
		begin, end := boundaries[i], boundaries[i+1]
		if end == begin {
			continue
		}
		slices = append(slices, makeSlice(len(slices), begin, &end, runs, tracks))
	}

	if len(boundaries) > 0 {
		last := boundaries[len(boundaries)-1]
		if hasUnboundedActive(runs, tracks, last) {
			slices = append(slices, makeSlice(len(slices), last, nil, runs, tracks))
		}
	}
	for i := range slices {
		slices[i].Index = i
	}
	return slices
}

func hasUnboundedActive(runs []TextRun, tracks []AnimationTrack, at time.Duration) bool {
	for _, run := range runs {
		if run.TimingVisible && run.End == nil && runActiveAt(run, at) {
			return true
		}
	}
	for _, track := range tracks {
		if track.End == nil && trackActiveAt(track, at) {
			return true
		}
	}
	return false
}

func makeSlice(index int, begin time.Duration, end *time.Duration, runs []TextRun, tracks []AnimationTrack) Slice {
	slice := Slice{
		Index:        index,
		Begin:        begin,
		BeginSeconds: durationSeconds(begin),
		End:          cloneDurationPtr(end),
		OpenEnded:    end == nil,
		Timecode:     formatDuration(begin),
	}
	if end != nil {
		sec := durationSeconds(*end)
		slice.EndSeconds = &sec
	}

	var text strings.Builder
	lastBlock := ""
	for _, run := range runs {
		if !runActiveAt(run, begin) {
			continue
		}
		effective := effectiveStyleForRun(run, begin, end, tracks)
		if effective.hidden() {
			continue
		}

		if run.Kind == "break" {
			text.WriteByte('\n')
			slice.Runs = append(slice.Runs, SliceRun{RunID: run.ID, Text: "\n", NodePath: run.NodePath, Style: effective.properties()})
			continue
		}

		if run.BlockPath != "" && lastBlock != "" && run.BlockPath != lastBlock && text.Len() > 0 {
			current := text.String()
			if !strings.HasSuffix(current, "\n") {
				text.WriteByte('\n')
			}
		}
		text.WriteString(run.Text)
		lastBlock = run.BlockPath
		if !run.Whitespace {
			slice.Runs = append(slice.Runs, SliceRun{
				RunID:    run.ID,
				Text:     run.Text,
				NodePath: run.NodePath,
				Style:    effective.properties(),
			})
		}
	}
	slice.Text = normalizeSliceText(text.String())
	slice.Empty = slice.Text == "" && len(slice.Runs) == 0
	return slice
}

func normalizeSliceText(raw string) string {
	if raw == "" {
		return ""
	}
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	raw = strings.ReplaceAll(raw, "\r", "\n")
	lines := strings.Split(raw, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimSpace(whitespaceRun.ReplaceAllString(line, " "))
	}
	out := strings.Join(lines, "\n")
	out = strings.Trim(out, "\n")
	return out
}

func groupEvents(events []Event, slices []Slice) []EventGroup {
	groups := make([]EventGroup, 0)
	indexByBegin := make(map[time.Duration]int, len(slices))
	for _, slice := range slices {
		indexByBegin[slice.Begin] = slice.Index
	}

	for _, event := range events {
		if len(groups) == 0 || groups[len(groups)-1].Time != event.Time {
			groups = append(groups, EventGroup{
				Time:            event.Time,
				TimeSeconds:     event.TimeSeconds,
				Timecode:        event.Timecode,
				SliceIndexAfter: -1,
			})
		}
		g := &groups[len(groups)-1]
		g.Events = append(g.Events, event)
		if idx, ok := indexByBegin[event.Time]; ok {
			g.SliceIndexAfter = idx
		}
	}
	return groups
}

func eventTimeFields(t time.Duration) (float64, string) {
	return durationSeconds(t), formatDuration(t)
}

func targetRef(id, path string) string {
	if id != "" {
		return "#" + id
	}
	return path
}
