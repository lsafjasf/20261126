package main

import (
	"fmt"
	"strings"

	"fontaudit/internal/audit"
)

func renderText(r *audit.Report) string {
	var b strings.Builder

	fmt.Fprintf(&b, "FONT/SUBTITLE BURN-IN AUDIT\n")
	fmt.Fprintf(&b, "Generated:    %s\n", r.GeneratedAt.Format("2006-01-02 15:04:05 UTC"))
	fmt.Fprintf(&b, "Subtitle:     %s (%s, %d cues)\n", r.SubtitlePath, r.SubtitleFormat, r.CueCount)
	fmt.Fprintf(&b, "Primary font: %s [%s] face#%d, %d glyphs\n",
		r.PrimaryFont.FullName, r.PrimaryFont.Path, r.PrimaryFont.FaceIndex, r.PrimaryFont.NumGlyphs)

	b.WriteString("\n== 1. RENDER ENCODING ==\n")
	fmt.Fprintf(&b, "Detected: %s | U+FFFD: %d | embedded NULs: %d | mojibake: %v\n",
		r.Encoding.Detected, r.Encoding.ReplacementRunes, r.Encoding.EmbeddedNuls, r.Encoding.Mojibake)
	for _, n := range r.Encoding.Notes {
		fmt.Fprintf(&b, "  - %s\n", n)
	}

	if len(r.DeclaredFonts) > 0 {
		b.WriteString("\n== FONTS REQUESTED BY SUBTITLE ==\n")
		for _, f := range r.DeclaredFonts {
			fmt.Fprintf(&b, "  - %-28s (%s)\n", f.Name, f.Source)
		}
	}

	b.WriteString("\n== 2. GLYPH COVERAGE (cmap) ==\n")
	fmt.Fprintf(&b, "Distinct missing runes: %d | total missing occurrences: %d\n",
		r.DistinctMissing, r.TotalMissingHits)
	if r.DistinctMissing == 0 {
		b.WriteString("All visible runes map to real glyphs in the primary font. No tofu expected.\n")
	}

	for _, m := range r.Missing {
		fmt.Fprintf(&b, "\n  [%s] %q  block=%s script=%s category=%s hits=%d\n",
			m.Info.Codepoint, m.Info.Display, m.Info.Block, m.Info.Script, m.Info.Category, m.Occurrences)
		fmt.Fprintf(&b, "    first seen: cue %d @ %s\n", m.First.CueIndex, m.First.Start)
		fmt.Fprintf(&b, "    cue line : %s\n", truncate(m.First.Line, 80))
		fmt.Fprintf(&b, "    cues     : %v\n", m.Cues)
		if len(m.Substitutions) == 0 {
			b.WriteString("    substitute: none safe — keep the rune and add a covering font (do not delete CJK/Greek text)\n")
		}
		for _, s := range m.Substitutions {
			fmt.Fprintf(&b, "    substitute[%s]: %q -> %q  (%s)\n", s.Kind, s.From, s.To, s.Rationale)
		}
		if len(m.FallbackFonts) > 0 {
			b.WriteString("    fallback fonts (explicit approval required):\n")
			for _, f := range m.FallbackFonts {
				fmt.Fprintf(&b, "      * %s  %s\n", f.FullName, f.Path)
			}
		} else {
			b.WriteString("    fallback fonts: none found in scanned directories\n")
		}
	}

	if len(r.Candidates) > 0 {
		b.WriteString("\n== 3. FALLBACK CANDIDATE FONTS (ranked, not applied) ==\n")
		limit := len(r.Candidates)
		if limit > 8 {
			limit = 8
		}
		for i, c := range r.Candidates[:limit] {
			fmt.Fprintf(&b, "  %d. %s [%s] covers %d/%d missing runes\n",
				i+1, c.FullName, c.Path, c.CoveredCount, c.TotalMissing)
		}
		b.WriteString("  No font above is activated automatically. Add the chosen face to the render\n")
		b.WriteString("  pipeline (e.g. ffmpeg fontsdir / libass FontProvider) and re-run the burn-in.\n")
	}

	if len(r.Proofs) > 0 {
		b.WriteString("\n== 4. TOFU PROOF IMAGES (primary font, no fallback) ==\n")
		for _, p := range r.Proofs {
			fmt.Fprintf(&b, "  - cue %d @ %s boxes=%v\n    %s\n    %s\n",
				p.CueIndex, p.Start, p.Boxes, truncate(p.Line, 80), p.PNGPath)
		}
	}

	if len(r.Warnings) > 0 {
		b.WriteString("\n== PARSER WARNINGS ==\n")
		for _, w := range r.Warnings {
			fmt.Fprintf(&b, "  - %s\n", w)
		}
	}

	b.WriteString("\nRESULT: ")
	switch {
	case r.DistinctMissing > 0:
		b.WriteString("FAIL — tofu will appear in the burned-in output until coverage or encoding is fixed.\n")
	default:
		b.WriteString("PASS — no missing glyphs detected.\n")
	}
	return b.String()
}

func truncate(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " / ")
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n]) + "…"
}
