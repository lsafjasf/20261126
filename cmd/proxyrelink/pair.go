package main

import (
	"fmt"
	"os"

	"proxyrelink/internal/match"
	"proxyrelink/internal/media"
)

func showPair(ix media.Index, proxyID, origID string) error {
	p, okP := ix.ByID(proxyID)
	o, okO := ix.ByID(origID)
	if !okP || !okO {
		return fmt.Errorf("unknown media id")
	}
	r := match.ScorePair(p, o)
	renderEvidence(os.Stdout, match.Candidate{Result: r, ProxyPath: p.Path, OriginalPath: o.Path})
	fmt.Printf("  overall [%s] %.1f%%\n", bar(r.Score, 30), r.Score*100)
	return nil
}

func applyDecision(ix media.Index, dec *match.Decisions, action, proxyID, origID string) error {
	p, okP := ix.ByID(proxyID)
	o, okO := ix.ByID(origID)
	if !okP || !okO {
		return fmt.Errorf("unknown media id")
	}
	score := match.ScorePair(p, o).Score
	switch action {
	case "lock":
		return dec.Lock(proxyID, origID, score)
	case "reject":
		return dec.Reject(proxyID, origID, score)
	case "clear":
		dec.Clear(proxyID, origID)
		return nil
	}
	return fmt.Errorf("unknown action %q", action)
}
