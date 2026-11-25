package cafmarker

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Expectation gives optional ground truth for one marker.
type Expectation struct {
	MarkerIndex  *int    `json:"marker_index,omitempty"`
	MarkerID     *uint32 `json:"marker_id,omitempty"`
	MarkerName   string  `json:"marker_name,omitempty"`
	ActualFrame  *int64  `json:"actual_frame,omitempty"`
	ActualPacket *int    `json:"actual_packet,omitempty"`
}

// Options controls diagnosis.
type Options struct {
	Expectations     []Expectation `json:"expectations,omitempty"`
	MarkerIDIsPacket bool          `json:"marker_id_is_packet,omitempty"`
}

// MarkerDiagnosis contains mapping and off-by-one evidence for one marker.
type MarkerDiagnosis struct {
	Index            int            `json:"index"`
	Marker           Marker         `json:"marker"`
	Location         FrameLocation  `json:"location"`
	ReferencePacket  *int           `json:"reference_packet,omitempty"`
	ActualFrame      *int64         `json:"actual_frame,omitempty"`
	ActualFrameKnown bool           `json:"actual_frame_known,omitempty"`
	ActualPacket     *int           `json:"actual_packet,omitempty"`
	ActualLocation   *FrameLocation `json:"actual_location,omitempty"`
	FrameDelta       int64          `json:"frame_delta,omitempty"`
	PacketDelta      int            `json:"packet_delta,omitempty"`
	Classification   string         `json:"classification"`
	Confidence       string         `json:"confidence"`
	ErrorSource      string         `json:"error_source,omitempty"`
	CorrectedFrame   *int64         `json:"corrected_frame,omitempty"`
	CorrectedPacket  *int           `json:"corrected_packet,omitempty"`
	Evidence         []string       `json:"evidence,omitempty"`
}

// Report is the full CAF marker diagnostic report.
type Report struct {
	Path        string            `json:"path,omitempty"`
	FileSize    int64             `json:"file_size"`
	Format      *Format           `json:"format,omitempty"`
	PacketTable *PacketTable      `json:"packet_table,omitempty"`
	Map         *PacketMap        `json:"map,omitempty"`
	Markers     []MarkerDiagnosis `json:"markers"`
	Findings    []Issue           `json:"findings,omitempty"`
	Issues      []Issue           `json:"issues,omitempty"`
	Verdict     string            `json:"verdict"`
	RootCause   string            `json:"root_cause,omitempty"`
}

var packetReferenceRE = regexp.MustCompile(`(?i)(?:packet|pkt|p)[=_:\- ]*#?(\d+)`)

// ParsePacketReference extracts a packet index from an instrumentation label.
func ParsePacketReference(name string) (int, bool) {
	m := packetReferenceRE.FindStringSubmatch(name)
	if m == nil {
		return 0, false
	}
	n, err := strconv.Atoi(m[1])
	if err != nil || n < 0 {
		return 0, false
	}
	return n, true
}

// AnalyzeFile parses path and runs diagnosis without explicit expectations.
func AnalyzeFile(path string) (*Report, error) {
	f, err := ParseFile(path)
	if err != nil && f == nil {
		return nil, err
	}
	return Diagnose(f, Options{}), err
}

// Diagnose builds the packet map and classifies marker offset patterns.
func Diagnose(f *File, opts Options) *Report {
	report := &Report{Path: f.Path, FileSize: f.FileSize, Format: f.Format, PacketTable: f.PacketTable}
	report.Issues = append(report.Issues, f.Issues...)

	m, mapErr := f.BuildMap()
	if m != nil {
		report.Map = m
		report.Issues = append(report.Issues, m.Issues...)
	}
	if mapErr != nil {
		report.Findings = append(report.Findings, Issue{"map_unavailable", mapErr.Error()})
	}

	onePacketCount := 0
	for i, marker := range f.Markers {
		d := MarkerDiagnosis{Index: i, Marker: marker, Classification: "no_shift_detected", Confidence: "low"}
		if m != nil {
			d.Location = m.LocateFrame(marker.Frame, f)
		}
		if pkt, ok := markerPacketReference(marker, opts); ok {
			d.ReferencePacket = &pkt
		}
		if exp := findExpectation(opts.Expectations, i, marker); exp != nil && m != nil {
			applyExpectation(&d, *exp, m, f)
		}
		classify(&d, m, f)
		if strings.HasPrefix(d.Classification, "one_packet_late") {
			onePacketCount++
		}
		report.Markers = append(report.Markers, d)
	}

	report.Issues = dedupeIssues(report.Issues)
	report.Findings = dedupeIssues(report.Findings)
	if onePacketCount > 0 {
		report.Verdict = fmt.Sprintf("%d marker(s) show the canonical one-packet-late signature", onePacketCount)
		report.RootCause = "The instrumentation used the post-write packet cursor as the marker packet index. " +
			"After writing packet N, the cursor/count is N+1; converting that cursor directly to a CAF frame " +
			"stores FrameStart[N+1] (equivalently FrameEnd[N]) instead of FrameStart[N]. For variable packets, " +
			"the shift must be measured with cumulative pakt frame boundaries, not index*FramesPerPacket or byte sizes."
	} else if len(report.Markers) == 0 {
		report.Verdict = "no mark chunk markers were found"
	} else {
		report.Verdict = "no proven one-packet shift; inspect marker boundary and expectation evidence"
	}
	return report
}

