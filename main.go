package main

import (
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

// mkvtrackmap rebuilds Matroska track identity without relying on track
// order: tracks are matched by UID, CodecPrivate, language and default flag,
// then stale in-container references (tags, chapters, cues, blocks) are
// rewritten in place to the matched tracks.
func main() {
	var (
		inputPath = flag.String("input", "artifacts/demo-broken.mka", "damaged Matroska file to repair")
		baseline  = flag.String("baseline", "artifacts/roster.json", "baseline identities: roster JSON or reference Matroska file")
		outPath   = flag.String("out", "artifacts/demo-repaired.mka", "repaired Matroska output path")
		jsonPath  = flag.String("json", "artifacts/track-mapping.json", "track mapping JSON output path")
		mdPath    = flag.String("md", "artifacts/track-mapping.md", "track mapping Markdown output path")
		uidPolicy = flag.String("uid-policy", "reference", "UID repair policy: reference (rewrite stale references)")
	)
	flag.Parse()
	if *uidPolicy != "reference" {
		fatalf("unsupported uid-policy %q", *uidPolicy)
	}

	buf, err := readFile(*inputPath)
	if err != nil {
		fatalf("read input: %v", err)
	}
	inputHash := sha256.Sum256(buf)

	baselineTracks, err := loadBaseline(*baseline)
	if err != nil {
		fatalf("load baseline: %v", err)
	}
	currentTracks, err := extractTracks(buf)
	if err != nil {
		fatalf("parse input: %v", err)
	}

	matches := matchTracks(baselineTracks, currentTracks)

	refs, err := scanReferences(buf)
	if err != nil {
		fatalf("scan references: %v", err)
	}
	resolveReferences(refs, baselineTracks, matches)

	repaired := append([]byte(nil), buf...)
	if err := applyRepairs(repaired, refs); err != nil {
		fatalf("apply repairs: %v", err)
	}
	if err := writeFile(*outPath, repaired); err != nil {
		fatalf("write repaired file: %v", err)
	}

	report := buildReport(filepath.Base(*inputPath), hex.EncodeToString(inputHash[:]), *uidPolicy, matches, refs)
	jsonData, err := marshalJSON(report)
	if err != nil {
		fatalf("render json: %v", err)
	}
	if err := writeFile(*jsonPath, jsonData); err != nil {
		fatalf("write json: %v", err)
	}
	if err := writeFile(*mdPath, []byte(renderMarkdown(report))); err != nil {
		fatalf("write markdown: %v", err)
	}

	s := report.Summary
	fmt.Printf("tracks: %d mapped/%d, high confidence: %d, manual review: %d\n",
		s.Mapped, s.Tracks, s.HighConfidence, s.ManualReview)
	fmt.Printf("references: %d rewrite, %d keep, %d unresolved (patchable in place: %d)\n",
		s.RewriteReferences, s.KeepReferences, s.UnresolvedReferences, s.PatchableInPlace)
	fmt.Printf("wrote %s, %s, %s\n", *outPath, *jsonPath, *mdPath)
	if s.Problems > 0 {
		os.Exit(1)
	}
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "mkvtrackmap: "+format+"\n", args...)
	os.Exit(2)
}
