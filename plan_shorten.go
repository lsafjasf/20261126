package safereflow

import (
	"strings"
	"unicode"
)

const ellipsis = "…"

// buildShorten proposes the least aggressive truncation (plus ellipsis) that
// fits the common safe rectangle. Shortening is content-lossy; it is proposed
// but never applied by the automatic path unless explicitly selected.
func (e *Engine) buildShorten(rc ResolvedCue, f *Finding) *Plan {
	p := e.newPlan(f, ActionShorten)
	p.ContentLoss = true

	if f.CommonRect == nil || f.CommonRect.Empty() {
		p.Summary = "缺少覆盖整个 cue 时段的公共安全矩形，无法评估缩短"
		p.Reasons = append(p.Reasons, "no_common_safe_rect")
		return p
	}
	common := *f.CommonRect
	st := rc.Style

	contentW := common.W - 2*st.PaddingX
	if contentW < st.FontSize {
		p.Summary = "安全区过窄，缩短也无法保证可读"
		p.Reasons = append(p.Reasons, "safe_rect_too_narrow")
		return p
	}
	wrapW := rc.WrapWidth
	if contentW < wrapW {
		wrapW = contentW
	}

	runes := []rune(rc.Cue.Text)
	total := len(runes)

	buildCandidate := func(keep int) string {
		return shortenText(rc.Cue.Text, keep, total)
	}

	// fitsAt reports whether a candidate with keep runes can be laid out
	// inside the common rect, allowing a minimal follow-up translation.
	fitsAt := func(keep int) (bool, Rect, bool, []string) {
		text := buildCandidate(keep)
		layout, box := rc.layoutCandidate(text, wrapW, e.Measurer)
		if len(layout.Lines) > st.MaxLines {
			return false, box, false, layout.Lines
		}
		moved := false
		if !FitsInside(box, common) {
			dx, dy, fitX, fitY := MinimalTranslation(box, common)
			if !fitX || !fitY {
				return false, box, false, layout.Lines
			}
			box = box.Translate(dx, dy)
			moved = dx != 0 || dy != 0
			if !FitsInside(box, common) {
				return false, box, moved, layout.Lines
			}
		}
		return true, box, moved, layout.Lines
	}

	// Even the ellipsis-only candidate must fit.
	if ok, box, moved, lines := fitsAt(0); !ok {
		p.Summary = "安全区过小，连省略号都无法容纳"
		p.Reasons = append(p.Reasons, "shorten_cannot_fit_minimum")
		e.finalizeBox(p, rc, box, moved)
		p.Lines = lines
		return p
	}

	// Largest keep that still fits.
	best := 0
	lo, hi := 0, total
	for lo <= hi {
		mid := (lo + hi) / 2
		if ok, _, _, _ := fitsAt(mid); ok {
			best = mid
			lo = mid + 1
		} else {
			hi = mid - 1
		}
	}

	text := buildCandidate(best)
	ok, box, moved, lines := fitsAtReliable(text, rc, wrapW, common, e)
	if !ok {
		// Defensive: walk a little shorter if word-boundary trimming changed
		// the geometry after the binary search.
		found := false
		for k := best - 1; k >= 0; k-- {
			text = buildCandidate(k)
			if ok2, box2, moved2, lines2 := fitsAtReliable(text, rc, wrapW, common, e); ok2 {
				box, moved, lines, ok = box2, moved2, lines2, true
				found = true
				break
			}
		}
		if !found {
			p.Summary = "缩短后仍无法进入安全区"
			p.Reasons = append(p.Reasons, "shorten_failed")
			return p
		}
	}

	if text == rc.Cue.Text {
		p.Summary = "无需缩短即可容纳"
		p.Reasons = append(p.Reasons, "shorten_no_change")
		return p
	}

	p.Feasible = true
	p.Text = text
	p.Lines = lines
	e.finalizeBox(p, rc, box, moved)
	if moved {
		p.Reasons = append(p.Reasons, "includes_move")
		p.Summary = "缩短文案并做最小迁移以进入动态安全区"
	} else {
		p.Summary = "缩短文案（尾部加省略号）即可进入动态安全区"
	}
	// Shortening loses content: never an automatic default application, even
	// with trusted tracking. Operators apply the explicit plan id.
	p.AutoEligible = false
	p.Reasons = appendUnique(p.Reasons, "requires_content_confirmation")
	if moved {
		// If a move is involved it must additionally pass the hard gate;
		// markAuto records that reason without making it auto-eligible.
		e.markAuto(p, true)
		p.AutoEligible = false
	}
	return p
}

// fitsAtReliable validates a concrete candidate string.
func fitsAtReliable(text string, rc ResolvedCue, wrapW float64, common Rect, e *Engine) (bool, Rect, bool, []string) {
	layout, box := rc.layoutCandidate(text, wrapW, e.Measurer)
	if len(layout.Lines) > rc.Style.MaxLines {
		return false, box, false, layout.Lines
	}
	moved := false
	if !FitsInside(box, common) {
		dx, dy, fitX, fitY := MinimalTranslation(box, common)
		if !fitX || !fitY {
			return false, box, false, layout.Lines
		}
		box = box.Translate(dx, dy)
		moved = dx != 0 || dy != 0
		if !FitsInside(box, common) {
			return false, box, moved, layout.Lines
		}
	}
	return true, box, moved, layout.Lines
}

// shortenText keeps at most keep runes, backs off Latin cuts to a word
// boundary, and appends an ellipsis when text was cut.
func shortenText(text string, keep, total int) string {
	runes := []rune(text)
	if keep >= total {
		return text
	}
	if keep <= 0 {
		return ellipsis
	}

	head := runes[:keep]
	// Back off to the last whitespace when cutting through a Latin word.
	if keep < total {
		prev := runes[keep-1]
		next := runes[keep]
		if unicode.IsLetter(prev) || unicode.IsDigit(prev) {
			if unicode.IsLetter(next) || unicode.IsDigit(next) {
				lastSpace := -1
				for i := keep - 1; i >= 0; i-- {
					if unicode.IsSpace(runes[i]) {
						lastSpace = i
						break
					}
				}
				if lastSpace > 0 {
					head = runes[:lastSpace]
				}
			}
		}
	}
	trimmed := strings.TrimRight(string(head), " \t\r\n,，、;；")
	if trimmed == "" {
		return ellipsis
	}
	return trimmed + ellipsis
}
