package mp3timeline

import (
	"bytes"
	"encoding/binary"
)

const testFrameSize = 417

type synthOptions struct {
	frames     int
	xingFrames int
	noXing     bool
	kind       string
	delay      int
	padding    int
	id3v2      bool
	id3v1      bool
}

func synthMP3(o synthOptions) []byte {
	if o.kind == "" {
		o.kind = "Xing"
	}
	var out bytes.Buffer
	if o.id3v2 {
		out.Write([]byte{0x49, 0x44, 0x33, 0x04, 0x00, 0x00, 0, 0, 0, 0})
	}

	if !o.noXing {
		tag := mpegTestFrame()
		writeTestXing(tag, o.kind, uint32(o.xingFrames), o.delay, o.padding)
		out.Write(tag)
	}
	for i := 0; i < o.frames; i++ {
		frame := mpegTestFrame()
		frame[5] = byte(i)
		out.Write(frame)
	}
	if o.id3v1 {
		tag := make([]byte, 128)
		copy(tag, "TAG")
		out.Write(tag)
	}
	return out.Bytes()
}

func mpegTestFrame() []byte {
	frame := make([]byte, testFrameSize)
	copy(frame, []byte{0xff, 0xfb, 0x90, 0xc0})
	return frame
}

func writeTestXing(frame []byte, kind string, frames uint32, delay, padding int) {
	const rel = 21
	copy(frame[rel:rel+4], kind)
	binary.BigEndian.PutUint32(frame[rel+4:rel+8], 0x0f)
	binary.BigEndian.PutUint32(frame[rel+8:rel+12], frames)
	binary.BigEndian.PutUint32(frame[rel+12:rel+16], uint32(len(frame))*frames)
	for i := 0; i < 100; i++ {
		frame[rel+16+i] = byte(i * 255 / 99)
	}
	binary.BigEndian.PutUint32(frame[rel+116:rel+120], 50)

	lame := frame[rel+120:]
	copy(lame, "LAME3.100")
	packed := uint32(delay)<<12 | uint32(padding)
	lame[21] = byte(packed >> 16)
	lame[22] = byte(packed >> 8)
	lame[23] = byte(packed)
}
