package main

import (
	"encoding/json"
	"fmt"

	"mediaanchor/anchor"
)

func cmdValidate(args []string) error {
	path, err := requireSinglePath(args)
	if err != nil {
		return err
	}
	p, err := anchor.Load(path)
	if err != nil {
		return err
	}
	fmt.Fprintf(outWriter, "project %q valid: %d media, %d events, %d EDL segments\n",
		path, len(p.Media), len(p.Events), len(p.Edl))
	return nil
}

func cmdMap(args []string) error {
	path, err := requireSinglePath(args)
	if err != nil {
		return err
	}
	p, err := anchor.Load(path)
	if err != nil {
		return err
	}
	result, err := p.Map()
	if err != nil {
		return err
	}
	raw, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	fmt.Fprintln(outWriter, string(raw))

	mapped, unresolved := summarize(result)
	fmt.Fprintf(outWriter, "\nsummary: %d mapped piece(s), %d unresolved, %d stale decision(s)\n",
		mapped, unresolved, len(result.Stale))
	checksum, err := p.Checksum()
	if err != nil {
		return err
	}
	fmt.Fprintln(outWriter, "project checksum:", checksum)
	return nil
}

func cmdUnresolved(args []string) error {
	path, err := requireSinglePath(args)
	if err != nil {
		return err
	}
	p, err := anchor.Load(path)
	if err != nil {
		return err
	}
	result, err := p.Map()
	if err != nil {
		return err
	}
	if len(result.Unresolved) == 0 {
		fmt.Fprintln(outWriter, "no unresolved items; mapping is complete")
		return nil
	}
	for _, u := range result.Unresolved {
		if u.Legacy {
			fmt.Fprintf(outWriter, "[legacy]   event=%s kind=%s needs an explicit media anchor\n",
				u.EventID, u.Kind)
			continue
		}
		fmt.Fprintf(outWriter, "[%s] event=%s key=%s media=%s src=%s..%s\n",
			u.Status, u.EventID, u.Key, u.MediaID,
			p.FormatTime(u.SrcStart), p.FormatTime(u.SrcEnd))
		for _, c := range u.Candidates {
			fmt.Fprintf(outWriter, "           candidate segment=%s dst=%s..%s\n",
				c.SegmentID, p.FormatTime(c.DstStart), p.FormatTime(c.DstEnd))
		}
	}
	return nil
}

func cmdChecksum(args []string) error {
	path, err := requireSinglePath(args)
	if err != nil {
		return err
	}
	p, err := anchor.Load(path)
	if err != nil {
		return err
	}
	sum, err := p.Checksum()
	if err != nil {
		return err
	}
	fmt.Fprintln(outWriter, sum)
	return nil
}

func summarize(r *anchor.Result) (mapped, unresolved int) {
	for _, me := range r.Mapped {
		for _, piece := range me.Pieces {
			if piece.Status == anchor.StatusMapped {
				mapped++
			}
		}
	}
	return mapped, len(r.Unresolved)
}
