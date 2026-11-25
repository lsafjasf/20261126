package media

import (
	"os"
	"path/filepath"
	"testing"
)

func sampleFrames(seed byte) [][]byte {
	frames := make([][]byte, 4)
	for i := range frames {
		frames[i] = make([]byte, 32)
		for j := range frames[i] {
			frames[i][j] = seed + byte(i*31+j)
		}
	}
	return frames
}

func sampleAudio(seed byte) []byte {
	audio := make([]byte, 512)
	for i := range audio {
		audio[i] = seed + byte(i*7)
	}
	return audio
}

func TestProbeRoundtrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "clip.mov")
	if err := Encode(path, "AVC1", 4000, 24, 1, 48000, sampleAudio(3), sampleFrames(9)); err != nil {
		t.Fatal(err)
	}
	p, err := ProbeFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if p.Codec != "AVC1" || p.DurationMS != 4000 || p.FrameCount != 4 || p.FPSNum != 24 {
		t.Fatalf("unexpected probe: %+v", p)
	}
	if len(p.FrameHash) != 64 || len(p.AudioFingerprint) != 16 {
		t.Fatalf("bad identity signals: %+v", p)
	}
}

func TestProbeIgnoresExtension(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"clip.mov", "clip.mp4", "clip.mkv", "clip.wav"} {
		path := filepath.Join(dir, name)
		if err := Encode(path, "HEVC", 1000, 24, 1, 48000, sampleAudio(1), sampleFrames(2)); err != nil {
			t.Fatal(err)
		}
		if _, err := ProbeFile(path); err != nil {
			t.Fatalf("%s should probe regardless of extension: %v", name, err)
		}
	}
}

func TestProbeRejectsNonMedia(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "notes.txt")
	if err := Encode(path+"tmp", "AVC1", 1, 1, 1, 1, []byte{1}, [][]byte{{1}}); err != nil {
		t.Fatal(err)
	}
	if err := writeText(path); err != nil {
		t.Fatal(err)
	}
	if _, err := ProbeFile(path); err == nil {
		t.Fatal("expected ErrNotMedia")
	}
}

func writeText(path string) error {
	return os.WriteFile(path, []byte("plain text, not media"), 0o644)
}

func TestFingerprintSimilarity(t *testing.T) {
	a := Fingerprint(sampleAudio(5))
	if FingerprintSimilarity(a, a) != 1 {
		t.Fatal("identical audio must score 1")
	}
	b := Fingerprint(sampleAudio(6))
	sim := FingerprintSimilarity(a, b)
	if sim > 0.75 {
		t.Fatalf("unrelated audio scored too high: %.3f", sim)
	}
}

func TestScanSkipsNonMedia(t *testing.T) {
	dir := t.TempDir()
	if err := Encode(filepath.Join(dir, "a.mov"), "AVC1", 100, 24, 1, 48000, sampleAudio(1), sampleFrames(1)); err != nil {
		t.Fatal(err)
	}
	if err := writeText(filepath.Join(dir, "readme.txt")); err != nil {
		t.Fatal(err)
	}
	probes, skipped, err := Scan(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(probes) != 1 || len(skipped) != 1 {
		t.Fatalf("got %d probes, %d skipped", len(probes), len(skipped))
	}
}
