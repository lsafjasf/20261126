package audioedit

import "fmt"

// Format identifies the coded audio format that supplies timing defaults.
type Format string

const (
	FormatAAC  Format = "aac"
	FormatOpus Format = "opus"
)

// CodecSpec contains the timing properties that can be derived from a format.
//
// Delay is the number of decoded samples at the start of a stream that belong
// to codec priming rather than program content. FrameSamples is the number of
// samples produced by one coded frame. Both values are expressed at the
// format's native sample rate.
type CodecSpec struct {
	NativeRate   int64
	Delay        int64
	FrameSamples int64
}

var codecSpecs = map[Format]CodecSpec{
	// AAC-LC emits 1024 PCM samples per frame. Encoders commonly use one
	// frame of priming. Containers with an explicit priming/edit declaration
	// should override this default.
	FormatAAC: {
		NativeRate:   0,
		Delay:        1024,
		FrameSamples: 1024,
	},
	// Ogg/Opus always carries decoded PCM at 48 kHz. The default pre-skip is
	// the value recommended by the Opus specification for 48 kHz streams.
	FormatOpus: {
		NativeRate:   48000,
		Delay:        312,
		FrameSamples: 960,
	},
}

// Spec returns a copy of the timing defaults for f.
func Spec(f Format) (CodecSpec, bool) {
	spec, ok := codecSpecs[f]
	return spec, ok
}

func resolveFormat(f Format) (CodecSpec, error) {
	spec, ok := codecSpecs[f]
	if !ok {
		return CodecSpec{}, fmt.Errorf("unsupported audio format %q", f)
	}
	return spec, nil
}
