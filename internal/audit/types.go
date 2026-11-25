// Package audit ties subtitle parsing, encoding checks and font coverage
// together into one report. It only observes and recommends: no font is
// substituted and no subtitle byte is rewritten.
package audit

import (
	"time"

	"fontaudit/internal/fontscan"
	"fontaudit/internal/runeinfo"
	"fontaudit/internal/subtitle"
)

// EncodingReport summarizes the render-encoding findings.
type EncodingReport struct {
	Detected         string   `json:"detected"`
	Notes            []string `json:"notes"`
	ReplacementRunes int      `json:"replacementRunes"`
	EmbeddedNuls     int      `json:"embeddedNuls"`
	Mojibake         bool     `json:"mojibake"`
}

// FirstOccurrence records where a missing rune first appears.
type FirstOccurrence struct {
	CueIndex int    `json:"cueIndex"`
	Start    string `json:"start"`
	Line     string `json:"line"`
}

// Substitution is the human-reviewed replacement suggestion.
type Substitution struct {
	Kind      string `json:"kind"` // "nfkc" | "punctuation-alias"
	From      string `json:"from"`
	To        string `json:"to"`
	Codepoint string `json:"codepoint"`
	Rationale string `json:"rationale"`
}

// MissingRune is one distinct uncovered rune with full evidence.
type MissingRune struct {
	Info          runeinfo.Info   `json:"info"`
	Occurrences   int             `json:"occurrences"`
	First         FirstOccurrence `json:"firstOccurrence"`
	Cues          []int           `json:"cueIndexes"`
	Substitutions []Substitution  `json:"substitutions"`
	FallbackFonts []FallbackFont  `json:"fallbackFonts,omitempty"`
}

// FallbackFont is a scanned face that actually contains the rune.
type FallbackFont struct {
	Path     string `json:"path"`
	Family   string `json:"family"`
	FullName string `json:"fullName"`
}

// Proof references a rendered image that demonstrates tofu.
type Proof struct {
	CueIndex int      `json:"cueIndex"`
	Start    string   `json:"start"`
	Line     string   `json:"line"`
	Boxes    []string `json:"boxes"`
	PNGPath  string   `json:"pngPath"`
}

// Report is the complete audit result.
type Report struct {
	GeneratedAt      time.Time            `json:"generatedAt"`
	SubtitlePath     string               `json:"subtitlePath"`
	SubtitleFormat   subtitle.Format      `json:"subtitleFormat"`
	CueCount         int                  `json:"cueCount"`
	DeclaredFonts    []subtitle.FontRef   `json:"declaredFonts"`
	PrimaryFont      FontSummary          `json:"primaryFont"`
	Encoding         EncodingReport       `json:"encoding"`
	DistinctMissing  int                  `json:"distinctMissing"`
	TotalMissingHits int                  `json:"totalMissingHits"`
	Missing          []MissingRune        `json:"missing"`
	Candidates       []fontscan.Candidate `json:"candidates"`
	Proofs           []Proof              `json:"proofs,omitempty"`
	Warnings         []string             `json:"warnings,omitempty"`
}

// FontSummary identifies the face used for the burn-in.
type FontSummary struct {
	Path      string `json:"path"`
	FaceIndex int    `json:"faceIndex"`
	Family    string `json:"family"`
	FullName  string `json:"fullName"`
	NumGlyphs int    `json:"numGlyphs"`
}

// Options controls the audit run.
type Options struct {
	SubtitlePath string
	SubtitleRaw  []byte
	Primary      *fontscan.Face
	ScanDirs     []string
	ProofDir     string
	MaxProofs    int
}
