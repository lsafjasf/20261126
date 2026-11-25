package sdh

import (
	"strings"
	"time"
	"unicode"
)

// PlainSubtitles returns the ordinary subtitle track with default policy:
// all dialogue, voice-over and lyrics survive with original timing, while
// speaker labels and sound/music descriptions are removed.
func PlainSubtitles(cues []Cue) []Cue {
	return PlainWithOptions(cues, DefaultPlainOptions())
}

// Plain is the project method form of PlainWithOptions.
func (p *Project) Plain(opts PlainOptions) []Cue {
	return PlainWithOptions(p.Cues, opts)
}

// PlainWithOptions projects source cues to the SDH-off track. Cues without
// any remaining spoken/sung line (pure SDH annotations) are omitted. Output
// indexes are renumbered 1..n; IDs and timecodes are preserved.
func PlainWithOptions(cues []Cue, opts PlainOptions) []Cue {
	out := make([]Cue, 0, len(cues))
	for _, cue := range cues {
		text := stripSDHLines(cue.Text, opts)
		if text == "" {
			continue
		}
		plain := cue
		plain.Text = text
		plain.Index = len(out) + 1
		out = append(out, plain)
	}
	return out
}

func stripSDHLines(text string, opts PlainOptions) string {
	var kept []string
	for _, line := range strings.Split(text, "\n") {
		if opts.ExcludeLyrics && lineIsLyric(line) {
			continue
		}
		stripped := StripSDHLine(line, opts)
		if stripped == "" {
			continue
		}
		kept = append(kept, stripped)
	}
	return strings.Join(kept, "\n")
}

func lineIsLyric(line string) bool {
	for _, seg := range scanLine(line, 0, &cueParseState{}).segments {
		if seg.Role == RoleLyric {
			return true
		}
	}
	return false
}

// MissingItem describes one piece of spoken content not fully represented in
// a candidate plain track.
type MissingItem struct {
	CueID  string `json:"cue_id"`
	Role   Role   `json:"role"`
	Text   string `json:"text"`
	Reason string `json:"reason"` // "no-plain-cue" | "text-missing" | "time-mismatch"
}

// CompletenessReport is the result of VerifyCompleteness.
type CompletenessReport struct {
	Complete      bool          `json:"complete"`
	SourceCues    int           `json:"source_cues"`
	PlainCueCount int           `json:"plain_cue_count"`
	VocalUnits    int           `json:"vocal_units"`
	Matched       int           `json:"matched"`
	Missing       []MissingItem `json:"missing,omitempty"`
}

// VerifyCompleteness checks the default-policy plain projection invariant:
// every spoken/sung segment of every source cue must be present in a plain
// cue with the same ID (or an overlapping interval).
func VerifyCompleteness(src []Cue, plain []Cue) *CompletenessReport {
	return VerifyCompletenessWith(src, plain, DefaultVerifyOptions())
}

// VerifyCompletenessWith verifies against a candidate plain track with
// explicit options.
func VerifyCompletenessWith(src []Cue, plain []Cue, opts VerifyOptions) *CompletenessReport {
	report := &CompletenessReport{
		Complete:      true,
		SourceCues:    len(src),
		PlainCueCount: len(plain),
	}

	byID := map[string]Cue{}
	for _, cue := range plain {
		if cue.ID != "" {
			byID[cue.ID] = cue
		}
	}

	for _, cue := range src {
		segments := ClassifyCue(cue)
		for _, seg := range segments {
			if !seg.Role.Vocal() {
				continue
			}
			if opts.PlainOptions.ExcludeLyrics && seg.Role == RoleLyric {
				continue
			}
			report.VocalUnits++

			candidateText, candidateCue, found := findPlainMatch(cue, plain, byID, opts.TimeTolerance)
			if !found {
				report.Missing = append(report.Missing, MissingItem{
					CueID: cue.ID, Role: seg.Role, Text: seg.Text, Reason: "no-plain-cue",
				})
				report.Complete = false
				continue
			}

			if opts.TimeTolerance == 0 && cue.ID != "" && candidateCue.ID == cue.ID {
				if candidateCue.Start != cue.Start || candidateCue.End != cue.End {
					report.Missing = append(report.Missing, MissingItem{
						CueID: cue.ID, Role: seg.Role, Text: seg.Text, Reason: "time-mismatch",
					})
					report.Complete = false
				}
			}

			normalizedSeg := NormalizeForCompare(seg.Text)
			normalizedPlain := NormalizeForCompare(candidateText)
			if normalizedSeg == "" || strings.Contains(normalizedPlain, normalizedSeg) {
				report.Matched++
				continue
			}
			report.Missing = append(report.Missing, MissingItem{
				CueID: cue.ID, Role: seg.Role, Text: seg.Text, Reason: "text-missing",
			})
			report.Complete = false
		}
	}
	return report
}

func findPlainMatch(cue Cue, plain []Cue, byID map[string]Cue, tolerance time.Duration) (string, Cue, bool) {
	if candidate, ok := byID[cue.ID]; ok && cue.ID != "" {
		return candidate.Text, candidate, true
	}
	// Fall back to every temporally overlapping plain cue, combining text so
	// re-segmented candidates still cover the source words.
	var texts []string
	var first Cue
	found := false
	for _, candidate := range plain {
		if intervalsNear(cue.Start, cue.End, candidate.Start, candidate.End, tolerance) {
			texts = append(texts, candidate.Text)
			if !found {
				first = candidate
			}
			found = true
		}
	}
	return strings.Join(texts, "\n"), first, found
}

func intervalsNear(aStart, aEnd, bStart, bEnd, tol time.Duration) bool {
	if tol > 0 {
		aStart -= tol
		aEnd += tol
		bStart -= tol
		bEnd += tol
	}
	return IntervalsOverlap(aStart, aEnd, bStart, bEnd)
}

// NormalizeForCompare lowercases and removes everything that is not a letter
// or digit, so punctuation, whitespace, music notes and line wraps do not
// affect completeness checks. CJK characters are letters and stay intact.
func NormalizeForCompare(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}
