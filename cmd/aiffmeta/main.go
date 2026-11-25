package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"aiffmeta"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		usage(stderr)
		return 2
	}
	command := args[0]
	switch command {
	case "-h", "--help", "help":
		usage(stdout)
		return 0
	case "inspect", "info", "dump":
		return runInspect(args[1:], stdout, stderr)
	case "repack", "rewrite", "save":
		return runRepack(args[1:], stdout, stderr)
	case "extract", "audio":
		return runExtract(args[1:], stdout, stderr)
	case "compare", "diff":
		return runCompare(args[1:], stdout, stderr)
	default:
		return runInspect(args, stdout, stderr)
	}
}

func runInspect(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("inspect", flag.ContinueOnError)
	fs.SetOutput(stderr)
	jsonOut := fs.Bool("json", false, "write JSON instead of a text table")
	inPath := fs.String("in", "", "input AIFF/AIFC path")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	path, ok := chooseInput(*inPath, fs.Args(), stderr)
	if !ok {
		return 2
	}
	file, err := aiffmeta.ReadFile(path)
	if err != nil {
		return runtimeError(stderr, err)
	}
	defer file.Close()
	if *jsonOut {
		if err := aiffmeta.WriteJSON(stdout, file); err != nil {
			return runtimeError(stderr, err)
		}
		return 0
	}
	if err := aiffmeta.WriteInspection(stdout, file); err != nil {
		return runtimeError(stderr, err)
	}
	return 0
}

func runRepack(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("repack", flag.ContinueOnError)
	fs.SetOutput(stderr)
	outPath := fs.String("out", "", "output path, or - for stdout")
	shortOut := fs.String("o", "", "output path (shorthand)")
	drop := fs.String("drop", "", "comma-separated four-byte chunk IDs to omit")
	jsonOut := fs.Bool("json", false, "write the report as JSON")
	inPath := fs.String("in", "", "input AIFF/AIFC path")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	path, ok := chooseInput(*inPath, fs.Args(), stderr)
	if !ok {
		return 2
	}
	destination := *outPath
	if destination == "" {
		destination = *shortOut
	}
	if destination == "" {
		fmt.Fprintln(stderr, "aiffmeta repack: -out is required")
		return 2
	}
	file, err := aiffmeta.ReadFile(path)
	if err != nil {
		return runtimeError(stderr, err)
	}
	defer file.Close()

	opts := aiffmeta.RepackOptions{Drop: splitChunkIDs(*drop)}
	var report *aiffmeta.RepackReport
	if destination == "-" {
		report, err = file.Repack(stdout, opts)
	} else {
		report, err = file.RepackToFile(destination, opts)
	}
	if err != nil {
		return runtimeError(stderr, err)
	}
	reportOut := stdout
	if destination == "-" {
		reportOut = stderr
	}
	if *jsonOut {
		err = aiffmeta.WriteJSON(reportOut, report)
	} else {
		err = aiffmeta.WriteRepackReport(reportOut, report)
	}
	if err != nil {
		return runtimeError(stderr, err)
	}
	return 0
}

func runExtract(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("extract", flag.ContinueOnError)
	fs.SetOutput(stderr)
	outPath := fs.String("out", "", "output path, or - for stdout")
	shortOut := fs.String("o", "", "output path (shorthand)")
	inPath := fs.String("in", "", "input AIFF/AIFC path")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	path, ok := chooseInput(*inPath, fs.Args(), stderr)
	if !ok {
		return 2
	}
	destination := *outPath
	if destination == "" {
		destination = *shortOut
	}
	if destination == "" {
		fmt.Fprintln(stderr, "aiffmeta extract: -out is required")
		return 2
	}
	file, err := aiffmeta.ReadFile(path)
	if err != nil {
		return runtimeError(stderr, err)
	}
	defer file.Close()
	n, err := extractTo(file, destination, stdout)
	if err != nil {
		return runtimeError(stderr, err)
	}
	fmt.Fprintf(stderr, "wrote %d audio byte(s) to %s\n", n, destination)
	return 0
}

func runCompare(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("compare", flag.ContinueOnError)
	fs.SetOutput(stderr)
	jsonOut := fs.Bool("json", false, "write JSON instead of text")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if fs.NArg() != 2 {
		fmt.Fprintln(stderr, "aiffmeta compare: need source and resaved paths")
		return 2
	}
	source, err := aiffmeta.ReadFile(fs.Arg(0))
	if err != nil {
		return runtimeError(stderr, err)
	}
	defer source.Close()
	result, err := aiffmeta.ReadFile(fs.Arg(1))
	if err != nil {
		return runtimeError(stderr, err)
	}
	defer result.Close()
	report, err := aiffmeta.Compare(source, result)
	if err != nil {
		return runtimeError(stderr, err)
	}
	if *jsonOut {
		err = aiffmeta.WriteJSON(stdout, report)
	} else {
		err = aiffmeta.WriteComparison(stdout, report)
	}
	if err != nil {
		return runtimeError(stderr, err)
	}
	return 0
}

func chooseInput(flagPath string, positional []string, stderr io.Writer) (string, bool) {
	if flagPath != "" && len(positional) > 0 {
		fmt.Fprintln(stderr, "specify the input either with -in or positionally, not both")
		return "", false
	}
	if flagPath != "" {
		return flagPath, true
	}
	if len(positional) != 1 {
		fmt.Fprintln(stderr, "exactly one input AIFF/AIFC path is required")
		return "", false
	}
	return positional[0], true
}

func splitChunkIDs(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	ids := make([]string, 0, len(parts))
	for _, part := range parts {
		if id := strings.TrimSpace(part); id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

func extractTo(file *aiffmeta.File, destination string, stdout io.Writer) (int64, error) {
	if destination == "-" {
		return file.CopyAudio(stdout)
	}
	temp, err := os.CreateTemp(filepath.Dir(destination), ".aiffmeta-*")
	if err != nil {
		return 0, err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	n, copyErr := file.CopyAudio(temp)
	closeErr := temp.Close()
	if copyErr != nil {
		return n, copyErr
	}
	if closeErr != nil {
		return n, closeErr
	}
	if err := os.Rename(tempPath, destination); err != nil {
		return n, err
	}
	return n, nil
}

func runtimeError(stderr io.Writer, err error) int {
	fmt.Fprintf(stderr, "aiffmeta: %v\n", err)
	return 1
}

func usage(w io.Writer) {
	fmt.Fprintln(w, `usage:
  aiffmeta inspect [-json] (-in FILE | FILE)
  aiffmeta repack -out FILE [-drop ID,ID] [-json] (-in FILE | FILE)
  aiffmeta extract -out FILE (-in FILE | FILE)
  aiffmeta compare [-json] SOURCE RESAVED

repack preserves chunk order and refuses to drop COMM, SSND or AIFC FVER.
Use "-out -" for binary stdout. The report then goes to stderr.`)
}
