package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"adts"
)

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "adts-segment:", err)
		os.Exit(1)
	}
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("adts-segment", flag.ContinueOnError)
	flags.SetOutput(stderr)
	outputPath := flags.String("o", "", "write the JSON report to this file instead of stdout")
	flags.Usage = func() {
		fmt.Fprintln(stderr, "usage: adts-segment [-o report.json] [input.aac|-]")
		flags.PrintDefaults()
	}
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() > 1 {
		flags.Usage()
		return fmt.Errorf("expected at most one input file")
	}

	input := stdin
	inputName := "<stdin>"
	if flags.NArg() == 1 && flags.Arg(0) != "-" {
		file, err := os.Open(flags.Arg(0))
		if err != nil {
			return err
		}
		defer file.Close()
		input = file
		inputName = flags.Arg(0)
	}

	report, err := adts.Read(input)
	if err != nil {
		return fmt.Errorf("read %s: %w", inputName, err)
	}

	output := stdout
	if *outputPath != "" {
		file, err := os.Create(*outputPath)
		if err != nil {
			return err
		}
		defer file.Close()
		output = file
	}
	return adts.WriteJSON(output, report)
}
