// Package fontscan inspects TrueType/OpenType collections through their cmap
// and name tables. Coverage is answered from the font itself, not from font
// config heuristics, so a "covered" result means a real glyph outline exists.
package fontscan

import (
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/image/font/sfnt"
)

// Face is one parsed font face (a standalone file or one face of a TTC).
type Face struct {
	Path       string `json:"path"`
	Collection string `json:"collection,omitempty"`
	Index      int    `json:"index"`
	Family     string `json:"family"`
	Subfamily  string `json:"subfamily,omitempty"`
	FullName   string `json:"fullName"`
	NumGlyphs  int    `json:"numGlyphs"`
	HasNotdef  bool   `json:"hasNotdef"`
	font       *sfnt.Font
	buf        []byte
}

// FontFile is a possibly-collection font file.
type FontFile struct {
	Path  string
	Faces []*Face
}

var ttcMagic = []byte{'t', 't', 'c', 'f'}

// Open parses a TTF/OTF/TTC/OTC file.
func Open(path string) (*FontFile, error) {
	buf, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	ff := &FontFile{Path: path}
	if len(buf) >= 4 && string(buf[:4]) == string(ttcMagic) {
		collection, err := sfnt.ParseCollection(buf)
		if err != nil {
			return nil, fmt.Errorf("parse collection %s: %w", path, err)
		}
		for i := 0; i < collection.NumFonts(); i++ {
			fnt, err := collection.Font(i)
			if err != nil {
				return nil, err
			}
			face, err := describe(path, filepath.Base(path), i, fnt, buf)
			if err != nil {
				return nil, err
			}
			ff.Faces = append(ff.Faces, face)
		}
		return ff, nil
	}
	fnt, err := sfnt.Parse(buf)
	if err != nil {
		return nil, fmt.Errorf("parse font %s: %w", path, err)
	}
	face, err := describe(path, "", 0, fnt, buf)
	if err != nil {
		return nil, err
	}
	ff.Faces = []*Face{face}
	return ff, nil
}

func describe(path, collection string, index int, fnt *sfnt.Font, buf []byte) (*Face, error) {
	var hb sfnt.Buffer
	family, err := fnt.Name(&hb, sfnt.NameIDFamily)
	if err != nil {
		return nil, fmt.Errorf("name table %s: %w", path, err)
	}
	sub, _ := fnt.Name(&hb, sfnt.NameIDSubfamily)
	full, _ := fnt.Name(&hb, sfnt.NameIDFull)
	if full == "" {
		full = strings.TrimSpace(family + " " + sub)
	}
	return &Face{
		Path:       path,
		Collection: collection,
		Index:      index,
		Family:     family,
		Subfamily:  sub,
		FullName:   full,
		NumGlyphs:  fnt.NumGlyphs(),
		HasNotdef:  true, // sfnt always exposes glyph 0 (.notdef); the audit
		// treats glyph 0 as "will render tofu", never as coverage.
		font: fnt,
		buf:  buf,
	}, nil
}

// Coverage describes whether runes map to real glyphs.
type Coverage struct {
	Covered map[rune]bool
	Missing []rune
}

// Check returns the missing runes among the requested set. Glyph index 0 is
// .notdef, i.e. the box that appears in the burned-in frame.
func (f *Face) Check(runes []rune) Coverage {
	cov := Coverage{Covered: map[rune]bool{}}
	seen := map[rune]bool{}
	var hb sfnt.Buffer
	for _, r := range runes {
		if seen[r] {
			continue
		}
		seen[r] = true
		g, err := f.font.GlyphIndex(&hb, r)
		if err != nil || g == 0 {
			cov.Missing = append(cov.Missing, r)
			continue
		}
		cov.Covered[r] = true
	}
	return cov
}

// Covers reports a single rune.
func (f *Face) Covers(r rune) bool {
	var hb sfnt.Buffer
	g, err := f.font.GlyphIndex(&hb, r)
	return err == nil && g != 0
}

// Font returns the underlying sfnt handle for rendering.
func (f *Face) Font() *sfnt.Font { return f.font }

// IsCollection reports whether a buffer is a TTC/OTC from its tag.
func IsCollection(buf []byte) bool {
	return len(buf) >= 4 && binary.BigEndian.Uint32(buf) == 0x74746366
}

// ErrSkip is returned by scanners for unreadable files.
var ErrSkip = errors.New("font file skipped")

// SortMissing returns runes ordered by codepoint for stable reports.
func SortMissing(runes []rune) {
	sort.Slice(runes, func(i, j int) bool { return runes[i] < runes[j] })
}
