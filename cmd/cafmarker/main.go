package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"cafmarker"
)

type expectationsFile struct {
	Expectations []cafmarker.Expectation `json:"expectations"`
}

func main() {
	if len(os.Args) >= 2 && os.Args[1] == "help" {
		usage()
		return
	}

	args := os.Args[1:]
	mode := "diagnose"
	if len(args) > 0 && (args[0] == "inspect" || args[0] == "diagnose") {
		mode = args[0]
		args = args[1:]
	}
	args = reorderFlags(args)

	fs := flag.NewFlagSet("cafmarker "+mode, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	input := fs.String("in", "", "CAF file to parse (or pass path as first positional argument)")
	asJSON := fs.Bool("json", false, "emit machine-readable JSON")
	showPackets := fs.Bool("packets", false, "list packet mappings in text output")
	allPackets := fs.Bool("all-packets", false, "list every packet, not just the first/last few")
	expectFile := fs.String("expect", "", "JSON file containing expectation objects")
	actualFrame := fs.Int64("actual-frame", -1, "known real playback frame for -marker-index")
	actualPacket := fs.Int("actual-packet", -1, "known real playback packet for -marker-index")
	markerIndex := fs.Int("marker-index", 0, "marker index used by -actual-frame/-actual-packet")
	idIsPacket := fs.Bool("id-is-packet", false, "interpret numeric marker IDs as zero-based packet references")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		os.Exit(2)
	}

	path := *input
	if path == "" && fs.NArg() > 0 {
		path = fs.Arg(0)
	}
	if path == "" {
		fmt.Fprintln(os.Stderr, "missing CAF path")
		usage()
		os.Exit(2)
	}

	f, parseErr := cafmarker.ParseFile(path)
	if parseErr != nil && f == nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", parseErr)
		os.Exit(1)
	}

	opts := cafmarker.Options{}
	opts.MarkerIDIsPacket = *idIsPacket
	if *expectFile != "" {
		expectations, err := loadExpectations(*expectFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(2)
		}
		opts.Expectations = expectations
	}
	if *actualFrame >= 0 || *actualPacket >= 0 {
		idx := *markerIndex
		exp := cafmarker.Expectation{MarkerIndex: &idx}
		if *actualFrame >= 0 {
			frame := *actualFrame
			exp.ActualFrame = &frame
		}
		if *actualPacket >= 0 {
			packet := *actualPacket
			exp.ActualPacket = &packet
		}
		opts.Expectations = append(opts.Expectations, exp)
	}

	report := cafmarker.Diagnose(f, opts)
	if parseErr != nil {
		report.Findings = append(report.Findings, cafmarker.Issue{Code: "parse_error", Message: parseErr.Error()})
	}

	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(report); err != nil {
			fmt.Fprintf(os.Stderr, "encode JSON: %v\n", err)
			os.Exit(1)
		}
		return
	}

	printText(report, textOptions{showPackets: *showPackets || mode == "inspect", allPackets: *allPackets})
	if parseErr != nil {
		fmt.Fprintf(os.Stderr, "\nparse warning/error: %v\n", parseErr)
	}
}

func reorderFlags(args []string) []string {
	takesValue := map[string]bool{
		"in": true, "expect": true, "actual-frame": true,
		"actual-packet": true, "marker-index": true,
	}
	var flags, positional []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			positional = append(positional, args[i+1:]...)
			break
		}
		if len(arg) > 1 && arg[0] == '-' {
			flags = append(flags, arg)
			name := strings.TrimLeft(arg, "-")
			if before, _, ok := strings.Cut(name, "="); ok {
				name = before
			}
			if takesValue[name] && !strings.Contains(arg, "=") && i+1 < len(args) {
				i++
				flags = append(flags, args[i])
			}
			continue
		}
		positional = append(positional, arg)
	}
	return append(flags, positional...)
}

func loadExpectations(path string) ([]cafmarker.Expectation, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var wrapped expectationsFile
	if err := json.Unmarshal(data, &wrapped); err == nil && len(wrapped.Expectations) > 0 {
		return wrapped.Expectations, nil
	}
	var direct []cafmarker.Expectation
	if err := json.Unmarshal(data, &direct); err != nil {
		return nil, fmt.Errorf("parse expectations JSON: %w", err)
	}
	return direct, nil
}

func usage() {
	fmt.Fprintln(os.Stderr, `usage:
  cafmarker diagnose <file.caf> [-json] [-actual-frame N] [-actual-packet N] [-expect e.json]
  cafmarker inspect  <file.caf> [-json] [-packets] [-all-packets]

Parses CAF desc/pakt/mark/data chunks, maps frames to packets and bytes,
and identifies the canonical one-packet instrumentation offset.`)
}
