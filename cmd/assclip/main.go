package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"assclip/analyze"
	"assclip/ass"
	"assclip/layout"
)

var errConflicts = errors.New("error-severity conflicts found")

type rectFlag []layout.Rect

func (r *rectFlag) String() string { return fmt.Sprint([]layout.Rect(*r)) }

func (r *rectFlag) Set(value string) error {
	parts := strings.FieldsFunc(value, func(c rune) bool {
		return c == ',' || c == ' ' || c == 'x'
	})
	if len(parts) != 4 {
		return fmt.Errorf("subject must be x,y,w,h, got %q", value)
	}
	nums := make([]float64, 4)
	for i, part := range parts {
		n, err := strconv.ParseFloat(part, 64)
		if err != nil {
			return fmt.Errorf("subject coordinate %q: %w", part, err)
		}
		nums[i] = n
	}
	if nums[2] <= 0 || nums[3] <= 0 {
		return fmt.Errorf("subject width and height must be positive")
	}
	*r = append(*r, layout.Rect{
		MinX: nums[0], MinY: nums[1],
		MaxX: nums[0] + nums[2], MaxY: nums[1] + nums[3],
	})
	return nil
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "assclip:", err)
		if errors.Is(err, errConflicts) {
			os.Exit(1)
		}
		os.Exit(2)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("assclip", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var (
		input       = fs.String("i", "", "ASS file path; use - for stdin")
		width       = fs.Int("w", 0, "target width in pixels")
		height      = fs.Int("h", 0, "target height in pixels")
		strategyRaw = fs.String("strategy", "fit", "canvas scaling: fit, fill, or stretch")
		playResRaw  = fs.String("play-res", "", "override script resolution, e.g. 1920x1080")
		format      = fs.String("format", "text", "output format: text or json")
		fail        = fs.Bool("fail-on-conflict", false, "exit 1 when an error-severity conflict exists")
	)
	var subjects rectFlag
	fs.Var(&subjects, "subject", "protected subject zone x,y,w,h in output pixels (repeatable)")
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), "Usage: assclip -w WIDTH -h HEIGHT [options] FILE.ass")
		fs.PrintDefaults()
	}
	if len(args) == 1 && (args[0] == "-h" || args[0] == "-help" || args[0] == "--help") {
		fs.Usage()
		return nil
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *input == "" && fs.NArg() > 0 {
		*input = fs.Arg(0)
	}
	if *input == "" {
		fs.Usage()
		return fmt.Errorf("missing ASS input")
	}
	if *width <= 0 || *height <= 0 {
		return fmt.Errorf("-w and -h must be positive")
	}
	strategy, err := layout.ParseStrategy(*strategyRaw)
	if err != nil {
		return err
	}
	script, err := readScript(*input)
	if err != nil {
		return err
	}
	playW, playH := script.Info.PlayResX, script.Info.PlayResY
	if *playResRaw != "" {
		playW, playH, err = parsePlayRes(*playResRaw)
		if err != nil {
			return err
		}
	}
	canvas, err := layout.NewCanvas(playW, playH, *width, *height, strategy)
	if err != nil {
		return err
	}
	report := analyze.Analyze(script, analyze.Config{
		Canvas:   canvas,
		Subjects: subjects,
	})
	if err := writeReport(os.Stdout, report, *format); err != nil {
		return err
	}
	if *fail && report.Summary.Errors > 0 {
		return fmt.Errorf("%w: %d", errConflicts, report.Summary.Errors)
	}
	return nil
}

func readScript(path string) (*ass.Script, error) {
	if path != "-" {
		return ass.ParseFile(path)
	}
	return parseReader(os.Stdin)
}

func parseReader(r io.Reader) (*ass.Script, error) {
	return ass.Parse(r)
}

func parsePlayRes(value string) (float64, float64, error) {
	parts := strings.FieldsFunc(value, func(c rune) bool {
		return c == 'x' || c == 'X' || c == ':' || c == ','
	})
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("play-res must be WxH, got %q", value)
	}
	w, err := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
	if err != nil {
		return 0, 0, err
	}
	h, err := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	if err != nil {
		return 0, 0, err
	}
	if w <= 0 || h <= 0 {
		return 0, 0, fmt.Errorf("play-res must be positive")
	}
	return w, h, nil
}

