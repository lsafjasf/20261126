// Command vttlayout checks WebVTT region occlusion after an orientation
// switch, re lays cues out by time / line count / priority / canvas and
// renders SVG or terminal preview frames.
//
// Usage:
//
//	vttlayout check    <file.vtt> [--canvas portrait|landscape|WxH] [--json]
//	vttlayout plan     <file.vtt> [-o fixed.vtt] [--json] [--apply N]
//	vttlayout frames   <file.vtt> [--dir frames] [--at 2.5,4]
//	vttlayout preview  <file.vtt> [--at 3.5] [--mode before|after|both]
//	vttlayout demo     [--outdir demo-output]
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"vttlayout"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	cmd := os.Args[1]
	args := os.Args[2:]
	var err error
	switch cmd {
	case "check":
		err = runCheck(args)
	case "plan", "fix":
		err = runPlan(args)
	case "frames":
		err = runFrames(args)
	case "preview":
		err = runPreview(args)
	case "demo":
		err = runDemo(args)
	case "-h", "--help", "help":
		usage()
	default:
		err = fmt.Errorf("unknown command %q", cmd)
	}
	if err != nil {
		var se silentExit
		if errors.As(err, &se) {
			os.Exit(se.code)
		}
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(2)
	}
}

type silentExit struct{ code int }

func (e silentExit) Error() string { return fmt.Sprintf("exit %d", e.code) }

func usage() {
	fmt.Fprint(os.Stderr, `vttlayout - WebVTT region 竖屏遮挡重排工具 (pure Go)

commands:
  check    解析、检查遮挡/越界并给出重排与无解组合报告
  plan     输出重排后的 WebVTT（-o 文件，默认 stdout）
  frames   生成 before/after SVG 预览帧与 index.html
  preview  在终端打印字符预览帧
  demo     用内置示例端到端生成 fixed.vtt/report.json/frames

examples:
  vttlayout check examples/portrait_overlap.vtt
  vttlayout plan examples/infeasible_stack.vtt --apply 1 -o split.vtt
  vttlayout frames examples/portrait_overlap.vtt --dir frames
  vttlayout preview examples/portrait_overlap.vtt --at 3.5 --mode both
`)
}

// parseInterspersed reorders flags before positional arguments so users can
// write `vttlayout check file.vtt --json`.
func parseInterspersed(fs *flag.FlagSet, args []string) error {
	var flagArgs, posArgs []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			posArgs = append(posArgs, args[i+1:]...)
			break
		}
		if strings.HasPrefix(a, "-") && len(a) > 1 {
			flagArgs = append(flagArgs, a)
			name := strings.TrimLeft(a, "-")
			hasValue := strings.Contains(name, "=")
			name = strings.SplitN(name, "=", 2)[0]
			f := fs.Lookup(name)
			if f != nil {
				if bf, ok := f.Value.(interface{ IsBoolFlag() bool }); ok && bf.IsBoolFlag() {
					continue
				}
			}
			if !hasValue && i+1 < len(args) {
				i++
				flagArgs = append(flagArgs, args[i])
			}
			continue
		}
		posArgs = append(posArgs, a)
	}
	return fs.Parse(append(flagArgs, posArgs...))
}

type commonFlags struct {
	canvas string
	json   bool
}

func bindCommon(fs *flag.FlagSet) *commonFlags {
	cf := &commonFlags{canvas: "portrait"}
	fs.StringVar(&cf.canvas, "canvas", "portrait", "canvas preset (portrait|portrait-720|landscape) or WxH")
	fs.BoolVar(&cf.json, "json", false, "emit JSON report")
	return cf
}

func (cf *commonFlags) resolveCanvas() (vttlayout.Canvas, error) {
	if cv, ok := vttlayout.CanvasByName(cf.canvas); ok {
		return cv, nil
	}
	if strings.Contains(cf.canvas, "x") {
		parts := strings.SplitN(cf.canvas, "x", 2)
		w, err1 := strconv.ParseFloat(parts[0], 64)
		h, err2 := strconv.ParseFloat(parts[1], 64)
		if err1 == nil && err2 == nil && w > 0 && h > 0 {
			return vttlayout.CustomCanvas(w, h), nil
		}
	}
	return vttlayout.Canvas{}, fmt.Errorf("unknown canvas %q", cf.canvas)
}

func loadAndAnalyze(path string, cf *commonFlags) (*vttlayout.Document, *vttlayout.Plan, *vttlayout.Report, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, nil, err
	}
	doc, err := vttlayout.Parse(string(data))
	if err != nil {
		return nil, nil, nil, err
	}
	cv, err := cf.resolveCanvas()
	if err != nil {
		return nil, nil, nil, err
	}
	plan := vttlayout.Analyze(doc, cv, vttlayout.Options{})
	report := vttlayout.NewReport(path, doc, plan)
	return doc, plan, report, nil
}

func runCheck(args []string) error {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	cf := bindCommon(fs)
	strict := fs.Bool("strict", false, "exit 1 on any authored overlap/out of bounds, not only infeasible combos")
	if err := parseInterspersed(fs, args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("check requires exactly one .vtt file")
	}
	_, plan, report, err := loadAndAnalyze(fs.Arg(0), cf)
	if err != nil {
		return err
	}
	if cf.json {
		b, _ := report.JSON()
		fmt.Println(string(b))
	} else {
		fmt.Print(report.Text())
	}
	if !plan.Feasible || (*strict && len(plan.Before) > 0) {
		return silentExit{code: 1}
	}
	return nil
}

