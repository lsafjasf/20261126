package cafmarker

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"
)

func chunk(typ string, payload []byte) []byte {
	var b bytes.Buffer
	b.WriteString(typ)
	_ = binary.Write(&b, binary.BigEndian, int64(len(payload)))
	b.Write(payload)
	return b.Bytes()
}

func cafVarInt(n int64) []byte {
	v := uint64(n)
	var groups []byte
	for {
		b := byte(v & 0x7f)
		v >>= 7
		groups = append(groups, b)
		if v == 0 {
			break
		}
	}
	out := make([]byte, len(groups))
	for i, g := range groups {
		out[len(groups)-1-i] = g
	}
	for i := 0; i < len(out)-1; i++ {
		out[i] |= 0x80
	}
	return out
}

func descPayload(sampleRate float64, formatID string, flags, bpp, fpp, channels, bits uint32) []byte {
	p := make([]byte, 32)
	binary.BigEndian.PutUint64(p[0:8], math.Float64bits(sampleRate))
	copy(p[8:12], formatID)
	binary.BigEndian.PutUint32(p[12:16], flags)
	binary.BigEndian.PutUint32(p[16:20], bpp)
	binary.BigEndian.PutUint32(p[20:24], fpp)
	binary.BigEndian.PutUint32(p[24:28], channels)
	binary.BigEndian.PutUint32(p[28:32], bits)
	return p
}

func paktPayload(count, validFrames int64, priming, remainder int32, values ...int64) []byte {
	var b bytes.Buffer
	_ = binary.Write(&b, binary.BigEndian, count)
	_ = binary.Write(&b, binary.BigEndian, validFrames)
	_ = binary.Write(&b, binary.BigEndian, priming)
	_ = binary.Write(&b, binary.BigEndian, remainder)
	for _, v := range values {
		b.Write(cafVarInt(v))
	}
	return b.Bytes()
}

func markerPayload(frame float64, id uint32, name string) []byte {
	var chunkData bytes.Buffer
	_ = binary.Write(&chunkData, binary.BigEndian, uint32(0))
	_ = binary.Write(&chunkData, binary.BigEndian, uint32(1))
	m := make([]byte, 28)
	binary.BigEndian.PutUint32(m[0:4], 0)
	binary.BigEndian.PutUint64(m[4:12], math.Float64bits(frame))
	binary.BigEndian.PutUint32(m[12:16], id)
	for i := 16; i < 24; i++ {
		m[i] = 0xff
	}
	binary.BigEndian.PutUint32(m[24:28], 0)
	chunkData.Write(m)
	return chunkData.Bytes()
}

func stringsPayload(id uint32, value string) []byte {
	var b bytes.Buffer
	_ = binary.Write(&b, binary.BigEndian, uint32(1))
	_ = binary.Write(&b, binary.BigEndian, id)
	_ = binary.Write(&b, binary.BigEndian, int64(0))
	b.WriteString(value)
	b.WriteByte(0)
	return b.Bytes()
}

func dataChunk(declaredAudioSize int64, audio []byte) []byte {
	var b bytes.Buffer
	b.WriteString("data")
	_ = binary.Write(&b, binary.BigEndian, declaredAudioSize+4)
	_ = binary.Write(&b, binary.BigEndian, uint32(0))
	b.Write(audio)
	return b.Bytes()
}

func cafFile(chunks ...[]byte) []byte {
	var b bytes.Buffer
	b.WriteString("caff")
	_ = binary.Write(&b, binary.BigEndian, uint16(1))
	_ = binary.Write(&b, binary.BigEndian, uint16(0))
	for _, c := range chunks {
		b.Write(c)
	}
	return b.Bytes()
}

func parseBytes(t *testing.T, data []byte) *File {
	t.Helper()
	f, err := ParseReadSeeker(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("ParseReadSeeker returned error: %v", err)
	}
	return f
}

func TestCAFVarIntSpecVectors(t *testing.T) {
	cases := []struct {
		value int64
		want  []byte
	}{
		{1, []byte{0x01}},
		{127, []byte{0x7f}},
		{128, []byte{0x81, 0x00}},
		{130, []byte{0x81, 0x02}},
		{257, []byte{0x82, 0x01}},
		{16383, []byte{0xff, 0x7f}},
		{16384, []byte{0x81, 0x80, 0x00}},
	}
	for _, tc := range cases {
		gotEnc := cafVarInt(tc.value)
		if !bytes.Equal(gotEnc, tc.want) {
			t.Fatalf("encode %d: got % x, want % x", tc.value, gotEnc, tc.want)
		}
		got, n, err := decodeCAFVarInt(tc.want, 0)
		if err != nil || n != len(tc.want) || got != tc.value {
			t.Fatalf("decode % x: got (%d,%d,%v), want %d", tc.want, got, n, err, tc.value)
		}
	}
}
