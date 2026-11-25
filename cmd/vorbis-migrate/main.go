package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	vorbiscomment "github.com/lsafjasf/20261126"
)

type report struct {
	Input      string                   `json:"input"`
	Format     string                   `json:"format"`
	Mode       string                   `json:"mode"`
	Written    bool                     `json:"written"`
	Repairable int                      `json:"repairable"`
	Manual     int                      `json:"manual_confirmation"`
	Ogg        *vorbiscomment.OggResult `json:"ogg,omitempty"`
	Raw        *vorbiscomment.Comment   `json:"raw,omitempty"`
}

type options struct {
	write      bool
	output     string
	json       bool
	raw        bool
	mode       string
	maxValue   int
	failManual bool
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	opts, input, err := parseFlags(args, stderr)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		fmt.Fprintln(stderr, err)
		return 1
	}
	data, err := os.ReadFile(input)
	if err != nil {
		fmt.Fprintf(stderr, "read %s: %v\n", input, err)
		return 1
	}

	rewriteOpts := vorbiscomment.RewriteOptions{Mode: vorbiscomment.RewriteMode(opts.mode)}
	parseOpts := vorbiscomment.Options{MaxValueBytes: opts.maxValue}
	destination := opts.output
	if destination == "" && opts.write {
		destination = input
	}
	willWrite := destination != ""
	rep := report{Input: input, Mode: opts.mode}
	var out []byte
	if opts.raw {
		rep.Format = "raw"
		comment, err := vorbiscomment.Parse(data, parseOpts)
		if err != nil {
			fmt.Fprintf(stderr, "%s: %v\n", input, err)
			return 1
		}
		rep.Raw = comment
		if willWrite {
			out, err = comment.Marshal(rewriteOpts)
			if err != nil {
				fmt.Fprintf(stderr, "%s: %v\n", input, err)
				return 1
			}
		}
	} else if bytes.HasPrefix(data, []byte("OggS")) {
		rep.Format = "ogg"
		var result *vorbiscomment.OggResult
		if willWrite {
			out, result, err = vorbiscomment.RewriteOgg(data, parseOpts, rewriteOpts)
			if err != nil {
				fmt.Fprintf(stderr, "%s: %v\n", input, err)
				return 1
			}
		} else {
			result, err = vorbiscomment.InspectOgg(data, parseOpts)
			if err != nil {
				fmt.Fprintf(stderr, "%s: %v\n", input, err)
				return 1
			}
		}
		rep.Ogg = result
	} else {
		fmt.Fprintf(stderr, "%s: not an Ogg file (use -raw for a comment packet)\n", input)
		return 1
	}

	rep.Repairable, rep.Manual = countIssues(rep)
	if willWrite {
		if err := writeAtomic(destination, out); err != nil {
			fmt.Fprintf(stderr, "write %s: %v\n", destination, err)
			return 1
		}
		rep.Written = true
	}
	if opts.json {
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(rep); err != nil {
			fmt.Fprintf(stderr, "encode report: %v\n", err)
			return 1
		}
	} else {
		printHuman(stdout, rep)
	}
	if opts.failManual && rep.Manual > 0 {
		return 2
	}
	return 0
}

func parseFlags(args []string, stderr io.Writer) (options, string, error) {
	var opts options
	flags := flag.NewFlagSet("vorbis-migrate", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.BoolVar(&opts.write, "write", false, "replace the input file atomically")
	flags.StringVar(&opts.output, "o", "", "write the result to this path (also enables writing)")
	flags.BoolVar(&opts.json, "json", false, "emit a machine-readable report")
	flags.BoolVar(&opts.raw, "raw", false, "treat the input as a raw comment body or packet")
	flags.StringVar(&opts.mode, "mode", "safe", "rewrite mode: safe or conservative")
	flags.IntVar(&opts.maxValue, "max-value-bytes", vorbiscomment.DefaultMaxValueBytes,
		"per-value size limit; larger values are reported, never truncated")
	flags.BoolVar(&opts.failManual, "fail-on-manual", false,
		"exit with status 2 when any manual finding remains")
	flags.Usage = func() {
		fmt.Fprintln(stderr, "usage: vorbis-migrate [flags] FILE.ogg")
		flags.PrintDefaults()
	}
	if err := flags.Parse(args); err != nil {
		return opts, "", err
	}
	if flags.NArg() != 1 {
		flags.Usage()
		return opts, "", errors.New("exactly one input file is required")
	}
	if opts.mode != string(vorbiscomment.RewriteSafe) &&
		opts.mode != string(vorbiscomment.RewriteConservative) {
		return opts, "", fmt.Errorf("invalid -mode %q", opts.mode)
	}
	return opts, flags.Arg(0), nil
}

func countIssues(rep report) (repairable, manual int) {
	visit := func(comment *vorbiscomment.Comment) {
		for _, issue := range comment.Issues {
			switch issue.Class {
			case vorbiscomment.Repairable:
				repairable++
			case vorbiscomment.ManualConfirmation:
				manual++
			}
		}
	}
	if rep.Raw != nil {
		visit(rep.Raw)
	}
	if rep.Ogg != nil {
		for _, item := range rep.Ogg.Comments {
			visit(item.Comment)
		}
	}
	return repairable, manual
}

func printHuman(w io.Writer, rep report) {
	fmt.Fprintf(w, "%s (%s, %s)\n", rep.Input, rep.Format, rep.Mode)
	if rep.Raw != nil {
		printComment(w, "raw", rep.Raw)
	}
	if rep.Ogg != nil {
		for _, item := range rep.Ogg.Comments {
			label := fmt.Sprintf("serial=%d packet=%d offset=%d", item.Serial, item.PacketIndex, item.PacketOffset)
			printComment(w, label, item.Comment)
		}
	}
	fmt.Fprintf(w, "summary: %d repairable, %d manual_confirmation", rep.Repairable, rep.Manual)
	if rep.Written {
		fmt.Fprint(w, ", rewritten")
	}
	fmt.Fprintln(w)
}

func printComment(w io.Writer, label string, comment *vorbiscomment.Comment) {
	fmt.Fprintf(w, "%s: vendor=%q entries=%d\n", label, comment.VendorString, len(comment.Entries))
	for _, issue := range comment.Issues {
		fmt.Fprintf(w, "  %-20s %s offset=%d length=%d entry=%d",
			issue.Class, issue.Code, issue.Offset, issue.Length, issue.EntryIndex)
		if issue.Key != "" {
			fmt.Fprintf(w, " key=%q", issue.Key)
		}
		if issue.Message != "" {
			fmt.Fprintf(w, " (%s)", issue.Message)
		}
		fmt.Fprintln(w)
	}
}

func writeAtomic(path string, data []byte) error {
	info, err := os.Stat(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".vorbis-migrate-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if info != nil {
		if err := tmp.Chmod(info.Mode().Perm()); err != nil {
			tmp.Close()
			return err
		}
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
