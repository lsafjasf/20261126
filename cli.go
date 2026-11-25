package safereflow

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

// RunCLI implements the safereflow command. It returns a process-style exit
// code. stdout carries JSON reports; stderr carries human summaries.
func RunCLI(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("safereflow", flag.ContinueOnError)
	fs.SetOutput(stderr)

	var safePath = fs.String("safe", "", "dynamic safe-area JSON file (required)")
	var cuesPath = fs.String("cues", "", "cue document JSON file")
	var srtPath = fs.String("srt", "", "SubRip subtitle file (alternative to -cues)")
	var canvasSpec = fs.String("canvas", "1920x1080", "canvas size WxH used for -srt")
	var threshold = fs.Float64("threshold", 0, "confidence threshold (default from safe doc or 0.8)")
	var outPath = fs.String("out", "-", "analysis report JSON path ('-' for stdout)")
	var apply = fs.Bool("apply", false, "apply recommended auto-eligible plans")
	var planIDs multiFlag
	fs.Var(&planIDs, "plan", "explicit plan id to apply (repeatable)")
	var writeCues = fs.String("write-cues", "", "write post-apply cue JSON here")
	var writeSRT = fs.String("write-srt", "", "write post-apply SRT here")
	var applyOut = fs.String("apply-report", "", "write apply/audit JSON here")
	var pixelAlign = fs.Bool("pixel-align", false, "round proposed boxes to integer pixels")

	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}

	if *safePath == "" {
		fmt.Fprintln(stderr, "error: -safe is required")
		fs.Usage()
		return 2
	}
	if *cuesPath == "" && *srtPath == "" {
		fmt.Fprintln(stderr, "error: one of -cues or -srt is required")
		return 2
	}

	track, err := loadSafeInput(*safePath, *canvasSpec)
	if err != nil {
		fmt.Fprintf(stderr, "error: load safe areas: %v\n", err)
		return 1
	}

	doc, err := loadCueInput(*cuesPath, *srtPath, *canvasSpec)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}

	eng := NewEngine(doc, track)
	if *threshold > 0 {
		eng.Threshold = *threshold
	}
	eng.PixelAlign = *pixelAlign

	report, err := eng.Analyze()
	if err != nil {
		fmt.Fprintf(stderr, "error: analyze: %v\n", err)
		return 1
	}

	if *outPath == "" || *outPath == "-" {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		enc.SetEscapeHTML(false)
		if err := enc.Encode(report); err != nil {
			fmt.Fprintf(stderr, "error: write report: %v\n", err)
			return 1
		}
	} else {
		if err := WriteJSON(*outPath, report); err != nil {
			fmt.Fprintf(stderr, "error: write report: %v\n", err)
			return 1
		}
		fmt.Fprintf(stderr, "report -> %s\n", *outPath)
	}

	printSummary(stderr, report)

	wantApply := *apply || len(planIDs) > 0 || *writeCues != "" || *writeSRT != ""
	if !wantApply {
		return 0
	}

	applied, ar, err := eng.Apply(report, ApplyOptions{PlanIDs: []string(planIDs)})
	if ar != nil && *applyOut != "" {
		_ = WriteJSON(*applyOut, ar)
	}
	if err != nil {
		fmt.Fprintf(stderr, "error: apply blocked: %v\n", err)
		return 3
	}

	if *writeCues != "" {
		if err := WriteJSON(*writeCues, applied); err != nil {
			fmt.Fprintf(stderr, "error: write cues: %v\n", err)
			return 1
		}
		fmt.Fprintf(stderr, "cues -> %s\n", *writeCues)
	}
	if *writeSRT != "" {
		f, err := os.Create(*writeSRT)
		if err != nil {
			fmt.Fprintf(stderr, "error: create srt: %v\n", err)
			return 1
		}
		if err := WriteSRT(f, applied); err != nil {
			f.Close()
			fmt.Fprintf(stderr, "error: write srt: %v\n", err)
			return 1
		}
		f.Close()
		fmt.Fprintf(stderr, "srt -> %s (positions are not stored in SRT; see report/apply-report)\n", *writeSRT)
	}
	if *applyOut != "" {
		fmt.Fprintf(stderr, "apply report -> %s\n", *applyOut)
	}
	fmt.Fprintf(stderr, "applied=%d blocked=%d\n", len(ar.Applied), len(ar.Blocked))
	return 0
}

type multiFlag []string

func (m *multiFlag) String() string { return strings.Join(*m, ",") }
func (m *multiFlag) Set(v string) error {
	*m = append(*m, v)
	return nil
}

func loadCueInput(cuesPath, srtPath, canvasSpec string) (*CueDocument, error) {
	if cuesPath != "" {
		return LoadCueFile(cuesPath)
	}
	canvas, err := parseCanvas(canvasSpec)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(srtPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	doc, err := ParseSRT(f, canvas, DefaultStyle())
	if err != nil {
		return nil, err
	}
	doc.Format = "srt"
	return doc, nil
}

func loadSafeInput(safePath, canvasSpec string) (*SafeTrack, error) {
	if strings.HasSuffix(strings.ToLower(safePath), ".csv") {
		canvas, err := parseCanvas(canvasSpec)
		if err != nil {
			return nil, err
		}
		return LoadSafeCSVFile(safePath, canvas)
	}
	return LoadSafeFile(safePath)
}

func parseCanvas(spec string) (Canvas, error) {
	parts := strings.Split(strings.ToLower(strings.TrimSpace(spec)), "x")
	if len(parts) != 2 {
		return Canvas{}, fmt.Errorf("invalid -canvas %q, want WxH", spec)
	}
	w, err := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
	if err != nil {
		return Canvas{}, fmt.Errorf("invalid canvas width: %w", err)
	}
	h, err := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	if err != nil {
		return Canvas{}, fmt.Errorf("invalid canvas height: %w", err)
	}
	if w <= 0 || h <= 0 {
		return Canvas{}, fmt.Errorf("canvas dimensions must be positive")
	}
	return Canvas{Width: w, Height: h}, nil
}

func printSummary(w io.Writer, rep *Report) {
	s := rep.Summary
	fmt.Fprintf(w,
		"cues=%d safe=%d violation=%d suspected=%d indeterminate=%d auto_plan=%d low_conf_moves_blocked=%d\n",
		s.Cues, s.Safe, s.Violations, s.Suspected, s.Indeterminate, s.WithAutoPlan, s.MigrationsBlocked)
	for _, f := range rep.Findings {
		if f.Status == StatusSafe {
			continue
		}
		rec := "-"
		if f.RecommendedPlanID != "" {
			rec = f.RecommendedPlanID
		}
		fmt.Fprintf(w, "  [%s] %s status=%s conf=%.2f coverage=%v recommend=%s\n",
			f.CueID, f.Box, f.Status, f.Gate.MinConfidence, f.Gate.FullCoverage, rec)
		for _, p := range f.Plans {
			tag := "n/a"
			switch {
			case p.Recommended && p.AutoEligible:
				tag = "AUTO"
			case p.Feasible:
				tag = "ok"
			default:
				tag = "no"
			}
			fmt.Fprintf(w, "      - %s %s [%s] %s\n", p.ID, p.Action, tag, p.Summary)
		}
	}
}
