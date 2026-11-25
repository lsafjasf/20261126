package audiotracks

import (
	"errors"
	"testing"
)

func validProject() Project {
	return Project{
		ID:    "film-001",
		Title: "Film",
		Tracks: []Track{
			{ID: "zh-dx", Language: "zh_cn", Type: TrackDialogue, Default: true, Audible: true},
			{ID: "zh-mx", Language: "zxx", Type: TrackMusic, Audible: true},
			{ID: "en-dub", Language: "en_us", Type: TrackDub, Default: true, Audible: true},
		},
	}
}

func TestNormalizeProjectCanonicalizesFields(t *testing.T) {
	input := validProject()
	project, err := NormalizeProject(input)
	if err != nil {
		t.Fatalf("NormalizeProject() error = %v", err)
	}
	if project.Tracks[0].Language != "zh-CN" ||
		project.Tracks[2].Language != "en-US" {
		t.Fatalf("languages were not normalized: %#v", project.Tracks)
	}
	if input.Tracks[0].Language != "zh_cn" {
		t.Fatal("NormalizeProject mutated the input project")
	}
}

func TestDuplicateDefaultAfterLanguageNormalization(t *testing.T) {
	project := validProject()
	project.Tracks = append(project.Tracks,
		Track{
			ID: "en-dub-2", Language: "EN-us", Type: TrackDub,
			Default: true, Audible: true,
		})
	err := ValidateProject(project)
	var validation *ValidationError
	if !errors.As(err, &validation) {
		t.Fatalf("error = %v, want ValidationError", err)
	}
	if !validation.HasCode(CodeDuplicateDefault) {
		t.Fatalf("problems = %#v, want %s", validation.Problems, CodeDuplicateDefault)
	}
}

func TestDefaultsMayDifferByLanguage(t *testing.T) {
	if err := ValidateProject(validProject()); err != nil {
		t.Fatalf("ValidateProject() error = %v", err)
	}
}

func TestParseLanguageNormalizesNamesWithRegions(t *testing.T) {
	tests := map[string]Language{
		"English":    "en",
		"English-US": "en-US",
		"chinese_cn": "zh-CN",
		"ja-JP":      "ja-JP",
		"und":        "und",
	}
	for input, want := range tests {
		got, err := ParseLanguage(input)
		if err != nil {
			t.Errorf("ParseLanguage(%q) error = %v", input, err)
			continue
		}
		if got != want {
			t.Errorf("ParseLanguage(%q) = %q, want %q", input, got, want)
		}
	}
	if _, err := ParseLanguage("englishish"); !errors.Is(err, ErrInvalidLanguage) {
		t.Errorf("ParseLanguage(englishish) error = %v, want ErrInvalidLanguage", err)
	}
}

func TestRejectsMutedDefault(t *testing.T) {
	project := validProject()
	project.Tracks[0].Audible = false
	var validation *ValidationError
	if err := ValidateProject(project); !errors.As(err, &validation) {
		t.Fatalf("error = %v, want ValidationError", err)
	}
	if !validation.HasCode(CodeMutedDefault) {
		t.Fatalf("problems = %#v, want %s", validation.Problems, CodeMutedDefault)
	}
}

func TestRejectsNeutralVoiceLanguage(t *testing.T) {
	project := validProject()
	project.Tracks[0].Language = "und"
	var validation *ValidationError
	if err := ValidateProject(project); !errors.As(err, &validation) {
		t.Fatalf("error = %v, want ValidationError", err)
	}
	if !validation.HasCode(CodeVoiceNeedsLanguage) {
		t.Fatalf("problems = %#v, want %s", validation.Problems, CodeVoiceNeedsLanguage)
	}
}

func TestAggregatesInvalidCombinations(t *testing.T) {
	project := Project{
		ID: "film-001",
		Tracks: []Track{
			{ID: "same", Language: "en", Type: TrackDialogue, Default: true, Audible: true},
			{ID: "same", Language: "en", Type: TrackType("audio"), Default: true, Audible: false},
			{ID: "bad-lang", Language: "not a language!", Type: TrackDub, Audible: true},
		},
	}
	var validation *ValidationError
	if err := ValidateProject(project); !errors.As(err, &validation) {
		t.Fatalf("error = %v, want ValidationError", err)
	}
	for _, code := range []string{
		CodeDuplicateTrackID,
		CodeDuplicateDefault,
		CodeMutedDefault,
		CodeInvalidTrackType,
		CodeInvalidLanguage,
	} {
		if !validation.HasCode(code) {
			t.Errorf("missing problem %s in %#v", code, validation.Problems)
		}
	}
}
