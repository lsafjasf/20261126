package main

import (
	"fmt"
	"strings"
)

// elementNames maps EBML IDs to Matroska path segments.
var elementNames = map[uint32]string{
	idSegment: "Segment", idTracks: "Tracks", idTrackEntry: "TrackEntry",
	idTags: "Tags", idTag: "Tag", idTargets: "Targets", idTagTrackUID: "TagTrackUID",
	idChapters: "Chapters", idEditionEntry: "EditionEntry", idChapterAtom: "ChapterAtom",
	idChapterTrack: "ChapterTrack", idChapterTrackUID: "ChapterTrackUID",
	idCues: "Cues", idCuePoint: "CuePoint", idCueTrackPos: "CueTrackPositions",
	idCueTrack: "CueTrack", idCluster: "Cluster", idSimpleBlock: "SimpleBlock",
	idBlockGroup: "BlockGroup", idBlock: "Block",
}

func elementPath(path []uint32, id uint32) string {
	parts := make([]string, 0, len(path)+1)
	for _, ancestor := range path {
		parts = append(parts, elementNames[ancestor])
	}
	return strings.Join(append(parts, elementNames[id]), "/")
}

// Reference is one in-container pointer to a track, found by walking the
// Segment. Scheme is either "uid" (TagTrackUID, ChapterTrackUID) or
// "number" (CueTrack, block track numbers).
type Reference struct {
	Kind         string `json:"kind"`
	Scheme       string `json:"scheme"`
	Value        uint64 `json:"value"`
	Path         string `json:"path"`
	ElementOff   int64  `json:"elementOffset"`
	ValueOff     int64  `json:"valueOffset"`
	ValueLen     int    `json:"valueLength"`
	Vint         bool   `json:"vint,omitempty"`
	Action       string `json:"action"`
	TargetUID    uint64 `json:"targetTrackUid"`
	TargetNumber uint64 `json:"targetTrackNumber"`
	Patchable    bool   `json:"patchableInPlace"`
	Confidence   string `json:"confidence"`
	Reason       string `json:"reason"`
}

// scanReferences locates every in-container track reference.
func scanReferences(buf []byte) ([]Reference, error) {
	var refs []Reference
	err := walkElements(buf, 0, int64(len(buf)), nil, func(el Element, path []uint32) bool {
		parent := uint32(0)
		if len(path) > 0 {
			parent = path[len(path)-1]
		}
		switch {
		case el.ID == idTagTrackUID && parent == idTargets:
			refs = append(refs, Reference{
				Kind: "tag-track-uid", Scheme: "uid",
				Value: uintValue(buf, el), Path: elementPath(path, el.ID),
				ElementOff: el.HeaderOffset, ValueOff: el.DataOffset, ValueLen: int(el.DataSize),
			})
		case el.ID == idChapterTrackUID && parent == idChapterTrack:
			refs = append(refs, Reference{
				Kind: "chapter-track-uid", Scheme: "uid",
				Value: uintValue(buf, el), Path: elementPath(path, el.ID),
				ElementOff: el.HeaderOffset, ValueOff: el.DataOffset, ValueLen: int(el.DataSize),
			})
		case el.ID == idCueTrack && parent == idCueTrackPos:
			refs = append(refs, Reference{
				Kind: "cue-track", Scheme: "number",
				Value: uintValue(buf, el), Path: elementPath(path, el.ID),
				ElementOff: el.HeaderOffset, ValueOff: el.DataOffset, ValueLen: int(el.DataSize),
			})
		case (el.ID == idSimpleBlock && parent == idCluster) || (el.ID == idBlock && parent == idBlockGroup):
			track, width, err := readVINT(buf, el.DataOffset, true)
			if err != nil {
				return true
			}
			kind := "simple-block-track"
			if el.ID == idBlock {
				kind = "block-track"
			}
			refs = append(refs, Reference{
				Kind: kind, Scheme: "number",
				Value: track, Path: elementPath(path, el.ID),
				ElementOff: el.HeaderOffset, ValueOff: el.DataOffset, ValueLen: width,
				Vint: true,
			})
		}
		return true
	})
	if err != nil {
		return nil, fmt.Errorf("scan references: %w", err)
	}
	return refs, nil
}

// resolveReferences re-targets every reference through the identity mapping.
// uidPolicy "reference" rewrites stale references to the matched track's
// current UID/number; the TrackEntry headers themselves are never touched.
func resolveReferences(refs []Reference, baseline []Track, matches []MatchResult) {
	byUID := make(map[uint64]int)
	byNumber := make(map[uint64]int)
	for i, t := range baseline {
		byUID[t.TrackUID] = i
		byNumber[t.TrackNumber] = i
	}
	for i := range refs {
		ref := &refs[i]
		var baselineIndex int
		var ok bool
		if ref.Scheme == "uid" {
			baselineIndex, ok = byUID[ref.Value]
		} else {
			baselineIndex, ok = byNumber[ref.Value]
		}
		if !ok || matches[baselineIndex].Current == nil {
			ref.Action = "unresolved"
			ref.Confidence = "none"
			ref.Reason = "no baseline track matches this reference"
			continue
		}
		current := matches[baselineIndex].Current
		ref.TargetUID = current.TrackUID
		ref.TargetNumber = current.TrackNumber
		ref.Confidence = matches[baselineIndex].Confidence

		var currentValue uint64
		if ref.Scheme == "uid" {
			currentValue = current.TrackUID
		} else {
			currentValue = current.TrackNumber
		}
		if ref.Value == currentValue {
			ref.Action = "keep"
			ref.Patchable = true
			ref.Reason = "reference already points at the matched track"
			continue
		}
		ref.Action = "rewrite"
		ref.Reason = "rewrite stale reference to order-independent matched track"
		if ref.Scheme == "uid" {
			ref.Patchable = ref.ValueLen == 8
		} else if ref.Vint {
			ref.Patchable = ref.ValueLen == 1 && currentValue <= 0x7F
		} else {
			ref.Patchable = ref.ValueLen <= 8 && currentValue < uint64(1)<<(uint(ref.ValueLen)*8)
		}
	}
}

// applyRepairs patches every rewrite-action reference in place. All rewrites
// keep the original byte length, so the file layout never shifts.
func applyRepairs(buf []byte, refs []Reference) error {
	for _, ref := range refs {
		if ref.Action != "rewrite" {
			continue
		}
		if !ref.Patchable {
			return fmt.Errorf("%s at offset %d is not patchable in place", ref.Kind, ref.ElementOff)
		}
		var err error
		if ref.Scheme == "uid" {
			err = writeUint(buf, ref.ValueOff, ref.ValueLen, ref.TargetUID)
		} else if ref.Vint {
			err = writeVINTSameWidth(buf, ref.ValueOff, ref.ValueLen, ref.TargetNumber)
		} else {
			err = writeUint(buf, ref.ValueOff, ref.ValueLen, ref.TargetNumber)
		}
		if err != nil {
			return fmt.Errorf("patch %s at offset %d: %w", ref.Kind, ref.ElementOff, err)
		}
	}
	return nil
}
