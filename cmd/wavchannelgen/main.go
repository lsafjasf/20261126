package main

import (
	"flag"
	"fmt"
	"os"

	"channelmap"
)

func main() {
	output := flag.String("output", "sample.wav", "output WAV path")
	scenario := flag.String("scenario", "swapped", "correct | swapped | ambiguous")
	rate := flag.Int("rate", 48000, "sample rate")
	duration := flag.Float64("duration", 0.35, "tone duration in seconds")
	bits := flag.Int("bits", 16, "PCM bit depth: 16, 24, or 32")
	float32 := flag.Bool("float32", false, "write 32-bit IEEE float instead of integer PCM")
	flag.Parse()

	channels := channelmap.Standard51ToneChannels(*duration, *rate)
	switch *scenario {
	case "correct":
	case "swapped":
		channels = channelmap.SwapCenterBackLeft(channels)
	case "ambiguous":
		channels = channelmap.AmbiguousCenterSurroundChannels(*duration, *rate)
	default:
		fmt.Fprintf(os.Stderr, "unknown scenario %q\n", *scenario)
		os.Exit(64)
	}

	wav, err := channelmap.EncodeWAV(channels, channelmap.EncodeWAVOptions{
		SampleRate:    *rate,
		BitsPerSample: *bits,
		IEEEFloat:     *float32,
		ChannelMask:   channelmap.Mask5Point1,
		Extensible:    true,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "encode WAV: %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile(*output, wav, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "write %s: %v\n", *output, err)
		os.Exit(1)
	}
	fmt.Printf("wrote %s scenario=%s bytes=%d\n", *output, *scenario, len(wav))
}
