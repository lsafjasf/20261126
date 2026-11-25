package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"ttmlevents"
)

func main() {
	input := flag.String("i", "", "input TTML file (default: stdin)")
	format := flag.String("format", "markdown", "output format: markdown, json, csv")
	output := flag.String("o", "", "output file (default: stdout)")
	demo := flag.Bool("demo", false, "print the event table for the built-in nested-span sample")
	absolute := flag.Bool("absolute-times", false, "interpret every begin/end as document-media time instead of TTML parent-relative time")
	flag.Parse()

	var reader io.Reader
	switch {
	case *demo:
		reader = strings.NewReader(ttmlevents.SampleTTML())
	case *input != "":
		file, err := os.Open(*input)
		if err != nil {
			fatal(err)
		}
		defer file.Close()
		reader = file
	case flag.NArg() > 0:
		file, err := os.Open(flag.Arg(0))
		if err != nil {
			fatal(err)
		}
		defer file.Close()
		reader = file
	default:
		if stdinIsCharDevice() {
			fmt.Fprintln(os.Stderr, "ttml-events: no -i/input argument and stdin is interactive or null; running -demo")
			reader = strings.NewReader(ttmlevents.SampleTTML())
		} else {
			reader = os.Stdin
		}
	}

	table, err := ttmlevents.AnalyzeWithOptions(reader, ttmlevents.TimingOptions{AbsoluteTimes: *absolute})
	if err != nil {
		fatal(err)
	}

	writer := os.Stdout
	if *output != "" {
		file, err := os.Create(*output)
		if err != nil {
			fatal(err)
		}
		defer file.Close()
		writer = file
	}

	switch *format {
	case "json":
		err = ttmlevents.RenderJSON(writer, table)
	case "csv":
		err = ttmlevents.RenderEventCSV(writer, table)
	case "markdown", "md":
		_, err = io.WriteString(writer, ttmlevents.RenderMarkdown(table))
	default:
		err = fmt.Errorf("unsupported format %q", *format)
	}
	if err != nil {
		fatal(err)
	}
}

func stdinIsCharDevice() bool {
	info, err := os.Stdin.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "ttml-events:", err)
	os.Exit(1)
}
