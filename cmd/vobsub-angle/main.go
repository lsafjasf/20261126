// vobsub-angle selects a VobSub subtitle stream by angle (context number)
// and prints the cue active at a given playback position.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"vobsub-angle/vobsub"
)

func main() {
	idxPath := flag.String("idx", "", "path to the .idx file (required)")
	subPath := flag.String("sub", "", "path to the .sub file (optional; enables offset validation)")
	angle := flag.Int("angle", 0, "angle / context number to select")
	at := flag.String("at", "0", "playback position as hh:mm:ss:ms or seconds")
	flag.Parse()

	if *idxPath == "" {
		fmt.Fprintln(os.Stderr, "error: -idx is required")
		os.Exit(2)
	}
	position, err := parsePosition(*at)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: invalid -at value: %v\n", err)
		os.Exit(2)
	}

	idxFile, err := os.Open(*idxPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: cannot open idx: %v\n", err)
		os.Exit(1)
	}
	defer idxFile.Close()
	idx, err := vobsub.ParseIDX(idxFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: cannot parse idx: %v\n", err)
		os.Exit(1)
	}

	var sub *os.File
	if *subPath != "" {
		sub, err = os.Open(*subPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: cannot open sub: %v\n", err)
			os.Exit(1)
		}
		defer sub.Close()
	}

	sw := vobsub.NewSwitcher(idx, sub)
	entry, err := sw.SwitchAngle(*angle, position)
	if err != nil {
		var missing *vobsub.MissingAngleError
		if errors.As(err, &missing) {
			fmt.Fprintf(os.Stderr, "error: %v\n", missing)
			fmt.Fprintln(os.Stderr, "hint: choose one of the declared angles listed above; no subtitles were rendered")
			os.Exit(3)
		}
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	st := idx.Streams[*angle]
	fmt.Printf("angle %d (%s): cue at %s, filepos %#x\n",
		*angle, st.Lang, entry.Timestamp, entry.FilePos)
}

func parsePosition(s string) (time.Duration, error) {
	var h, m, sec, ms int
	if n, err := fmt.Sscanf(s, "%d:%d:%d:%d", &h, &m, &sec, &ms); err == nil && n == 4 {
		return time.Duration(h)*time.Hour + time.Duration(m)*time.Minute +
			time.Duration(sec)*time.Second + time.Duration(ms)*time.Millisecond, nil
	}
	var secs float64
	if _, err := fmt.Sscanf(s, "%g", &secs); err == nil {
		return time.Duration(secs * float64(time.Second)), nil
	}
	return 0, fmt.Errorf("cannot parse %q", s)
}
