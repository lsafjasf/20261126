package audit

import (
	"fmt"
	"sort"
	"time"
	"unicode"

	"fontaudit/internal/encodingx"
	"fontaudit/internal/fontscan"
	"fontaudit/internal/render"
	"fontaudit/internal/runeinfo"
	"fontaudit/internal/subtitle"
)

type occurrence struct {
	count int
	first FirstOccurrence
	seen  bool
	cues  map[int]bool
}

// Run executes the full audit and returns a structured report.
func Run(opts Options) (*Report, error) {
	if opts.Primary == nil {
		return nil, fmt.Errorf("primary font face is required")
	}
	sub, err := subtitle.Parse(opts.SubtitlePath, opts.SubtitleRaw)
	if err != nil {
		return nil, err
	}

	rep := &Report{
		GeneratedAt:    time.Now().UTC(),
		SubtitlePath:   opts.SubtitlePath,
		SubtitleFormat: sub.Format,
		CueCount:       len(sub.Cues),
		DeclaredFonts:  sub.Fonts,
		PrimaryFont: FontSummary{
			Path:      opts.Primary.Path,
			FaceIndex: opts.Primary.Index,
			Family:    opts.Primary.Family,
			FullName:  opts.Primary.FullName,
			NumGlyphs: opts.Primary.NumGlyphs,
		},
		Warnings: sub.Warnings,
	}
	rep.Encoding = encodingSummary(encodingx.Decode(opts.SubtitleRaw))

	occ := map[rune]*occurrence{}
	var allRunes []rune
	for _, cue := range sub.Cues {
		for _, r := range cue.Visible {
			if skipRune(r) {
				continue
			}
			allRunes = append(allRunes, r)
			if _, seen := occ[r]; !seen {
				occ[r] = &occurrence{cues: map[int]bool{}}
			}
			o := occ[r]
			o.count++
			o.cues[cue.Index] = true
			if !o.seen {
				o.seen = true
				o.first = FirstOccurrence{
					CueIndex: cue.Index,
					Start:    subtitle.Timestamp(cue.Start),
					Line:     cue.Visible,
				}
			}
		}
	}

	cov := opts.Primary.Check(allRunes)
	fontscan.SortMissing(cov.Missing)

	files := fontscan.ScanDir(opts.ScanDirs)
	candidates := fontscan.FindCandidates(files, cov.Missing, opts.Primary)
	rep.Candidates = candidates

	for _, r := range cov.Missing {
		o := occ[r]
		mr := MissingRune{
			Info:        runeinfo.Lookup(r),
			Occurrences: o.count,
			First:       o.first,
			Cues:        sortedCues(o.cues),
		}
		mr.Substitutions = buildSubstitutions(r)
		mr.FallbackFonts = fontsCovering(candidates, r)
		rep.Missing = append(rep.Missing, mr)
		rep.TotalMissingHits += o.count
	}
	rep.DistinctMissing = len(rep.Missing)

	if opts.ProofDir != "" && rep.DistinctMissing > 0 {
		rep.Proofs = renderProofs(opts, sub, cov.Missing)
	}
	return rep, nil
}

// skipRune ignores ASCII controls/whitespace: they never produce glyphs and
// would only noise up the missing list.
func skipRune(r rune) bool {
	if r == '\n' || r == '\t' || r == '\r' {
		return true
	}
	if r <= 0x20 || unicode.IsControl(r) {
		return true
	}
	return false
}

func encodingSummary(dec encodingx.Result) EncodingReport {
	return EncodingReport{
		Detected:         string(dec.Encoding),
		Notes:            dec.Notes(),
		ReplacementRunes: dec.ReplacementCount,
		EmbeddedNuls:     dec.NulCount,
		Mojibake:         dec.Mojibake,
	}
}

func sortedCues(m map[int]bool) []int {
	out := make([]int, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Ints(out)
	return out
}

func buildSubstitutions(r rune) []Substitution {
	var subs []Substitution
	info := runeinfo.Lookup(r)
	if info.PunctAlias != "" {
		subs = append(subs, Substitution{
			Kind:      "punctuation-alias",
			From:      string(r),
			To:        info.PunctAlias,
			Codepoint: info.Codepoint,
			Rationale: "typographic punctuation mapped to basic ASCII/CJK punctuation usually present in the primary font",
		})
	}
	if folded, ok := runeinfo.NFKCFold(r); ok {
		subs = append(subs, Substitution{
			Kind:      "nfkc",
			From:      string(r),
			To:        folded,
			Codepoint: info.Codepoint,
			Rationale: "NFKC compatibility fold (fullwidth/halfwidth, ligature or enclosed form) onto base characters",
		})
	}
	return subs
}

func fontsCovering(cands []fontscan.Candidate, r rune) []FallbackFont {
	var out []FallbackFont
	for _, c := range cands {
		for _, covered := range c.Covers {
			if covered == r {
				out = append(out, FallbackFont{
					Path:     c.Path,
					Family:   c.Family,
					FullName: c.FullName,
				})
				break
			}
		}
		if len(out) >= 5 {
			break
		}
	}
	return out
}

func renderProofs(opts Options, sub *subtitle.File, missing []rune) []Proof {
	r, err := render.NewRenderer(opts.Primary)
	if err != nil {
		rep := []Proof{{Line: "<renderer error: " + err.Error() + ">"}}
		return rep
	}
	want := map[rune]bool{}
	for _, mr := range missing {
		want[mr] = true
	}
	var proofs []Proof
	for _, cue := range sub.Cues {
		if opts.MaxProofs > 0 && len(proofs) >= opts.MaxProofs {
			break
		}
		hasMissing := false
		for _, c := range cue.Visible {
			if want[c] {
				hasMissing = true
				break
			}
		}
		if !hasMissing {
			continue
		}
		p, err := r.DrawLine(cue.Index, subtitle.Timestamp(cue.Start), cue.Visible, opts.ProofDir)
		if err != nil {
			continue
		}
		boxStr := make([]string, 0, len(p.Boxes))
		for _, b := range p.Boxes {
			boxStr = append(boxStr, fmt.Sprintf("U+%04X", b))
		}
		proofs = append(proofs, Proof{
			CueIndex: p.CueIndex,
			Start:    p.StartText,
			Line:     p.Line,
			Boxes:    boxStr,
			PNGPath:  p.PNGPath,
		})
	}
	return proofs
}
