package cafmarker

import (
	"testing"
)

func TestVariablePacketMarkerOnePacketLateByName(t *testing.T) {
	sizes := []int64{100, 200, 150}
	audio := make([]byte, 450)
	payload := paktPayload(3, 3*1024, 0, 0, sizes...)
	data := cafFile(
		chunk("desc", descPayload(48000, "aac ", 2, 0, 1024, 2, 0)),
		chunk("pakt", payload),
		chunk("strg", stringsPayload(7, "instrument/packet=1")),
		chunk("mark", markerPayload(2048, 7, "instrument/packet=1")),
		dataChunk(450, audio),
	)
	f := parseBytes(t, data)
	if len(f.Markers) != 1 || f.Markers[0].Name != "instrument/packet=1" {
		t.Fatalf("marker/name parse failed: %+v strings=%v", f.Markers, f.Strings)
	}
	m, err := f.BuildMap()
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Packets) != 3 {
		t.Fatalf("packets = %d, want 3", len(m.Packets))
	}
	if m.Packets[1].FrameStart != 1024 || m.Packets[2].FrameStart != 2048 {
		t.Fatalf("bad frame starts: %+v", m.Packets)
	}
	if m.Packets[2].ByteOffset != 300 {
		t.Fatalf("variable byte offset = %d, want 300", m.Packets[2].ByteOffset)
	}

	r := Diagnose(f, Options{})
	d := r.Markers[0]
	if d.Classification != "one_packet_late" || d.Confidence != "high" {
		t.Fatalf("classification = %q/%q source=%q evidence=%v", d.Classification, d.Confidence, d.ErrorSource, d.Evidence)
	}
	if d.CorrectedFrame == nil || *d.CorrectedFrame != 1024 || d.CorrectedPacket == nil || *d.CorrectedPacket != 1 {
		t.Fatalf("correction = %+v/%+v, want frame 1024 packet 1", d.CorrectedFrame, d.CorrectedPacket)
	}
}

func TestExplicitActualPacketDetectsShift(t *testing.T) {
	audio := make([]byte, 300)
	sizes := []int64{10, 20, 30, 40, 200}
	pakt := paktPayload(int64(len(sizes)), 5*512, 0, 0, sizes...)
	file := cafFile(
		chunk("desc", descPayload(48000, "aac ", 0, 0, 512, 2, 0)),
		chunk("pakt", pakt),
		chunk("mark", markerPayload(1024, 9, "")),
		dataChunk(300, audio),
	)
	f := parseBytes(t, file)
	actualPacket := 1
	r := Diagnose(f, Options{Expectations: []Expectation{{MarkerIndex: intPtr(0), ActualPacket: &actualPacket}}})
	d := r.Markers[0]
	if d.Classification != "one_packet_late" {
		t.Fatalf("classification=%q source=%s evidence=%v", d.Classification, d.ErrorSource, d.Evidence)
	}
	if d.FrameDelta != 512 || d.PacketDelta != 1 {
		t.Fatalf("deltas frame=%d packet=%d, want 512/1", d.FrameDelta, d.PacketDelta)
	}
}

func TestShortTailIsReportedAndMapped(t *testing.T) {
	sizes := []int64{10, 20, 30}
	audio := make([]byte, 45)
	file := cafFile(
		chunk("desc", descPayload(48000, "aac ", 0, 0, 1024, 2, 0)),
		chunk("pakt", paktPayload(3, 3072, 0, 0, sizes...)),
		chunk("mark", markerPayload(2048, 1, "")),
		dataChunk(60, audio),
	)
	f := parseBytes(t, file)
	if f.Data == nil || !f.Data.Truncated {
		t.Fatalf("data truncation not detected: %+v", f.Data)
	}
	m, err := f.BuildMap()
	if err != nil {
		t.Fatal(err)
	}
	last := m.Packets[2]
	if last.AvailableBytes != 15 || last.MissingBytes != 15 {
		t.Fatalf("tail availability = %+v, want available/missing 15", last)
	}
	loc := m.LocateFrame(2048, f)
	if loc.PacketIndex != 2 || !contains(loc.Status, "short_byte_tail") {
		t.Fatalf("location=%+v missing short_byte_tail", loc)
	}
	if !hasIssue(m.Issues, "packet_data_short") || !hasIssue(f.Issues, "truncated_chunk") {
		t.Fatalf("issues not reported: map=%v file=%v", m.Issues, f.Issues)
	}
}

func TestVariableFramePacketsAndPCMByteMapping(t *testing.T) {
	frameCounts := []int64{512, 400, 600}
	pakt := paktPayload(3, 1512, 0, 0, frameCounts...)
	file := cafFile(
		chunk("desc", descPayload(48000, "opus", 0, 128, 0, 2, 0)),
		chunk("pakt", pakt),
		chunk("mark", markerPayload(912, 1, "")),
		dataChunk(384, make([]byte, 384)),
	)
	f := parseBytes(t, file)
	m, err := f.BuildMap()
	if err != nil {
		t.Fatal(err)
	}
	if got := []int64{m.Packets[0].FrameEnd, m.Packets[1].FrameStart, m.Packets[2].FrameStart}; got[0] != 512 || got[1] != 512 || got[2] != 912 {
		t.Fatalf("variable frame boundaries wrong: %v", got)
	}
	if m.Packets[1].ByteSize != 128 {
		t.Fatalf("constant byte size not inherited: %+v", m.Packets[1])
	}
	loc := m.LocateFrame(912, f)
	if loc.PacketIndex != 2 || !loc.Boundary || loc.BoundaryAfterPacket != 1 {
		t.Fatalf("boundary location wrong: %+v", loc)
	}

	pcm := cafFile(chunk("desc", descPayload(48000, "lpcm", 0, 4, 1, 2, 16)),
		dataChunk(100, make([]byte, 100)))
	pf := parseBytes(t, pcm)
	pm, err := pf.BuildMap()
	if err != nil {
		t.Fatal(err)
	}
	if len(pm.Packets) != 25 {
		t.Fatalf("pcm packets=%d, want 25", len(pm.Packets))
	}
	pl := pm.LocateFrame(10, pf)
	if pl.AudioByteOffset != 40 || pl.FileByteOffset != pf.Data.AudioOffset+40 {
		t.Fatalf("pcm byte map wrong: %+v audioOffset=%d", pl, pf.Data.AudioOffset)
	}
}

func intPtr(v int) *int { return &v }

func contains(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}

func hasIssue(xs []Issue, code string) bool {
	for _, x := range xs {
		if x.Code == code {
			return true
		}
	}
	return false
}
