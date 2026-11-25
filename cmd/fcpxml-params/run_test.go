package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fcpxml"
)

const losslessXML = `<fcpxml version="1.10">
  <resources><effect id="r1" name="Blur" effectID="blur"/></resources>
  <clip><filter-video ref="r1"><param name="Amount" value="1"/></filter-video></clip>
</fcpxml>`

const lossyXML = `<fcpxml version="1.10">
  <resources><effect id="r1" name="Blur" effectID="blur"/></resources>
  <clip><filter-video ref="r1"><data key="effectData">AQID</data></filter-video></clip>
</fcpxml>`

func writeFixture(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "input.fcpxml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestInspectPrintsCanonicalSnapshot(t *testing.T) {
	input := writeFixture(t, losslessXML)
	var stdout, stderr bytes.Buffer
	if code := run([]string{"inspect", input}, &stdout, &stderr); code != 0 {
		t.Fatalf("inspect exit = %d, stderr=%s", code, stderr.String())
	}
	var snapshot fcpxml.Snapshot
	if err := json.Unmarshal(stdout.Bytes(), &snapshot); err != nil {
		t.Fatalf("inspect output is not JSON: %v\n%s", err, stdout.String())
	}
	if !snapshot.Lossless || len(snapshot.Parameters) != 1 {
		t.Fatalf("unexpected snapshot: %#v", snapshot)
	}
}

func TestInspectLossyExitsOneButStillReports(t *testing.T) {
	input := writeFixture(t, lossyXML)
	var stdout, stderr bytes.Buffer
	if code := run([]string{"inspect", input}, &stdout, &stderr); code != 1 {
		t.Fatalf("inspect exit = %d, want 1", code)
	}
	if !strings.Contains(stdout.String(), `"lossless": false`) {
		t.Fatalf("lossy inspect did not report status:\n%s", stdout.String())
	}
	if !strings.Contains(stderr.String(), "saving is blocked") {
		t.Fatalf("lossy inspect did not explain refusal: %s", stderr.String())
	}
}

func TestSaveRefusesLossyWithoutTouchingDestination(t *testing.T) {
	input := writeFixture(t, lossyXML)
	output := filepath.Join(t.TempDir(), "output.fcpxml")
	if err := os.WriteFile(output, []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	if code := run([]string{"save", "-out", output, input}, nil, &stderr); code != 1 {
		t.Fatalf("save exit = %d, want 1; stderr=%s", code, stderr.String())
	}
	got, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "original" {
		t.Fatalf("lossy save changed destination to %q", got)
	}
}

func TestSaveLossless(t *testing.T) {
	input := writeFixture(t, losslessXML)
	output := filepath.Join(t.TempDir(), "output.fcpxml")
	var stderr bytes.Buffer
	if code := run([]string{"save", input, output}, nil, &stderr); code != 0 {
		t.Fatalf("save exit = %d, stderr=%s", code, stderr.String())
	}
	got, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != losslessXML {
		t.Fatalf("saved bytes differ from input")
	}
}
