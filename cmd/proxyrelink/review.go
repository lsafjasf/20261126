package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"strings"

	"proxyrelink/internal/match"
	"proxyrelink/internal/media"
)

const reviewHelp = `commands:
  list                      ranked open candidates
  best                      best candidate per unmatched proxy
  show <proxy> <original>   full evidence table for one pair
  lock <proxy> <original>   lock a pair (confirmed match)
  reject <proxy> <original> reject a pair
  clear <proxy> <original>  undo lock/reject for a pair
  locked                    list locked pairs
  save                      persist decisions
  help                      this text
  quit                      save and exit
`

func cmdReview(args []string) error {
	fs := flag.NewFlagSet("review", flag.ContinueOnError)
	indexPath := fs.String("index", "", "media index JSON (required)")
	decPath := fs.String("decisions", "", "decisions JSON (required, created if missing)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *indexPath == "" || *decPath == "" {
		return fmt.Errorf("-index and -decisions are required")
	}
	ix, err := media.LoadIndex(*indexPath)
	if err != nil {
		return err
	}
	dec, err := match.LoadDecisions(*decPath)
	if err != nil {
		return err
	}
	fmt.Printf("review session: %d proxies, %d originals; decisions -> %s\n",
		len(ix.Proxies()), len(ix.Originals()), *decPath)
	fmt.Print("type 'help' for commands\n")

	dirty := false
	sc := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("review> ")
		if !sc.Scan() {
			break
		}
		fields := strings.Fields(sc.Text())
		if len(fields) == 0 {
			continue
		}
		switch fields[0] {
		case "quit", "exit":
			if dirty {
				if err := dec.Save(*decPath); err != nil {
					return err
				}
			}
			return nil
		case "help":
			fmt.Print(reviewHelp)
		case "list":
			renderRanked(os.Stdout, match.RankedCandidates(ix, dec))
		case "best":
			best := match.BestPerProxy(match.RankedCandidates(ix, dec))
			for pid, c := range best {
				fmt.Printf("%-14s -> %-14s score=%.3f conf=%.3f verdict=%s\n",
					pid, c.OriginalID, c.Score, c.Confidence, c.Verdict)
			}
		case "show":
			if len(fields) != 3 {
				fmt.Println("usage: show <proxy> <original>")
				continue
			}
			if err := showPair(ix, fields[1], fields[2]); err != nil {
				fmt.Println("error:", err)
			}
		case "lock", "reject", "clear":
			if len(fields) != 3 {
				fmt.Printf("usage: %s <proxy> <original>\n", fields[0])
				continue
			}
			if err := applyDecision(ix, dec, fields[0], fields[1], fields[2]); err != nil {
				fmt.Println("error:", err)
				continue
			}
			dirty = true
			if err := dec.Save(*decPath); err != nil {
				return err
			}
			dirty = false
			fmt.Printf("%s recorded: %s -> %s\n", fields[0], fields[1], fields[2])
		case "locked":
			for _, d := range dec.LockedPairs() {
				fmt.Printf("%-14s -> %-14s (score %.3f, %s)\n", d.ProxyID, d.OriginalID, d.Score, d.At.Format("2006-01-02 15:04"))
			}
		case "save":
			if err := dec.Save(*decPath); err != nil {
				return err
			}
			dirty = false
			fmt.Println("saved")
		default:
			fmt.Println("unknown command; type 'help'")
		}
	}
	return sc.Err()
}
