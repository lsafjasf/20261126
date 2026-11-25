package main

import (
	"bytes"
	"encoding/binary"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"wavfix"
)

type chunk struct {
	id      string
	payload []byte
}

type config struct {
	name       string
	tag        uint16
	channels   uint16
	rate       uint32
	audio      []byte
	fact       []byte
	preChunks  []chunk
	postChunks []chunk
	riffDelta  int
	dataDelta  int
}

func main() {
	outDir := flag.String("out", "testdata", "directory for generated WAV samples")
	flag.Parse()

	samples := []config{
		badStereoFact(),
		missingMonoFact(),
		correctALaw(),
		badRIFFSize(),
		overlongDataLength(),
	}
	for _, sample := range samples {
		path := filepath.Join(*outDir, sample.name)
		raw := build(sample)
		if err := os.WriteFile(path, raw, 0o644); err != nil {
			panic(err)
		}
		fmt.Println("wrote", path)

		fixed, _, err := wavfix.Repair(raw)
		if err != nil {
			fmt.Println("skip golden for", sample.name+":", err)
			continue
		}
		expectedDir := filepath.Join(*outDir, "expected")
		if err := os.MkdirAll(expectedDir, 0o755); err != nil {
			panic(err)
		}
		goldenName := strings.TrimSuffix(sample.name, ".wav") + ".fixed.wav"
		goldenPath := filepath.Join(expectedDir, goldenName)
		if err := os.WriteFile(goldenPath, fixed, 0o644); err != nil {
			panic(err)
		}
		fmt.Println("wrote", goldenPath)
	}
}

func badStereoFact() config {
	audio := patternedAudio(2000)
	return config{
		name:     "g711-stereo-bad-fact.wav",
		tag:      7,
		channels: 2,
		rate:     8000,
		audio:    audio,
		fact:     u32(uint32(len(audio))),
		preChunks: []chunk{{
			id:      "xUNK",
			payload: []byte("unknown chunk before data"),
		}},
		postChunks: []chunk{{
			id:      "META",
			payload: []byte("preserve after data"),
		}},
	}
}

func missingMonoFact() config {
	return config{
		name:     "g711-mono-missing-fact.wav",
		tag:      7,
		channels: 1,
		rate:     8000,
		audio:    patternedAudio(1601),
		postChunks: []chunk{{
			id:      "LIST",
			payload: append([]byte("INFO"), []byte("IART\x04\x00\x00\x00Go\x00\x00")...),
		}},
	}
}

func correctALaw() config {
	audio := patternedAudio(800)
	return config{
		name:     "g711-mono-alaw-correct.wav",
		tag:      6,
		channels: 1,
		rate:     8000,
		audio:    audio,
		fact:     u32(uint32(len(audio))),
	}
}

func badRIFFSize() config {
	audio := patternedAudio(1200)
	return config{
		name:      "g711-stereo-bad-riff-size.wav",
		tag:       7,
		channels:  2,
		rate:      8000,
		audio:     audio,
		fact:      u32(uint32(len(audio) / 2)),
		riffDelta: -32,
		postChunks: []chunk{{
			id:      "TAIL",
			payload: []byte("still parse by EOF"),
		}},
	}
}

func overlongDataLength() config {
	audio := patternedAudio(1000)
	return config{
		name:      "g711-stereo-overlong-data.wav",
		tag:       7,
		channels:  2,
		rate:      8000,
		audio:     audio,
		fact:      u32(500),
		dataDelta: 96,
	}
}

func patternedAudio(n int) []byte {
	audio := make([]byte, n)
	for i := range audio {
		audio[i] = byte((i*37 + 11) & 0xff)
	}
	return audio
}

func u32(value uint32) []byte {
	payload := make([]byte, 4)
	binary.LittleEndian.PutUint32(payload, value)
	return payload
}

func build(cfg config) []byte {
	blockAlign := cfg.channels
	bits := uint16(8)
	byteRate := cfg.rate * uint32(blockAlign)

	var fmtChunk bytes.Buffer
	_ = binary.Write(&fmtChunk, binary.LittleEndian, cfg.tag)
	_ = binary.Write(&fmtChunk, binary.LittleEndian, cfg.channels)
	_ = binary.Write(&fmtChunk, binary.LittleEndian, cfg.rate)
	_ = binary.Write(&fmtChunk, binary.LittleEndian, byteRate)
	_ = binary.Write(&fmtChunk, binary.LittleEndian, blockAlign)
	_ = binary.Write(&fmtChunk, binary.LittleEndian, bits)

	var out bytes.Buffer
	out.WriteString("RIFF")
	out.Write([]byte{0, 0, 0, 0})
	out.WriteString("WAVE")
	writeChunk(&out, "fmt ", fmtChunk.Bytes())
	for _, item := range cfg.preChunks {
		writeChunk(&out, item.id, item.payload)
	}
	if cfg.fact != nil {
		writeChunk(&out, "fact", cfg.fact)
	}

	out.WriteString("data")
	var dataSize [4]byte
	binary.LittleEndian.PutUint32(dataSize[:], uint32(len(cfg.audio)+cfg.dataDelta))
	out.Write(dataSize[:])
	out.Write(cfg.audio)
	if len(cfg.audio)%2 == 1 {
		out.WriteByte(0)
	}

	for _, item := range cfg.postChunks {
		writeChunk(&out, item.id, item.payload)
	}

	raw := out.Bytes()
	riffSize := uint32(len(raw) - 8 + cfg.riffDelta)
	binary.LittleEndian.PutUint32(raw[4:8], riffSize)
	return raw
}

func writeChunk(out *bytes.Buffer, id string, payload []byte) {
	out.WriteString(id)
	var size [4]byte
	binary.LittleEndian.PutUint32(size[:], uint32(len(payload)))
	out.Write(size[:])
	out.Write(payload)
	if len(payload)%2 == 1 {
		out.WriteByte(0)
	}
}
