// Command mediaanchor maps media-anchored chapters and subtitles onto a
// re-cut timeline. It only depends on the Go standard library.
package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "help", "-h", "--help":
		usage()
		return
	}
	err := dispatch(os.Args[1:])
	if err != nil {
		fmt.Fprintln(errWriter, "error:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(errWriter, `mediaanchor - media-anchored event mapping for re-cut films

Usage:
  mediaanchor <command> [flags]

Commands:
  validate     <project.json>
  map          <project.json>
  unresolved   <project.json>
  resolve      <project.json> --key <piece-key> (--segment <id> | --drop | --start <t> --end <t>) [--reason <text>]
  anchor       <project.json> --event <id> --media <id> --start <t> --end <t> [--reason <text>]
  unresolve    <project.json> (--key <piece-key> | --anchor-event <id>)
  checksum     <project.json>
  demo         <dir>

Timestamps are integer ticks by default; "12s", "250ms", "1500us" and
"HH:MM:SS.mmm" are converted using the project's ticks_per_second.
`)
}
