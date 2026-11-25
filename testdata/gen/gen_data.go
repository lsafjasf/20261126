package main

import (
	"os"
	"path/filepath"

	"edlrelink/internal/edl"
)

// generate lays out every scenario exercised by the resolver tests.
func generate() error {
	// ---- Old offline library (project version 3) -----------------------
	oldSpecs := []spec{
		{"c01", "media/intro.mov", "intro-v1"},
		{"c02", "media/interview_a.mov", "interview-a"},
		{"c03", "media/broll_city.mp4", "broll-city"},
		{"c04", "media/product_shot.mov", "product"},
		{"c05", "media/ambient_park.mov", "park"},
		{"c06", "media/music_bed.wav", "music-bed"},
		{"c07", "media/ending_logo.mov", "logo"},
		{"c08", "media/backup_take.mov", "backup-take"},
	}
	oldAssets := make([]edl.Asset, len(oldSpecs))
	for i, s := range oldSpecs {
		oldAssets[i] = asset(s)
	}
	offline := edl.Library{
		SchemaVersion: "v3-offline",
		ProjectID:     "promo-2026",
		Assets:        oldAssets,
	}
	if err := writeMedia("offline_media", oldSpecs); err != nil {
		return err
	}

	// ---- Current online library (project version 7) --------------------
	// c01: unchanged on both identity dimensions.
	// c02: content moved to clip c12 (ID changed); old ID c02 reused by
	//      brand-new content -> id_reused conflict, needs confirmation.
	// c03: content renamed/moved to footage/broll_city_4k.mp4 under c13,
	//      while a different asset c13b reuses the old file name
	//      media/broll_city.mp4 -> file-name collision, needs confirmation.
	// c04: clean hash relocation to c14, no conflicting dimensions.
	// c05: deleted outright (its hash appears nowhere online).
	// c06: content duplicated on c16a and c16b -> ambiguous, needs review.
	// c07: deleted outright; a same-name decoy c17 with different content
	//      remains, which must not be treated as a match.
	// c08: unchanged, used for the offline-inconsistency sanity op only.
	newSpecs := []spec{
		{"c01", "media/intro.mov", "intro-v1"},
		{"c02", "media/interview_a_retake.mov", "interview-a-retake"},
		{"c12", "media/interview_a_archive.mov", "interview-a"},
		{"c13", "footage/broll_city_4k.mp4", "broll-city"},
		{"c13b", "media/broll_city.mp4", "broll-city-regrade"},
		{"c14", "footage/product_shot_graded.mov", "product"},
		{"c16a", "audio/music_bed.wav", "music-bed"},
		{"c16b", "audio/music_bed_copy.wav", "music-bed"},
		{"c17", "media/ending_logo.mov", "logo-redesign"},
		{"c08", "media/backup_take.mov", "backup-take"},
	}
	newAssets := make([]edl.Asset, len(newSpecs))
	for i, s := range newSpecs {
		newAssets[i] = asset(s)
	}
	online := edl.Library{
		SchemaVersion: "v7-online",
		ProjectID:     "promo-2026",
		Assets:        newAssets,
	}
	if err := writeMedia("online_media", newSpecs); err != nil {
		return err
	}

	// ---- Offline change list based on the old v3 library ---------------
	find := func(id string) edl.Asset {
		for _, a := range oldAssets {
			if a.ClipID == id {
				return a
			}
		}
		panic("missing old asset " + id)
	}
	op := func(seq int, action, id string, extra func(*edl.Op)) edl.Op {
		a := find(id)
		o := edl.Op{
			Seq:       seq,
			Action:    action,
			OldClipID: a.ClipID,
			MediaHash: a.MediaHash,
			OldPath:   a.Path,
		}
		if extra != nil {
			extra(&o)
		}
		return o
	}
	cl := edl.ChangeList{
		SchemaVersion: "v3-offline",
		ProjectID:     "promo-2026",
		BasedOn:       "v3-offline",
		Ops: []edl.Op{
			op(1, "trim", "c01", func(o *edl.Op) {
				o.InFrame, o.OutFrame = 24, 180
			}),
			op(2, "move", "c02", func(o *edl.Op) {
				o.NewTrack, o.NewStartMs = "V2", 4000
			}),
			op(3, "trim", "c03", func(o *edl.Op) {
				o.InFrame, o.OutFrame = 10, 240
			}),
			op(4, "set_note", "c04", func(o *edl.Op) {
				o.Note = "hold 2 frames on hero frame"
			}),
			op(5, "trim", "c05", func(o *edl.Op) {
				o.InFrame, o.OutFrame = 0, 120
			}),
			op(6, "move", "c06", func(o *edl.Op) {
				o.NewTrack, o.NewStartMs = "A1", 0
			}),
			op(7, "trim", "c07", func(o *edl.Op) {
				o.InFrame, o.OutFrame = 0, 60
			}),
			op(8, "move", "c08", func(o *edl.Op) {
				o.NewTrack, o.NewStartMs = "V3", 12000
			}),
		},
	}

	// Op 9: an internally inconsistent offline record (ID from c01 paired
	// with c08's content hash). Must be flagged pending, never auto-matched.
	c01, c08 := find("c01"), find("c08")
	cl.Ops = append(cl.Ops, edl.Op{
		Seq:       9,
		Action:    "set_note",
		OldClipID: c01.ClipID,
		MediaHash: c08.MediaHash,
		OldPath:   c01.Path,
		Note:      "corrupted offline record",
	})

	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}
	if err := edl.WriteJSON(filepath.Join(root, "offline_library.json"), offline); err != nil {
		return err
	}
	if err := edl.WriteJSON(filepath.Join(root, "online_library.json"), online); err != nil {
		return err
	}
	return edl.WriteJSON(filepath.Join(root, "offline_changelist.json"), cl)
}
