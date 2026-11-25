package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"audiodesc/scheduler"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "audiodesc:", err)
		os.Exit(1)
	}
}

func run() error {
	inputPath := flag.String("input", "-", `input JSON path ("-" for stdin)`)
	outputPath := flag.String("output", "-", `output path ("-" for stdout)`)
	format := flag.String("format", "json", "output format: json or text")
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage: %s [options]\n\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() != 0 {
		return fmt.Errorf("unexpected arguments: %v", flag.Args())
	}
	if *format != "json" && *format != "text" {
		return fmt.Errorf("unsupported format %q (want json or text)", *format)
	}

	reader, closeInput, err := openInput(*inputPath)
	if err != nil {
		return err
	}
	defer closeInput()
	var plan scheduler.Plan
	decoder := json.NewDecoder(reader)
	if err := decoder.Decode(&plan); err != nil {
		return fmt.Errorf("decode input: %w", err)
	}
	if err := ensureEOF(decoder); err != nil {
		return err
	}

	result, err := scheduler.Schedule(plan)
	if err != nil {
		return fmt.Errorf("schedule: %w", err)
	}
	writer, closeOutput, err := openOutput(*outputPath)
	if err != nil {
		return err
	}
	defer closeOutput()

	if *format == "text" {
		_, err = io.WriteString(writer, scheduler.FormatText(result))
		return err
	}
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(false)
	return encoder.Encode(result)
}

func ensureEOF(decoder *json.Decoder) error {
	var extra any
	err := decoder.Decode(&extra)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err == nil {
		return errors.New("input contains more than one JSON value")
	}
	return fmt.Errorf("decode input: %w", err)
}

func openInput(path string) (io.Reader, func(), error) {
	if path == "-" {
		return os.Stdin, func() {}, nil
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, func() {}, fmt.Errorf("open input: %w", err)
	}
	return file, func() { _ = file.Close() }, nil
}

func openOutput(path string) (io.Writer, func(), error) {
	if path == "-" {
		return os.Stdout, func() {}, nil
	}
	file, err := os.Create(path)
	if err != nil {
		return nil, func() {}, fmt.Errorf("open output: %w", err)
	}
	return file, func() { _ = file.Close() }, nil
}
