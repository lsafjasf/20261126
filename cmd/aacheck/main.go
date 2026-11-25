// Command aacheck prints AAC-HE core-frame, extension-state and output-sample
// timelines. It reads a JSON scenario and never invokes a media framework.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"aache"
)

type scenario struct {
	Streams []aache.Stream `json:"streams"`
}

func main() {
	input := flag.String("input", "", "scenario JSON file; empty uses the built-in transition demo")
	asJSON := flag.Bool("json", false, "print the comparison as JSON")
	frameLimit := flag.Int("frames", 24, "maximum frames to print in text mode")
	flag.Parse()

	streams, err := load(*input)
	if err != nil {
		fmt.Fprintln(os.Stderr, "aacheck:", err)
		os.Exit(1)
	}
	comparison, err := aache.Compare(streams)
	if err != nil {
		fmt.Fprintln(os.Stderr, "aacheck:", err)
		os.Exit(1)
	}

	if *asJSON {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(comparison); err != nil {
			fmt.Fprintln(os.Stderr, "aacheck:", err)
			os.Exit(1)
		}
		return
	}
	printText(comparison, *frameLimit)
}

func load(path string) ([]aache.Stream, error) {
	if path == "" {
		return demoStreams(), nil
	}
	var reader io.Reader
	if path == "-" {
		reader = os.Stdin
	} else {
		file, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		defer file.Close()
		reader = file
	}
	var input scenario
	decoder := json.NewDecoder(reader)
	if err := decoder.Decode(&input); err != nil {
		return nil, fmt.Errorf("decode scenario: %w", err)
	}
	if len(input.Streams) == 0 {
		return nil, errors.New("scenario contains no streams")
	}
	return input.Streams, nil
}

func demoStreams() []aache.Stream {
	return []aache.Stream{
		{
			Name:     "first-cut",
			CoreRate: 24000,
			Initial: &aache.ExtensionUpdate{
				SBR: &aache.SBRConfig{Enabled: true, OutputRate: 48000},
				PS:  &aache.PSConfig{Enabled: true},
			},
			Frames: []aache.Frame{
				{Config: &aache.ExtensionUpdate{
					SBR: &aache.SBRConfig{Enabled: true, OutputRate: 48000},
					PS:  &aache.PSConfig{Enabled: true},
				}},
				{},
				{},
				{Config: &aache.ExtensionUpdate{
					SBR: &aache.SBRConfig{Enabled: true, OutputRate: 48000},
				}},
				{},
			},
		},
		{
			Name:     "second-cut",
			CoreRate: 24000,
			Initial: &aache.ExtensionUpdate{
				SBR: &aache.SBRConfig{Enabled: true, OutputRate: 48000},
				PS:  &aache.PSConfig{Enabled: true},
			},
			Frames: []aache.Frame{{}, {}, {}},
		},
	}
}

func printText(comparison *aache.Comparison, limit int) {
	timeline := comparison.Persisted
	fmt.Printf("persisted: %d frames, %d core samples, %d output samples, %s\n",
		len(timeline.Frames), timeline.CoreSamples, timeline.OutputSamples, timeline.Duration)
	printGap(timeline.FirstConfigGap)
	printMismatch(timeline.FirstOutputMismatch)
	fmt.Println()
	fmt.Printf("%5s %6s %7s %10s %10s %10s %12s %12s %8s\n",
		"frame", "stream", "local", "core(ms)", "out(ms)", "cfg@", "start(ms)", "end(ms)", "out/rate")
	for index, frame := range timeline.Frames {
		if index >= limit {
			fmt.Printf("... %d more frames\n", len(timeline.Frames)-limit)
			break
		}
		fmt.Printf("%5d %6d %7d %10.3f %10.3f %10s %12.3f %12.3f %8d/%d\n",
			frame.Index,
			frame.StreamIndex,
			frame.FrameInStream,
			millis(frame.CoreDuration),
			millis(frame.Duration()),
			stateLabel(frame.Config),
			millis(frame.Start),
			millis(frame.End),
			frame.OutputSamples,
			frame.OutputRate,
		)
	}
	fmt.Println()
	if comparison.FirstLoss == nil {
		fmt.Println("frame-local comparison: no divergence")
		return
	}
	loss := comparison.FirstLoss
	fmt.Printf("frame-local first loss: frame %d (stream %d local %d)\n",
		loss.Frame, loss.StreamIndex, loss.FrameInStream)
	fmt.Printf("  samples: persisted %d/%d Hz, frame-local %d/%d Hz\n",
		loss.PersistedSamples, loss.PersistedRate,
		loss.FrameLocalSamples, loss.FrameLocalRate)
	fmt.Printf("  durations: persisted %s, frame-local %s\n",
		loss.PersistedDuration, loss.FrameLocalDuration)
	fmt.Printf("  total delta: %s over %d divergent frames\n",
		comparison.DurationDelta, comparison.DivergenceCount)
}

func printGap(gap *aache.ConfigGap) {
	if gap == nil {
		fmt.Println("first config gap: none")
		return
	}
	fmt.Printf("first config gap: frame %d (stream %d local %d), last header frame %d, carried %s\n",
		gap.Frame, gap.StreamIndex, gap.FrameInStream, gap.LastConfig, stateLabel(gap.Effective))
}

func printMismatch(mismatch *aache.OutputMismatch) {
	if mismatch == nil {
		fmt.Println("first observed mismatch: none")
		return
	}
	fmt.Printf("first observed mismatch: frame %d, expected %d/%d Hz, observed %d/%d Hz: %s\n",
		mismatch.Frame, mismatch.ExpectedSamples, mismatch.ExpectedRate,
		mismatch.ObservedSamples, mismatch.ObservedRate, mismatch.Reason)
}

func millis(duration time.Duration) float64 {
	return float64(duration) / float64(time.Millisecond)
}

func stateLabel(state aache.ExtensionState) string {
	switch {
	case state.SBR.Enabled && state.PS.Enabled:
		return fmt.Sprintf("sbr+ps@%d", state.SBR.OutputRate)
	case state.SBR.Enabled:
		return fmt.Sprintf("sbr@%d", state.SBR.OutputRate)
	case state.PS.Enabled:
		return "ps"
	default:
		return "core"
	}
}
