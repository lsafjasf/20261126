package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"edlframe"
)

const exitFindings = 2

func main() {
	if len(os.Args) < 2 {
		usage(os.Stderr)
		os.Exit(1)
	}

	var err error
	switch os.Args[1] {
	case "check":
		err = runCheck(os.Args[2:])
	case "fix":
		err = runFix(os.Args[2:])
	case "-h", "--help", "help":
		usage(os.Stdout)
		return
	default:
		err = fmt.Errorf("unknown command %q", os.Args[1])
	}
	if err != nil {
		var coded *commandError
		if errors.As(err, &coded) {
			fmt.Fprintln(os.Stderr, coded.Error())
			os.Exit(coded.code)
		}
		fmt.Fprintf(os.Stderr, "edlframe: %v\n", err)
		os.Exit(1)
	}
}

type commandError struct {
	code int
	err  error
}

func (e *commandError) Error() string { return e.err.Error() }
func (e *commandError) Unwrap() error { return e.err }

func usage(w io.Writer) {
	fmt.Fprintln(w, `edlframe checks and unifies EDL source-list/event-table frame rates.

Usage:
  edlframe check -input sample.json [-json] [-expand-frames] [-target 25] [-prefer source-list|event-table]
  edlframe fix   -input sample.json -out fixed.json [-report report.json] [-target 25] [-prefer source-list|event-table]

Input is the versioned JSON projection documented in README.md.
Exit status: 0 clean, 1 runtime/usage error, 2 conflicts or invalid rows remain.`)
}

type commonFlags struct {
	input        string
	target       string
	prefer       string
	expandFrames bool
	asJSON       bool
}

func bindCommon(fs *flag.FlagSet) *commonFlags {
	cfg := &commonFlags{}
	fs.StringVar(&cfg.input, "input", "", "input JSON file, or - for stdin")
	fs.StringVar(&cfg.input, "i", "", "input JSON file (shorthand)")
	fs.StringVar(&cfg.target, "target", "", "force canonical rate, e.g. 25 or 30000/1001DF")
	fs.StringVar(&cfg.prefer, "prefer", "source-list", "canonical source when no target is set: source-list or event-table")
	fs.BoolVar(&cfg.expandFrames, "expand-frames", false, "include every frame number for small intervals")
	fs.BoolVar(&cfg.asJSON, "json", false, "write machine-readable JSON")
	return cfg
}

func (c commonFlags) options() edlframe.UnifyOptions {
	return edlframe.UnifyOptions{
		CanonicalRate: c.target,
		Prefer:        c.prefer,
		Policy:        "preserve-wall-clock",
		ExpandFrames:  c.expandFrames,
	}
}

func runCheck(args []string) error {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	cfg := bindCommon(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	applyPositionalInput(fs, cfg)

	doc, err := loadDocument(cfg.input)
	if err != nil {
		return err
	}
	report, err := edlframe.Analyze(doc, cfg.options())
	if err != nil {
		return err
	}

	if cfg.asJSON {
		if err := writeJSON("-", report); err != nil {
			return err
		}
	} else {
		renderText(os.Stdout, report)
	}
	if report.Summary.ReelConflictCount > 0 || report.Summary.EventConflictCount > 0 || report.Summary.InvalidEventCount > 0 {
		return &commandError{code: exitFindings, err: fmt.Errorf("findings: %d reel conflicts, %d event conflicts, %d invalid events", report.Summary.ReelConflictCount, report.Summary.EventConflictCount, report.Summary.InvalidEventCount)}
	}
	return nil
}

func runFix(args []string) error {
	fs := flag.NewFlagSet("fix", flag.ContinueOnError)
	cfg := bindCommon(fs)
	output := fs.String("out", "", "output unified JSON file, or - for stdout")
	reportPath := fs.String("report", "", "optional JSON report output path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	applyPositionalInput(fs, cfg)

	doc, err := loadDocument(cfg.input)
	if err != nil {
		return err
	}
	fixed, report, err := edlframe.Unify(doc, cfg.options())
	if err != nil {
		return err
	}

	if *reportPath != "" {
		if err := writeJSON(*reportPath, report); err != nil {
			return err
		}
	}
	if *output == "" || *output == "-" {
		if cfg.asJSON {
			if err := writeJSON("-", map[string]any{"document": fixed, "report": report}); err != nil {
				return err
			}
		} else {
			if err := writeJSON("-", fixed); err != nil {
				return err
			}
		}
	} else if err := writeJSON(*output, fixed); err != nil {
		return err
	}

	after, err := edlframe.Analyze(*fixed, cfg.options())
	if err != nil {
		return err
	}
	if !cfg.asJSON && *output != "" {
		renderText(os.Stdout, report)
		fmt.Fprintf(os.Stdout, "\nWrote unified document to %s\n", *output)
	}
	if after.Summary.EventConflictCount > 0 || after.Summary.InvalidEventCount > 0 {
		return &commandError{code: exitFindings, err: fmt.Errorf("manual review remains: %d event conflicts, %d invalid events", after.Summary.EventConflictCount, after.Summary.InvalidEventCount)}
	}
	return nil
}

func applyPositionalInput(fs *flag.FlagSet, cfg *commonFlags) {
	if cfg.input == "" && fs.NArg() > 0 {
		cfg.input = fs.Arg(0)
	}
}

func loadDocument(path string) (edlframe.Document, error) {
	var data []byte
	var err error
	if path == "" || path == "-" {
		data, err = io.ReadAll(os.Stdin)
	} else {
		data, err = os.ReadFile(path)
	}
	if err != nil {
		return edlframe.Document{}, fmt.Errorf("read input: %w", err)
	}

	var doc edlframe.Document
	if err := json.Unmarshal(data, &doc); err != nil {
		return edlframe.Document{}, fmt.Errorf("parse JSON EDL projection: %w", err)
	}
	return doc, nil
}

func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if path == "-" || path == "" {
		_, err = os.Stdout.Write(data)
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
