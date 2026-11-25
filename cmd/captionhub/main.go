package main

import (
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"text/tabwriter"

	"captionhub"
)

func main() {
	format := flag.String("format", "json", "output format: json or table")
	outPath := flag.String("out", "", "optional output file (default: stdout)")
	serveAddr := flag.String("serve", "", "optional HTTP listen address, e.g. 127.0.0.1:8080")
	flag.Parse()

	hub := buildScenario()
	if *serveAddr != "" {
		fmt.Fprintf(os.Stderr, "captionhub state listening on http://%s/connections\n", *serveAddr)
		if err := http.ListenAndServe(*serveAddr, hub.Handler()); err != nil {
			fatal(err)
		}
		return
	}
	var out io.Writer = os.Stdout
	if *outPath != "" {
		file, err := os.Create(*outPath)
		if err != nil {
			fatal(err)
		}
		defer file.Close()
		out = file
	}

	switch *format {
	case "json":
		if err := hub.WriteJSON(out); err != nil {
			fatal(err)
		}
	case "table":
		if err := writeTable(out, hub.States()); err != nil {
			fatal(err)
		}
	default:
		fatal(fmt.Errorf("unsupported format %q", *format))
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "captionhub:", err)
	os.Exit(1)
}

func buildScenario() *captionhub.Hub {
	hub := captionhub.NewHub(captionhub.WithIDPrefix("live"))

	news, err := hub.SwitchProgram("NEWS", "news-2026-11-26-a")
	if err != nil {
		fatal(err)
	}
	if _, err := hub.AddCue(captionhub.Cue{
		ID: "news-welcome", SessionID: news.SessionID, Epoch: news.Epoch,
		StartMS: 2_000, EndMS: 8_000, Text: "欢迎收看晚间新闻",
	}); err != nil {
		fatal(err)
	}
	if _, err := hub.ConnectAt("conn-live", 3_000); err != nil {
		fatal(err)
	}
	if _, err := hub.ConnectAt("conn-late", 5_000); err != nil {
		fatal(err)
	}

	drama, err := hub.SwitchProgram("DRAMA", "drama-ep42")
	if err != nil {
		fatal(err)
	}
	if _, err := hub.AddCue(captionhub.Cue{
		ID: "drama-opening", SessionID: drama.SessionID, Epoch: drama.Epoch,
		StartMS: 0, EndMS: 5_000, Text: "第四十二集",
	}); err != nil {
		fatal(err)
	}
	if _, err := hub.ReportPosition("conn-live", 1_000); err != nil {
		fatal(err)
	}
	if _, err := hub.ReportPosition("conn-late", 2_000); err != nil {
		fatal(err)
	}

	afterBreak, err := hub.Discontinuity("EXT-X-DISCONTINUITY")
	if err != nil {
		fatal(err)
	}
	if _, err := hub.AddCue(captionhub.Cue{
		ID: "signal-restored", SessionID: afterBreak.SessionID, Epoch: afterBreak.Epoch,
		StartMS: 1_000, EndMS: 4_000, Text: "直播信号已恢复",
	}); err != nil {
		fatal(err)
	}
	if _, err := hub.ReportPosition("conn-live", 2_000); err != nil {
		fatal(err)
	}

	if _, err := hub.RecoverPosition("conn-replay", captionhub.Checkpoint{
		SessionID:   news.SessionID,
		Epoch:       news.Epoch,
		Mode:        captionhub.ModeReplay,
		MediaTimeMS: 3_000,
	}); err != nil {
		fatal(err)
	}

	// The error is intentional in the demo: it records a resync-required state.
	_, _ = hub.RecoverPosition("conn-reconnect", captionhub.Checkpoint{
		SessionID:   "live-expired",
		Epoch:       9,
		Mode:        captionhub.ModeReplay,
		MediaTimeMS: 12_000,
	})

	return hub
}

func writeTable(w io.Writer, states []captionhub.ConnectionState) error {
	tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
	if _, err := fmt.Fprintln(tw, "CONNECTION\tMODE\tSTATUS\tTIMELINE\tTIME\tCUE\tREASON"); err != nil {
		return err
	}
	for _, state := range states {
		time := "-"
		if state.MediaTimeMS != nil {
			time = state.MediaTimeMS.String()
		}
		cue := "-"
		if state.ActiveCue != nil {
			cue = state.ActiveCue.ID + ": " + state.ActiveCue.Text
		}
		timeline := state.TimelineID
		if timeline == "" {
			timeline = "-"
		}
		if _, err := fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			state.ConnectionID, state.Mode, state.Status, timeline, time, cue, state.Reason,
		); err != nil {
			return err
		}
	}
	return tw.Flush()
}
