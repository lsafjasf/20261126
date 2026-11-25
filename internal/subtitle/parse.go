package subtitle

import (
	"fmt"
	"path/filepath"
	"strings"

	"fontaudit/internal/encodingx"
)

// Parse reads subtitle bytes, decodes them through the encoding layer and
// dispatches to the right container parser. Encoding notes are surfaced on
// the returned file so callers can report them alongside glyph coverage.
func Parse(path string, raw []byte) (*File, error) {
	dec := encodingx.Decode(raw)

	format := sniffFormat(path, dec.Text)
	var f *File
	var err error
	switch format {
	case FormatASS:
		f, err = parseASS(dec.Text)
	case FormatVTT:
		f, err = parseSRTVTT(dec.Text, FormatVTT)
	default:
		f, err = parseSRTVTT(dec.Text, FormatSRT)
	}
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	f.Path = path
	f.Warnings = append(f.Warnings, dec.Notes()...)
	return f, nil
}

func sniffFormat(path, text string) Format {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".ass":
		return FormatASS
	case ".ssa":
		return FormatASS
	case ".vtt":
		return FormatVTT
	case ".srt":
		return FormatSRT
	}
	head := text
	if len(head) > 4096 {
		head = head[:4096]
	}
	switch {
	case strings.Contains(head, "[Script Info]"):
		return FormatASS
	case strings.HasPrefix(strings.TrimSpace(head), "WEBVTT"):
		return FormatVTT
	default:
		return FormatSRT
	}
}
