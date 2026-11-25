package audiopkg

import (
	"strings"
	"testing"
)

func validPackage() *Package {
	return &Package{
		Name: "demo",
		Tracks: []Track{
			{ID: "en-main", Language: "en", Role: RoleMain, Default: true},
			{ID: "zh-main", Language: "zh-Hans", Role: RoleMain},
			{ID: "en-sdh", Language: "en", Role: RoleAlternate, Accessibility: AccessibilityHearingImpaired},
		},
	}
}

func TestValidPackagePublishes(t *testing.T) {
	if err := validPackage().Publish(); err != nil {
		t.Fatalf("expected publishable package, got %v", err)
	}
}

func TestTwoDefaultsBlocked(t *testing.T) {
	p := validPackage()
	p.Tracks[1].Default = true // zh-Hans also marked default? no: same language needed
	p.Tracks[1].Language = "en"
	p.Tracks[1].Role = RoleAlternate // avoid duplicate-combo noise
	err := p.Publish()
	if err == nil {
		t.Fatal("expected two default en tracks to block publication")
	}
	if !strings.Contains(err.Error(), "already has default track") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestHearingImpairedMarkedMainBlocked(t *testing.T) {
	p := validPackage()
	p.Tracks[2].Role = RoleMain // hearing-impaired track as main
	err := p.Publish()
	if err == nil {
		t.Fatal("expected hearing-impaired main track to block publication")
	}
	if !strings.Contains(err.Error(), "cannot have role") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAccessibilityTrackCannotBeDefault(t *testing.T) {
	p := validPackage()
	p.Tracks[0].Default = false
	p.Tracks[2].Default = true
	if err := p.Publish(); err == nil {
		t.Fatal("expected accessibility default track to block publication")
	}
}

func TestDescriptiveRoleRequiresVisuallyImpaired(t *testing.T) {
	p := validPackage()
	p.Tracks = append(p.Tracks, Track{ID: "en-ad", Language: "en", Role: RoleDescriptive})
	if err := p.Publish(); err == nil {
		t.Fatal("expected descriptive track without accessibility to block publication")
	}
	p.Tracks[3].Accessibility = AccessibilityVisuallyImpaired
	if err := p.Publish(); err != nil {
		t.Fatalf("expected fixed track to publish, got %v", err)
	}
}

func TestInvalidLanguageTagBlocked(t *testing.T) {
	p := validPackage()
	p.Tracks[0].Language = "english!"
	if err := p.Publish(); err == nil {
		t.Fatal("expected invalid language tag to block publication")
	}
}

func TestDuplicateComboBlocked(t *testing.T) {
	p := validPackage()
	p.Tracks = append(p.Tracks, Track{ID: "zh-main-2", Language: "zh-Hans", Role: RoleMain})
	if err := p.Publish(); err == nil {
		t.Fatal("expected duplicate language+role combo to block publication")
	}
}
