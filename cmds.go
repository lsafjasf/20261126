package main

import (
	"flag"
	"fmt"
	"os"
	"sort"
)

// resolveTimeline builds the segment axis: explicit --segs wins, otherwise
// a single segment spanning the file's actual samples at the header rate.
func resolveTimeline(info *Info, segsSpec string) (*Timeline, error) {
	var segs []Segment
	if segsSpec != "" {
		var err error
		segs, err = parseSegs(segsSpec)
		if err != nil {
			return nil, err
		}
	} else {
		segs = []Segment{{Samples: info.ActualSampleCount(), Rate: info.SampleFreq}}
	}
	return NewTimeline(segs)
}

func cmdMap(args []string) error {
	fs := flag.NewFlagSet("map", flag.ExitOnError)
	segsSpec := fs.String("segs", "", "segments as samples@rate,samples@rate (e.g. 2822400@2822400,5644800@5644800)")
	var chapSpecs strList
	fs.Var(&chapSpecs, "chap", "chapter as sampleIndex=title (repeatable)")
	fs.Parse(args)
	if fs.NArg() != 1 {
		return fmt.Errorf("usage: dsfmeta map [--segs spec] [--chap n=title]... <file.dsf>")
	}
	f, info, err := openInfo(fs.Arg(0))
	if err != nil {
		return err
	}
	defer f.Close()

	tl, err := resolveTimeline(info, *segsSpec)
	if err != nil {
		return err
	}
	chapters, err := parseChaps(chapSpecs)
	if err != nil {
		return err
	}
	sort.Slice(chapters, func(a, b int) bool { return chapters[a].Sample < chapters[b].Sample })

	total, totalN := tl.Totals()
	fmt.Printf("segments: %d, total samples %d, true duration %s\n",
		len(tl.segs), total, fmtNanos(totalN))
	for i, b := range tl.segs {
		fmt.Printf("  seg%d: %d samples @ %d Hz, starts %s\n",
			i, b.seg.Samples, b.seg.Rate, fmtNanos(b.startNanos))
	}
	for i, c := range chapters {
		start, end := tl.ChapterTime(chapters, i)
		fmt.Printf("  chap %q: sample %d -> %s .. %s\n",
			c.Title, c.Sample, fmtNanos(start), fmtNanos(end))
	}
	return nil
}

func cmdTag(args []string) error {
	fs := flag.NewFlagSet("tag", flag.ExitOnError)
	segsSpec := fs.String("segs", "", "segments as samples@rate,... (default: whole file at header rate)")
	title := fs.String("title", "", "TIT2")
	artist := fs.String("artist", "", "TPE1")
	album := fs.String("album", "", "TALB")
	track := fs.String("track", "", "TRCK")
	genre := fs.String("genre", "", "TCON")
	date := fs.String("date", "", "TDRC")
	var chapSpecs strList
	fs.Var(&chapSpecs, "chap", "chapter as sampleIndex=title (repeatable)")
	fs.Parse(args)
	if fs.NArg() != 1 {
		return fmt.Errorf("usage: dsfmeta tag [flags] <file.dsf>")
	}
	f, info, err := openInfo(fs.Arg(0))
	if err != nil {
		return err
	}
	f.Close()

	tl, err := resolveTimeline(info, *segsSpec)
	if err != nil {
		return err
	}
	chapters, err := parseChaps(chapSpecs)
	if err != nil {
		return err
	}
	sort.Slice(chapters, func(a, b int) bool { return chapters[a].Sample < chapters[b].Sample })

	tag := &Tag{
		Title: *title, Artist: *artist, Album: *album,
		Track: *track, Genre: *genre, Date: *date,
	}
	for i, c := range chapters {
		start, end := tl.ChapterTime(chapters, i)
		tag.Chapters = append(tag.Chapters, ChapterTime{
			Title:   c.Title,
			StartMs: uint32(start / 1e6),
			EndMs:   uint32(end / 1e6),
		})
	}
	blob := BuildID3(tag)
	if err := WriteMetadata(fs.Arg(0), info, blob); err != nil {
		return err
	}
	fmt.Printf("wrote %d-byte ID3v2.4 tag at offset %d; DSD payload verified unchanged\n",
		len(blob), info.DataOffset+info.DataSize)
	return nil
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: dsfmeta <info|map|tag> ...")
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "info":
		err = cmdInfo(os.Args[2:])
	case "map":
		err = cmdMap(os.Args[2:])
	case "tag":
		err = cmdTag(os.Args[2:])
	default:
		err = fmt.Errorf("unknown subcommand %q", os.Args[1])
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "dsfmeta:", err)
		os.Exit(1)
	}
}
