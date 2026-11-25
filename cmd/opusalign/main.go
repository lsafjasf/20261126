package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"opusalign"
)

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "opusalign:", err)
		os.Exit(1)
	}
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("opusalign", flag.ContinueOnError)
	flags.SetOutput(stderr)
	inputPath := flags.String("in", "-", "JSON timeline input path, or - for stdin")
	oggPath := flags.String("ogg", "", "analyze an Ogg Opus file instead of JSON input")
	outputPath := flags.String("out", "-", "JSON report output path, or - for stdout")
	srtOutputPath := flags.String("srt-out", "", "optional path for synchronized SRT output")
	compact := flags.Bool("compact", false, "emit compact JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected arguments: %v", flags.Args())
	}

	var report opusalign.Report
	if *oggPath != "" {
		file, err := os.Open(*oggPath)
		if err != nil {
			return err
		}
		report, _, err = opusalign.AnalyzeOgg(file)
		_ = file.Close()
		if err != nil {
			return err
		}
	} else {
		input, closeInput, err := openInput(*inputPath, stdin)
		if err != nil {
			return err
		}
		defer closeInput()

		var timeline opusalign.TimelineInput
		decoder := json.NewDecoder(input)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&timeline); err != nil {
			return fmt.Errorf("decode timeline JSON: %w", err)
		}

		report, err = opusalign.BuildTimeline(timeline)
		if err != nil {
			return err
		}
	}
	if *srtOutputPath != "" {
		srtFile, err := os.Create(*srtOutputPath)
		if err != nil {
			return err
		}
		if err := opusalign.WriteSRT(srtFile, report.Subtitles); err != nil {
			_ = srtFile.Close()
			return err
		}
		if err := srtFile.Close(); err != nil {
			return err
		}
	}

	output, closeOutput, err := createOutput(*outputPath, stdout)
	if err != nil {
		return err
	}
	defer closeOutput()

	encoder := json.NewEncoder(output)
	if !*compact {
		encoder.SetIndent("", "  ")
	}
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(report); err != nil {
		return fmt.Errorf("encode report: %w", err)
	}
	return nil
}

func openInput(path string, stdin io.Reader) (io.Reader, func(), error) {
	if path == "" || path == "-" {
		return stdin, func() {}, nil
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, func() {}, err
	}
	return file, func() { _ = file.Close() }, nil
}

func createOutput(path string, stdout io.Writer) (io.Writer, func(), error) {
	if path == "" || path == "-" {
		return stdout, func() {}, nil
	}
	file, err := os.Create(path)
	if err != nil {
		return nil, func() {}, err
	}
	return file, func() { _ = file.Close() }, nil
}
