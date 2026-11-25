package clipfade

import (
	"math"
	"math/rand"
)

const testSR = 48000

// standard cut / 200ms geometry used across tests.
const (
	testCut   = 48000
	testFadeD = 9600
	testHalfD = testFadeD / 2
	// handleMargin models extra media beyond the minimum crossfade handles.
	handleMargin = 512
)

// waveClip builds a clip whose sample at timeline coordinate t equals wave(t).
// This keeps both sides phase-coherent by construction when they share wave.
func waveClip(id ClipID, group GroupID, start, inOffset, visibleLen int64, mediaLen int, kind ClipKind, wave func(t int64) float64) Clip {
	if kind == "" {
		kind = KindAudio
	}
	c := Clip{
		ID:         id,
		Kind:       kind,
		Group:      group,
		Start:      start,
		InOffset:   inOffset,
		VisibleLen: visibleLen,
	}
	if kind == KindAudio {
		c.Samples = make([]float64, mediaLen)
		for j := range c.Samples {
			t := c.Start - c.InOffset + int64(j)
			c.Samples[j] = wave(t)
		}
	}
	return c
}

// adjacentPair returns an adjacent L/R pair at cut with enough centered
// handles for a testFadeD crossfade.
func adjacentPair(cut int64, group GroupID, waveL, waveR func(int64) float64) (Clip, Clip) {
	left := waveClip("A", group, 0, 0, cut, int(cut+testHalfD+handleMargin), KindAudio, waveL)
	rightInOffset := int64(testHalfD + handleMargin)
	right := waveClip("B", group, cut, rightInOffset, cut,
		int(rightInOffset+cut+handleMargin), KindAudio, waveR)
	return left, right
}

func sineAt(freq float64, amp float64) func(int64) float64 {
	return func(t int64) float64 {
		return amp * math.Sin(2*math.Pi*freq*float64(t)/float64(testSR))
	}
}

func anti(wave func(int64) float64) func(int64) float64 {
	return func(t int64) float64 { return -wave(t) }
}

func noiseWave(seed int64) func(int64) float64 {
	r := rand.New(rand.NewSource(seed))
	return func(int64) float64 { return r.Float64()*2 - 1 }
}

// zcPattern is an exact periodic sequence with zero crossing edges at
// timeline coordinates divisible by 4.
func zcPattern(t int64) float64 {
	pattern := [...]float64{0, 0.7071067811865475, 1, 0.7071067811865476,
		0, -0.7071067811865475, -1, -0.7071067811865476}
	r := t % 8
	if r < 0 {
		r += 8
	}
	return pattern[r]
}
