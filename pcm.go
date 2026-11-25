// Package pcmconv converts packed PCM data between planar and interleaved
// layouts without relying on fixed-size blocks.
package pcmconv

import (
	"errors"
	"fmt"
)

var (
	// ErrInvalidChannels means that Channels is not positive.
	ErrInvalidChannels = errors.New("pcmconv: channels must be positive")
	// ErrInvalidSampleBytes means that BytesPerSample is not positive.
	ErrInvalidSampleBytes = errors.New("pcmconv: bytes per sample must be positive")
	// ErrChannelCount means that the number of input slices or channel counts
	// does not match Format.Channels.
	ErrChannelCount = errors.New("pcmconv: channel count does not match format")
	// ErrPartialSample means that a byte stream ends in the middle of a sample.
	ErrPartialSample = errors.New("pcmconv: data ends with a partial sample")
	// ErrInconsistentChannels means that planar channels cannot form valid
	// frames, even when the final frame is partial.
	ErrInconsistentChannels = errors.New("pcmconv: inconsistent planar channel lengths")
	// ErrCountsMismatch means that supplied per-channel counts do not describe
	// the supplied data.
	ErrCountsMismatch = errors.New("pcmconv: channel sample counts do not match data")
)

// Format describes a packed PCM stream. A frame contains one sample for each
// channel, with each sample occupying BytesPerSample bytes. The final frame may
// contain fewer than Channels samples.
type Format struct {
	Channels       int
	BytesPerSample int
}

// Validate reports whether the format can describe a PCM stream.
func (f Format) Validate() error {
	if f.Channels <= 0 {
		return fmt.Errorf("%w: got %d", ErrInvalidChannels, f.Channels)
	}
	if f.BytesPerSample <= 0 {
		return fmt.Errorf("%w: got %d", ErrInvalidSampleBytes, f.BytesPerSample)
	}
	if f.Channels > maxInt()/f.BytesPerSample {
		return fmt.Errorf("pcmconv: frame size overflows int")
	}
	return nil
}

// FrameBytes returns the size of a complete frame in bytes. It returns zero
// for an invalid format.
func (f Format) FrameBytes() int {
	if f.Validate() != nil {
		return 0
	}
	return f.Channels * f.BytesPerSample
}

// Planar contains one byte slice per channel. A channel slice must contain
// whole samples. Counts is populated by conversion functions and records the
// number of samples in each channel, including samples in a partial final
// frame.
type Planar struct {
	Format   Format
	Channels [][]byte
	Counts   []int
}

// Interleaved contains frame-major PCM bytes. The final frame may be partial.
// Counts is populated by conversion functions and records how many samples
// each channel contributed.
type Interleaved struct {
	Format Format
	Data   []byte
	Counts []int
}

func maxInt() int {
	return int(^uint(0) >> 1)
}
