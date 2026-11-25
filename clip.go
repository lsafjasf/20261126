package clipfade

import (
	"hash/fnv"
	"math"
)

// ClipID identifies a timeline clip.
type ClipID string

// FadeID identifies a clip-level crossfade.
type FadeID string

// GroupID links timeline items that move together (for example a video
// clip and its bound audio clips). Empty means "not linked".
type GroupID string

// ClipKind distinguishes audio from picture clips. Picture clips carry no
// samples but moving one still invalidates every bound edit.
type ClipKind string

const (
	KindAudio ClipKind = "audio"
	KindVideo ClipKind = "video"
)

// Clip is a timeline item.
//
// Samples is the full native media buffer, including handles outside the
// used range. Start is the timeline position of the used In point; InOffset
// is the index in Samples that maps to Start. VisibleLen is the used length
// in samples, so the cut/out point is Start+VisibleLen.
//
//	timeline coordinate t -> Samples[InOffset + (t - Start)]
type Clip struct {
	ID         ClipID
	Kind       ClipKind
	Group      GroupID
	Start      int64
	InOffset   int64
	VisibleLen int64
	Samples    []float64

	contentHash uint64
}

// End returns the timeline out point (exclusive), i.e. the cut coordinate
// for the left clip of an adjacent pair.
func (c *Clip) End() int64 { return c.Start + c.VisibleLen }

// sourceIndex converts a timeline coordinate to a Samples index.
func (c *Clip) sourceIndex(timelineSample int64) int64 {
	return c.InOffset + (timelineSample - c.Start)
}

// timelineSample is the inverse of sourceIndex.
func (c *Clip) timelineSample(sourceIndex int64) int64 {
	return c.Start - c.InOffset + sourceIndex
}

// sampleAt returns the media value at a timeline coordinate.
func (c *Clip) sampleAt(timelineSample int64) (float64, bool) {
	idx := c.sourceIndex(timelineSample)
	if idx < 0 || int(idx) >= len(c.Samples) {
		return 0, false
	}
	return c.Samples[idx], true
}

// hashContent returns a stable FNV hash of the media buffer. It is computed
// once when the clip is added to the model.
func hashSamples(samples []float64) uint64 {
	h := fnv.New64a()
	var bits [8]byte
	for _, v := range samples {
		u := math.Float64bits(v)
		for i := 0; i < 8; i++ {
			bits[i] = byte(u >> (8 * i))
		}
		_, _ = h.Write(bits[:])
	}
	return h.Sum64()
}
