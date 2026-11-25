package wavfix_test

import (
	"bytes"
	"encoding/binary"
	"testing"
)

type testChunk struct {
	id      string
	payload []byte
}

type wavFixture struct {
	t *testing.T

	tag        uint16
	channels   uint16
	sampleRate uint32
	bits       uint16
	blockAlign uint16
	audio      []byte

	factCount   *uint32
	factPayload []byte
	omitFact    bool

	beforeFact []testChunk
	afterData  []testChunk

	riffSizeOverride *uint32
	dataSizeOverride *uint32
}

func newG711Fixture(t *testing.T) *wavFixture {
	t.Helper()
	return &wavFixture{
		t:          t,
		tag:        7,
		channels:   2,
		sampleRate: 8000,
		bits:       8,
		blockAlign: 2,
		audio:      bytes.Repeat([]byte{0x80, 0x7f}, 1000),
	}
}

func writeTestChunk(buf *bytes.Buffer, id string, payload []byte) {
	if len(id) != 4 {
		panic("chunk id must be four bytes")
	}
	buf.WriteString(id)
	var size [4]byte
	binary.LittleEndian.PutUint32(size[:], uint32(len(payload)))
	buf.Write(size[:])
	buf.Write(payload)
	if len(payload)%2 == 1 {
		buf.WriteByte(0)
	}
}

func (f *wavFixture) build() []byte {
	f.t.Helper()

	var fmtPayload bytes.Buffer
	_ = binary.Write(&fmtPayload, binary.LittleEndian, f.tag)
	_ = binary.Write(&fmtPayload, binary.LittleEndian, f.channels)
	_ = binary.Write(&fmtPayload, binary.LittleEndian, f.sampleRate)

	blockAlign := f.blockAlign
	if blockAlign == 0 {
		blockAlign = f.channels * (f.bits / 8)
	}
	byteRate := f.sampleRate * uint32(blockAlign)
	_ = binary.Write(&fmtPayload, binary.LittleEndian, byteRate)
	_ = binary.Write(&fmtPayload, binary.LittleEndian, blockAlign)
	_ = binary.Write(&fmtPayload, binary.LittleEndian, f.bits)

	var file bytes.Buffer
	file.WriteString("RIFF")
	file.Write([]byte{0, 0, 0, 0})
	file.WriteString("WAVE")
	writeTestChunk(&file, "fmt ", fmtPayload.Bytes())

	for _, chunk := range f.beforeFact {
		writeTestChunk(&file, chunk.id, chunk.payload)
	}

	if f.factPayload != nil {
		writeTestChunk(&file, "fact", f.factPayload)
	} else if !f.omitFact {
		count := uint32(len(f.audio) / int(blockAlign))
		if f.factCount != nil {
			count = *f.factCount
		}
		var payload [4]byte
		binary.LittleEndian.PutUint32(payload[:], count)
		writeTestChunk(&file, "fact", payload[:])
	}

	dataSize := uint32(len(f.audio))
	if f.dataSizeOverride != nil {
		dataSize = *f.dataSizeOverride
	}
	file.WriteString("data")
	var sizeBytes [4]byte
	binary.LittleEndian.PutUint32(sizeBytes[:], dataSize)
	file.Write(sizeBytes[:])
	file.Write(f.audio)
	if len(f.audio)%2 == 1 {
		file.WriteByte(0)
	}

	for _, chunk := range f.afterData {
		writeTestChunk(&file, chunk.id, chunk.payload)
	}

	raw := file.Bytes()
	if f.riffSizeOverride != nil {
		binary.LittleEndian.PutUint32(raw[4:8], *f.riffSizeOverride)
	} else {
		binary.LittleEndian.PutUint32(raw[4:8], uint32(len(raw)-8))
	}
	return raw
}

func uint32Ptr(v uint32) *uint32 { return &v }
