package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"bwfdiag/internal/bwf"
)

func main() {
	jsonOut := flag.Bool("json", false, "emit the complete report as JSON")
	projectPath := flag.String("project", "", "optional project metadata JSON sidecar")
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "usage: %s [-json] [-project sidecar.json] FILE\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(2)
	}
	report, err := bwf.Analyze(flag.Arg(0), *projectPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "bwfdiag:", err)
		os.Exit(1)
	}
	if *jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(report); err != nil {
			fmt.Fprintln(os.Stderr, "bwfdiag:", err)
			os.Exit(1)
		}
		return
	}
	printReport(report)
}

func printReport(r *bwf.Report) {
	fmt.Printf("FILE %s\n", r.File)
	fmt.Printf("CONTAINER %s form=%s declaredSize=%d fileSize=%d\n",
		r.Container.Kind, r.Container.Form, r.Container.DeclaredSize, r.Container.FileSize)
	for _, ch := range r.Container.Chunks {
		flag := ""
		if ch.Truncated {
			flag = " TRUNCATED"
		}
		if ch.Placeholder {
			flag += " PLACEHOLDER"
		}
		fmt.Printf("  chunk %-8q offset=%d payload=%d%s\n", ch.ID, ch.Offset, ch.Size, flag)
	}
	if r.Fmt != nil {
		f := r.Fmt
		fmt.Printf("FMT tag=0x%04x channels=%d rate=%d byteRate=%d blockAlign=%d bits=%d",
			f.FormatTag, f.Channels, f.SampleRate, f.ByteRate, f.BlockAlign, f.BitsPerSample)
		if f.Extensible {
			fmt.Printf(" extensible subTag=0x%04x validBits=%d", f.SubFormatTag, f.ValidBits)
		}
		fmt.Println()
	}
	fmt.Printf("AUDIO dataBytes=%d frames=%d duration=%.9fs", r.Audio.DataBytes, r.Audio.Frames, r.Audio.DurationSeconds)
	if r.Audio.PartialBytes > 0 {
		fmt.Printf(" partialBytes=%d", r.Audio.PartialBytes)
	}
	fmt.Println()
	if b := r.BEXT; b != nil {
		fmt.Printf("BEXT originator=%q originatorReference=%q\n", b.Originator, b.OriginatorRef)
		fmt.Printf("  date=%q time=%q version=%d\n", b.OriginationDate, b.OriginationTime, b.Version)
		fmt.Printf("  timeReference=%d low=%d high=%d\n", b.TimeReference, b.TimeReferenceLow, b.TimeReferenceHigh)
	}
	if x := r.IXML; x != nil {
		fmt.Printf("IXML root=%s project=%q scene=%q take=%q\n", x.Root, x.Project, x.Scene, x.Take)
		fmt.Printf("  timecodeRate=%q flag=%q sampleRate=%d", x.TimecodeRate, x.TimecodeFlag, x.TimecodeSampleRate)
		if x.StartTime != nil {
			fmt.Printf(" startTime=%s", x.StartTime.Text)
		}
		if x.DirectTimeReference != nil {
			fmt.Printf(" bwfTimeReference=%d", *x.DirectTimeReference)
		}
		fmt.Println()
	}
	if p := r.Project; p != nil {
		fmt.Printf("PROJECT path=%s name=%q", p.Path, p.Name)
		if p.StartSample != nil {
			fmt.Printf(" startSample=%d", *p.StartSample)
		}
		if p.TimeReference != nil {
			fmt.Printf(" timeReference=%d", *p.TimeReference)
		}
		if p.DurationSamples != nil {
			fmt.Printf(" durationSamples=%d", *p.DurationSamples)
		}
		fmt.Println()
	}
	fmt.Println("START SOURCES")
	for _, s := range r.Starts {
		fmt.Printf("  %-24s %12d samples @ %d Hz  [%s]\n", s.ID, s.Start, s.SampleRate, s.Confidence)
	}
	fmt.Println("COMPARISONS")
	for _, c := range r.Comparisons {
		fmt.Printf("  %s - %s = %+d samples (%.6f ms)", c.A, c.B, c.DeltaSamples, c.DeltaSeconds*1000)
		if c.FrameSamples > 0 {
			fmt.Printf(", one frame≈%d samples", c.FrameSamples)
		}
		fmt.Printf("\n    %s\n", c.Meaning)
	}
	fmt.Println("FINDINGS")
	for _, f := range r.Findings {
		fmt.Printf("  [%s] %s: %s\n", f.Severity, f.Code, f.Message)
	}
	fmt.Println("SEMANTIC NOTES")
	for _, s := range r.Semantics {
		fmt.Printf("  - %s\n", s)
	}
	fmt.Println("MOST LIKELY TRUTH")
	for _, h := range r.Hypotheses {
		fmt.Printf("  #%d [%s] %s\n", h.Rank, h.Confidence, h.Statement)
		for _, e := range h.Evidence {
			fmt.Printf("      evidence: %s\n", e)
		}
	}
	fmt.Println("UNDECIDABLE")
	for _, u := range r.Undecidable {
		fmt.Printf("  - %s\n", u)
	}
}
