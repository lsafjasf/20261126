package media

import (
	"crypto/sha256"
	"fmt"
	"math/bits"
)

// fingerprintChunk is the audio window hashed into one SimHash vote.
const fingerprintChunk = 32

// Fingerprint computes a 64-bit SimHash over raw audio samples. Identical
// audio yields identical fingerprints; unrelated audio differs in roughly
// half the bits, so Hamming similarity is a usable identity signal.
func Fingerprint(samples []byte) uint64 {
	var votes [64]int
	for off := 0; off < len(samples); off += fingerprintChunk {
		end := off + fingerprintChunk
		if end > len(samples) {
			end = len(samples)
		}
		h := sha256.Sum256(samples[off:end])
		for i := 0; i < 64; i++ {
			if h[i/8]>>(7-uint(i%8))&1 == 1 {
				votes[i]++
			} else {
				votes[i]--
			}
		}
	}
	var fp uint64
	for i := 0; i < 64; i++ {
		if votes[i] > 0 {
			fp |= 1 << uint(63-i)
		}
	}
	return fp
}

// FingerprintHex renders a fingerprint as 16 hex chars.
func FingerprintHex(fp uint64) string {
	return fmt.Sprintf("%016x", fp)
}

// ParseFingerprint decodes a hex fingerprint.
func ParseFingerprint(s string) (uint64, error) {
	var fp uint64
	_, err := fmt.Sscanf(s, "%016x", &fp)
	if err != nil || len(s) != 16 {
		return 0, fmt.Errorf("invalid audio fingerprint %q", s)
	}
	return fp, nil
}

// FingerprintSimilarity returns 1.0 for identical fingerprints and ~0.5
// for unrelated audio, based on normalised Hamming distance.
func FingerprintSimilarity(a, b uint64) float64 {
	return 1 - float64(bits.OnesCount64(a^b))/64
}