func writeReport(w io.Writer, report analyze.Report, format string) error {
	switch strings.ToLower(format) {
	case "json":
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		enc.SetEscapeHTML(false)
		return enc.Encode(report)
	case "text", "table":
		writeText(w, report)
		return nil
	default:
		return fmt.Errorf("unknown format %q (use text or json)", format)
	}
}

func writeText(w io.Writer, r analyze.Report) {
	fmt.Fprintf(w, "script %.1fx%.1f -> %dx%d %s (scale %.4f,%.4f offset %.2f,%.2f)\n",
		r.Script.PlayResX, r.Script.PlayResY,
		r.Target.TargetWidth, r.Target.TargetHeight, r.Target.Strategy,
		r.Target.ScaleX, r.Target.ScaleY, r.Target.OffsetX, r.Target.OffsetY)
	fmt.Fprintf(w, "summary cues=%d clipped=%d conflicts=%d errors=%d warnings=%d\n\n",
		r.Summary.Cues, r.Summary.CuesWithClip, r.Summary.Conflicts,
		r.Summary.Errors, r.Summary.Warnings)
	for _, cue := range r.Cues {
		fmt.Fprintf(w, "#%d %s-%s style=%s an=%d text=%q\n",
			cue.Index, cue.Start, cue.End, cue.Style, cue.Alignment, cue.Text)
		if cue.Clip == nil {
			fmt.Fprintln(w, "  clip: none")
		} else {
			fmt.Fprintf(w, "  clip: %s %s script=%s pixels=%s clamped=%s\n",
				cue.Clip.Mode, clipKind(cue.Clip.IsVector),
				fmtBounds(cue.Clip.ScriptBounds), fmtRect(cue.Clip.Bounds),
				fmtRect(cue.Clip.ClampedBounds))
		}
		fmt.Fprintf(w, "  margins: L=%.2f R=%.2f V=%.2f px | safe=%s (%s)\n",
			cue.MarginsPixels.Left, cue.MarginsPixels.Right, cue.MarginsPixels.Vertical,
			fmtRect(cue.SafeArea.Rect), cue.SafeArea.VerticalAnchor)
		if cue.EstimatedText != nil {
			fmt.Fprintf(w, "  text~: %s\n", fmtRect(*cue.EstimatedText))
		}
		for _, c := range cue.Conflicts {
			detail := ""
			if c.Overflow != nil {
				detail = " " + fmtOverflow(*c.Overflow)
			}
			if c.Area > 0 {
				detail += fmt.Sprintf(" area=%.1fpx2", c.Area)
			}
			if c.Zone != "" {
				detail += " zone=" + c.Zone
			}
			fmt.Fprintf(w, "  [%s] %s: %s%s\n", c.Severity, c.Code, c.Message, detail)
		}
		for _, warning := range cue.Warnings {
			fmt.Fprintf(w, "  [warning] %s\n", warning)
		}
		fmt.Fprintln(w)
	}
	for _, warning := range r.Warnings {
		fmt.Fprintf(w, "script warning: %s\n", warning)
	}
}

func clipKind(vector bool) string {
	if vector {
		return "vector"
	}
	return "rectangle"
}

func fmtBounds(b ass.Bounds) string {
	return fmt.Sprintf("(%.2f,%.2f)-(%.2f,%.2f)", b.MinX, b.MinY, b.MaxX, b.MaxY)
}

func fmtRect(r layout.Rect) string {
	return fmt.Sprintf("(%.2f,%.2f)-(%.2f,%.2f)", r.MinX, r.MinY, r.MaxX, r.MaxY)
}

func fmtOverflow(o layout.Overflow) string {
	var parts []string
	if o.Left > 0 {
		parts = append(parts, fmt.Sprintf("left=%.2f", o.Left))
	}
	if o.Top > 0 {
		parts = append(parts, fmt.Sprintf("top=%.2f", o.Top))
	}
	if o.Right > 0 {
		parts = append(parts, fmt.Sprintf("right=%.2f", o.Right))
	}
	if o.Bottom > 0 {
		parts = append(parts, fmt.Sprintf("bottom=%.2f", o.Bottom))
	}
	if len(parts) == 0 {
		return "overflow=none"
	}
	return "overflow{" + strings.Join(parts, ", ") + "}"
}
