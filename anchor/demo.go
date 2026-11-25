package anchor

// DemoProject builds a deterministic scenario that exercises every mapping
// branch on a 100-second source film (timebase: 1 000 000 ticks/second).
//
// The re-cut removes a middle block (30s..40s), reorders parts of the film and
// reuses one source range twice (a flashback). Events include:
//   - a chapter cut cleanly in two by an edit boundary,
//   - a forced subtitle spanning removed footage,
//   - an ordinary subtitle landing in a reused (ambiguous) range,
//   - another forced subtitle split across a cut,
//   - a legacy event without an anchor (multi-media project => human choice),
//   - a second legacy event in the only media covering its age, to demonstrate
//     automatic single-media anchoring guidance.
func DemoProject() *Project {
	const sec Ticks = 1_000_000

	mediaA := Media{ID: "media-A", Path: "feature/full-reel.mov", Duration: 100 * sec}
	mediaB := Media{ID: "media-B", Path: "feature/insert-reel.mov", Duration: 20 * sec}

	segments := []Segment{
		// Source 0..30s moves to the timeline start.
		{ID: "seg-1", MediaID: "media-A", SrcStart: 0, SrcEnd: 30 * sec, DstStart: 0},
		// Source 30..40s is deleted (no segment).
		// Source 40..70s moves earlier, leaving a visible destination gap.
		{ID: "seg-2", MediaID: "media-A", SrcStart: 40 * sec, SrcEnd: 50 * sec, DstStart: 30 * sec},
		{ID: "seg-3", MediaID: "media-A", SrcStart: 50 * sec, SrcEnd: 70 * sec, DstStart: 40 * sec},
		// Flashback: source 5s..10s is reused a second time.
		{ID: "seg-4", MediaID: "media-A", SrcStart: 5 * sec, SrcEnd: 10 * sec, DstStart: 80 * sec},
		{ID: "seg-5", MediaID: "media-A", SrcStart: 70 * sec, SrcEnd: 90 * sec, DstStart: 85 * sec},
		{ID: "seg-6", MediaID: "media-B", SrcStart: 0, SrcEnd: 10 * sec, DstStart: 105 * sec},
		{ID: "seg-7", MediaID: "media-A", SrcStart: 90 * sec, SrcEnd: 100 * sec, DstStart: 115 * sec},
	}

	events := []Event{
		// Chapter straddles the first cut: two mapped pieces on seg-1/seg-2.
		{ID: "CH1", Kind: KindChapter, Text: "Opening",
			Anchor: &Anchor{MediaID: "media-A", Start: 5 * sec, End: 35 * sec}},
		// Chapter fully inside removed block 30s..40s => removed.
		{ID: "CH2", Kind: KindChapter, Text: "Lost Interlude",
			Anchor: &Anchor{MediaID: "media-A", Start: 31 * sec, End: 38 * sec}},
		// Forced subtitle crosses the 40s cut => split, both pieces mapped.
		{ID: "FS1", Kind: KindForced, Text: "Door slams",
			Anchor: &Anchor{MediaID: "media-A", Start: 38 * sec, End: 42 * sec}},
		// Ordinary subtitle inside the reused range => ambiguous (seg-1/seg-4).
		{ID: "SUB1", Kind: KindSubtitle, Text: "Long ago...",
			Anchor: &Anchor{MediaID: "media-A", Start: 7 * sec, End: 9 * sec}},
		// Forced subtitle split across the seg-5/seg-7 cut at 90s.
		{ID: "FS2", Kind: KindForced, Text: "The end begins",
			Anchor: &Anchor{MediaID: "media-A", Start: 88 * sec, End: 93 * sec}},
		// Legacy events: no anchor at all.
		{ID: "LEGACY1", Kind: KindSubtitle, Text: "Unanchored caption"},
		{ID: "LEGACY2", Kind: KindChapter, Text: "Unanchored chapter"},
	}

	return &Project{
		Version:        1,
		TicksPerSecond: 1_000_000,
		Media:          []Media{mediaA, mediaB},
		Events:         events,
		Edl:            segments,
	}
}
