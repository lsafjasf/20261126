package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"mediarelink/internal/project"
	"mediarelink/internal/report"
)

const (
	fixtureProject = "../../testdata/project.json"
	fixtureMedia   = "../../testdata/media"
)

func doCheck(t *testing.T, outDir string) report.Report {
	t.Helper()
	code := run([]string{"check", "-project", fixtureProject,
		"-media-root", fixtureMedia, "-out-dir", outDir})
	if code != 0 {
		t.Fatalf("check exited %d", code)
	}
	data, err := os.ReadFile(filepath.Join(outDir, "relink-report.json"))
	if err != nil {
		t.Fatal(err)
	}
	var rep report.Report
	if err := json.Unmarshal(data, &rep); err != nil {
		t.Fatal(err)
	}
	return rep
}

func TestEndToEndCheck(t *testing.T) {
	rep := doCheck(t, t.TempDir())
	s := rep.Summary
	if s.Total != 12 || s.OK != 2 || s.Missing != 10 || s.Relinked != 7 || s.Blocked != 3 {
		t.Fatalf("unexpected summary: %+v", s)
	}

	want := map[string]string{
		"m01": "intro.mp4",
		"m02": "logo.mkv",
		"m03": "archive/interview_a_master.mov",
		"m04": "broll_city_take2.mov",
		"m06": "footage/product_shot.mov",
		"m07": "ambient_park_v2.mov",
		"m10": "scene_extra.mov",
	}
	got := map[string]string{}
	blocked := map[string]string{}
	for _, a := range rep.Assets {
		if a.Status == "relinked" {
			rel, err := filepath.Rel(fixtureMedia, a.NewPath)
			if err != nil {
				t.Fatal(err)
			}
			got[a.MediaID] = filepath.ToSlash(rel)
		} else {
			blocked[a.MediaID] = a.Reason
		}
	}
	for id, path := range want {
		if got[id] != path {
			t.Errorf("%s: want %s, got %s", id, path, got[id])
		}
	}
	for _, id := range []string{"m05", "m08", "m09"} {
		if _, ok := blocked[id]; !ok {
			t.Errorf("%s should be blocked", id)
		}
	}
	if blocked["m08"] != "low_confidence" {
		t.Errorf("m08 should block on low_confidence, got %q", blocked["m08"])
	}
}

func TestEndToEndApplyBlocks(t *testing.T) {
	outDir := t.TempDir()
	code := run([]string{"apply", "-project", fixtureProject,
		"-media-root", fixtureMedia, "-out-dir", outDir})
	if code != 2 {
		t.Fatalf("apply with blocked assets must exit 2, got %d", code)
	}
	if _, err := os.Stat(filepath.Join(outDir, "project.relinked.json")); !os.IsNotExist(err) {
		t.Fatal("blocked apply must not write the relinked project")
	}
}

func TestEndToEndApplyPartial(t *testing.T) {
	outDir := t.TempDir()
	code := run([]string{"apply", "-project", fixtureProject,
		"-media-root", fixtureMedia, "-out-dir", outDir, "-allow-partial"})
	if code != 0 {
		t.Fatalf("partial apply exited %d", code)
	}
	relinked, err := project.Load(filepath.Join(outDir, "project.relinked.json"))
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]string{}
	for _, a := range relinked.Media {
		byID[a.ID] = a.Path
	}
	if filepath.Base(byID["m01"]) != "intro.mp4" {
		t.Errorf("m01 not relinked: %s", byID["m01"])
	}
	if byID["m09"] != "old_media/backup_take.mov" {
		t.Errorf("blocked m09 must keep its recorded path, got %s", byID["m09"])
	}
	for _, fx := range relinked.Effects {
		if _, ok := byID[fx.MediaID]; !ok {
			t.Errorf("effect %s lost its media reference", fx.ID)
		}
	}
}
