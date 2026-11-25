// Package audiopkg models a multilingual audio package and enforces
// consistency between language, role, accessibility and default-state
// metadata before a package may be published.
package audiopkg

// Role describes the editorial function of an audio track.
type Role string

const (
	RoleMain        Role = "main"        // primary program audio
	RoleCommentary  Role = "commentary"  // commentary / director's track
	RoleDescriptive Role = "descriptive" // audio description for the visually impaired
	RoleAlternate   Role = "alternate"   // alternate mix of the main program
)

// Accessibility marks a track as an accessibility rendition.
type Accessibility string

const (
	AccessibilityNone             Accessibility = ""
	AccessibilityHearingImpaired  Accessibility = "hearing-impaired"  // SDH-style track
	AccessibilityVisuallyImpaired Accessibility = "visually-impaired" // described video
)

// Track is a single audio track in a package.
type Track struct {
	ID            string        `json:"id"`
	Language      string        `json:"language"` // BCP 47 language tag, e.g. "en", "zh-Hans"
	Role          Role          `json:"role"`
	Accessibility Accessibility `json:"accessibility,omitempty"`
	Default       bool          `json:"default"`
	Title         string        `json:"title,omitempty"`
}

// Package is an ordered set of audio tracks ready for publication.
type Package struct {
	Name   string  `json:"name"`
	Tracks []Track `json:"tracks"`
}

// IsAccessibilityTrack reports whether the track is an accessibility rendition.
func (t Track) IsAccessibilityTrack() bool {
	return t.Accessibility != AccessibilityNone
}
