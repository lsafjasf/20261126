package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
)

func fmtNanos(n uint64) string {
	ms := n / 1e6
	return fmt.Sprintf("%02d:%02d:%02d.%03d",
		ms/3600000, ms/60000%60, ms/1000%60, ms%1000)
}

func parseSegs(spec string) ([]Segment, error) {
	var segs []Segment
	for _, part := range strings.Split(spec, ",") {
		samples, rate, ok := strings.Cut(part, "@")
		if !ok {
			return nil, fmt.Errorf("segment %q: want samples@rate", part)
		}
		n, err := strconv.ParseUint(samples, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("segment %q: %v", part, err)
		}
		r, err := strconv.ParseUint(rate, 10, 32)
		if err != nil {
			return nil, fmt.Errorf("segment %q: %v", part, err)
		}
		segs = append(segs, Segment{Samples: n, Rate: uint32(r)})
	}
	return segs, nil
}

func parseChaps(specs []string) ([]Chapter, error) {
	var ch []Chapter
	for _, s := range specs {
		pos, title, ok := strings.Cut(s, "=")
		if !ok {
			return nil, fmt.Errorf("chapter %q: want sampleIndex=title", s)
		}
		n, err := strconv.ParseUint(pos, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("chapter %q: %v", s, err)
		}
		ch = append(ch, Chapter{Title: title, Sample: n})
	}
	return ch, nil
}

type strList []string

func (s *strList) String() string { return strings.Join(*s, ",") }
func (s *strList) Set(v string) error {
	*s = append(*s, v)
	return nil
}

func openInfo(path string) (*os.File, *Info, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, nil, err
	}
	info, err := Parse(f, st.Size())
	if err != nil {
		f.Close()
		return nil, nil, err
	}
	return f, info, nil
}

func cmdInfo(args []string) error {
	fs := flag.NewFlagSet("info", flag.ExitOnError)
	fs.Parse(args)
	if fs.NArg() != 1 {
		return fmt.Errorf("usage: dsfmeta info <file.dsf>")
	}
	f, info, err := openInfo(fs.Arg(0))
	if err != nil {
		return err
	}
	defer f.Close()

	l := info.Layout()
	fmt.Printf("file size        : %d (header declares %d)\n", info.RealFileSize, info.DeclaredFileSize)
	fmt.Printf("channels         : %d (type %d)\n", info.ChannelNum, info.ChannelType)
	fmt.Printf("sample rate      : %d Hz (DSD%d)\n", info.SampleFreq, info.SampleFreq/44100)
	fmt.Printf("bits per sample  : %d\n", info.BitsPerSample)
	fmt.Printf("block/channel    : %d bytes\n", info.BlockSizePerChannel)
	fmt.Printf("data payload     : %d bytes @ offset %d\n", info.DataSize, info.DataOffset)
	fmt.Printf("blocks           : %d full + %d tail bytes (%d/ch, %d stray)\n",
		l.FullBlocks, l.RemainderBytes, l.PartialPerChannel, l.StrayBytes)
	fmt.Printf("samples (header) : %d -> %s\n", info.SampleCount, fmtNanos(info.DurationNanos()))
	fmt.Printf("samples (actual) : %d -> %s\n", info.ActualSampleCount(), fmtNanos(info.ActualDurationNanos()))
	fmt.Printf("naive PCM @176.4k: %s  (%.2fx stretch)\n",
		fmtNanos(NaiveNanos(info.ActualSampleCount(), 176400)),
		float64(NaiveNanos(info.ActualSampleCount(), 176400))/float64(max(info.ActualDurationNanos(), 1)))
	fmt.Printf("metadata offset  : %d\n", info.MetadataOffset)
	return nil
}
