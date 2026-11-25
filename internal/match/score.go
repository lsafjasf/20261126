// Package match scores migration candidates against the metadata a
// project recorded before the move, and decides which relinks are safe
// to apply automatically. File names are never used as identity.
package match

import (
	"fmt"
	"math"

	"mediarelink/internal/media"
)

// Expected holds the pre-migration metadata a project stored for one
// media asset. It is the ground truth candidates are scored against.
type Expected struct {
	MediaID          string
	Name             string
	OldPath          string
	Codec            string
	DurationMS       int64
	FPSNum           int
	FPSDen           int
	FrameCount       int
	FrameHash        string
	AudioFingerprint string
}

// Evidence is one scored dimension of a candidate comparison.
type Evidence struct {
	Dimension string  `json:"dimension"`
	Score     float64 `json:"score"`
	Weight    float64 `json:"weight"`
	Detail    string  `json:"detail"`
}

// Dimension weights. Frame hash dominates because it is content identity;
// codec alone is weak but still breaks ties between near-duplicates.
const (
	wFrameHash = 0.40
	wAudioFP   = 0.25
	wDuration  = 0.20
	wCodec     = 0.15
)

// audioRejectBelow: below this audio similarity a candidate whose frame
// hash also differs is treated as unrelated content (a decoy).
const audioRejectBelow = 0.90

// Score compares one candidate file against the expected metadata and
// returns the weighted confidence plus per-dimension evidence.
func Score(exp Expected, cand media.Probe) (confidence float64, ev []Evidence) {
	ev = append(ev, scoreFrameHash(exp, cand))
	ev = append(ev, scoreAudio(exp, cand))
	ev = append(ev, scoreDuration(exp, cand))
	ev = append(ev, scoreCodec(exp, cand))
	var total, weight float64
	for _, e := range ev {
		total += e.Score * e.Weight
		weight += e.Weight
	}
	if weight == 0 {
		return 0, ev
	}
	return total / weight, ev
}

// IsDecoy reports whether the evidence proves the candidate is different
// content: frame hash mismatch plus clearly unrelated audio.
func IsDecoy(ev []Evidence) bool {
	var frameOK bool
	var audio float64
	for _, e := range ev {
		switch e.Dimension {
		case "frame_hash":
			frameOK = e.Score == 1
		case "audio_fingerprint":
			audio = e.Score
		}
	}
	return !frameOK && audio < audioRejectBelow
}

func scoreFrameHash(exp Expected, cand media.Probe) Evidence {
	e := Evidence{Dimension: "frame_hash", Weight: wFrameHash}
	if exp.FrameHash == cand.FrameHash {
		e.Score = 1
		e.Detail = "exact content match"
	} else {
		e.Detail = "frame payload differs"
	}
	return e
}

func scoreAudio(exp Expected, cand media.Probe) Evidence {
	e := Evidence{Dimension: "audio_fingerprint", Weight: wAudioFP}
	want, err := media.ParseFingerprint(exp.AudioFingerprint)
	if err != nil {
		e.Detail = "expected fingerprint missing"
		return e
	}
	got, err := media.ParseFingerprint(cand.AudioFingerprint)
	if err != nil {
		e.Detail = "candidate fingerprint missing"
		return e
	}
	e.Score = media.FingerprintSimilarity(want, got)
	e.Detail = fmt.Sprintf("hamming similarity %.3f", e.Score)
	return e
}

func scoreDuration(exp Expected, cand media.Probe) Evidence {
	e := Evidence{Dimension: "duration", Weight: wDuration}
	d := exp.DurationMS - cand.DurationMS
	if d < 0 {
		d = -d
	}
	switch {
	case d == 0:
		e.Score = 1
	case d <= 40:
		e.Score = 0.9
	default:
		// Exponential decay: ~0.37 at 1s drift, ~0 at several seconds.
		e.Score = math.Exp(-float64(d) / 1000)
	}
	e.Detail = fmt.Sprintf("expected %dms, got %dms", exp.DurationMS, cand.DurationMS)
	return e
}

func scoreCodec(exp Expected, cand media.Probe) Evidence {
	e := Evidence{Dimension: "codec", Weight: wCodec}
	if exp.Codec == cand.Codec {
		e.Score = 1
		e.Detail = "codec " + cand.Codec
	} else {
		e.Detail = fmt.Sprintf("expected %s, got %s", exp.Codec, cand.Codec)
	}
	return e
}