func markerPacketReference(marker Marker, opts Options) (int, bool) {
	if pkt, ok := ParsePacketReference(marker.Name); ok {
		return pkt, true
	}
	if marker.Name != "" {
		if n, err := strconv.Atoi(strings.TrimSpace(marker.Name)); err == nil && n >= 0 {
			return n, true
		}
	}
	if opts.MarkerIDIsPacket {
		return int(marker.MarkerID), true
	}
	return 0, false
}

func findExpectation(exps []Expectation, index int, marker Marker) *Expectation {
	for i := range exps {
		e := &exps[i]
		if e.MarkerIndex != nil && *e.MarkerIndex == index {
			return e
		}
		if e.MarkerID != nil && *e.MarkerID == marker.MarkerID {
			return e
		}
		if e.MarkerName != "" && (e.MarkerName == marker.Name || e.MarkerName == fmt.Sprint(marker.MarkerID)) {
			return e
		}
	}
	return nil
}

func applyExpectation(d *MarkerDiagnosis, e Expectation, m *PacketMap, f *File) {
	if e.ActualFrame != nil {
		frame := *e.ActualFrame
		d.ActualFrame = &frame
		d.ActualFrameKnown = true
		loc := m.LocateFrame(frame, f)
		d.ActualLocation = &loc
		if d.Marker.FrameValid {
			d.FrameDelta = d.Marker.Frame - frame
		}
		markerIndex := packetIndexForDelta(d.Location)
		actualIndex := packetIndexForDelta(loc)
		if markerIndex >= 0 && actualIndex >= 0 {
			d.PacketDelta = markerIndex - actualIndex
		}
	}
	if e.ActualPacket != nil {
		p := *e.ActualPacket
		d.ActualPacket = &p
		if d.ActualLocation == nil && m != nil {
			packet, ok := m.PacketAt(p)
			if !ok {
				return
			}
			loc := m.LocateFrame(packet.FrameStart, f)
			d.ActualLocation = &loc
			frameStart := packet.FrameStart
			d.ActualFrame = &frameStart
			if d.Marker.FrameValid {
				d.FrameDelta = d.Marker.Frame - *d.ActualFrame
			}
			if markerIndex := packetIndexForDelta(d.Location); markerIndex >= 0 {
				d.PacketDelta = markerIndex - p
			}
		}
	}
}

func packetIndexForDelta(loc FrameLocation) int {
	if loc.PacketIndex >= 0 {
		return loc.PacketIndex
	}
	if loc.BoundaryAfterPacket >= 0 {
		return loc.BoundaryAfterPacket + 1
	}
	return -1
}

