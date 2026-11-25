package trackselect

import "testing"

func issuesOfKind(issues []Issue, kind IssueKind) []Issue {
	var out []Issue
	for _, i := range issues {
		if i.Kind == kind {
			out = append(out, i)
		}
	}
	return out
}

func TestDetectForcedConflict(t *testing.T) {
	tracks := []Track{
		{ID: "f1", Kind: KindSubtitle, Type: TypeForced, Language: "zh-CN", Default: true},
		{ID: "f2", Kind: KindSubtitle, Type: TypeForced, Language: "zh-CN", Default: true},
		{ID: "f3", Kind: KindSubtitle, Type: TypeForced, Language: "en", Default: true},
	}
	issues := issuesOfKind(Validate(tracks, Preferences{}), IssueForcedConflict)
	if len(issues) != 1 {
		t.Fatalf("应报告 1 起强制轨冲突，得到 %d", len(issues))
	}
	if len(issues[0].TrackIDs) != 2 {
		t.Fatalf("冲突应涉及 f1 与 f2，得到 %v", issues[0].TrackIDs)
	}
}

func TestDetectFallbackCycle(t *testing.T) {
	tracks := []Track{
		{ID: "a", Kind: KindSubtitle, Language: "en", Default: true, Fallback: "b"},
		{ID: "b", Kind: KindSubtitle, Language: "en", Fallback: "c"},
		{ID: "c", Kind: KindSubtitle, Language: "en", Fallback: "a"},
	}
	issues := issuesOfKind(Validate(tracks, Preferences{}), IssueFallbackCycle)
	if len(issues) != 1 {
		t.Fatalf("应报告 1 个回退环，得到 %d: %v", len(issues), issues)
	}
}

func TestDetectDanglingFallback(t *testing.T) {
	tracks := []Track{
		{ID: "a", Kind: KindSubtitle, Language: "en", Fallback: "ghost"},
	}
	issues := issuesOfKind(Validate(tracks, Preferences{}), IssueDanglingFallback)
	if len(issues) != 1 {
		t.Fatalf("应报告悬空回退，得到 %v", issues)
	}
}

func TestDetectMissingLanguage(t *testing.T) {
	tracks := []Track{
		{ID: "a-en", Kind: KindAudio, Language: "en"},
		{ID: "s-en", Kind: KindSubtitle, Language: "en"},
	}
	prefs := Preferences{Languages: []string{"zh-CN", "en"}, AudioLanguages: []string{"ja"}}
	issues := issuesOfKind(Validate(tracks, prefs), IssueMissingLanguage)
	if len(issues) != 2 {
		t.Fatalf("应报告缺 zh-CN 字幕与 ja 音轨共 2 条，得到 %v", issues)
	}
}

func TestFallbackChainTerminatesOnCycle(t *testing.T) {
	byID := map[string]Track{
		"a": {ID: "a", Fallback: "b"},
		"b": {ID: "b", Fallback: "a"},
	}
	chain := fallbackChain(byID, "a")
	if len(chain) != 2 {
		t.Fatalf("回退链遇环应截断为 2 条，得到 %d", len(chain))
	}
}
