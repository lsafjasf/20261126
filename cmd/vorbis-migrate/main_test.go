package main

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

func testCommentBody() []byte {
	vendor := []byte("encoder")
	entries := [][]byte{
		[]byte("ARTIST=one"),
		[]byte("artist=two"),
		[]byte("TITLE=\xff"),
	}
	out := make([]byte, 4)
	binary.LittleEndian.PutUint32(out, uint32(len(vendor)))
	out = append(out, vendor...)
	out = appendUint32(out, uint32(len(entries)))
	for _, entry := range entries {
		out = appendUint32(out, uint32(len(entry)))
		out = append(out, entry...)
	}
	return out
}

func appendUint32(dst []byte, value uint32) []byte {
	var buf [4]byte
	binary.LittleEndian.PutUint32(buf[:], value)
	return append(dst, buf[:]...)
}

func TestRunCheckDoesNotWriteAndReportsManual(t *testing.T) {
	path := filepath.Join(t.TempDir(), "comment.bin")
	input := testCommentBody()
	if err := os.WriteFile(path, input, 0o640); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"-raw", "-json", path}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, stderr.String())
	}
	if !bytes.Contains(stdout.Bytes(), []byte(`"manual_confirmation": 2`)) {
		t.Fatalf("report missing manual count: %s", stdout.String())
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, input) {
		t.Fatal("check-only run modified the input")
	}
}

func TestRunConservativeWriteKeepsBytes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "comment.bin")
	input := testCommentBody()
	if err := os.WriteFile(path, input, 0o640); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"-raw", "-write", "-mode", "conservative", path}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, stderr.String())
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, input) {
		t.Fatal("conservative rewrite changed the input")
	}
}

func TestRunFailOnManual(t *testing.T) {
	path := filepath.Join(t.TempDir(), "comment.bin")
	if err := os.WriteFile(path, testCommentBody(), 0o640); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"-raw", "-fail-on-manual", path}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("exit=%d, want 2; stderr=%s", code, stderr.String())
	}
}
