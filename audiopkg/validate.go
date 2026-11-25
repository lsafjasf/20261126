package audiopkg

import (
	"fmt"
	"regexp"
	"strings"
)

// bcp47ish accepts common BCP 47 tags such as "en", "zh-Hans", "pt-BR".
// Only the standard library is available, so this is a structural check
// rather than full registry validation.
var bcp47ish = regexp.MustCompile(`^[a-z]{2,3}(-[A-Za-z0-9]{2,8})*$`)

var knownRoles = map[Role]bool{
	RoleMain:        true,
	RoleCommentary:  true,
	RoleDescriptive: true,
	RoleAlternate:   true,
}

var knownAccessibility = map[Accessibility]bool{
	AccessibilityNone:             true,
	AccessibilityHearingImpaired:  true,
	AccessibilityVisuallyImpaired: true,
}

// ValidationError describes one consistency violation.
type ValidationError struct {
	TrackID string
	Field   string
	Message string
}

func (e ValidationError) Error() string {
	if e.TrackID == "" {
		return fmt.Sprintf("%s: %s", e.Field, e.Message)
	}
	return fmt.Sprintf("track %q: %s: %s", e.TrackID, e.Field, e.Message)
}

// Validate checks language, role, accessibility and default-state
// consistency across the whole package. It returns every violation found;
// an empty slice means the package is publishable.
func (p *Package) Validate() []ValidationError {
	var errs []ValidationError
	seen := make(map[string]string)     // language|role|accessibility -> track ID
	defaults := make(map[string]string) // language -> default track ID

	for i := range p.Tracks {
		t := &p.Tracks[i]
		errs = append(errs, validateTrack(t)...)

		key := string(t.Role) + "|" + string(t.Accessibility) + "|" + strings.ToLower(t.Language)
		if prev, dup := seen[key]; dup {
			errs = append(errs, ValidationError{t.ID, "role",
				fmt.Sprintf("duplicates track %q (same language, role and accessibility)", prev)})
		} else {
			seen[key] = t.ID
		}

		if t.Default {
			lang := strings.ToLower(t.Language)
			if prev, dup := defaults[lang]; dup {
				errs = append(errs, ValidationError{t.ID, "default",
					fmt.Sprintf("language %q already has default track %q", t.Language, prev)})
			} else {
				defaults[lang] = t.ID
			}
		}
	}
	return errs
}

func validateTrack(t *Track) []ValidationError {
	var errs []ValidationError

	if t.ID == "" {
		errs = append(errs, ValidationError{t.ID, "id", "must not be empty"})
	}
	if !bcp47ish.MatchString(t.Language) {
		errs = append(errs, ValidationError{t.ID, "language",
			fmt.Sprintf("%q is not a valid BCP 47 language tag", t.Language)})
	}
	if !knownRoles[t.Role] {
		errs = append(errs, ValidationError{t.ID, "role",
			fmt.Sprintf("unknown role %q", t.Role)})
	}
	if !knownAccessibility[t.Accessibility] {
		errs = append(errs, ValidationError{t.ID, "accessibility",
			fmt.Sprintf("unknown accessibility value %q", t.Accessibility)})
	}

	// Accessibility renditions must not be presented as the main program.
	if t.IsAccessibilityTrack() && t.Role == RoleMain {
		errs = append(errs, ValidationError{t.ID, "role",
			fmt.Sprintf("accessibility track (%s) cannot have role %q", t.Accessibility, RoleMain)})
	}
	// A descriptive track is by definition a visually-impaired rendition.
	if t.Role == RoleDescriptive && t.Accessibility != AccessibilityVisuallyImpaired {
		errs = append(errs, ValidationError{t.ID, "accessibility",
			fmt.Sprintf("role %q requires accessibility %q", RoleDescriptive, AccessibilityVisuallyImpaired)})
	}
	// Only a plain main track may be the default for its language.
	if t.Default && (t.Role != RoleMain || t.IsAccessibilityTrack()) {
		errs = append(errs, ValidationError{t.ID, "default",
			"only a non-accessibility main track may be marked default"})
	}
	return errs
}

// Publish gates publication on Validate: any violation blocks the release.
func (p *Package) Publish() error {
	errs := p.Validate()
	if len(errs) == 0 {
		return nil
	}
	msgs := make([]string, len(errs))
	for i, e := range errs {
		msgs[i] = e.Error()
	}
	return fmt.Errorf("publication blocked: %d consistency violation(s):\n  %s",
		len(errs), strings.Join(msgs, "\n  "))
}
