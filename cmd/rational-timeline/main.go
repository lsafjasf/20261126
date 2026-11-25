package main

import (
	"flag"
	"fmt"
	"os"

	"rational-timeline/internal/timeline"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "rational-timeline:", err)
		os.Exit(1)
	}
}

func run(arguments []string) error {
	flags := flag.NewFlagSet("rational-timeline", flag.ContinueOnError)
	inputPath := flags.String("input", "", "JSON timeline input; defaults to the built-in mixed fps demo")
	jsonPath := flags.String("json", "", "JSON report output path; '-' or empty writes to stdout")
	markdownPath := flags.String("markdown", "", "Markdown report output path; '-' writes to stdout")
	if err := flags.Parse(arguments); err != nil {
		return err
	}

	var model timeline.Timeline
	var err error
	if *inputPath == "" {
		model = timeline.DemoTimeline()
	} else {
		model, err = timeline.ReadInput(*inputPath)
		if err != nil {
			return err
		}
	}
	report, err := timeline.BuildReport(model)
	if err != nil {
		return err
	}
	if *jsonPath != "" {
		if err := timeline.WriteJSON(report, *jsonPath); err != nil {
			return err
		}
	}
	if *markdownPath != "" {
		if err := timeline.WriteMarkdown(timeline.RenderMarkdown(report), *markdownPath); err != nil {
			return err
		}
	}
	if *jsonPath == "" && *markdownPath == "" {
		return timeline.WriteJSON(report, "-")
	}
	return nil
}
