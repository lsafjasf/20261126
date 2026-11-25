package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/lsafjasf/20261126/audioedit"
)

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "audioedit:", err)
		os.Exit(1)
	}
}

func run(args []string, stdin io.Reader, stdout io.Writer) error {
	flags := flag.NewFlagSet("audioedit", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	input := flags.String("in", "-", "project JSON path, or - for stdin")
	output := flags.String("out", "-", "position-table JSON path, or - for stdout")
	compact := flags.Bool("compact", false, "emit compact JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected arguments: %v", flags.Args())
	}

	reader, closeInput, err := openInput(*input, stdin)
	if err != nil {
		return err
	}
	defer closeInput()

	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	var project audioedit.Project
	if err := decoder.Decode(&project); err != nil {
		return fmt.Errorf("decode project: %w", err)
	}
	if err := ensureEOF(decoder); err != nil {
		return err
	}
	table, err := audioedit.Compile(project)
	if err != nil {
		return err
	}

	writer, closeOutput, err := openOutput(*output, stdout)
	if err != nil {
		return err
	}
	defer closeOutput()

	encoder := json.NewEncoder(writer)
	if !*compact {
		encoder.SetIndent("", "  ")
	}
	if err := encoder.Encode(table); err != nil {
		return fmt.Errorf("encode table: %w", err)
	}
	return nil
}

func openInput(path string, stdin io.Reader) (io.Reader, func(), error) {
	if path == "-" {
		return stdin, func() {}, nil
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, nil, fmt.Errorf("open project: %w", err)
	}
	return file, func() { _ = file.Close() }, nil
}

func openOutput(path string, stdout io.Writer) (io.Writer, func(), error) {
	if path == "-" {
		return stdout, func() {}, nil
	}
	file, err := os.Create(path)
	if err != nil {
		return nil, nil, fmt.Errorf("create table: %w", err)
	}
	return file, func() { _ = file.Close() }, nil
}

func ensureEOF(decoder *json.Decoder) error {
	var extra any
	err := decoder.Decode(&extra)
	if err == io.EOF {
		return nil
	}
	if err == nil {
		return fmt.Errorf("decode project: multiple JSON values")
	}
	return fmt.Errorf("decode project: %w", err)
}
