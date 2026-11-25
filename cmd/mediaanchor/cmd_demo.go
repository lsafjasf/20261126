package main

import (
	"fmt"
	"path/filepath"

	"mediaanchor/anchor"
)

func cmdDemo(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: mediaanchor demo <dir>")
	}
	dir := args[0]

	p := anchor.DemoProject()
	initialPath := filepath.Join(dir, "demo-initial.json")
	if err := p.Save(initialPath); err != nil {
		return err
	}

	result, err := p.Map()
	if err != nil {
		return err
	}
	fmt.Fprintf(outWriter, "wrote %s\n", initialPath)
	fmt.Fprintf(outWriter, "initial mapping: %d unresolved item(s)\n", len(result.Unresolved))

	if err := applyDemoDecisions(p); err != nil {
		return err
	}

	resolvedPath := filepath.Join(dir, "demo-resolved.json")
	if err := p.Save(resolvedPath); err != nil {
		return err
	}

	reloaded, err := anchor.Load(resolvedPath)
	if err != nil {
		return err
	}
	reloadedResult, err := reloaded.Map()
	if err != nil {
		return err
	}

	before, err := p.Checksum()
	if err != nil {
		return err
	}
	after, err := reloaded.Checksum()
	if err != nil {
		return err
	}
	if before != after {
		return fmt.Errorf("stability check failed: checksums differ after reload")
	}

	fmt.Fprintf(outWriter, "wrote %s\n", resolvedPath)
	fmt.Fprintf(outWriter, "after manual resolution: %d unresolved, %d stale\n",
		len(reloadedResult.Unresolved), len(reloadedResult.Stale))
	fmt.Fprintln(outWriter, "checksum before/after reload:", before)
	fmt.Fprintln(outWriter, "reopen stability: OK (identical mapping and checksum)")
	return nil
}

func applyDemoDecisions(p *anchor.Project) error {
	const sec anchor.Ticks = 1_000_000

	// The opening chapter also touches the reused 5s..10s range; keep it on
	// the original segment rather than the flashback copy.
	chapterAmbiguous := anchor.PieceKey("CH1", anchor.Anchor{
		MediaID: "media-A", Start: 5 * sec, End: 10 * sec,
	})
	if err := p.SetSegmentDecision(chapterAmbiguous, "seg-1",
		"chapter follows the original reel order"); err != nil {
		return err
	}

	// Part of the opening chapter was cut; place the surviving chapter marker
	// manually at an explicit timeline position chosen by the editor.
	chapterRemoved := anchor.PieceKey("CH1", anchor.Anchor{
		MediaID: "media-A", Start: 30 * sec, End: 35 * sec,
	})
	if err := p.SetCustomDecision(chapterRemoved, 28*sec, 30*sec,
		"editor relocated the chapter tail just before the next segment"); err != nil {
		return err
	}

	// Reused-range subtitle: keep the original placement (seg-1), not the
	// flashback copy.
	subKey := anchor.PieceKey("SUB1", anchor.Anchor{
		MediaID: "media-A", Start: 7 * sec, End: 9 * sec,
	})
	if err := p.SetSegmentDecision(subKey, "seg-1",
		"keep original placement; flashback copy is not the canonical subtitle"); err != nil {
		return err
	}

	// Removed chapter: the footage was cut, intentionally drop it.
	removedKey := anchor.PieceKey("CH2", anchor.Anchor{
		MediaID: "media-A", Start: 31 * sec, End: 38 * sec,
	})
	if err := p.SetDropDecision(removedKey, "interlude chapter removed with its footage"); err != nil {
		return err
	}

	// The forced subtitle starts inside removed footage; the missing half is
	// dropped while the surviving half keeps its mapped destination.
	forcedRemoved := anchor.PieceKey("FS1", anchor.Anchor{
		MediaID: "media-A", Start: 38 * sec, End: 40 * sec,
	})
	if err := p.SetDropDecision(forcedRemoved,
		"first half of the forced subtitle was cut with the deleted reel"); err != nil {
		return err
	}

	// Legacy events need explicit source anchors.
	if err := p.SetAnchorDecision("LEGACY1", "media-A", 12*sec, 14*sec,
		"manual recovery from legacy project file"); err != nil {
		return err
	}
	if err := p.SetAnchorDecision("LEGACY2", "media-B", 2*sec, 6*sec,
		"insert-reel chapter found during review"); err != nil {
		return err
	}
	return nil
}