func runPlan(args []string) error {
	fs := flag.NewFlagSet("plan", flag.ContinueOnError)
	cf := bindCommon(fs)
	out := fs.String("o", "", "output WebVTT path (default stdout)")
	apply := fs.Int("apply", 0, "apply Nth repair candidate (1 based from report)")
	if err := parseInterspersed(fs, args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("plan requires exactly one .vtt file")
	}
	doc, plan, report, err := loadAndAnalyze(fs.Arg(0), cf)
	if err != nil {
		return err
	}

	var vtt string
	switch {
	case *apply > 0:
		if *apply > len(plan.Candidates) {
			return fmt.Errorf("only %d candidates available", len(plan.Candidates))
		}
		cand := plan.Candidates[*apply-1]
		doc2, err := vttlayout.ApplyCandidate(doc, cand)
		if err != nil {
			return err
		}
		vtt = vttlayout.Serialize(doc2)
		fmt.Fprintf(os.Stderr, "applied candidate #%d [%s] resolves=%v: %s\n",
			*apply, cand.Kind, cand.Resolves, cand.Title)
		if cv, cerr := cf.resolveCanvas(); cerr == nil {
			plan2 := vttlayout.Analyze(doc2, cv, vttlayout.Options{})
			report = vttlayout.NewReport(fs.Arg(0)+"#applied", doc2, plan2)
		}
	default:
		vtt = vttlayout.WriteFixedVTT(plan)
	}

	if *out != "" {
		if err := os.WriteFile(*out, []byte(vtt), 0o644); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "wrote %s\n", *out)
	} else {
		fmt.Print(vtt)
	}

	if cf.json {
		b, _ := report.JSON()
		fmt.Fprintln(os.Stderr, string(b))
	} else if *out != "" {
		fmt.Fprint(os.Stderr, report.Text())
	}
	return nil
}

func runFrames(args []string) error {
	fs := flag.NewFlagSet("frames", flag.ContinueOnError)
	cf := bindCommon(fs)
	dir := fs.String("dir", "frames", "output directory")
	atFlag := fs.String("at", "", "comma separated times (seconds or HH:MM:SS.mmm); default key frames")
	if err := parseInterspersed(fs, args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("frames requires exactly one .vtt file")
	}
	doc, plan, _, err := loadAndAnalyze(fs.Arg(0), cf)
	if err != nil {
		return err
	}
	times, err := parseTimes(*atFlag)
	if err != nil {
		return err
	}
	written, index, err := vttlayout.WriteFrames(*dir, doc, plan, times)
	if err != nil {
		return err
	}
	fmt.Printf("wrote %d SVG frames to %s\n", len(written), *dir)
	fmt.Printf("contact sheet: %s\n", index)
	return nil
}

func runPreview(args []string) error {
	fs := flag.NewFlagSet("preview", flag.ContinueOnError)
	cf := bindCommon(fs)
	atFlag := fs.String("at", "", "time (seconds or timestamp); default midpoint of first stage")
	mode := fs.String("mode", "both", "before|after|both")
	cols := fs.Int("cols", 72, "terminal columns")
	noColor := fs.Bool("no-color", false, "disable ANSI colors")
	if err := parseInterspersed(fs, args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("preview requires exactly one .vtt file")
	}
	doc, plan, _, err := loadAndAnalyze(fs.Arg(0), cf)
	if err != nil {
		return err
	}
	at := vttlayout.Milliseconds(-1)
	if *atFlag != "" {
		ts, err := parseOneTime(*atFlag)
		if err != nil {
			return err
		}
		at = ts
	} else {
		at = vttlayout.RichestFrameTime(doc)
	}
	color := !*noColor
	switch *mode {
	case "before":
		fmt.Print(vttlayout.RenderTerminalFrame(plan, at, vttlayout.FrameBefore, *cols, color))
	case "after":
		fmt.Print(vttlayout.RenderTerminalFrame(plan, at, vttlayout.FrameAfter, *cols, color))
	default:
		fmt.Print(vttlayout.RenderTerminalFrame(plan, at, vttlayout.FrameBefore, *cols, color))
		fmt.Println()
		fmt.Print(vttlayout.RenderTerminalFrame(plan, at, vttlayout.FrameAfter, *cols, color))
	}
	return nil
}

func parseTimes(s string) ([]vttlayout.Milliseconds, error) {
	if strings.TrimSpace(s) == "" {
		return nil, nil
	}
	var out []vttlayout.Milliseconds
	for _, part := range strings.Split(s, ",") {
		ts, err := parseOneTime(strings.TrimSpace(part))
		if err != nil {
			return nil, err
		}
		out = append(out, ts)
	}
	return out, nil
}

func parseOneTime(s string) (vttlayout.Milliseconds, error) {
	if strings.Contains(s, ":") {
		return vttlayout.ParseTimestamp(s)
	}
	sec, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, fmt.Errorf("bad time %q", s)
	}
	return vttlayout.Milliseconds(sec * 1000), nil
}
