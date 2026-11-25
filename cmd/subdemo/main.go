// Command subdemo demonstrates the fixed subtitle selector: two isolated
// browser sessions, time-base rebasing, fallback and pause/resume.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"subselect"
	"subselect/httpapi"
)

func main() {
	serveAddr := flag.String("serve", "", "serve the HTTP API on host:port (demo runs inline when empty)")
	flag.Parse()

	catalog := buildCatalog()
	selector := subselect.NewSelector(catalog, subselect.SelectorConfig{
		FallbackLanguages: []string{"en"},
	})

	if *serveAddr != "" {
		hub := subselect.NewHub(selector)
		server := httpapi.NewServer(hub)
		log.Printf("subtitle session API listening on http://%s", *serveAddr)
		log.Fatal(http.ListenAndServe(*serveAddr, server.Handler()))
	}

	runConcurrentDemo(selector)
	runPausedResumeDemo(selector)
	runFallbackAndVersionsDemo(selector)
}

func buildCatalog() *subselect.Catalog {
	cat, err := subselect.NewCatalog(
		subselect.Track{
			ID: "zh-main", Language: "zh-CN", Version: "theatrical", Default: true,
			TimeBase: subselect.TimeBase{Timescale: 1000},
			Cues: []subselect.Cue{
				{ID: "zh-1", Start: 4000, End: 7000, Text: "我们走吧"},
			},
		},
		subselect.Track{
			ID: "zh-dir", Language: "zh", Version: "director-cut",
			TimeBase: subselect.TimeBase{Timescale: 1000, Offset: 500 * time.Millisecond},
			Cues: []subselect.Cue{
				{ID: "zhd-1", Start: 4600, End: 7600, Text: "走，离开这里"},
			},
		},
		subselect.Track{
			ID: "en-main", Language: "en", Version: "theatrical",
			TimeBase: subselect.TimeBase{Timescale: 90000},
			Cues: []subselect.Cue{
				{ID: "en-1", Start: 405000, End: 675000, Text: "Let's go"},
			},
		},
	)
	if err != nil {
		log.Fatal(err)
	}
	return cat
}

func runConcurrentDemo(selector *subselect.Selector) {
	fmt.Println("== two browsers switching concurrently ==")
	clock := subselect.NewFakeClock(time.Unix(1_700_000_000, 0))
	a := subselect.NewPlayer("browser-A", selector, subselect.WithClock(clock))
	b := subselect.NewPlayer("browser-B", selector, subselect.WithClock(clock))

	var wg sync.WaitGroup
	for i, p := range []*subselect.Player{a, b} {
		wg.Add(1)
		go func(i int, p *subselect.Player) {
			defer wg.Done()
			for n := 0; n < 50; n++ {
				criteria := subselect.Criteria{Language: "zh"}
				if i == 1 {
					criteria = subselect.Criteria{Language: "en"}
				}
				if _, err := p.Switch(context.Background(), criteria); err != nil {
					log.Printf("%s switch: %v", p.ID(), err)
				}
			}
		}(i, p)
	}
	wg.Wait()

	for _, p := range []*subselect.Player{a, b} {
		v := p.Snapshot()
		cueText, startMS, endMS := "<none>", int64(-1), int64(-1)
		if v.Cue != nil {
			cueText, startMS, endMS = v.Cue.Text, v.Cue.StartMS(), v.Cue.EndMS()
		}
		trackID, language := "<off>", ""
		if v.Decision.Track != nil {
			trackID, language = v.Decision.Track.ID, v.Decision.Track.Language
		}
		fmt.Printf("%s -> track=%s lang=%s gen=%d cue=%q timing=%d..%dms\n",
			p.ID(), trackID, language, v.Generation, cueText, startMS, endMS)
	}
}

func runPausedResumeDemo(selector *subselect.Selector) {
	fmt.Println("\n== pause, switch, resume ==")
	clock := subselect.NewFakeClock(time.Unix(1_700_000_000, 0))
	p := subselect.NewPlayer("browser-A", selector, subselect.WithClock(clock))

	p.Play()
	clock.Advance(5 * time.Second)
	fmt.Printf("playing at %dms, paused at %dms\n", p.Position().Milliseconds(), p.Pause().Milliseconds())

	result, err := p.Switch(context.Background(), subselect.Criteria{Language: "en"})
	if err != nil {
		log.Fatal(err)
	}
	v := result.View
	fmt.Printf("paused switch -> %q at %d..%dms (position %dms, status %s)\n",
		v.Cue.Text, v.Cue.StartMS(), v.Cue.EndMS(), v.PositionMS(), v.Status)

	clock.Advance(time.Hour)
	fmt.Printf("after one paused hour: position %dms\n", p.Snapshot().PositionMS())

	p.Resume()
	clock.Advance(500 * time.Millisecond)
	v = p.Snapshot()
	fmt.Printf("resumed +500ms -> position %dms status %s cue=%q\n",
		v.PositionMS(), v.Status, v.Cue.Text)
}

func runFallbackAndVersionsDemo(selector *subselect.Selector) {
	fmt.Println("\n== missing-track fallback and same-language versions ==")
	for _, criteria := range []subselect.Criteria{
		{Language: "ja"},
		{Language: "zh", Version: "director-cut"},
		{Language: "zh", Version: "missing-version"},
	} {
		d := selector.Select(criteria)
		trackID := "<off>"
		if d.Track != nil {
			trackID = d.Track.ID
		}
		fmt.Printf("request=%+v -> track=%s reason=%s chain=%v variants=%v\n",
			criteria, trackID, d.Reason, d.Candidates, d.Variants)
	}
}
