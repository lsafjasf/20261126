package project

import (
	"path/filepath"
	"testing"

	"mediarelink/internal/match"
	"mediarelink/internal/media"
)

func testProject() *Project {
	return &Project{
		Name: "test",
		Media: []Asset{
			{ID: "m01", Name: "a", Path: "old/a.mov", Codec: "AVC1", DurationMS: 1000,
				FrameHash: "h1", AudioFingerprint: media.FingerprintHex(1)},
			{ID: "m02", Name: "b", Path: "old/b.mov", Codec: "AVC1", DurationMS: 2000,
				FrameHash: "h2", AudioFingerprint: media.FingerprintHex(2)},
		},
		Effects: []Effect{
			{ID: "fx1", Type: "color_grade", MediaID: "m01"},
			{ID: "fx2", Type: "blur", MediaID: "m02"},
		},
	}
}

func plans(relinked, blocked bool) []match.AssetPlan {
	out := []match.AssetPlan{{
		MediaID: "m01", Status: match.StatusRelinked,
		NewPath: "new/a.mov", Confidence: 1,
	}}
	if blocked {
		out = append(out, match.AssetPlan{
			MediaID: "m02", Status: match.StatusBlocked, Reason: match.ReasonNoCandidate,
		})
	}
	return out
}

func TestApplyBlocksEverythingWhenAnyBlocked(t *testing.T) {
	p := testProject()
	rerr, err := p.Apply(plans(true, true), false)
	if err != nil {
		t.Fatal(err)
	}
	if rerr == nil {
		t.Fatal("expected RelinkError")
	}
	if p.Media[0].Path != "old/a.mov" {
		t.Fatalf("blocked apply must not touch paths, got %s", p.Media[0].Path)
	}
}

func TestApplyPartialRewritesOnlyConfident(t *testing.T) {
	p := testProject()
	rerr, err := p.Apply(plans(true, true), true)
	if err != nil || rerr != nil {
		t.Fatalf("partial apply: rerr=%v err=%v", rerr, err)
	}
	if p.Media[0].Path != "new/a.mov" || p.Media[1].Path != "old/b.mov" {
		t.Fatalf("unexpected paths: %+v", p.Media)
	}
	if p.Effects[0].MediaID != "m01" || p.Effects[1].MediaID != "m02" {
		t.Fatal("effect references must stay untouched")
	}
}

func TestVerifyDetectsIntactChain(t *testing.T) {
	dir := t.TempDir()
	frames := [][]byte{{1, 2, 3}}
	if err := media.Encode(filepath.Join(dir, "a.mov"), "AVC1", 1000, 24, 1, 48000,
		[]byte{9, 9}, frames); err != nil {
		t.Fatal(err)
	}
	probe, err := media.ProbeFile(filepath.Join(dir, "a.mov"))
	if err != nil {
		t.Fatal(err)
	}

	p := testProject()
	p.Media[0].FrameHash = probe.FrameHash
	pl := []match.AssetPlan{{
		MediaID: "m01", Status: match.StatusRelinked,
		NewPath: filepath.Join(dir, "a.mov"), Confidence: 1,
	}}
	if _, err := p.Apply(pl, true); err != nil {
		t.Fatal(err)
	}
	v := Verify([]string{"m01", "m02"}, p, pl)
	if !v.OK() {
		t.Fatalf("verification should pass: %+v", v)
	}
	if len(v.EffectMediaPairs) != 2 {
		t.Fatalf("expected 2 effect->media pairs, got %v", v.EffectMediaPairs)
	}
}

func TestVerifyFlagsLostMediaID(t *testing.T) {
	p := testProject()
	v := Verify([]string{"m01", "m02", "m99"}, p, nil)
	if v.OK() || v.MediaIDsIntact {
		t.Fatalf("lost id must fail verification: %+v", v)
	}
}

func TestPartitionFindsMissing(t *testing.T) {
	dir := t.TempDir()
	if err := media.Encode(filepath.Join(dir, "present.mov"), "AVC1", 1000, 24, 1,
		48000, []byte{1}, [][]byte{{1}}); err != nil {
		t.Fatal(err)
	}
	p := testProject()
	p.Media[0].Path = "present.mov"
	present, missing := p.Partition(dir)
	if len(present) != 1 || present[0] != "m01" {
		t.Fatalf("present: %v", present)
	}
	if len(missing) != 1 || missing[0].MediaID != "m02" {
		t.Fatalf("missing: %+v", missing)
	}
}
