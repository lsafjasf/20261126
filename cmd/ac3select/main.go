package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	ac3 "ac3select"
)

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr); err != nil {
		var exitErr silentExitError
		if errors.As(err, &exitErr) {
			os.Exit(exitErr.code)
		}
		fmt.Fprintf(os.Stderr, "ac3select: %v\n", err)
		os.Exit(2)
	}
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("ac3select", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.Usage = func() {
		fmt.Fprintf(stderr, "usage: %s [options] [file.ac3]\n\n", "ac3select")
		fmt.Fprintln(stderr, "Parses AC-3 bsmod/acmod/lfeon/coupling and reports program-track selection.")
		fs.PrintDefaults()
	}

	asJSON := fs.Bool("json", false, "emit the report as indented JSON")
	showFrames := fs.Bool("frames", false, "include every parsed frame in JSON output")
	strictCRC := fs.Bool("strict-crc", false, "treat CRC-invalid frames as bad and bypass them")
	strict := fs.Bool("strict", false, "exit with status 1 when bad frames were bypassed")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}

	var data []byte
	var err error
	if fs.NArg() == 0 || fs.Arg(0) == "-" {
		data, err = io.ReadAll(stdin)
	} else if fs.NArg() == 1 {
		data, err = os.ReadFile(fs.Arg(0))
	} else {
		fs.Usage()
		return fmt.Errorf("expected at most one AC-3 file argument")
	}
	if err != nil {
		return err
	}

	report := ac3.ParseBytesWithOptions(data, ac3.Options{StrictCRC: *strictCRC})
	if !*showFrames {
		report.Frames = nil
	}

	if *asJSON {
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		encoder.SetEscapeHTML(false)
		if err := encoder.Encode(report); err != nil {
			return err
		}
	} else {
		writeTextReport(stdout, report, *showFrames)
	}

	if *strict && report.BadCount > 0 {
		return silentExitError{code: 1}
	}
	return nil
}

type silentExitError struct{ code int }

func (e silentExitError) Error() string { return fmt.Sprintf("exit %d", e.code) }

func writeTextReport(w io.Writer, report ac3.Report, showFrames bool) {
	fmt.Fprintf(w, "AC-3 scan: %d bytes, %d frames, %d bad range(s), %.3f ms\n",
		report.Bytes, report.FrameCount, report.BadCount, report.DurationMS)

	for _, segment := range report.Segments {
		chosen := ""
		if segment.Track.Chosen {
			chosen = " [CHOSEN]"
		}
		fmt.Fprintf(w, "\nprogram %d%s: offset=%d end=%d frames=%d time=%.3f-%.3f ms\n",
			segment.Index, chosen, segment.StartOffset, segment.EndOffset,
			segment.FrameCount, segment.StartMS, segment.EndMS)
		fmt.Fprintf(w, "  bsmod=%d %s (%s)\n",
			segment.BitstreamMode, segment.BitstreamModeName, segment.Track.Label)
		fmt.Fprintf(w, "  acmod=%s layout=%s lfeon=%t channels=%d (%s)\n",
			segment.ACModName, segment.LayoutShort, segment.LFEOn,
			segment.ChannelCount, segment.Layout)
		fmt.Fprintf(w, "  channel order: %s\n", channelLabels(segment.Channels))
		writeCouplingSummary(w, segment.Coupling)
		fmt.Fprintf(w, "  track: category=%s selectedAs=%s chosen=%t; %s\n",
			segment.Track.Category, segment.Track.SelectedAs, segment.Track.Chosen, segment.Track.Reason)
	}

	if len(report.BadFrames) > 0 {
		fmt.Fprintln(w, "\nbypassed bad frames:")
		for _, bad := range report.BadFrames {
			fmt.Fprintf(w, "  #%d offset=%d length=%d kind=%s: %s",
				bad.Index, bad.Offset, bad.Length, bad.Kind, bad.Reason)
			if bad.RecoveredAt > bad.Offset {
				fmt.Fprintf(w, " (resumed at %d)", bad.RecoveredAt)
			}
			fmt.Fprintln(w)
		}
	}

	fmt.Fprintln(w, "\nselection:")
	if report.Selection.Main != nil {
		fmt.Fprintf(w, "  main       -> program %d (%s, %d frames)\n",
			report.Selection.Main.SegmentIndex, report.Selection.Main.Role, report.Selection.Main.FrameCount)
	} else {
		fmt.Fprintln(w, "  main       -> not determined")
	}
	if report.Selection.Commentary != nil {
		fmt.Fprintf(w, "  commentary -> program %d (%s, %d frames)\n",
			report.Selection.Commentary.SegmentIndex, report.Selection.Commentary.Role,
			report.Selection.Commentary.FrameCount)
	} else {
		fmt.Fprintln(w, "  commentary -> no exact bsmod=5 segment")
	}
	fmt.Fprintf(w, "  annotation tracks: %d\n", len(report.Selection.AnnotationTracks))
	for _, note := range report.Selection.Notes {
		fmt.Fprintf(w, "  note: %s\n", note)
	}

	if showFrames {
		fmt.Fprintln(w, "\nframes:")
		for _, frame := range report.Frames {
			c := frame.Header.Coupling
			fmt.Fprintf(w, "  #%d offset=%d size=%d bsmod=%d acmod=%d lfe=%t cpl=%t channels=%d\n",
				frame.Index, frame.Header.Offset, frame.Header.FrameSize,
				frame.Header.BitstreamMode, frame.Header.ACMod, frame.Header.LFEOn,
				c.InUse, frame.Header.ChannelCount)
		}
	}
}

func writeCouplingSummary(w io.Writer, c ac3.CouplingSummary) {
	if c.ParsedFrames == 0 {
		fmt.Fprintln(w, "  coupling: no frame metadata")
		return
	}
	if !c.EverInUse {
		fmt.Fprintf(w, "  coupling: unused in block 0 across %d frames\n", c.ParsedFrames)
		return
	}
	fmt.Fprintf(w, "  coupling: inUse=%t inUseFrames=%d/%d always=%t channels=%v band=%d freq=%d-%dHz\n",
		c.InUse, c.InUseFrames, c.ParsedFrames, c.AlwaysInUse,
		c.CoupledChannels, c.BandCount, c.StartFrequencyHz, c.EndFrequencyHz)
}

func channelLabels(channels []ac3.Channel) string {
	out := ""
	for i, ch := range channels {
		if i > 0 {
			out += ", "
		}
		out += ch.Label
		if ch.InCoupling {
			out += "(cpl)"
		}
	}
	return out
}
