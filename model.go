package audiotracks

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode"
)

// TrackType is the semantic role of an audio track. The role must not be
// left as "audio": that is the generic container kind, not a usable role.
type TrackType string

const (
	TrackDialogue TrackType = "dialogue"
	TrackMusic    TrackType = "music"
	TrackEffects  TrackType = "effects"
	TrackDub      TrackType = "dub"
)

var ErrUnknownTrackType = errors.New("unknown track type")

func (t TrackType) Valid() bool {
	switch t {
	case TrackDialogue, TrackMusic, TrackEffects, TrackDub:
		return true
	default:
		return false
	}
}

func (t TrackType) IsVoice() bool {
	return t == TrackDialogue || t == TrackDub
}

// ParseTrackType accepts common edit-table role names and returns the
// canonical role. Generic container names such as "audio" are deliberately
// rejected; callers must recover the real role from another field or a name.
func ParseTrackType(raw string) (TrackType, error) {
	key := strings.ToLower(strings.TrimSpace(raw))
	key = strings.NewReplacer("-", "", "_", "", " ", "").Replace(key)
	switch key {
	case "dialogue", "dialog", "dlg", "dx", "对白", "台词":
		return TrackDialogue, nil
	case "music", "mus", "mx", "bgm", "score", "配乐", "音乐":
		return TrackMusic, nil
	case "effects", "effect", "fx", "sfx", "foley", "ambience", "amb", "音效", "效果":
		return TrackEffects, nil
	case "dub", "dubbing", "voiceover", "vo", "配音":
		return TrackDub, nil
	default:
		return "", fmt.Errorf("%w: %q", ErrUnknownTrackType, raw)
	}
}

// IsGenericAudioRole reports whether a legacy edit table merely says that a
// track belongs to the audio container.
func IsGenericAudioRole(raw string) bool {
	key := strings.ToLower(strings.TrimSpace(raw))
	return key == "" || key == "audio" || key == "sound" || key == "audio track"
}

// Language is a normalized BCP 47-style language tag. The empty value is
// invalid for voice tracks; neutral music/effects tracks use "und" or "zxx".
type Language string

const (
	LanguageUndetermined Language = "und"
	LanguageNoContent    Language = "zxx"
)

var ErrInvalidLanguage = errors.New("invalid language tag")

var languageTokenPattern = regexp.MustCompile(`^[A-Za-z0-9]{1,8}$`)

// ParseLanguage normalizes language names and tags commonly found in editing
// tables. Examples: "zh_cn" -> "zh-CN", "English" -> "en".
func ParseLanguage(raw string) (Language, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", ErrInvalidLanguage
	}
	value = strings.ReplaceAll(value, "_", "-")
	if canonical, ok := commonLanguageNames[strings.ToLower(value)]; ok {
		return canonical, nil
	}

	parts := strings.Split(value, "-")
	for i, part := range parts {
		if !languageTokenPattern.MatchString(part) {
			return "", fmt.Errorf("%w: %q", ErrInvalidLanguage, raw)
		}
		if i == 0 {
			if canonical, ok := commonLanguageNames[strings.ToLower(part)]; ok {
				parts[i] = string(canonical)
				continue
			}
			if !isASCIIAlpha(part) || len(part) < 2 || len(part) > 3 {
				return "", fmt.Errorf("%w: %q", ErrInvalidLanguage, raw)
			}
			parts[i] = strings.ToLower(part)
			continue
		}
		switch {
		case len(part) == 4 && isASCIIAlpha(part):
			parts[i] = strings.ToUpper(part[:1]) + strings.ToLower(part[1:])
		case (len(part) == 2 && isASCIIAlpha(part)) || (len(part) == 3 && isASCIIDigit(part)):
			parts[i] = strings.ToUpper(part)
		default:
			parts[i] = strings.ToLower(part)
		}
	}
	return Language(strings.Join(parts, "-")), nil
}

func (l Language) IsNeutral() bool {
	return l == LanguageUndetermined || l == LanguageNoContent
}

func isASCIIAlpha(s string) bool {
	for _, r := range s {
		if r > unicode.MaxASCII || !unicode.IsLetter(r) {
			return false
		}
	}
	return s != ""
}

func isASCIIDigit(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return s != ""
}

var commonLanguageNames = map[string]Language{
	"arabic":                "ar",
	"cantonese":             "yue",
	"chinese":               "zh",
	"dutch":                 "nl",
	"english":               "en",
	"french":                "fr",
	"german":                "de",
	"hindi":                 "hi",
	"italian":               "it",
	"japanese":              "ja",
	"korean":                "ko",
	"mandarin":              "zh",
	"portuguese":            "pt",
	"russian":               "ru",
	"spanish":               "es",
	"thai":                  "th",
	"turkish":               "tr",
	"undetermined":          LanguageUndetermined,
	"no linguistic content": LanguageNoContent,
	"no-linguistic-content": LanguageNoContent,
}

// Track is one canonical audio track. Audible is the single source of truth
// for audibility; muted is its inverse and is never stored separately.
type Track struct {
	ID       string    `json:"id"`
	Name     string    `json:"name,omitempty"`
	Language Language  `json:"language"`
	Type     TrackType `json:"type"`
	Default  bool      `json:"default"`
	Audible  bool      `json:"audible"`
}

func (t Track) Muted() bool { return !t.Audible }

// Project is the complete set of tracks for one film. A save replaces the
// project as a unit, never just the valid subset of tracks.
type Project struct {
	ID     string  `json:"projectId"`
	Title  string  `json:"title,omitempty"`
	Tracks []Track `json:"tracks"`
}

func (p Project) Clone() Project {
	clone := p
	clone.Tracks = append([]Track(nil), p.Tracks...)
	return clone
}

// DefaultsByLanguage returns the selected default track IDs keyed by
// canonical language. It is useful to callers that need to display the
// uniqueness invariant directly.
func (p Project) DefaultsByLanguage() map[Language][]string {
	defaults := make(map[Language][]string)
	for _, track := range p.Tracks {
		if track.Default {
			defaults[track.Language] = append(defaults[track.Language], track.ID)
		}
	}
	for language := range defaults {
		sort.Strings(defaults[language])
	}
	return defaults
}
