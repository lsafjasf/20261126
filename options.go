package sdh

import "time"

// PlainOptions controls the SDH-off projection produced by Plain /
// PlainSubtitles. The zero value keeps spoken dialogue, voice-over and
// lyrics and drops speaker labels, sound descriptions and music cues.
type PlainOptions struct {
	// KeepSpeakerLabels preserves "JOHN:" / "张三：" prefixes in the plain
	// track. Default (false) removes them with the rest of the SDH layer.
	KeepSpeakerLabels bool
	// ExcludeLyrics drops sung lyrics from the plain track. The default
	// keeps them because lyrics are program content, not annotation.
	ExcludeLyrics bool
	// StripMusicNotes removes ♪/♫ markers while keeping lyric words.
	// Default keeps the markers, as ordinary subtitles commonly show them.
	StripMusicNotes bool
}

// DefaultPlainOptions returns the editorial default for switching SDH off:
// all spoken/sung content survives, annotations disappear.
func DefaultPlainOptions() PlainOptions {
	return PlainOptions{}
}

// LayoutOptions configures overlap resolution.
type LayoutOptions struct {
	// MaxLanes caps the preferred number of lanes per anchor band. Zero
	// means unlimited. Over-cap entries are reported in Layout.Overflow
	// but still assigned a lane so no content is dropped.
	MaxLanes int
}

// DefaultLayoutOptions returns unlimited-lane overlap resolution.
func DefaultLayoutOptions() LayoutOptions {
	return LayoutOptions{}
}

// VerifyOptions tunes completeness verification.
type VerifyOptions struct {
	// TimeTolerance allows candidate plain cues to differ slightly from the
	// source interval when matching by time (ID matching is exact).
	TimeTolerance time.Duration
	// Plain should be produced with the same policy used here.
	PlainOptions PlainOptions
}

// DefaultVerifyOptions returns zero tolerance and default plain policy.
func DefaultVerifyOptions() VerifyOptions {
	return VerifyOptions{}
}
