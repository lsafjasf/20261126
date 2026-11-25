package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type cliChunk struct {
	id   string
	data []byte
}

func cliAIFF(chunks ...cliChunk) []byte {
	body := []byte("AIFF")
	for _, chunk := range chunks {
		header := make([]byte, 8)
		copy(header, chunk.id)
		binary.BigEndian.PutUint32(header[4:], uint32(len(chunk.data)))
		body = append(body, header...)
		body = append(body, chunk.data...)
		if len(chunk.data)&1 != 0 {
			body = append(body, 0)
		}
	}
	out := make([]byte, 8, 8+len(body))
	copy(out, "FORM")
	binary.BigEndian.PutUint32(out[4:], uint32(len(body)))
	return append(out, body...)
}

func cliFixture() []byte {
	comm := make([]byte, 18)
	binary.BigEndian.PutUint16(comm[0:2], 1)
	binary.BigEndian.PutUint32(comm[2:6], 2)
	binary.BigEndian.PutUint16(comm[6:8], 16)
	copy(comm[8:], []byte{0x40, 0x0e, 0xac, 0x44, 0, 0, 0, 0, 0, 0})
	ssnd := make([]byte, 8+4)
	copy(ssnd[8:], []byte{1, 2, 3, 4})
	return cliAIFF(
		cliChunk{"NAME", []byte("CLI tone")},
		cliChunk{"ANNO", []byte("keep this annotation")},
		cliChunk{"SSND", ssnd},
		cliChunk{"COMM", comm},
	)
}

func TestCLIInspectRepackCompare(t *testing.T) {
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "source.aiff")
	if err := os.WriteFile(sourcePath, cliFixture(), 0o600); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if code := run([]string{"inspect", sourcePath}, &stdout, &stderr); code != 0 {
		t.Fatalf("inspect code=%d stderr=%s", code, stderr.String())
	}
	for _, want := range []string{"Chunk offset table", "SSND", "COMM", "keep this annotation"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("inspect missing %q: %s", want, stdout.String())
		}
	}
	stdout.Reset()
	stderr.Reset()
	if code := run([]string{"inspect", "-json", sourcePath}, &stdout, &stderr); code != 0 {
		t.Fatalf("inspect -json code=%d stderr=%s", code, stderr.String())
	}
	var decoded map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &decoded); err != nil {
		t.Fatalf("inspect -json produced invalid JSON: %v", err)
	}
	if decoded["format"] != "AIFF" {
		t.Fatalf("json format = %v", decoded["format"])
	}

	resavedPath := filepath.Join(dir, "resaved.aiff")
	stdout.Reset()
	stderr.Reset()
	if code := run([]string{"repack", "-out", resavedPath, "-drop", "ANNO", sourcePath},
		&stdout, &stderr); code != 0 {
		t.Fatalf("repack code=%d stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "keep this annotation") {
		t.Fatalf("repack report omitted lost annotation: %s", stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	if code := run([]string{"compare", sourcePath, resavedPath}, &stdout, &stderr); code != 0 {
		t.Fatalf("compare code=%d stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "audio: identical") ||
		!strings.Contains(stdout.String(), "keep this annotation") ||
		!strings.Contains(stdout.String(), "missing chunks:") {
		t.Fatalf("compare report incomplete:\n%s", stdout.String())
	}
}

func TestCLIExtractAudio(t *testing.T) {
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "source.aiff")
	audioPath := filepath.Join(dir, "audio.raw")
	if err := os.WriteFile(sourcePath, cliFixture(), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := run([]string{"extract", "-out", audioPath, sourcePath}, &stdout, &stderr); code != 0 {
		t.Fatalf("extract code=%d stderr=%s", code, stderr.String())
	}
	got, err := os.ReadFile(audioPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, []byte{1, 2, 3, 4}) {
		t.Fatalf("audio = %v", got)
	}
}
