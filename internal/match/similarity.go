package match

import (
	"encoding/hex"
	"path/filepath"
	"strings"
)

// hexHammingSimilarity compares two hex-encoded fingerprints bit by bit.
// Returns similarity in [0,1]; 1 means identical. Returns ok=false when
// either input is missing or not comparable hex.
func hexHammingSimilarity(a, b string) (float64, bool) {
	a = strings.TrimSpace(strings.ToLower(a))
	b = strings.TrimSpace(strings.ToLower(b))
	if a == "" || b == "" {
		return 0, false
	}
	ba, errA := hex.DecodeString(a)
	bb, errB := hex.DecodeString(b)
	if errA != nil || errB != nil || len(ba) == 0 || len(ba) != len(bb) {
		return 0, false
	}
	total := len(ba) * 8
	diff := 0
	for i := range ba {
		x := ba[i] ^ bb[i]
		for x != 0 {
			diff += int(x & 1)
			x >>= 1
		}
	}
	return 1 - float64(diff)/float64(total), true
}

var proxySuffixes = []string{
	"_proxy", "-proxy", ".proxy", "_prox", "_p", "_lowres", "_low", "_half", "_hq4", "_mezz",
}

// normalizeStem strips directory, extension and common proxy/original
// suffixes so name comparison focuses on the shared clip identity.
func normalizeStem(path string) string {
	base := filepath.Base(path)
	ext := filepath.Ext(base)
	stem := strings.ToLower(strings.TrimSuffix(base, ext))
	for {
		trimmed := false
		for _, suf := range proxySuffixes {
			if strings.HasSuffix(stem, suf) {
				stem = strings.TrimSuffix(stem, suf)
				trimmed = true
			}
		}
		if !trimmed {
			break
		}
	}
	return stem
}

// levenshtein computes the edit distance between two strings.
func levenshtein(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	prev := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur := make([]int, len(rb)+1)
		cur[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 0
			if ra[i-1] != rb[j-1] {
				cost = 1
			}
			cur[j] = min3(cur[j-1]+1, prev[j]+1, prev[j-1]+cost)
		}
		prev = cur
	}
	return prev[len(rb)]
}

func min3(a, b, c int) int {
	if a < b {
		if a < c {
			return a
		}
		return c
	}
	if b < c {
		return b
	}
	return c
}

// nameSimilarity returns normalized stem similarity in [0,1].
func nameSimilarity(pathA, pathB string) float64 {
	sa, sb := normalizeStem(pathA), normalizeStem(pathB)
	if sa == "" || sb == "" {
		return 0
	}
	if sa == sb {
		return 1
	}
	longest := len([]rune(sa))
	if l := len([]rune(sb)); l > longest {
		longest = l
	}
	return 1 - float64(levenshtein(sa, sb))/float64(longest)
}
