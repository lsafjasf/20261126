package sdh

import (
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// dashRe captures the conventional leading dialogue dash ("- Wait.").
var dashRe = regexp.MustCompile(`^[\s　]*[-–—][\s　]*`)

// labelRe captures a leading "NAME [(V.O.)]:" speaker label. Group 1 is the
// name, group 2 the optional parenthetical delivery modifier.
var labelRe = regexp.MustCompile(
	`^([^:：()（）\[\]【】]{1,40}?)\s*(?:[（(]([^）)]{0,60})[）)])?\s*[:：][ \t　]*`)

var styleTagRe = regexp.MustCompile(`(?i)</?(?:i|b|u|em|strong|font|span)(?:\s[^<>]*)?>`)
var assOverrideRe = regexp.MustCompile(`\{\\[^}]*\}`)

// stripStyleTags removes inline SRT/HTML styling tags and ASS override blocks
// that carry no textual content.
func stripStyleTags(s string) string {
	s = assOverrideRe.ReplaceAllString(s, "")
	s = styleTagRe.ReplaceAllString(s, "")
	return s
}

type cueParseState struct {
	speaker   string
	voiceOver bool
	offScreen bool
}

type delKind int

const (
	delAnnotation delKind = iota
	delLabel
	delNote
)

type deletion struct {
	start int
	end   int
	kind  delKind
}

type lineScan struct {
	segments []Segment
	deletes  []deletion
}

// ClassifyCue splits one cue's text into ordered semantic segments.
func ClassifyCue(cue Cue) []Segment {
	if strings.TrimSpace(cue.Text) == "" {
		return nil
	}
	state := &cueParseState{}
	var out []Segment
	for i, rawLine := range strings.Split(cue.Text, "\n") {
		line := stripStyleTags(rawLine)
		scanned := scanLine(line, i, state)
		out = append(out, scanned.segments...)
	}
	return out
}

func roleOrder(r Role) int {
	if m, ok := roleMetadata[r]; ok {
		return m.order
	}
	return 999
}

// scanLine classifies a single source line. Byte offsets in the returned
// segments/deletions are relative to line.
func scanLine(line string, lineIdx int, state *cueParseState) lineScan {
	var scan lineScan

	pos := 0
	dashPresent := false
	if loc := dashRe.FindStringIndex(line); loc != nil && loc[0] == 0 {
		pos = loc[1]
		dashPresent = true
	}

	speakerEmitted := state.speaker != ""
	labelMatched := false

	// 1. Leading speaker label with colon.
	if loc := labelRe.FindStringSubmatchIndex(line[pos:]); loc != nil && loc[0] == 0 {
		name := strings.TrimSpace(line[pos+loc[2] : pos+loc[3]])
		if looksLikeSpeakerName(name) {
			labelMatched = true
			labelEnd := pos + loc[1]
			scan.deletes = append(scan.deletes, deletion{pos, labelEnd, delLabel})

			vo, off := false, false
			if loc[4] >= 0 {
				mod := strings.TrimSpace(line[pos+loc[4] : pos+loc[5]])
				mvo, moff, pure := parseModifier(mod)
				vo, off = mvo, moff
				if !pure && mod != "" {
					// Manner/action qualifier such as "(whispering)".
					openStart := pos + loc[4]
					if _, size := utf8.DecodeLastRuneInString(line[:openStart]); size > 0 {
						openStart -= size
					}
					closeEnd := pos + loc[5]
					if _, size := utf8.DecodeRuneInString(line[closeEnd:]); size > 0 {
						closeEnd += size
					}
					scan.segments = append(scan.segments, Segment{
						Role: RoleSound, Text: mod, Raw: line[openStart:closeEnd],
						Line: lineIdx, pos: pos + loc[4],
					})
				}
			}

			// A fresh label resets delivery flags from any prior speaker.
			labelVO := vo || isNarratorName(name)
			labelOS := off
			state.speaker = name
			state.voiceOver = labelVO
			state.offScreen = labelOS
			scan.segments = append(scan.segments, Segment{
				Role: RoleSpeaker, Text: name, Raw: line[pos:labelEnd],
				Line: lineIdx, Speaker: name, VoiceOver: labelVO, OffScreen: labelOS,
				pos: pos,
			})
			speakerEmitted = true
			pos = labelEnd
		}
	}

	// 2. Scan bracket groups and music notes, collecting spoken chunks.
	type chunk struct {
		text string
		note bool
		pos  int
	}
	var chunks []chunk
	chunkStart := 0
	if labelMatched {
		chunkStart = pos
	}
	chunkHasNote := false
	seenNonSpace := false
	firstBracket := true

	flushChunk := func(end int) {
		raw := line[chunkStart:end]
		if strings.TrimSpace(raw) != "" {
			chunks = append(chunks, chunk{text: raw, note: chunkHasNote, pos: chunkStart})
			chunkHasNote = false
		}
		chunkStart = end
	}

	p := pos
	for p < len(line) {
		r, size := utf8.DecodeRuneInString(line[p:])
		closeR, isBracket := bracketPairs[r]
		isStarPair := r == '*'
		if isStarPair {
			closeR = '*'
		}
		if isBracket || isStarPair {
			closeIdxRel := strings.IndexRune(line[p+size:], closeR)
			if closeIdxRel < 0 {
				p += size
				continue
			}
			closeIdx := p + size + closeIdxRel
			closeSize := utf8.RuneLen(closeR)
			bracketEnd := closeIdx + closeSize
			inner := line[p+size : closeIdx]

			before := strings.TrimSpace(line[:p])
			leading := firstBracket && !seenNonSpace &&
				(!speakerEmitted || before == "" || onlyDash(before))

			flushChunk(p)

			if vo, off, pure := parseModifier(inner); pure {
				if vo {
					state.voiceOver = true
				}
				if off {
					state.offScreen = true
				}
				scan.deletes = append(scan.deletes, deletion{p, bracketEnd, delAnnotation})
			} else if leading && looksLikeBracketSpeaker(inner) {
				name := strings.TrimSpace(inner)
				state.speaker = name
				speakerEmitted = true
				scan.segments = append(scan.segments, Segment{
					Role: RoleSpeaker, Text: name, Raw: line[p:bracketEnd],
					Line: lineIdx, Speaker: name, pos: p,
				})
				scan.deletes = append(scan.deletes, deletion{p, bracketEnd, delLabel})
			} else {
				role := classifyBracketText(inner)
				text := strings.TrimSpace(inner)
				scan.segments = append(scan.segments, Segment{
					Role: role, Text: text, Raw: line[p:bracketEnd],
					Line: lineIdx, Speaker: state.speaker, pos: p,
				})
				scan.deletes = append(scan.deletes, deletion{p, bracketEnd, delAnnotation})
			}
			firstBracket = false
			p = bracketEnd
			chunkStart = p
			continue
		}

		if musicNotes[r] {
			scan.deletes = append(scan.deletes, deletion{p, p + size, delNote})
			chunkHasNote = true
		}
		if !unicode.IsSpace(r) {
			seenNonSpace = true
		}
		p += size
	}
	flushChunk(len(line))

	// 3. Merge spoken chunks into one segment for the line.
	var parts []string
	anyNote := false
	firstPos := -1
	for _, c := range chunks {
		clean := stripNoteRunes(c.text)
		clean = normalizeSpaces(clean)
		if clean == "" {
			continue
		}
		if firstPos < 0 {
			firstPos = c.pos
		}
		parts = append(parts, clean)
		if c.note {
			anyNote = true
		}
	}
	if spoken := smartJoin(parts); spoken != "" {
		if dashPresent && labelMatched {
			spoken = "- " + spoken
		}
		role := RoleDialogue
		switch {
		case anyNote:
			role = RoleLyric
		case state.voiceOver || state.offScreen:
			role = RoleVoiceOver
		}
		scan.segments = append(scan.segments, Segment{
			Role: role, Text: spoken, Line: lineIdx, Speaker: state.speaker,
			VoiceOver: state.voiceOver, OffScreen: state.offScreen, pos: firstPos,
		})
	}

	sort.SliceStable(scan.segments, func(i, j int) bool {
		return scan.segments[i].pos < scan.segments[j].pos
	})
	return scan
}

func onlyDash(s string) bool {
	for _, r := range s {
		if r != '-' && r != '–' && r != '—' && !unicode.IsSpace(r) {
			return false
		}
	}
	return true
}

func classifyBracketText(inner string) Role {
	trimmed := strings.TrimSpace(inner)
	if strings.TrimSpace(normalizeLex(trimmed)) == "" && containsMusicNote(trimmed) {
		return RoleMusic
	}
	return classifyDescription(trimmed)
}

func stripNoteRunes(s string) string {
	var b strings.Builder
	for _, r := range s {
		if musicNotes[r] {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// normalizeSpaces collapses whitespace runs to a single space and trims.
func normalizeSpaces(s string) string {
	var b strings.Builder
	lastSpace := true
	for _, r := range s {
		if unicode.IsSpace(r) {
			if !lastSpace {
				b.WriteRune(' ')
				lastSpace = true
			}
			continue
		}
		b.WriteRune(r)
		lastSpace = false
	}
	return strings.TrimSpace(b.String())
}

// smartJoin joins cleaned chunks, omitting the separator between two CJK
// boundaries so inline annotations can be removed without leaving gaps.
func smartJoin(parts []string) string {
	if len(parts) == 0 {
		return ""
	}
	out := parts[0]
	for _, next := range parts[1:] {
		if next == "" {
			continue
		}
		r1, _ := utf8.DecodeLastRuneInString(out)
		r2, _ := utf8.DecodeRuneInString(next)
		if isCJK(r1) && isCJK(r2) {
			out += next
		} else {
			out += " " + next
		}
	}
	return out
}

func isCJK(r rune) bool {
	return r >= 0x2e80 && r <= 0x9fff ||
		r >= 0x3000 && r <= 0x303f ||
		r >= 0xff00 && r <= 0xffef
}

// StripSDHLine returns the ordinary subtitle line after removing SDH
// annotation ranges according to opts.
func StripSDHLine(line string, opts PlainOptions) string {
	line = stripStyleTags(line)
	scan := scanLine(line, 0, &cueParseState{})

	drops := make([]deletion, 0, len(scan.deletes))
	for _, d := range scan.deletes {
		switch d.kind {
		case delLabel:
			if opts.KeepSpeakerLabels {
				continue
			}
		case delNote:
			if !opts.StripMusicNotes {
				continue
			}
		case delAnnotation:
		}
		drops = append(drops, d)
	}
	sort.Slice(drops, func(i, j int) bool { return drops[i].start < drops[j].start })

	var b strings.Builder
	last := 0
	for _, d := range drops {
		if d.start < last {
			continue
		}
		b.WriteString(line[last:d.start])
		last = d.end
	}
	b.WriteString(line[last:])

	out := normalizeSpaces(b.String())
	// Drop wordless remnants such as a dangling "-" from a dash+label-only
	// line, but preserve music-note-only lines ("♪♪").
	if out != "" && !hasWordRune(out) && !containsMusicNote(out) {
		return ""
	}
	return out
}

// StripSDHText applies StripSDHLine to every line and drops emptied lines.
func StripSDHText(text string, opts PlainOptions) string {
	var kept []string
	for _, line := range strings.Split(text, "\n") {
		stripped := StripSDHLine(line, opts)
		if stripped == "" {
			continue
		}
		kept = append(kept, stripped)
	}
	return strings.Join(kept, "\n")
}
