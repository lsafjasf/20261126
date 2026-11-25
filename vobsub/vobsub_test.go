package vobsub

import (
	"bytes"
	"encoding/binary"
	"errors"
	"strings"
	"testing"
	"time"
)

const testIDX = `# VobSub index file
size: 720x576
org: 0, 0

id: en, index: 0
timestamp: 00:00:01:000, filepos: 000000000
timestamp: 00:00:10:000, filepos: 000001000

id: zh, index: 1
timestamp: 00:00:02:000, filepos: 000002000
timestamp: 00:00:12:000, filepos: 000003000
`

// makePacket builds a minimal MPEG-PS pack carrying one subpicture PES
// packet with the given sub-stream id.
func makePacket(subID byte) []byte {
	var b bytes.Buffer
	b.Write([]byte{0x00, 0x00, 0x01, 0xBA})             // pack start code
	b.Write([]byte{0x44, 0x00, 0x04, 0x00, 0x04, 0x01}) // MPEG-2 pack header stub
	b.Write([]byte{0x00, 0x00, 0x01, 0xBD})             // private_stream_1
	binary.Write(&b, binary.BigEndian, uint16(4))       // PES length
	b.Write([]byte{0x80, 0x00, 0x00})                   // flags + header_data_length=0
	b.WriteByte(subID)                                  // sub-stream id
	b.Write([]byte{0xAA, 0xBB, 0xCC})                   // dummy payload
	return b.Bytes()
}

// makeSUB builds a .sub image with packets at the offsets used by testIDX:
// stream 0 packets at 0x0 and 0x1000, stream 1 packets at 0x2000 and 0x3000.
func makeSUB(t *testing.T) *bytes.Reader {
	t.Helper()
	data := make([]byte, 0x4000)
	for _, spec := range []struct {
		off   int
		subID byte
	}{{0x0000, 0x20}, {0x1000, 0x20}, {0x2000, 0x21}, {0x3000, 0x21}} {
		copy(data[spec.off:], makePacket(spec.subID))
	}
	return bytes.NewReader(data)
}

func TestParseIDX(t *testing.T) {
	idx, err := ParseIDX(strings.NewReader(testIDX))
	if err != nil {
		t.Fatalf("ParseIDX: %v", err)
	}
	if got := idx.Indices(); len(got) != 2 || got[0] != 0 || got[1] != 1 {
		t.Fatalf("Indices = %v, want [0 1]", got)
	}
	st := idx.Streams[1]
	if st.Lang != "zh" || len(st.Entries) != 2 {
		t.Fatalf("stream 1 = %+v", st)
	}
	if st.Entries[0].Timestamp != 2*time.Second || st.Entries[0].FilePos != 0x2000 {
		t.Fatalf("entry = %+v", st.Entries[0])
	}
}

func TestParseIDXErrors(t *testing.T) {
	cases := map[string]string{
		"timestamp before id": "timestamp: 00:00:01:000, filepos: 000000000\n",
		"duplicate index":     "id: en, index: 0\nid: zh, index: 0\n",
		"no streams":          "size: 720x576\n",
		"bad filepos":         "id: en, index: 0\ntimestamp: 00:00:01:000, filepos: zz\n",
	}
	for name, in := range cases {
		if _, err := ParseIDX(strings.NewReader(in)); err == nil {
			t.Errorf("%s: expected error, got nil", name)
		}
	}
}

func TestValidateOffset(t *testing.T) {
	sub := makeSUB(t)
	if err := ValidateOffset(sub, 0x2000, 1); err != nil {
		t.Fatalf("valid offset rejected: %v", err)
	}
	if err := ValidateOffset(sub, 0x2000, 0); err == nil {
		t.Fatal("mismatched sub-stream id accepted")
	}
	if err := ValidateOffset(sub, 0x0800, 0); err == nil {
		t.Fatal("offset without pack start code accepted")
	}
	if err := ValidateOffset(sub, 0xFFFF0000, 0); err == nil {
		t.Fatal("out-of-range offset accepted")
	}
}

func TestSwitchAngleKeepsPosition(t *testing.T) {
	idx, err := ParseIDX(strings.NewReader(testIDX))
	if err != nil {
		t.Fatalf("ParseIDX: %v", err)
	}
	sw := NewSwitcher(idx, makeSUB(t))

	e, err := sw.SwitchAngle(0, 11*time.Second)
	if err != nil {
		t.Fatalf("SwitchAngle(0): %v", err)
	}
	if e.Timestamp != 10*time.Second || e.FilePos != 0x1000 {
		t.Fatalf("angle 0 cue = %+v", e)
	}

	// Switching angle at the same playback position must land on the cue
	// covering that position in the new stream, not restart from the top.
	e, err = sw.SwitchAngle(1, 11*time.Second)
	if err != nil {
		t.Fatalf("SwitchAngle(1): %v", err)
	}
	if e.Timestamp != 2*time.Second || e.FilePos != 0x2000 {
		t.Fatalf("angle 1 cue = %+v, want cue at 2s/0x2000", e)
	}

	// Position before the first cue clamps to the first cue.
	e, err = sw.SwitchAngle(1, 0)
	if err != nil {
		t.Fatalf("SwitchAngle(1, 0): %v", err)
	}
	if e.Timestamp != 2*time.Second {
		t.Fatalf("clamped cue = %+v", e)
	}
}

func TestSwitchAngleMissing(t *testing.T) {
	idx, err := ParseIDX(strings.NewReader(testIDX))
	if err != nil {
		t.Fatalf("ParseIDX: %v", err)
	}
	sw := NewSwitcher(idx, nil)
	_, err = sw.SwitchAngle(7, time.Second)
	var missing *MissingAngleError
	if !errors.As(err, &missing) {
		t.Fatalf("err = %v, want MissingAngleError", err)
	}
	if missing.Requested != 7 || len(missing.Available) != 2 {
		t.Fatalf("missing = %+v", missing)
	}
	if !strings.Contains(err.Error(), "0, 1") {
		t.Fatalf("error should list available angles: %v", err)
	}
}

func TestSwitchAngleEmptyStream(t *testing.T) {
	idx, err := ParseIDX(strings.NewReader("id: en, index: 0\ntimestamp: 00:00:01:000, filepos: 000000000\nid: fr, index: 1\n"))
	if err != nil {
		t.Fatalf("ParseIDX: %v", err)
	}
	sw := NewSwitcher(idx, nil)
	_, err = sw.SwitchAngle(1, 0)
	var empty *EmptyStreamError
	if !errors.As(err, &empty) {
		t.Fatalf("err = %v, want EmptyStreamError", err)
	}
}

func TestCueAtValidatesOffset(t *testing.T) {
	// idx claims a stream-0 cue at 0x2000, but that packet holds sub-stream
	// id 0x21: validation must reject the stale offset.
	idx, err := ParseIDX(strings.NewReader(
		"id: en, index: 0\ntimestamp: 00:00:01:000, filepos: 000002000\n"))
	if err != nil {
		t.Fatalf("ParseIDX: %v", err)
	}
	sw := NewSwitcher(idx, makeSUB(t))
	if _, err := sw.SwitchAngle(0, 2*time.Second); err == nil {
		t.Fatal("stale offset accepted")
	}
}
