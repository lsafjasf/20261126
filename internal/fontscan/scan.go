package fontscan

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

var fontExts = map[string]bool{
	".ttf": true, ".otf": true, ".ttc": true, ".otc": true,
}

// Candidate is a font face that covers one or more missing runes. It is a
// recommendation only; nothing in the audit activates it automatically.
type Candidate struct {
	Face         *Face  `json:"-"`
	Path         string `json:"path"`
	Family       string `json:"family"`
	FullName     string `json:"fullName"`
	Covers       []rune `json:"covers"`
	CoveredCount int    `json:"coveredCount"`
	TotalMissing int    `json:"totalMissing"`
}

// ScanDir walks dirs (non-recursively first, then one level of common font
// subdirectories) and parses every supported font file. Unreadable files are
// skipped rather than aborting the audit.
func ScanDir(dirs []string) []*FontFile {
	var out []*FontFile
	seen := map[string]bool{}
	for _, dir := range dirs {
		filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			if !fontExts[strings.ToLower(filepath.Ext(path))] || seen[path] {
				return nil
			}
			seen[path] = true
			ff, err := Open(path)
			if err != nil {
				return nil
			}
			out = append(out, ff)
			return nil
		})
	}
	return out
}

// FindCandidates tests every scanned face against the missing rune set and
// ranks faces that cover the most runes first. The primary face is excluded
// so we never "recommend" the font that already failed.
func FindCandidates(files []*FontFile, missing []rune, primary *Face) []Candidate {
	want := map[rune]bool{}
	for _, r := range missing {
		want[r] = true
	}
	var cands []Candidate
	for _, ff := range files {
		for _, face := range ff.Faces {
			if primary != nil && sameFace(face, primary) {
				continue
			}
			var covers []rune
			for r := range want {
				if face.Covers(r) {
					covers = append(covers, r)
				}
			}
			if len(covers) == 0 {
				continue
			}
			sort.Slice(covers, func(i, j int) bool { return covers[i] < covers[j] })
			cands = append(cands, Candidate{
				Face:         face,
				Path:         face.Path,
				Family:       face.Family,
				FullName:     face.FullName,
				Covers:       covers,
				CoveredCount: len(covers),
				TotalMissing: len(want),
			})
		}
	}
	sort.SliceStable(cands, func(i, j int) bool {
		if cands[i].CoveredCount != cands[j].CoveredCount {
			return cands[i].CoveredCount > cands[j].CoveredCount
		}
		return cands[i].Path < cands[j].Path
	})
	return cands
}

func sameFace(a, b *Face) bool {
	return a.Path == b.Path && a.Index == b.Index
}
