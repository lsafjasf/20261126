package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"proxyrelink/internal/match"
	"proxyrelink/internal/media"
)

func cmdScore(args []string) error {
	fs := flag.NewFlagSet("score", flag.ContinueOnError)
	indexPath := fs.String("index", "", "media index JSON (required)")
	decPath := fs.String("decisions", "", "decisions JSON (optional)")
	proxyID := fs.String("proxy", "", "only show candidates for this proxy")
	asJSON := fs.Bool("json", false, "emit JSON instead of text")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *indexPath == "" {
		return fmt.Errorf("-index is required")
	}
	ix, err := media.LoadIndex(*indexPath)
	if err != nil {
		return err
	}
	dec := &match.Decisions{}
	if *decPath != "" {
		dec, err = match.LoadDecisions(*decPath)
		if err != nil {
			return err
		}
	}
	cands := match.RankedCandidates(ix, dec)
	if *proxyID != "" {
		var filtered []match.Candidate
		for _, c := range cands {
			if c.ProxyID == *proxyID {
				filtered = append(filtered, c)
			}
		}
		cands = filtered
	}
	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(cands)
	}
	if len(cands) == 0 {
		fmt.Println("no open candidates (all pairs locked or rejected)")
		return nil
	}
	renderRanked(os.Stdout, cands)
	fmt.Println()
	limit := len(cands)
	if limit > 10 {
		limit = 10
	}
	for _, c := range cands[:limit] {
		renderEvidence(os.Stdout, c)
		fmt.Printf("  overall [%s] %.1f%%\n\n", bar(c.Score, 30), c.Score*100)
	}
	return nil
}
