package trackselect

import "testing"

func sampleTracks() []Track {
	return []Track{
		{ID: "a-en", Kind: KindAudio, Type: TypeRegular, Language: "en", Default: true},
		{ID: "a-zh", Kind: KindAudio, Type: TypeRegular, Language: "zh-CN"},
		{ID: "a-comment", Kind: KindAudio, Type: TypeCommentary, Language: "zh-CN"},
		{ID: "s-zh", Kind: KindSubtitle, Type: TypeRegular, Language: "zh-CN", Default: true},
		{ID: "s-zh-sdh", Kind: KindSubtitle, Type: TypeSDH, Language: "zh-CN"},
		{ID: "s-en", Kind: KindSubtitle, Type: TypeRegular, Language: "en"},
		{ID: "s-zh-forced", Kind: KindSubtitle, Type: TypeForced, Language: "zh-CN"},
	}
}

func TestSelectDefaultScenario(t *testing.T) {
	prefs := DefaultPreferences(ScenarioDefault)
	prefs.AudioLanguages = []string{"en"} // 原声观影：英文音轨 + 中文字幕
	sel := Select(sampleTracks(), prefs)
	if sel.Audio == nil || sel.Audio.ID != "a-en" {
		t.Fatalf("默认场景应选默认音轨 a-en，得到 %+v", sel.Audio)
	}
	if sel.Subtitle == nil || sel.Subtitle.ID != "s-zh" {
		t.Fatalf("默认场景应选普通中文字幕 s-zh，得到 %+v", sel.Subtitle)
	}
	if sel.Forced == nil || sel.Forced.ID != "s-zh-forced" {
		t.Fatalf("音轨 en 与字幕 zh 不一致，应叠加强制轨，得到 %+v", sel.Forced)
	}
}

func TestSelectNoisyPrefersSDH(t *testing.T) {
	sel := Select(sampleTracks(), DefaultPreferences(ScenarioNoisy))
	if sel.Subtitle == nil || sel.Subtitle.ID != "s-zh-sdh" {
		t.Fatalf("嘈杂场景应优先 SDH，得到 %+v", sel.Subtitle)
	}
}

func TestSelectImpairedPrefersCommentary(t *testing.T) {
	sel := Select(sampleTracks(), DefaultPreferences(ScenarioImpaired))
	if sel.Audio == nil || sel.Audio.ID != "a-comment" {
		t.Fatalf("视障场景应优先解说轨，得到 %+v", sel.Audio)
	}
}

func TestSelectLanguageFallbackOrder(t *testing.T) {
	tracks := []Track{
		{ID: "a-en", Kind: KindAudio, Language: "en", Default: true},
		{ID: "s-en", Kind: KindSubtitle, Language: "en"},
		{ID: "s-ja", Kind: KindSubtitle, Language: "ja"},
	}
	prefs := Preferences{Scenario: ScenarioDefault, Languages: []string{"zh-CN", "en"}}
	sel := Select(tracks, prefs)
	if sel.Subtitle == nil || sel.Subtitle.ID != "s-en" {
		t.Fatalf("zh-CN 缺失应回退到第二偏好 en，得到 %+v", sel.Subtitle)
	}
}

func TestSelectFollowsFallbackChain(t *testing.T) {
	tracks := []Track{
		{ID: "a-en", Kind: KindAudio, Language: "en", Default: true},
		{ID: "s-fr", Kind: KindSubtitle, Language: "fr", Default: true, Fallback: "s-en"},
		{ID: "s-en", Kind: KindSubtitle, Language: "en"},
	}
	prefs := Preferences{Scenario: ScenarioDefault, Languages: []string{"de"}}
	sel := Select(tracks, prefs)
	if sel.Subtitle == nil || sel.Subtitle.ID != "s-en" {
		t.Fatalf("语言全未命中应沿默认轨回退链到 s-en，得到 %+v", sel.Subtitle)
	}
}

func TestForcedNotDuplicatedWhenLanguagesMatch(t *testing.T) {
	tracks := []Track{
		{ID: "a-zh", Kind: KindAudio, Language: "zh-CN", Default: true},
		{ID: "s-zh", Kind: KindSubtitle, Language: "zh-CN", Default: true},
		{ID: "s-zh-forced", Kind: KindSubtitle, Type: TypeForced, Language: "zh-CN"},
	}
	sel := Select(tracks, DefaultPreferences(ScenarioDefault))
	if sel.Forced != nil {
		t.Fatalf("音轨与字幕同语言不应叠加强制轨，得到 %+v", sel.Forced)
	}
}
