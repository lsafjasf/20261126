package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"proxyrelink/internal/match"
	"proxyrelink/internal/media"
	"proxyrelink/internal/timeline"
)

func cmdRelink(args []string) error {
	fs := flag.NewFlagSet("relink", flag.ContinueOnError)
	indexPath := fs.String("index", "", "media index JSON (required)")
	decPath := fs.String("decisions", "", "decisions JSON (required)")
	tlPath := fs.String("timeline", "", "timeline JSON (required)")
	tol := fs.Int64("tolerance", 0, "allowed frame-count difference between proxy and original")
	asJSON := fs.Bool("json", false, "emit JSON instead of text")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *indexPath == "" || *decPath == "" || *tlPath == "" {
		return fmt.Errorf("-index, -decisions and -timeline are required")
	}
	ix, err := media.LoadIndex(*indexPath)
	if err != nil {
		return err
	}
	dec, err := match.LoadDecisions(*decPath)
	if err != nil {
		return err
	}
	tl, err := timeline.LoadTimeline(*tlPath)
	if err != nil {
		return err
	}
	rep := timeline.Relink(tl, ix, dec, *tol)
	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(rep)
	}
	fmt.Printf("timeline %q @ %d/%d fps\n", rep.Timeline, rep.FPSNum, rep.FPSDen)
	for _, c := range rep.Cuts {
		line := fmt.Sprintf("  cut %-3d [%6d,%6d) %-14s -> %s", c.Seq, c.InFrame, c.OutFrame, c.ClipID, c.Action)
		if c.NewClip != "" {
			line += " " + c.NewClip
		}
		if c.Reason != "" {
			line += " (" + c.Reason + ")"
		}
		fmt.Println(line)
	}
	fmt.Printf("relinked=%d unchanged=%d errors=%d\n", rep.Relinked, rep.Unchanged, rep.Errors)
	fmt.Println("cut points preserved: in/out frames copied verbatim, never recomputed")
	if rep.Errors > 0 {
		return fmt.Errorf("%d cut(s) failed verification", rep.Errors)
	}
	return nil
}
