package main

import (
	"crypto/sha256"
	"fmt"
	"sort"
)

// Scoring weights for order-independent identity signals. The track UID is
// treated as a hint only: a remuxing editor may rewrite every UID, so a UID
// change is reported as a warning, never as a hard conflict.
const (
	scoreUIDExact          = 40
	scoreCodecPrivateExact = 25
	scoreLanguageExact     = 25
	scoreDefaultFlagMatch  = 10

	highConfidenceScore = scoreCodecPrivateExact + scoreLanguageExact + scoreDefaultFlagMatch
)

// Candidate scores one baseline track against one current track.
type Candidate struct {
	BaselineIndex int      `json:"baselineIndex"`
	CurrentIndex  int      `json:"currentIndex"`
	Score         int      `json:"score"`
	Allowed       bool     `json:"allowed"`
	Evidence      []string `json:"evidence,omitempty"`
	Conflicts     []string `json:"conflicts,omitempty"`
}

func uidChangedNote(baseline, current Track) string {
	return fmt.Sprintf("uid changed: 0x%016x -> 0x%016x", baseline.TrackUID, current.TrackUID)
}

// scorePair compares one baseline identity with one current track.
func scorePair(baselineIndex, currentIndex int, baseline, current Track) Candidate {
	cand := Candidate{BaselineIndex: baselineIndex, CurrentIndex: currentIndex, Allowed: true}

	if baseline.TrackUID == current.TrackUID {
		cand.Score += scoreUIDExact
		cand.Evidence = append(cand.Evidence, "uid-exact")
	} else {
		cand.Conflicts = append(cand.Conflicts, uidChangedNote(baseline, current))
	}

	if len(baseline.CodecPrivateHash) > 0 || len(current.CodecPrivate) > 0 || baseline.CodecPrivate != nil {
		baselineHash := baseline.CodecPrivateHash
		currentHash := current.CodecPrivateHash
		if currentHash == "" && current.CodecPrivate != nil {
			currentHash = hashCodecPrivate(current.CodecPrivate)
		}
		if baselineHash != "" && baselineHash == currentHash {
			cand.Score += scoreCodecPrivateExact
			cand.Evidence = append(cand.Evidence, "codec-private-exact")
		} else {
			cand.Conflicts = append(cand.Conflicts, "codec-private-mismatch")
			cand.Allowed = false
		}
	}

	if baseline.effectiveLanguage() != "" && current.effectiveLanguage() != "" {
		if baseline.effectiveLanguage() == current.effectiveLanguage() {
			cand.Score += scoreLanguageExact
			cand.Evidence = append(cand.Evidence, "language-exact")
		} else {
			cand.Conflicts = append(cand.Conflicts, "language-mismatch")
			cand.Allowed = false
		}
	}

	if baseline.Default == current.Default {
		cand.Score += scoreDefaultFlagMatch
		cand.Evidence = append(cand.Evidence, "default-flag-match")
	} else {
		cand.Conflicts = append(cand.Conflicts, "default-flag-mismatch")
	}

	if baseline.CodecID != "" && current.CodecID != "" && baseline.CodecID != current.CodecID {
		cand.Conflicts = append(cand.Conflicts, "codec-id-mismatch")
		cand.Allowed = false
	}

	return cand
}

// sortCandidates orders alternatives by score (desc) then current index (asc).
func sortCandidates(cands []Candidate) {
	sort.SliceStable(cands, func(i, j int) bool {
		if cands[i].Score != cands[j].Score {
			return cands[i].Score > cands[j].Score
		}
		return cands[i].CurrentIndex < cands[j].CurrentIndex
	})
}

// MatchResult is the chosen one-to-one mapping for one baseline track.
type MatchResult struct {
	Baseline   Track
	Current    *Track // nil when unmatched
	Best       Candidate
	All        []Candidate
	Confidence string
}

// matchTracks builds a one-to-one assignment between baseline and current
// tracks using only order-independent signals. Candidates with hard
// conflicts (codec private, language, codec ID) are never assigned.
func matchTracks(baseline, current []Track) []MatchResult {
	results := make([]MatchResult, len(baseline))
	type pair struct {
		b, c, score int
	}
	var allowed []pair
	for bi, bt := range baseline {
		results[bi].Baseline = bt
		for ci, ct := range current {
			cand := scorePair(bi, ci, bt, ct)
			results[bi].All = append(results[bi].All, cand)
			if cand.Allowed {
				allowed = append(allowed, pair{bi, ci, cand.Score})
			}
		}
		sortCandidates(results[bi].All)
	}

	sort.SliceStable(allowed, func(i, j int) bool {
		if allowed[i].score != allowed[j].score {
			return allowed[i].score > allowed[j].score
		}
		if allowed[i].b != allowed[j].b {
			return allowed[i].b < allowed[j].b
		}
		return allowed[i].c < allowed[j].c
	})

	usedBaseline := make([]bool, len(baseline))
	usedCurrent := make([]bool, len(current))
	chosen := make([]int, len(baseline))
	for i := range chosen {
		chosen[i] = -1
	}
	for _, p := range allowed {
		if usedBaseline[p.b] || usedCurrent[p.c] {
			continue
		}
		usedBaseline[p.b] = true
		usedCurrent[p.c] = true
		chosen[p.b] = p.c
	}

	for bi := range results {
		ci := chosen[bi]
		if ci < 0 {
			results[bi].Confidence = "none"
			continue
		}
		results[bi].Current = &current[ci]
		for _, cand := range results[bi].All {
			if cand.CurrentIndex == ci {
				results[bi].Best = cand
				break
			}
		}
		if results[bi].Best.Score >= highConfidenceScore {
			results[bi].Confidence = "high"
		} else {
			results[bi].Confidence = "low"
		}
	}
	return results
}

func hashCodecPrivate(data []byte) string {
	if data == nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return fmt.Sprintf("%x", sum)
}
