package main

import (
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"

	"smptett"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("smpte-tt", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var input, output, timesCSV string
	var fps float64
	var width, height int
	var maxFrames int
	var strict bool
	fs.StringVar(&input, "in", "", "input SMPTE-TT file")
	fs.StringVar(&input, "input", "", "alias for -in")
	fs.StringVar(&output, "out", "smpte-tt-preview", "output directory")
	fs.StringVar(&output, "output", "smpte-tt-preview", "alias for -out")
	fs.StringVar(&timesCSV, "times", "", "comma-separated sample times, e.g. 0s,1.5s,3s")
	fs.Float64Var(&fps, "fps", 0, "uniform preview rate; 0 samples cue transitions")
	fs.IntVar(&maxFrames, "max-frames", 200, "maximum number of preview moments")
	fs.IntVar(&width, "width", 0, "override canvas width")
	fs.IntVar(&height, "height", 0, "override canvas height")
	fs.BoolVar(&strict, "strict", false, "fail if a hidden annotation would become visible")
	fs.Usage = func() {
		fmt.Fprintf(stderr, "Usage: smpte-tt -in captions.ttml [-out preview]\n\n")
		fmt.Fprintf(stderr, "Renders background-image, foreground-text, and annotation layers\n")
		fmt.Fprintf(stderr, "plus a per-moment composite and visibility manifest.\n\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if input == "" {
		fs.Usage()
		return 2
	}
	if fps < 0 || math.IsNaN(fps) || math.IsInf(fps, 0) {
		fmt.Fprintln(stderr, "error: -fps must be zero or positive")
		return 2
	}

	doc, err := smptett.ParseFile(input, smptett.ParseOptions{
		Width:  width,
		Height: height,
	})
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	times, err := smptett.ParseTimes(timesCSV, doc.FrameRate, doc.TickRate)
	if err != nil {
		fmt.Fprintf(stderr, "error: -times: %v\n", err)
		return 2
	}
	report, err := smptett.GeneratePreview(doc, output, smptett.PreviewOptions{
		Times:     times,
		FPS:       fps,
		MaxFrames: maxFrames,
		Strict:    strict,
		Input:     input,
	})
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}

	fmt.Fprintf(stdout, "wrote %d moments to %s\n", len(report.Frames), output)
	fmt.Fprintf(stdout, "background images: %d ever visible, %d never visible\n",
		len(report.Visibility.BackgroundImages.EverVisible),
		len(report.Visibility.BackgroundImages.NeverVisible))
	fmt.Fprintf(stdout, "foreground text:   %d ever visible, %d never visible\n",
		len(report.Visibility.ForegroundText.EverVisible),
		len(report.Visibility.ForegroundText.NeverVisible))
	fmt.Fprintf(stdout, "hidden annotations: %d, visible leaks: %d\n",
		report.Visibility.HiddenAnnotations.Total,
		len(report.Visibility.HiddenAnnotations.EverVisible))
	fmt.Fprintf(stdout, "manifest: %s\n", filepath.Join(output, "manifest.json"))
	if !report.Visibility.AllChecksPassed {
		fmt.Fprintln(stderr, "warning: one or more visibility checks failed")
	}
	return 0
}
