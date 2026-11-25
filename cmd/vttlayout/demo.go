package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"vttlayout"
)

func runDemo(args []string) error {
	fs := flag.NewFlagSet("demo", flag.ContinueOnError)
	outdir := fs.String("outdir", "demo-output", "directory for all generated artifacts")
	if err := parseInterspersed(fs, args); err != nil {
		return err
	}

	cases := []struct {
		name string
		vtt  string
	}{
		{"portrait_overlap", vttlayout.ExampleOverlapVTT},
		{"infeasible_stack", vttlayout.ExampleInfeasibleVTT},
	}

	for _, tc := range cases {
		dir := filepath.Join(*outdir, tc.name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		doc, err := vttlayout.Parse(tc.vtt)
		if err != nil {
			return err
		}
		cv := vttlayout.Portrait1080x1920()
		plan := vttlayout.Analyze(doc, cv, vttlayout.Options{})
		report := vttlayout.NewReport(tc.name+".vtt", doc, plan)

		mustWrite(filepath.Join(dir, "input.vtt"), tc.vtt)
		mustWrite(filepath.Join(dir, "fixed.vtt"), vttlayout.WriteFixedVTT(plan))
		b, err := report.JSON()
		if err != nil {
			return err
		}
		mustWrite(filepath.Join(dir, "report.json"), string(b))
		mustWrite(filepath.Join(dir, "report.txt"), report.Text())

		if _, _, err := vttlayout.WriteFrames(filepath.Join(dir, "frames"), doc, plan, nil); err != nil {
			return err
		}

		// Persist one verified VTT candidate per useful repair kind.
		wrote := map[vttlayout.CandidateKind]bool{}
		for i, cand := range plan.Candidates {
			if !cand.Resolves || wrote[cand.Kind] {
				continue
			}
			switch cand.Kind {
			case vttlayout.CandidateSplitCue, vttlayout.CandidateShiftTime, vttlayout.CandidateExtendTime:
				doc2, err := vttlayout.ApplyCandidate(doc, cand)
				if err != nil {
					continue
				}
				name := fmt.Sprintf("candidate_%02d_%s.vtt", i+1, cand.Kind)
				mustWrite(filepath.Join(dir, name), vttlayout.Serialize(doc2))
				wrote[cand.Kind] = true
			}
		}

		fmt.Printf("\n================ %s ================\n", tc.name)
		fmt.Print(report.Text())
		at := vttlayout.RichestFrameTime(doc)
		fmt.Print(vttlayout.RenderTerminalFrame(plan, at, vttlayout.FrameAfter, 72, true))
		fmt.Printf("artifacts: %s\n", dir)
	}
	fmt.Println("\npreview contact sheets:")
	for _, tc := range cases {
		fmt.Printf("  %s\n", filepath.Join(*outdir, tc.name, "frames", "index.html"))
	}
	return nil
}

func mustWrite(path, content string) {
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "write failed:", path, err)
		os.Exit(1)
	}
}
