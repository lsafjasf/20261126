package subselect

// Kind classifies a subtitle track's presentation role.
type Kind string

const (
	KindUnspecified Kind = ""
	KindSubtitles   Kind = "subtitles"
	KindCaptions    Kind = "captions"
	KindForced      Kind = "forced"
)

// TriState is a JSON/HTTP friendly yes/no/any preference.
type TriState string

const (
	Any TriState = ""
	Yes TriState = "yes"
	No  TriState = "no"
)

// Cue is one source subtitle event. Start and End are expressed in the
// owning Track's TimeBase units and the interval is half-open [Start, End).
type Cue struct {
	ID    string `json:"id"`
	Start int64  `json:"start"`
	End   int64  `json:"end"`
	Text  string `json:"text"`
}

// Track is one selectable subtitle version. Tracks are value objects owned
// by an immutable Catalog and may be shared by every browser session.
type Track struct {
	// ID is the unique track/version id, e.g. "zh-CN-director-cut".
	ID string `json:"id"`

	// Language is a BCP 47-ish tag ("zh", "zh-CN", "en-US").
	Language string `json:"language"`

	// Version disambiguates multiple releases/versions of one language,
	// e.g. "theatrical", "director-cut", "v2". Empty means unversioned.
	Version string `json:"version"`

	Title string `json:"title"`
	Kind  Kind   `json:"kind"`

	Forced          bool `json:"forced"`
	HearingImpaired bool `json:"hearing_impaired"`
	Default         bool `json:"default"`

	// Ranking is a manual same-language tie breaker; higher wins.
	Ranking int `json:"ranking"`

	// Unavailable marks a track the catalog knows cannot currently be
	// loaded (e.g. 404 / transcoding). Selectors skip it.
	Unavailable bool `json:"unavailable"`

	TimeBase TimeBase `json:"time_base"`

	// Cues may be populated for in-memory/static catalogs. When a Loader
	// is configured it is authoritative for cue bytes.
	Cues []Cue `json:"cues,omitempty"`
}

// Criteria is a subtitle selection request. TrackID and Version exist so
// that several tracks sharing one language can be told apart.
type Criteria struct {
	// Language is the desired language tag. Empty means "any language".
	Language string `json:"language,omitempty"`

	// TrackID requests one exact track/version and wins over all other
	// preferences when the track exists and is usable.
	TrackID string `json:"track_id,omitempty"`

	// Version requests one same-language version label.
	Version string `json:"version,omitempty"`

	Kind Kind `json:"kind,omitempty"`

	Forced          TriState `json:"forced,omitempty"`
	HearingImpaired TriState `json:"hearing_impaired,omitempty"`
}

func (c Criteria) withDefaults() Criteria {
	if c.Forced == "" {
		c.Forced = Any
	}
	if c.HearingImpaired == "" {
		c.HearingImpaired = Any
	}
	return c
}
