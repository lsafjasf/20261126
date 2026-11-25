package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

const (
	sampleRate  = uint32(48000)
	channels    = uint16(1)
	bitsPer     = uint16(16)
	blockAlign  = uint16(2)
	frames      = uint64(240000)
	bextStart   = uint64(1728000384)
	ixmlStart   = uint64(1728000000)
	projectFram = int64(240384)
)

const ixml = `<?xml version="1.0" encoding="UTF-8"?>
<BWFXML>
  <IXML_VERSION>1.5</IXML_VERSION>
  <PROJECT>timeref-conflict</PROJECT>
  <TAPE>DAY02</TAPE>
  <SCENE>12A</SCENE>
  <TAKE>03</TAKE>
  <NOTE>metadata-only diagnostic fixture; acquisition truth is intentionally absent</NOTE>
  <TIMECODE_RATE>25</TIMECODE_RATE>
  <TIMECODE_FLAG>NDF</TIMECODE_FLAG>
  <TIMECODE_SAMPLE_RATE>48000</TIMECODE_SAMPLE_RATE>
  <START_TIME>10:00:00:00</START_TIME>
  <BWF_TIME_REFERENCE>1728000000</BWF_TIME_REFERENCE>
</BWFXML>
`

func main() {
	dir := flag.String("dir", "testdata", "output directory")
	flag.Parse()
	if err := os.MkdirAll(*dir, 0o755); err != nil {
		fail(err)
	}
	bwfPath := filepath.Join(*dir, "timeref-conflict.bwf")
	projectPath := filepath.Join(*dir, "timeref-conflict.project.json")
	if err := writeFixture(bwfPath); err != nil {
		fail(err)
	}
	if err := writeProject(projectPath); err != nil {
		fail(err)
	}
	fmt.Println(bwfPath)
	fmt.Println(projectPath)
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "mkfixture:", err)
	os.Exit(1)
}

func writeFixture(path string) error {
	var out bytes.Buffer
	out.WriteString("RIFF")
	out.Write(make([]byte, 4))
	out.WriteString("WAVE")
	writeChunk(&out, "fmt ", makeFmt())
	writeChunk(&out, "bext", makeBext())
	writeChunk(&out, "iXML", []byte(ixml))
	writeChunk(&out, "data", make([]byte, frames*uint64(blockAlign)))
	b := out.Bytes()
	binary.LittleEndian.PutUint32(b[4:8], uint32(len(b)-8))
	return os.WriteFile(path, b, 0o644)
}

func writeChunk(out *bytes.Buffer, id string, payload []byte) {
	out.WriteString(id)
	var size [4]byte
	binary.LittleEndian.PutUint32(size[:], uint32(len(payload)))
	out.Write(size[:])
	out.Write(payload)
	if len(payload)&1 == 1 {
		out.WriteByte(0)
	}
}

func makeFmt() []byte {
	b := make([]byte, 16)
	binary.LittleEndian.PutUint16(b[0:2], 1)
	binary.LittleEndian.PutUint16(b[2:4], channels)
	binary.LittleEndian.PutUint32(b[4:8], sampleRate)
	binary.LittleEndian.PutUint32(b[8:12], sampleRate*uint32(blockAlign))
	binary.LittleEndian.PutUint16(b[12:14], blockAlign)
	binary.LittleEndian.PutUint16(b[14:16], bitsPer)
	return b
}

func makeBext() []byte {
	b := make([]byte, 602)
	copy(b[0:256], "BWF time-reference conflict fixture")
	copy(b[256:288], "bwfdiag")
	copy(b[288:320], "BWFDIAG-SAMPLE-0001")
	copy(b[320:330], "2026-11-26")
	copy(b[330:338], "00:30:00")
	binary.LittleEndian.PutUint32(b[338:342], uint32(bextStart))
	binary.LittleEndian.PutUint32(b[342:346], uint32(bextStart>>32))
	binary.LittleEndian.PutUint16(b[346:348], 0)
	return b
}

func writeProject(path string) error {
	doc := map[string]any{
		"name":             "timeref-conflict",
		"sampleRate":       sampleRate,
		"timecodeRate":     "25",
		"durationSamples":  projectFram,
		"durationSeconds":  float64(projectFram) / float64(sampleRate),
		"expectedFileName": "timeref-conflict.bwf",
		"notes":            "locked program duration retained; no trusted acquisition startSample",
	}
	b, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	return os.WriteFile(path, b, 0o644)
}
