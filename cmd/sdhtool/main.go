// Command sdhtool splits SDH subtitles into semantic layers, resolves
// overlaps and produces an SDH-off plain track plus a layered ASS file.
package main

import (
	"os"

	"sdh"
)

func main() {
	os.Exit(sdh.RunCLI(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}
