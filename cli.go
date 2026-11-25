package sdh

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// RunCLI implements the sdhtool command line. It returns the desired exit
// code and writes user-facing errors to stderr.
//
// Usage:
//
//	sdhtool input.srt [-out dir] [-base name]
//	sdhtool -in input.srt -print-report
//	cat input.srt | sdhtool -in - -print-plain
func RunCLI(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("sdhtool", flag.ContinueOnError)
	fs.SetOutput(stderr)

	inPath := fs.String("in", "", "input SRT file ('-' or empty for stdin)")
	outDir := fs.String("out", ".", "directory for plain SRT, layered ASS, per-role SRT and JSON report; '' disables file output")
	baseName := fs.String("base", "", "output base name (default: input file name without extension, or 'sdh')")
	keepLabels := fs.Bool("keep-labels", false, "keep speaker labels in the plain track")
	excludeLyrics := fs.Bool("exclude-lyrics", false, "drop sung lyrics from the plain track")
	stripNotes := fs.Bool("strip-notes", false, "strip ♪/♫ markers while keeping lyric words")
	maxLanes := fs.Int("max-lanes", 0, "preferred lane cap per anchor band (0 = unlimited)")
	printPlain := fs.Bool("print-plain", false, "print the SDH-off plain SRT to stdout")
	printASS := fs.Bool("print-ass", false, "print the layered ASS to stdout")
	printReport := fs.Bool("print-report", false, "print the JSON analysis report to stdout")

	flagArgs, posArgs := reorderInterspersed(args, fs)
	if err := fs.Parse(flagArgs); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}

	path := strings.TrimSpace(*inPath)
	if path == "" && len(posArgs) > 0 {
		path = posArgs[0]
	}

	var data []byte
	var err error
	switch {
	case path == "" || path == "-":
		data, err = io.ReadAll(stdin)
		path = "-"
	default:
		data, err = os.ReadFile(path)
	}
	if err != nil {
		fmt.Fprintf(stderr, "sdhtool: read input: %v\n", err)
		return 1
	}

	cues, err := ParseSRTBytes(data)
	if err != nil {
		fmt.Fprintf(stderr, "sdhtool: parse srt: %v\n", err)
		return 1
	}

	plainOpts := PlainOptions{
		KeepSpeakerLabels: *keepLabels,
		ExcludeLyrics:     *excludeLyrics,
		StripMusicNotes:   *stripNotes,
	}
	project := SplitWithOptions(cues, LayoutOptions{MaxLanes: *maxLanes})
	plain := project.Plain(plainOpts)
	verification := VerifyCompletenessWith(cues, plain, VerifyOptions{PlainOptions: plainOpts})
	report := BuildReport(project, plain, verification)

	if *printPlain {
		if _, err := stdout.Write(RenderSRT(plain)); err != nil {
			fmt.Fprintf(stderr, "sdhtool: write plain: %v\n", err)
			return 1
		}
	}
	if *printASS {
		if _, err := stdout.Write(RenderASS(project, DefaultASSOptions())); err != nil {
			fmt.Fprintf(stderr, "sdhtool: write ass: %v\n", err)
			return 1
		}
	}
	if *printReport {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(report); err != nil {
			fmt.Fprintf(stderr, "sdhtool: write report: %v\n", err)
			return 1
		}
	}

	if strings.TrimSpace(*outDir) != "" {
		base := *baseName
		if base == "" {
			if path == "-" {
				base = "sdh"
			} else {
				base = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
			}
		}
		if err := writeArtifacts(*outDir, base, project, plain, report); err != nil {
			fmt.Fprintf(stderr, "sdhtool: %v\n", err)
			return 1
		}
	}

	if !verification.Complete {
		fmt.Fprintf(stderr, "sdhtool: WARNING plain track missing %d vocal unit(s)\n", len(verification.Missing))
		return 3
	}
	return 0
}

func writeArtifacts(dir, base string, project *Project, plain []Cue, report *Report) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create out dir: %w", err)
	}
	write := func(suffix string, data []byte) error {
		name := filepath.Join(dir, base+suffix)
		if err := os.WriteFile(name, data, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", name, err)
		}
		return nil
	}

	if err := write(".plain.srt", RenderSRT(plain)); err != nil {
		return err
	}
	if err := write(".sdh.ass", RenderASS(project, DefaultASSOptions())); err != nil {
		return err
	}
	reportJSON, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	if err := write(".layers.json", append(reportJSON, '\n')); err != nil {
		return err
	}
	for role, data := range project.LayerTracks() {
		if err := write("."+string(role)+".srt", data); err != nil {
			return err
		}
	}
	return nil
}

// reorderInterspersed moves flags (and their values) ahead of positional
// arguments so "sdhtool input.srt -out dir" works like "sdhtool -out dir
// input.srt".
func reorderInterspersed(args []string, fs *flag.FlagSet) (flagArgs, posArgs []string) {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			posArgs = append(posArgs, args[i+1:]...)
			break
		}
		if len(arg) >= 2 && strings.HasPrefix(arg, "-") {
			flagArgs = append(flagArgs, arg)
			name := strings.TrimLeft(arg, "-")
			if strings.Contains(name, "=") {
				name = name[:strings.IndexByte(name, '=')]
				continue
			}
			lookup := fs.Lookup(name)
			boolFlag := false
			if lookup != nil {
				if bf, ok := lookup.Value.(interface{ IsBoolFlag() bool }); ok {
					boolFlag = bf.IsBoolFlag()
				}
			}
			if !boolFlag && i+1 < len(args) {
				i++
				flagArgs = append(flagArgs, args[i])
			}
			continue
		}
		posArgs = append(posArgs, arg)
	}
	return flagArgs, posArgs
}
