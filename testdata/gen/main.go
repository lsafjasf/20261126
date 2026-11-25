// gen builds the deterministic migration fixture: a project.json whose
// recorded media paths are all broken, plus the post-migration media
// tree. Ten of twelve assets are "missing":
//
//	m01, m02  exist under a changed extension (content identical)
//	m03, m04  same-name decoy at the old name; real content relocated
//	m05       same-name decoy only; real content is gone -> blocked
//	m06       moved into a subdirectory, same content
//	m07, m10  renamed, same content
//	m08       re-encoded (same audio/duration/codec, new frames) -> blocked
//	m09       truly missing -> blocked
//
// Run from the repository root: go run ./testdata/gen
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"mediarelink/internal/media"
	"mediarelink/internal/project"
)

const (
	fpsNum     = 24
	fpsDen     = 1
	projectOut = "testdata/project.json"
	mediaRoot  = "testdata/media"
)

type assetSpec struct {
	id        string
	name      string
	oldPath   string // recorded in project.json; broken after migration
	codec     string
	duration  int64
	frames    int
	seed      uint64
	newPath   string // where the content lives now; "" = gone
	reencode  bool   // place a re-encoded variant at newPath
	decoyPath string // same-name trap with different content; "" = none
	decoySeed uint64
}

func specs() []assetSpec {
	return []assetSpec{
		{"m01", "intro", "old_media/intro.mov", "AVC1", 4000, 96, 101, "intro.mp4", false, "", 0},
		{"m02", "logo", "old_media/logo.mov", "AVC1", 2000, 48, 102, "logo.mkv", false, "", 0},
		{"m03", "interview_a", "old_media/interview_a.mov", "AVC1", 8000, 192, 103,
			"archive/interview_a_master.mov", false, "interview_a.mov", 903},
		{"m04", "broll_city", "old_media/broll_city.mov", "HEVC", 6000, 144, 104,
			"broll_city_take2.mov", false, "broll_city.mov", 904},
		{"m05", "music_bed", "old_media/music_bed.mov", "AVC1", 12000, 288, 105,
			"", false, "music_bed.mov", 905},
		{"m06", "product_shot", "old_media/product_shot.mov", "AVC1", 5000, 120, 106,
			"footage/product_shot.mov", false, "", 0},
		{"m07", "ambient_park", "old_media/ambient_park.mov", "AVC1", 7000, 168, 107,
			"ambient_park_v2.mov", false, "", 0},
		{"m08", "ending_logo", "old_media/ending_logo.mov", "AVC1", 3000, 72, 108,
			"ending_logo.mov", true, "", 0},
		{"m09", "backup_take", "old_media/backup_take.mov", "AVC1", 4500, 108, 109, "", false, "", 0},
		{"m10", "extra_scene", "old_media/extra_scene.mov", "HEVC", 6500, 156, 110,
			"scene_extra.mov", false, "", 0},
		{"m11", "healthy_a", "healthy_a.mov", "AVC1", 2500, 60, 111, "healthy_a.mov", false, "", 0},
		{"m12", "healthy_b", "healthy_b.mov", "HEVC", 3500, 84, 112, "healthy_b.mov", false, "", 0},
	}
}

func main() {
	if err := generate(); err != nil {
		fmt.Fprintln(os.Stderr, "gen:", err)
		os.Exit(1)
	}
	fmt.Println("fixtures written to testdata/ (project.json + media tree)")
}

func generate() error {
	tmp, err := os.MkdirTemp("", "mediarelink-gen")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)

	if err := os.RemoveAll(mediaRoot); err != nil {
		return err
	}

	proj := project.Project{Name: "migration-demo"}
	effectTypes := []string{"color_grade", "transform", "speed_ramp", "blur", "audio_gain"}

	for i, spec := range specs() {
		orig := makeContent(spec.seed, spec.codec, spec.duration, fpsNum, fpsDen, spec.frames)

		// Probe the original bytes so project metadata matches reality.
		tmpFile := filepath.Join(tmp, spec.id+".mrl")
		if err := orig.write(tmpFile); err != nil {
			return err
		}
		probe, err := media.ProbeFile(tmpFile)
		if err != nil {
			return err
		}
		proj.Media = append(proj.Media, project.Asset{
			ID:               spec.id,
			Name:             spec.name,
			Path:             spec.oldPath,
			Codec:            probe.Codec,
			DurationMS:       probe.DurationMS,
			FPSNum:           probe.FPSNum,
			FPSDen:           probe.FPSDen,
			FrameCount:       probe.FrameCount,
			FrameHash:        probe.FrameHash,
			AudioFingerprint: probe.AudioFingerprint,
		})

		// Effects reference media by ID and must survive the relink.
		nEffects := 1
		if spec.id <= "m10" && spec.id >= "m01" {
			nEffects = 2
		}
		for k := 0; k < nEffects; k++ {
			proj.Effects = append(proj.Effects, project.Effect{
				ID:      fmt.Sprintf("fx_%s_%d", spec.id, k+1),
				Type:    effectTypes[(i+k)%len(effectTypes)],
				MediaID: spec.id,
				Params:  map[string]any{"amount": float64(k+1) / 10},
			})
		}

		// Place the content (or its re-encode) in the new tree.
		if spec.newPath != "" {
			placed := orig
			if spec.reencode {
				placed = orig.reencode(spec.seed * 7)
			}
			if err := placed.write(filepath.Join(mediaRoot, spec.newPath)); err != nil {
				return err
			}
		}
		// Place the same-name decoy with unrelated content.
		if spec.decoyPath != "" {
			decoy := makeContent(spec.decoySeed, spec.codec, spec.duration, fpsNum, fpsDen, spec.frames)
			if err := decoy.write(filepath.Join(mediaRoot, spec.decoyPath)); err != nil {
				return err
			}
		}
	}

	if err := os.MkdirAll(filepath.Dir(projectOut), 0o755); err != nil {
		return err
	}
	return proj.Save(projectOut)
}
