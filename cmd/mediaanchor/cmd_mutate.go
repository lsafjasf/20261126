package main

import (
	"fmt"

	"mediaanchor/anchor"
)

func cmdResolve(args []string) error {
	fs := newFlagSet("resolve")
	key := fs.String("key", "", "stable piece key from `unresolved`")
	segment := fs.String("segment", "", "choose this EDL segment (ambiguous fragments)")
	drop := fs.Bool("drop", false, "mark the fragment as intentionally dropped")
	start := fs.String("start", "", "custom destination start")
	end := fs.String("end", "", "custom destination end")
	reason := fs.String("reason", "", "human-readable rationale")
	if err := fs.Parse(args); err != nil {
		return err
	}
	path, err := requireSinglePath(fs.Args())
	if err != nil {
		return err
	}
	p, err := loadProject(path)
	if err != nil {
		return err
	}
	if *key == "" {
		return fmt.Errorf("--key is required")
	}

	modes := 0
	if *segment != "" {
		modes++
	}
	if *drop {
		modes++
	}
	if *start != "" || *end != "" {
		modes++
	}
	if modes != 1 {
		return fmt.Errorf("exactly one of --segment, --drop or --start/--end must be given")
	}

	switch {
	case *segment != "":
		err = p.SetSegmentDecision(*key, *segment, *reason)
	case *drop:
		err = p.SetDropDecision(*key, *reason)
	default:
		var s, e anchor.Ticks
		s, err = p.ParseTime(*start)
		if err == nil {
			e, err = p.ParseTime(*end)
		}
		if err == nil {
			err = p.SetCustomDecision(*key, s, e, *reason)
		}
	}
	if err != nil {
		return err
	}
	if err := p.Save(path); err != nil {
		return err
	}
	fmt.Fprintf(outWriter, "decision recorded for %s and saved to %s\n", *key, path)
	return reportRemaining(p)
}

func cmdAnchor(args []string) error {
	fs := newFlagSet("anchor")
	eventID := fs.String("event", "", "legacy event id")
	mediaID := fs.String("media", "", "media id to anchor against")
	start := fs.String("start", "", "source interval start")
	end := fs.String("end", "", "source interval end")
	reason := fs.String("reason", "", "human-readable rationale")
	if err := fs.Parse(args); err != nil {
		return err
	}
	path, err := requireSinglePath(fs.Args())
	if err != nil {
		return err
	}
	p, err := loadProject(path)
	if err != nil {
		return err
	}
	if *eventID == "" || *mediaID == "" {
		return fmt.Errorf("--event and --media are required")
	}
	s, err := p.ParseTime(*start)
	if err != nil {
		return fmt.Errorf("--start: %w", err)
	}
	e, err := p.ParseTime(*end)
	if err != nil {
		return fmt.Errorf("--end: %w", err)
	}
	if err := p.SetAnchorDecision(*eventID, *mediaID, s, e, *reason); err != nil {
		return err
	}
	if err := p.Save(path); err != nil {
		return err
	}
	fmt.Fprintf(outWriter, "anchor decision recorded for event %s and saved to %s\n", *eventID, path)
	return reportRemaining(p)
}

func cmdUnresolve(args []string) error {
	fs := newFlagSet("unresolve")
	key := fs.String("key", "", "piece decision key to remove")
	anchorEvent := fs.String("anchor-event", "", "event id whose manual anchor should be removed")
	if err := fs.Parse(args); err != nil {
		return err
	}
	path, err := requireSinglePath(fs.Args())
	if err != nil {
		return err
	}
	p, err := loadProject(path)
	if err != nil {
		return err
	}
	if (*key == "") == (*anchorEvent == "") {
		return fmt.Errorf("exactly one of --key or --anchor-event must be given")
	}
	if *key != "" {
		err = p.RemoveDecision(*key)
	} else {
		err = p.RemoveAnchorDecision(*anchorEvent)
	}
	if err != nil {
		return err
	}
	if err := p.Save(path); err != nil {
		return err
	}
	fmt.Fprintf(outWriter, "decision removed; saved to %s\n", path)
	return reportRemaining(p)
}

func reportRemaining(p *anchor.Project) error {
	result, err := p.Map()
	if err != nil {
		return err
	}
	fmt.Fprintf(outWriter, "remaining unresolved: %d, stale decisions: %d\n",
		len(result.Unresolved), len(result.Stale))
	for _, s := range result.Stale {
		fmt.Fprintf(outWriter, "  stale %s: %s\n", s.Key, s.Reason)
	}
	return nil
}
