// Command proxyrelink scores proxy/original video pairs across multiple
// evidence dimensions, lets the user lock or reject candidates, and
// relinks timelines without moving any cut point. All computation is Go.
package main

import (
	"fmt"
	"os"
)

const usage = `proxyrelink - multi-dimensional proxy/original matcher

Usage:
  proxyrelink score   -index media.json [-decisions d.json] [-proxy ID] [-json]
  proxyrelink review  -index media.json -decisions d.json
  proxyrelink relink  -index media.json -decisions d.json -timeline t.json [-tolerance N] [-json]

Commands:
  score   Rank candidates and print per-dimension evidence + confidence.
  review  Interactive session to lock / reject / clear candidate pairs.
  relink  Replace locked proxies with originals; cut points never move.
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "score":
		err = cmdScore(os.Args[2:])
	case "review":
		err = cmdReview(os.Args[2:])
	case "relink":
		err = cmdRelink(os.Args[2:])
	case "-h", "--help", "help":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n%s", os.Args[1], usage)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
