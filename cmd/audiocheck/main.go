// Command audiocheck validates a multilingual audio package manifest
// (JSON) and exits non-zero when any consistency violation would
// block publication.
//
// Usage:
//
//	audiocheck [-sort] manifest.json
//
// With -sort the tracks are canonically reordered before validation;
// reordering never changes any track field.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"audiopkg/audiopkg"
)

func main() {
	sortFlag := flag.Bool("sort", false, "canonically reorder tracks before validating")
	flag.Parse()
	if flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: audiocheck [-sort] manifest.json")
		os.Exit(2)
	}

	data, err := os.ReadFile(flag.Arg(0))
	if err != nil {
		fmt.Fprintln(os.Stderr, "audiocheck:", err)
		os.Exit(2)
	}
	var pkg audiopkg.Package
	if err := json.Unmarshal(data, &pkg); err != nil {
		fmt.Fprintln(os.Stderr, "audiocheck: invalid manifest:", err)
		os.Exit(2)
	}

	if *sortFlag {
		pkg.SortCanonical()
	}

	if err := pkg.Publish(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("audiocheck: %q is publishable (%d tracks)\n", pkg.Name, len(pkg.Tracks))
}
