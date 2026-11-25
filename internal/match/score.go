package match

import (
	"fmt"
	"math"

	"proxyrelink/internal/media"
)

// Evidence is one scored dimension of a proxy/original comparison.
type Evidence struct {
	Dimension  string  `json:"dimension"`
	Available  bool    `json:"available"`
	Score      float64 `json:"score"`      // 0..1, how well this dimension matches
	Weight     float64 `json:"weight"`     // contribution to the aggregate
	Confidence float64 `json:"confidence"` // 0..1, reliability of this evidence
	Detail     string  `json:"detail"`     // human-readable explanation
}

// Result is the full multi-dimensional comparison of one candidate pair.
type Result struct {
	ProxyID    string     `json:"proxy_id"`
	OriginalID string     `json:"original_id"`
	Score      float64    `json:"score"`
	Confidence float64    `json:"confidence"`
	Verdict    string     `json:"verdict"` // auto | review | weak
	Evidence   []Evidence `json:"evidence"`
}

// Verdict thresholds.
const (
	AutoScoreMin   = 0.85
	AutoConfMin    = 0.70
	ReviewScoreMin = 0.60
)

// ScorePair computes the multi-dimensional match between a proxy and an
// original. All math is done here in Go.
func ScorePair(proxy, orig media.MediaInfo) Result {
	evs := []Evidence{
		scoreDuration(proxy, orig),
		scoreFrameCount(proxy, orig),
		scoreFPS(proxy, orig),
		scoreFirstFrame(proxy, orig),
		scoreAudio(proxy, orig),
		scoreName(proxy, orig),
		scoreResolutionScale(proxy, orig),
	}
	var wSum, wsSum, wcSum float64
	for _, e := range evs {
		if !e.Available {
			continue
		}
		wSum += e.Weight
		wsSum += e.Weight * e.Score
		wcSum += e.Weight * e.Confidence
	}
	res := Result{ProxyID: proxy.ID, OriginalID: orig.ID, Evidence: evs}
	if wSum > 0 {
		res.Score = wsSum / wSum
		res.Confidence = wcSum / wSum
	}
	switch {
	case res.Score >= AutoScoreMin && res.Confidence >= AutoConfMin:
		res.Verdict = "auto"
	case res.Score >= ReviewScoreMin:
		res.Verdict = "review"
	default:
		res.Verdict = "weak"
	}
	return res
}

func scoreDuration(p, o media.MediaInfo) Evidence {
	e := Evidence{Dimension: "duration", Weight: 0.25, Confidence: 0.95}
	if p.DurationMs <= 0 || o.DurationMs <= 0 {
		e.Detail = "duration missing"
		return e
	}
	e.Available = true
	diffMs := math.Abs(float64(p.DurationMs - o.DurationMs))
	frameMs := o.FrameDurationMs()
	if frameMs <= 0 {
		frameMs = 40 // assume 25fps when unknown
	}
	switch {
	case diffMs == 0:
		e.Score = 1
	case diffMs <= frameMs:
		e.Score = 0.9
	case diffMs <= 2*frameMs:
		e.Score = 0.7
	default:
		// exponential decay: half score per 500ms of drift
		e.Score = math.Max(0, 0.5*math.Exp(-(diffMs-2*frameMs)/500))
	}
	e.Detail = fmt.Sprintf("proxy=%dms original=%dms diff=%.0fms", p.DurationMs, o.DurationMs, diffMs)
	return e
}

func scoreFrameCount(p, o media.MediaInfo) Evidence {
	e := Evidence{Dimension: "frame_count", Weight: 0.20, Confidence: 0.95}
	if p.FrameCount <= 0 || o.FrameCount <= 0 {
		e.Detail = "frame count missing"
		return e
	}
	e.Available = true
	diff := math.Abs(float64(p.FrameCount - o.FrameCount))
	switch {
	case diff == 0:
		e.Score = 1
	case diff == 1:
		e.Score = 0.85
	case diff == 2:
		e.Score = 0.6
	default:
		e.Score = math.Max(0, 0.4*math.Exp(-(diff-2)/10))
	}
	e.Detail = fmt.Sprintf("proxy=%d original=%d diff=%.0f frames", p.FrameCount, o.FrameCount, diff)
	return e
}

func scoreFPS(p, o media.MediaInfo) Evidence {
	e := Evidence{Dimension: "fps", Weight: 0.10, Confidence: 0.90}
	if p.FPS() <= 0 || o.FPS() <= 0 {
		e.Detail = "fps missing"
		return e
	}
	e.Available = true
	if math.Abs(p.FPS()-o.FPS()) < 0.001 {
		e.Score = 1
	} else {
		e.Score = 0
	}
	e.Detail = fmt.Sprintf("proxy=%.3f original=%.3f", p.FPS(), o.FPS())
	return e
}

func scoreFirstFrame(p, o media.MediaInfo) Evidence {
	// Proxies are re-encodes, so pixel hashes legitimately differ;
	// this dimension carries low weight and moderate confidence.
	e := Evidence{Dimension: "first_frame_hash", Weight: 0.10, Confidence: 0.60}
	sim, ok := hexHammingSimilarity(p.FirstFrameHash, o.FirstFrameHash)
	if !ok {
		e.Detail = "first-frame hash missing or not comparable"
		return e
	}
	e.Available = true
	e.Score = sim
	e.Detail = fmt.Sprintf("perceptual hash similarity=%.1f%% (re-encode expected to differ)", sim*100)
	return e
}

func scoreAudio(p, o media.MediaInfo) Evidence {
	e := Evidence{Dimension: "audio_fingerprint", Weight: 0.20, Confidence: 0.80}
	sim, ok := hexHammingSimilarity(p.AudioFingerprint, o.AudioFingerprint)
	if !ok {
		e.Detail = "audio fingerprint missing or not comparable"
		return e
	}
	e.Available = true
	e.Score = sim
	e.Detail = fmt.Sprintf("fingerprint similarity=%.1f%%", sim*100)
	return e
}

func scoreName(p, o media.MediaInfo) Evidence {
	e := Evidence{Dimension: "filename", Weight: 0.10, Confidence: 0.70, Available: true}
	e.Score = nameSimilarity(p.Path, o.Path)
	e.Detail = fmt.Sprintf("stem similarity=%.1f%%", e.Score*100)
	return e
}

func scoreResolutionScale(p, o media.MediaInfo) Evidence {
	e := Evidence{Dimension: "resolution_scale", Weight: 0.05, Confidence: 0.80}
	if p.Width <= 0 || p.Height <= 0 || o.Width <= 0 || o.Height <= 0 {
		e.Detail = "resolution missing"
		return e
	}
	e.Available = true
	rw := float64(o.Width) / float64(p.Width)
	rh := float64(o.Height) / float64(p.Height)
	// A proper proxy is a clean rational downscale (2x, 4x, 1.5x...).
	clean := func(r float64) bool {
		for _, d := range []float64{1, 2, 3, 4, 8} {
			for _, n := range []float64{1, 2, 3} {
				if math.Abs(r-n/d) < 0.01 {
					return true
				}
			}
		}
		return false
	}
	switch {
	case math.Abs(rw-rh) > 0.01:
		e.Score = 0.2 // non-uniform scale, suspicious
	case clean(rw):
		e.Score = 1
	default:
		e.Score = 0.5
	}
	e.Detail = fmt.Sprintf("proxy=%dx%d original=%dx%d scale=%.2fx", p.Width, p.Height, o.Width, o.Height, rw)
	return e
}
