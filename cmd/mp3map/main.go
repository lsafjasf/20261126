package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"mp3timeline"
)

func main() {
	var (
		asJSON     = flag.Bool("json", false, "emit the complete mapping as JSON")
		showFrames = flag.Bool("frames", false, "include every frame in the report")
		at         = flag.Duration("at", 0, "also print a seek point for this time (e.g. 1.25s)")
	)
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "usage: %s [-json] [-frames] [-at time] file.mp3\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()
	atSet := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == "at" {
			atSet = true
		}
	})
	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(2)
	}

	timeline, err := mp3timeline.AnalyzeFile(flag.Arg(0))
	if err != nil {
		fmt.Fprintln(os.Stderr, "mp3map:", err)
		os.Exit(1)
	}
	report := timeline.Report(*showFrames)
	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(report); err != nil {
			fmt.Fprintln(os.Stderr, "mp3map:", err)
			os.Exit(1)
		}
	} else {
		printHuman(report)
	}
	if atSet {
		point, ok := timeline.SeekPointAt(*at)
		if !ok {
			fmt.Fprintln(os.Stderr, "mp3map: no seekable frame")
			os.Exit(1)
		}
		fmt.Printf("seek @ %s -> frame %d, byte %d, frame time %s\n",
			formatUS(*at),
			point.Frame, point.Offset,
			time.Duration(point.FrameTimeUS)*time.Microsecond)
	}
}

func printHuman(r mp3timeline.Report) {
	c := r.Container
	fmt.Printf("file: %s (%d bytes)\n", r.File, r.FileSize)
	fmt.Printf("format: MPEG-%s layer %d, %d Hz, %d channel(s), %d samples/frame\n",
		c.MPEGVersion, c.Layer, c.SampleRate, c.Channels, c.SamplesPerFrame)
	fmt.Printf("bytes: id3v2=%d, audio=%d..%d\n", c.ID3v2Bytes, c.AudioStart, c.AudioEnd)
	if r.Xing != nil {
		fmt.Printf("%s: %d frames, %d bytes, raw samples=%d, duration=%s\n",
			r.Xing.Kind, r.Xing.Frames, r.Xing.StreamBytes,
			r.Xing.DeclaredRawSamples, formatSamples(r.Xing.DeclaredRawSamples, r.Mapping.SampleRate))
	} else {
		fmt.Println("Xing/Info: absent")
	}
	fmt.Printf("frame scan: %d frames, raw samples=%d, duration=%s, complete=%t\n",
		r.Scan.AudioFrames, r.Scan.RawSamples,
		formatSamples(r.Scan.RawSamples, r.Mapping.SampleRate), r.Scan.Complete)
	fmt.Printf("comparison: %s, delta=%d frames (%d samples)\n",
		r.Comparison.Relation, r.Comparison.FrameDelta, r.Comparison.RawSampleDelta)
	if r.Gapless != nil {
		fmt.Printf("gapless: encoder=%q, delay=%d, tail padding=%d, presentation=%s\n",
			r.Gapless.Encoder, r.Gapless.EncoderDelaySamples,
			r.Gapless.TailPaddingSamples, formatSamples(r.Gapless.PresentationSamples, r.Mapping.SampleRate))
	} else {
		fmt.Println("gapless: no LAME/Lavf delay/padding tag")
	}
	fmt.Printf("stable mapping: authority=%s, duration=%s\n",
		r.Mapping.Authority, formatSamples(r.Mapping.PresentationSamples, r.Mapping.SampleRate))
	fmt.Println("  time->frame:", r.Mapping.TimeToFrame)
	fmt.Println("  frame->time:", r.Mapping.FrameToTime)
	for _, warning := range r.Warnings {
		fmt.Println("warning:", warning)
	}
	if len(r.Frames) > 0 {
		fmt.Println("frame,offset,size,bitrate,start_us")
		for _, f := range r.Frames {
			fmt.Printf("%d,%d,%d,%d,%d\n",
				f.Number, f.Offset, f.Size, f.Bitrate, f.StartUS)
		}
	}
}

func formatSamples(samples int64, sampleRate int) string {
	if sampleRate <= 0 || samples <= 0 {
		return "0s"
	}
	d := time.Duration(samples/int64(sampleRate))*time.Second +
		time.Duration(samples%int64(sampleRate))*time.Second/time.Duration(sampleRate)
	return formatUS(d)
}

func formatUS(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	hours := int64(d / time.Hour)
	d -= time.Duration(hours) * time.Hour
	minutes := int64(d / time.Minute)
	d -= time.Duration(minutes) * time.Minute
	seconds := float64(d) / float64(time.Second)
	if hours > 0 {
		return fmt.Sprintf("%02d:%02d:%06.3f", hours, minutes, seconds)
	}
	return fmt.Sprintf("%02d:%06.3f", minutes, seconds)
}
