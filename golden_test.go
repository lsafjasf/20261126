package wavfix_test

import (
	"bytes"
	"path/filepath"
	"testing"

	"wavfix"
)

func TestGoldenRepairedFiles(t *testing.T) {
	cases := []struct {
		input  string
		golden string
	}{
		{"g711-stereo-bad-fact.wav", "g711-stereo-bad-fact.fixed.wav"},
		{"g711-mono-missing-fact.wav", "g711-mono-missing-fact.fixed.wav"},
		{"g711-mono-alaw-correct.wav", "g711-mono-alaw-correct.fixed.wav"},
		{"g711-stereo-bad-riff-size.wav", "g711-stereo-bad-riff-size.fixed.wav"},
		{"g711-stereo-overlong-data.wav", "g711-stereo-overlong-data.fixed.wav"},
	}

	for _, tc := range cases {
		t.Run(tc.golden, func(t *testing.T) {
			input := readSample(t, tc.input)
			golden := readSample(t, filepath.Join("expected", tc.golden))

			got, _, err := wavfix.Repair(input)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, golden) {
				t.Fatalf("repair output does not match testdata/expected/%s", tc.golden)
			}
		})
	}
}