func classify(d *MarkerDiagnosis, m *PacketMap, f *File) {
	if !d.Marker.FrameValid {
		d.Classification = "invalid_frame_position"
		d.Confidence = "high"
		d.ErrorSource = "marker mFramePosition is NaN, infinite, negative, fractional beyond conversion, or out of int64 range"
		return
	}

	for _, status := range d.Location.Status {
		switch status {
		case "short_byte_tail", "packet_data_short", "packet_table_truncated":
			d.Evidence = append(d.Evidence, "tail/table insufficiency: "+status)
		case "priming_frame":
			d.Evidence = append(d.Evidence, "frame lies in encoder priming padding before audible playback starts")
		case "remainder_frame":
			d.Evidence = append(d.Evidence, "frame lies in trailing remainder padding removed during playback")
		}
	}

	if d.ActualPacket != nil || d.ActualFrame != nil {
		classifyAgainstExpectation(d, m)
		if d.Classification != "no_shift_detected" {
			return
		}
	}

	if d.ReferencePacket != nil && m != nil {
		ref := *d.ReferencePacket
		if markerPacket := packetIndexForDelta(d.Location); markerPacket >= 0 {
			delta := markerPacket - ref
			switch {
			case delta == 0:
				d.Classification = "reference_matches"
				d.Confidence = "high"
				d.Evidence = append(d.Evidence, fmt.Sprintf("marker reference packet %d matches CAF packet mapping", ref))
				return
			case delta == 1:
				setOnePacketLate(d, m, ref)
				d.Confidence = "high"
				d.Evidence = append(d.Evidence, fmt.Sprintf("marker label references packet %d but frame maps to packet %d", ref, markerPacket))
				return
			case delta == -1:
				d.Classification = "one_packet_early"
				d.Confidence = "high"
				d.ErrorSource = "one-based packet ordinal or pre-decrement index was converted directly to a zero-based packet boundary"
				d.Evidence = append(d.Evidence, fmt.Sprintf("marker label references packet %d but frame maps to packet %d", ref, markerPacket))
				return
			default:
				d.Classification = "packet_delta"
				d.Confidence = "high"
				d.Evidence = append(d.Evidence, fmt.Sprintf("marker label references packet %d but frame maps to packet %d (delta=%d)", ref, d.Location.PacketIndex, delta))
				return
			}
		}
	}

	if d.Location.Boundary && m != nil && d.Location.BoundaryAfterPacket >= 0 {
		p := d.Location.BoundaryAfterPacket
		if packet, ok := m.PacketAt(p); ok && packet.FrameCount > 0 {
			d.Classification = "packet_boundary"
			d.Confidence = "info"
			d.ErrorSource = "if playback is known to be packet N while this marker maps to N+1, the source is post-increment packet accounting: the writer recorded the advanced cursor/FrameEnd[N] instead of FrameStart[N]"
			d.Evidence = append(d.Evidence,
				fmt.Sprintf("CAF zero-based mapping: frame %d is start of packet %d; the same coordinate is the exclusive end of packet %d (%d frames, %d bytes)",
					d.Marker.Frame, p+1, p, packet.FrameCount, packet.ByteSize))
			d.Evidence = append(d.Evidence,
				fmt.Sprintf("if instrumentation intended packet %d but reports this coordinate, the likely source is post-increment/one-based indexing: its cursor was already %d", p, p+1))
		}
	}
}

func classifyAgainstExpectation(d *MarkerDiagnosis, m *PacketMap) {
	if d.PacketDelta == 0 && (!d.ActualFrameKnown || d.FrameDelta == 0) {
		d.Classification = "aligned"
		d.Confidence = "high"
		d.Evidence = append(d.Evidence, "marker packet matches supplied real playback position")
		return
	}
	if d.PacketDelta == 1 && d.ActualLocation != nil && d.ActualLocation.PacketIndex >= 0 {
		actualPacket := d.ActualLocation.PacketIndex
		if m != nil {
			p, ok := m.PacketAt(actualPacket)
			if !ok {
				return
			}
			deltaIsOnePacket := d.FrameDelta == p.FrameCount
			next, hasNext := m.PacketAt(actualPacket + 1)
			atNextStart := hasNext && d.Marker.Frame == next.FrameStart
			if deltaIsOnePacket || atNextStart {
				setOnePacketLate(d, m, actualPacket)
				d.Confidence = "high"
				d.Evidence = append(d.Evidence, fmt.Sprintf("marker frame %d versus actual frame %d: delta=%d frames; packet %d contains %d frames and %d bytes",
					d.Marker.Frame, *d.ActualFrame, d.FrameDelta, actualPacket, p.FrameCount, p.ByteSize))
				return
			}
		}
		d.Classification = "packet_index_one_late"
		d.Confidence = "medium"
		d.ErrorSource = "marker maps one packet after the supplied real playback packet; likely post-increment/one-based packet indexing"
		d.Evidence = append(d.Evidence, fmt.Sprintf("packet index delta=%d, frame delta=%d", d.PacketDelta, d.FrameDelta))
		if d.ActualPacket != nil {
			correctedPacket := *d.ActualPacket
			d.CorrectedPacket = &correctedPacket
		}
		return
	}
	if d.FrameDelta != 0 {
		d.Classification = "offset"
		d.Confidence = "high"
		d.Evidence = append(d.Evidence, fmt.Sprintf("frame delta=%d, packet delta=%d", d.FrameDelta, d.PacketDelta))
	}
}

func setOnePacketLate(d *MarkerDiagnosis, m *PacketMap, packet int) {
	d.Classification = "one_packet_late"
	d.ErrorSource = "post_increment_packet_cursor_or_end_boundary: instrumentation recorded the next packet start after incrementing the packet count"
	p, ok := m.PacketAt(packet)
	if m == nil || !ok {
		return
	}
	correctedFrame := p.FrameStart
	correctedPacket := packet
	d.CorrectedFrame = &correctedFrame
	d.CorrectedPacket = &correctedPacket
	d.Evidence = append(d.Evidence, fmt.Sprintf("correct zero-based packet %d starts at frame %d; one-packet shift is %d frames / %d bytes for this variable packet",
		packet, p.FrameStart, p.FrameCount, p.ByteSize))
}

func dedupeIssues(in []Issue) []Issue {
	seen := map[string]bool{}
	out := make([]Issue, 0, len(in))
	for _, issue := range in {
		key := issue.Code + "\x00" + issue.Message
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, issue)
	}
	return out
}
