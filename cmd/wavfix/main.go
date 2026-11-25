package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"

	"wavfix"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	var err error
	switch os.Args[1] {
	case "inspect":
		err = runInspect(os.Args[2:])
	case "check":
		err = runCheck(os.Args[2:])
	case "fix":
		err = runFix(os.Args[2:])
	case "-h", "--help", "help":
		usage()
	default:
		err = fmt.Errorf("unknown command %q", os.Args[1])
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "wavfix:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: wavfix <inspect|check|fix> [flags] <input.wav>")
}

func loadReport(path string) (*wavfix.File, *wavfix.Report, []byte, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, nil, err
	}
	file, parseErr := wavfix.Parse(raw)
	if file == nil {
		return nil, nil, raw, parseErr
	}
	report := file.BuildReport()
	return file, report, raw, parseErr
}

func runInspect(args []string) error {
	flags := flag.NewFlagSet("inspect", flag.ContinueOnError)
	asJSON := flags.Bool("json", false, "print report as JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 1 {
		return errors.New("inspect requires one input.wav path")
	}

	_, report, _, parseErr := loadReport(flags.Arg(0))
	if report == nil {
		return parseErr
	}
	if *asJSON {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(report); err != nil {
			return err
		}
	} else {
		printReport(report)
	}
	return parseErr
}

func runCheck(args []string) error {
	flags := flag.NewFlagSet("check", flag.ContinueOnError)
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 1 {
		return errors.New("check requires one input.wav path")
	}
	_, report, _, parseErr := loadReport(flags.Arg(0))
	if report != nil {
		printReport(report)
	}
	if parseErr != nil {
		return parseErr
	}
	if len(report.Issues) > 0 {
		return fmt.Errorf("%d container issue(s)", len(report.Issues))
	}
	return nil
}

func runFix(args []string) error {
	flags := flag.NewFlagSet("fix", flag.ContinueOnError)
	output := flags.String("o", "", "output WAV path")
	noInsertFact := flags.Bool("no-insert-fact", false, "do not add a missing fact chunk")
	allowOther := flags.Bool("allow-other-fixed-formats", false, "allow format tags beyond PCM/float/G.711 when block align is usable")
	allowPartial := flags.Bool("allow-partial-frames", false, "floor data length when a partial frame remains")
	strictTruncation := flags.Bool("strict-truncation", false, "reject chunks whose declared length exceeds available bytes")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 1 || *output == "" {
		return errors.New("fix requires -o output.wav and input.wav")
	}

	options := wavfix.Options{
		NoInsertMissingFact:    *noInsertFact,
		AllowOtherFixedFormats: *allowOther,
		AllowPartialFrames:     *allowPartial,
		StrictTruncation:       *strictTruncation,
	}
	report, err := wavfix.RepairFile(flags.Arg(0), *output, options)
	if report != nil {
		printReport(report)
	}
	if err != nil {
		return err
	}
	fmt.Printf("wrote %s: fact=%d frames, duration=%.6fs, audio bytes unchanged=%d\n",
		*output, report.RepairedFactCount, report.DurationFromData, report.DataBytes)
	return nil
}

func printReport(report *wavfix.Report) {
	formatName := fmt.Sprintf("tag=0x%04x effective=0x%04x", report.FormatTag, report.EffectiveFormatTag)
	if report.EffectiveFormatTag == wavfix.FormatMuLaw {
		formatName = "G.711 μ-law"
	}
	if report.EffectiveFormatTag == wavfix.FormatALaw {
		formatName = "G.711 A-law"
	}
	fmt.Printf("format: %s, channels=%d, rate=%d Hz, block_align=%d, bits=%d\n",
		formatName, report.Channels, report.SampleRate, report.BlockAlign, report.BitsPerSample)
	fmt.Printf("data: %d bytes / %d frame-byte(s) = %d frames, duration=%.6fs\n",
		report.DataBytes, report.FrameBytes, report.DataFrames, report.DurationFromData)
	fmt.Printf("fact: present=%v valid=%v declared=%d repaired=%d changed=%v inserted=%v, fact_duration=%.6fs\n",
		report.FactPresent, report.FactValid, report.FactDeclaredCount, report.RepairedFactCount,
		report.FactChanged, report.FactInserted, report.DurationFromFact)
	fmt.Printf("float relation: values=%d, interleaved duration=%.6fs; naive byte-count duration=%.6fs (x%.3g)\n",
		report.FloatValues, report.FloatDurationSeconds, report.NaiveByteCountSeconds, report.DurationErrorFactor())
	fmt.Printf("audio sha256: %s\n", report.AudioSHA256)
	if len(report.UnknownChunks) > 0 {
		fmt.Printf("unknown/preserved chunks: %v\n", report.UnknownChunks)
	}
	for _, issue := range report.Issues {
		fmt.Printf("issue[%s] %s offset=%d: %s\n", issue.Code, issue.Chunk, issue.Offset, issue.Message)
	}
}
