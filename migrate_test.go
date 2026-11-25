package audiotracks

import (
	"errors"
	"testing"
)

func TestMigrateFlattenedAudioRoles(t *testing.T) {
	input := []byte(`{
	  "project_id": "film-001",
	  "title": "多语版影片",
	  "tracks": [
	    {"track_id": "dx-zh", "track_name": "A1 Dialogue 对白",
	     "track_role": "audio", "language_code": "zh_cn",
	     "is_default": true, "is_muted": false},
	    {"track_id": "mx", "track_name": "A2 Music 配乐",
	     "track_role": "audio", "language": "zxx"},
	    {"track_id": "sfx", "track_name": "A3 SFX 音效",
	     "track_role": "audio", "language": "zxx", "audible": false},
	    {"track_id": "dub-en", "track_name": "English dub",
	     "track_role": "audio", "language": "English",
	     "is_default": true, "muted": false},
	    {"track_id": "dub-ja", "track_name": "日本語配音",
	     "track_role": "audio", "language": "ja_jp",
	     "is_default": true, "muted": false}
	  ]
	}`)

	project, err := MigrateJSON(input)
	if err != nil {
		t.Fatalf("MigrateJSON() error = %v", err)
	}
	if project.ID != "film-001" || project.Title != "多语版影片" {
		t.Fatalf("unexpected project identity: %#v", project)
	}
	if len(project.Tracks) != 5 {
		t.Fatalf("got %d tracks, want 5", len(project.Tracks))
	}

	want := []struct {
		id       string
		language Language
		track    TrackType
		def      bool
		audible  bool
	}{
		{"dx-zh", "zh-CN", TrackDialogue, true, true},
		{"mx", "zxx", TrackMusic, false, true},
		{"sfx", "zxx", TrackEffects, false, false},
		{"dub-en", "en", TrackDub, true, true},
		{"dub-ja", "ja-JP", TrackDub, true, true},
	}
	for i, expected := range want {
		got := project.Tracks[i]
		if got.ID != expected.id || got.Language != expected.language ||
			got.Type != expected.track || got.Default != expected.def ||
			got.Audible != expected.audible {
			t.Errorf("track[%d] = %#v, want %#v", i, got, expected)
		}
	}
	if !project.Tracks[2].Muted() {
		t.Error("sfx track should report muted")
	}
	defaults := project.DefaultsByLanguage()
	if len(defaults["zh-CN"]) != 1 || len(defaults["en"]) != 1 ||
		len(defaults["ja-JP"]) != 1 {
		t.Fatalf("unexpected defaults: %#v", defaults)
	}
}

func TestMigrateRejectsUnmappedGenericAudio(t *testing.T) {
	_, err := Migrate(EditTable{
		ProjectID: "film-001",
		Tracks: []EditTableTrack{{
			ID: "a1", Name: "Audio 1", Role: "audio", Language: "en",
		}},
	})
	var validation *ValidationError
	if !errors.As(err, &validation) {
		t.Fatalf("error = %v, want ValidationError", err)
	}
	if !validation.HasCode(CodeUnmappedTrackType) {
		t.Fatalf("problems = %#v, want %s", validation.Problems, CodeUnmappedTrackType)
	}
}

func TestMigrateRejectsAmbiguousTrackName(t *testing.T) {
	_, err := Migrate(EditTable{
		ProjectID: "film-001",
		Tracks: []EditTableTrack{{
			ID: "a1", Name: "Dialogue Music Mix", Role: "audio", Language: "en",
		}},
	})
	var validation *ValidationError
	if !errors.As(err, &validation) || !validation.HasCode(CodeUnmappedTrackType) {
		t.Fatalf("error = %#v, want unmapped type", err)
	}
}

func TestMigrateRejectsConflictingAudibility(t *testing.T) {
	audible, muted := true, true
	_, err := Migrate(EditTable{
		ProjectID: "film-001",
		Tracks: []EditTableTrack{{
			ID: "dx", Name: "Dialogue", Role: "audio", Language: "en",
			Audible: &audible, Muted: &muted,
		}},
	})
	var validation *ValidationError
	if !errors.As(err, &validation) || !validation.HasCode(CodeConflictingAudible) {
		t.Fatalf("error = %#v, want conflicting audibility", err)
	}
}

func TestMigrateRejectsConflictingSemanticTypes(t *testing.T) {
	_, err := Migrate(EditTable{
		ProjectID: "film-001",
		Tracks: []EditTableTrack{{
			ID: "a1", Name: "A1", Role: "dialogue", Type: "music", Language: "en",
		}},
	})
	var validation *ValidationError
	if !errors.As(err, &validation) || !validation.HasCode(CodeConflictingType) {
		t.Fatalf("error = %#v, want conflicting type", err)
	}
}

func TestMigrateRejectsUnknownSecondaryType(t *testing.T) {
	_, err := Migrate(EditTable{
		ProjectID: "film-001",
		Tracks: []EditTableTrack{{
			ID: "a1", Name: "Dialogue", Role: "dialogue", Type: "subtitle", Language: "en",
		}},
	})
	var validation *ValidationError
	if !errors.As(err, &validation) || !validation.HasCode(CodeInvalidTrackType) {
		t.Fatalf("error = %#v, want invalid track type", err)
	}
}

func TestMigrateRejectsDuplicateDefaultsInOneLanguage(t *testing.T) {
	def := true
	_, err := Migrate(EditTable{
		ProjectID: "film-001",
		Tracks: []EditTableTrack{
			{ID: "en-1", Name: "Dialogue", Role: "audio", Language: "en_US", Default: &def},
			{ID: "en-2", Name: "Dub", Role: "audio", Language: "EN-us", Default: &def},
		},
	})
	var validation *ValidationError
	if !errors.As(err, &validation) || !validation.HasCode(CodeDuplicateDefault) {
		t.Fatalf("error = %#v, want duplicate default", err)
	}
}
