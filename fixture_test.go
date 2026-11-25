package aiffmeta

import (
	"encoding/binary"
	"testing"
)

type testChunk struct {
	id   string
	data []byte
}

func buildAIFF(t *testing.T, form string, chunks ...testChunk) []byte {
	t.Helper()
	body := make([]byte, 0)
	body = append(body, form...)
	for _, chunk := range chunks {
		if len(chunk.id) != 4 {
			t.Fatalf("chunk ID %q is not four bytes", chunk.id)
		}
		header := make([]byte, 8)
		copy(header[0:4], chunk.id)
		binary.BigEndian.PutUint32(header[4:8], uint32(len(chunk.data)))
		body = append(body, header...)
		body = append(body, chunk.data...)
		if len(chunk.data)&1 != 0 {
			body = append(body, 0xee)
		}
	}
	file := make([]byte, 8, 8+len(body))
	copy(file[0:4], "FORM")
	binary.BigEndian.PutUint32(file[4:8], uint32(len(body)))
	return append(file, body...)
}

func commAIFF(channels uint16, frames uint32, bits uint16, rate []byte) []byte {
	data := make([]byte, 18)
	binary.BigEndian.PutUint16(data[0:2], channels)
	binary.BigEndian.PutUint32(data[2:6], frames)
	binary.BigEndian.PutUint16(data[6:8], bits)
	copy(data[8:18], rate)
	return data
}

func commAIFC(channels uint16, frames uint32, bits uint16, rate []byte, compression, name string) []byte {
	data := commAIFF(channels, frames, bits, rate)
	data = append(data, compression...)
	data = append(data, byte(len(name)))
	data = append(data, name...)
	if len(data)&1 != 0 {
		data = append(data, 0)
	}
	return data
}

func ssnd(offset uint32, blockSize uint32, audio []byte) []byte {
	data := make([]byte, 8, 8+int(offset)+len(audio))
	binary.BigEndian.PutUint32(data[0:4], offset)
	binary.BigEndian.PutUint32(data[4:8], blockSize)
	data = append(data, make([]byte, int(offset))...)
	for i := range data[8:] {
		data[8+i] = byte(0xa0 + i)
	}
	data = append(data, audio...)
	return data
}

func pstring(value string) []byte {
	data := append([]byte{byte(len(value))}, []byte(value)...)
	if len(data)&1 != 0 {
		data = append(data, 0)
	}
	return data
}

func markerChunk(markers ...Marker) []byte {
	data := make([]byte, 2)
	binary.BigEndian.PutUint16(data[0:2], uint16(len(markers)))
	for _, marker := range markers {
		entry := make([]byte, 6)
		binary.BigEndian.PutUint16(entry[0:2], uint16(marker.ID))
		binary.BigEndian.PutUint32(entry[2:6], marker.Position)
		entry = append(entry, pstring(marker.Name)...)
		data = append(data, entry...)
	}
	return data
}

func commentChunk(comments ...Comment) []byte {
	data := make([]byte, 2)
	binary.BigEndian.PutUint16(data[0:2], uint16(len(comments)))
	for _, comment := range comments {
		entry := make([]byte, 8)
		binary.BigEndian.PutUint32(entry[0:4], comment.TimeStamp)
		binary.BigEndian.PutUint16(entry[4:6], uint16(comment.MarkerID))
		binary.BigEndian.PutUint16(entry[6:8], uint16(len(comment.Text)))
		entry = append(entry, []byte(comment.Text)...)
		if len(comment.Text)&1 != 0 {
			entry = append(entry, 0)
		}
		data = append(data, entry...)
	}
	return data
}

func testAudio() []byte {
	return []byte{
		0x00, 0x01, 0x00, 0x02, 0x00, 0x03, 0x00, 0x04,
		0x00, 0x05, 0x00, 0x06, 0x00, 0x07, 0x00, 0x08,
	}
}

func unusualAIFC(t *testing.T) []byte {
	t.Helper()
	rate44100 := []byte{0x40, 0x0e, 0xac, 0x44, 0, 0, 0, 0, 0, 0}
	return buildAIFF(t, "AIFC",
		testChunk{"SSND", ssnd(5, 0, testAudio())},
		testChunk{"ANNO", []byte("first annotation")},
		testChunk{"MARK", markerChunk(
			Marker{ID: 1, Position: 0, Name: "Intro"},
			Marker{ID: 2, Position: 8, Name: "Outro"},
		)},
		testChunk{"COMT", commentChunk(
			Comment{TimeStamp: 0xdeadbeef, MarkerID: 1, Text: "first comment"},
			Comment{TimeStamp: 0xdeadbeee, MarkerID: 0, Text: "second"},
		)},
		testChunk{"NAME", []byte("Take 7")},
		testChunk{"AUTH", []byte("Audio Lab")},
		testChunk{"(c) ", []byte("2026 Example")},
		testChunk{"FVER", []byte{0xa2, 0x80, 0x51, 0x40}},
		testChunk{"COMM", commAIFC(1, 8, 16, rate44100, "sowt", "little-endian PCM")},
		testChunk{"JUNK", []byte("opaque")},
	)
}
