package main

import (
	"fmt"
	"io"
	"strings"

	"proxyrelink/internal/match"
)

// renderEvidence writes one candidate's evidence table.
func renderEvidence(w io.Writer, c match.Candidate) {
	fmt.Fprintf(w, "  proxy    %s  (%s)\n", c.ProxyID, c.ProxyPath)
	fmt.Fprintf(w, "  original %s  (%s)\n", c.OriginalID, c.OriginalPath)
	fmt.Fprintf(w, "  score %.3f  confidence %.3f  verdict %s\n", c.Score, c.Confidence, c.Verdict)
	fmt.Fprintf(w, "  %-18s %-6s %-6s %-6s  %s\n", "dimension", "score", "weight", "conf", "detail")
	for _, e := range c.Evidence {
		if !e.Available {
			fmt.Fprintf(w, "  %-18s %-6s %-6.2f %-6s  %s\n", e.Dimension, "n/a", e.Weight, "-", e.Detail)
			continue
		}
		fmt.Fprintf(w, "  %-18s %-6.3f %-6.2f %-6.2f  %s\n", e.Dimension, e.Score, e.Weight, e.Confidence, e.Detail)
	}
}

// renderRanked writes the compact ranked list of candidates.
func renderRanked(w io.Writer, cands []match.Candidate) {
	fmt.Fprintf(w, "%-3s %-14s %-14s %-8s %-8s %s\n", "#", "proxy", "original", "score", "conf", "verdict")
	for i, c := range cands {
		fmt.Fprintf(w, "%-3d %-14s %-14s %-8.3f %-8.3f %s\n",
			i+1, c.ProxyID, c.OriginalID, c.Score, c.Confidence, c.Verdict)
	}
}

func bar(v float64, width int) string {
	filled := int(v*float64(width) + 0.5)
	if filled > width {
		filled = width
	}
	return strings.Repeat("#", filled) + strings.Repeat("-", width-filled)
}
