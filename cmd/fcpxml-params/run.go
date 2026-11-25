package main

import (
	"errors"
	"flag"
	"fmt"
	"io"

	"fcpxml"
)

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		usage(stderr)
		return 2
	}
	switch args[0] {
	case "inspect":
		return runInspect(args[1:], stdout, stderr)
	case "save":
		return runSave(args[1:], stderr)
	case "help", "-h", "--help":
		usage(stdout)
		return 0
	default:
		fmt.Fprintf(stderr, "unknown command %q\n", args[0])
		usage(stderr)
		return 2
	}
}

func runInspect(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("inspect", flag.ContinueOnError)
	flags.SetOutput(stderr)
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if flags.NArg() != 1 {
		fmt.Fprintln(stderr, "inspect requires exactly one FCPXML input")
		return 2
	}
	doc, err := fcpxml.ParseFile(flags.Arg(0))
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	data, err := doc.CanonicalJSON()
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	if _, err := fmt.Fprintln(stdout, string(data)); err != nil {
		return 2
	}
	if !doc.Snapshot().Lossless {
		fmt.Fprintln(stderr, "inspection found unmapped parameters; saving is blocked")
		return 1
	}
	return 0
}

func runSave(args []string, stderr io.Writer) int {
	flags := flag.NewFlagSet("save", flag.ContinueOnError)
	flags.SetOutput(stderr)
	out := flags.String("out", "", "destination FCPXML path")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	var input, output string
	switch {
	case *out != "" && flags.NArg() == 1:
		input, output = flags.Arg(0), *out
	case *out == "" && flags.NArg() == 2:
		input, output = flags.Arg(0), flags.Arg(1)
	default:
		fmt.Fprintln(stderr, "save requires INPUT OUTPUT or -out OUTPUT INPUT")
		return 2
	}
	doc, err := fcpxml.ParseFile(input)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	if err := doc.SaveFile(output); err != nil {
		if errors.Is(err, fcpxml.ErrLossySave) {
			fmt.Fprintln(stderr, err)
			return 1
		}
		fmt.Fprintln(stderr, err)
		return 2
	}
	return 0
}

func usage(w io.Writer) {
	fmt.Fprintln(w, `usage:
  fcpxml-params inspect FILE
  fcpxml-params save INPUT OUTPUT
  fcpxml-params save -out OUTPUT INPUT

inspect prints a deterministic JSON snapshot and exits 1 when any parameter
cannot be mapped. save refuses lossy documents and leaves the destination
unchanged.`)
}
