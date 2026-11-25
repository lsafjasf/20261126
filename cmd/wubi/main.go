package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"wubi/vtt"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "wubi:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		usage()
		return fmt.Errorf("missing subcommand")
	}
	switch args[0] {
	case "dedupe":
		return dedupe(args[1:])
	case "validate":
		return validate(args[1:])
	case "count":
		return count(args[1:])
	case "-h", "--help", "help":
		usage()
		return nil
	default:
		return fmt.Errorf("unknown subcommand %q", args[0])
	}
}

func commonFlags(flags *flag.FlagSet) (*string, *string, *bool, *bool) {
	in := flags.String("in", "", "input WebVTT file (default stdin)")
	out := flags.String("out", "", "output WebVTT file (default stdout)")
	bom := flags.Bool("bom", false, "emit a UTF-8 BOM")
	crlf := flags.Bool("crlf", false, "emit CRLF line endings")
	return in, out, bom, crlf
}

func dedupe(args []string) error {
	flags := flag.NewFlagSet("dedupe", flag.ContinueOnError)
	in, out, bom, crlf := commonFlags(flags)
	if err := flags.Parse(args); err != nil {
		return err
	}

	document, closer, err := load(*in)
	if err != nil {
		return err
	}
	defer closer()

	before := len(document.Cues())
	report := document.Deduplicate()
	data, err := document.Serialize(vtt.WriteOptions{WithBOM: *bom, CRLF: *crlf})
	if err != nil {
		return err
	}
	if err := store(*out, data); err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "cues: %d -> %d, removed %d duplicate(s)\n",
		before, report.Kept, report.Removed)
	if len(report.RepeatedIDs) > 0 {
		fmt.Fprintf(os.Stderr, "repeated (legal) cue identifiers kept: %v\n", report.RepeatedIDs)
	}
	return nil
}

func validate(args []string) error {
	flags := flag.NewFlagSet("validate", flag.ContinueOnError)
	in := flags.String("in", "", "input WebVTT file (default stdin)")
	if err := flags.Parse(args); err != nil {
		return err
	}
	document, closer, err := load(*in)
	if err != nil {
		return err
	}
	defer closer()
	if err := document.Validate(); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "valid WebVTT, %d cue(s)\n", len(document.Cues()))
	return nil
}

func count(args []string) error {
	flags := flag.NewFlagSet("count", flag.ContinueOnError)
	in := flags.String("in", "", "input WebVTT file (default stdin)")
	if err := flags.Parse(args); err != nil {
		return err
	}
	document, closer, err := load(*in)
	if err != nil {
		return err
	}
	defer closer()
	fmt.Println(len(document.Cues()))
	return nil
}

func load(path string) (*vtt.Document, func(), error) {
	var reader io.Reader = os.Stdin
	closer := func() {}
	if path != "" {
		file, err := os.Open(path)
		if err != nil {
			return nil, nil, err
		}
		reader = file
		closer = func() { file.Close() }
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		closer()
		return nil, nil, err
	}
	document, err := vtt.Parse(data)
	if err != nil {
		closer()
		return nil, nil, err
	}
	return document, closer, nil
}

func store(path string, data []byte) error {
	if path == "" {
		_, err := os.Stdout.Write(data)
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func usage() {
	fmt.Fprint(os.Stderr, `wubi - WebVTT cue identity editor

usage:
  wubi dedupe    [-in file] [-out file] [--bom] [--crlf]
  wubi validate  [-in file]
  wubi count     [-in file]

Duplicate means identical cue ID, time interval AND text. Repeated cue
identifiers with different timing or text are kept as distinct segments.
`)
}
