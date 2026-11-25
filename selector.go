package trackselect

import "fmt"

// Selection 是一次轨道解析的结果。
type Selection struct {
	Audio    *Track // 选中的音轨，可能为 nil
	Subtitle *Track // 选中的主字幕轨，可能为 nil（用户关闭字幕）
	Forced   *Track // 自动叠加的强制叙述轨，可能为 nil
	Reason   string // 人类可读的决策说明
}

// Select 按场景偏好从轨道集合中选出音轨与字幕轨。
// 优先级：偏好语言顺序 > 场景类型权重 > 容器默认标记 > 回退链 > 首个可用轨。
func Select(tracks []Track, prefs Preferences) Selection {
	var sel Selection
	audio := selectAudio(tracks, prefs)
	sel.Audio = audio
	sel.Subtitle = selectSubtitle(tracks, prefs, audio)
	if prefs.AllowForced {
		sel.Forced = selectForced(tracks, prefs, audio, sel.Subtitle)
	}
	sel.Reason = describe(sel)
	return sel
}

// langRank 返回语言在偏好列表中的名次，未命中返回 len(langs)。
func langRank(lang string, langs []string) int {
	for i, l := range langs {
		if l == lang {
			return i
		}
	}
	return len(langs)
}

// fallbackChain 沿 Fallback 链展开候选 ID，遇环或悬空即截断。
func fallbackChain(byID map[string]Track, start string) []Track {
	seen := map[string]bool{}
	var out []Track
	for id := start; id != "" && !seen[id]; {
		seen[id] = true
		t, ok := byID[id]
		if !ok {
			break
		}
		out = append(out, t)
		id = t.Fallback
	}
	return out
}

func indexByID(tracks []Track) map[string]Track {
	byID := make(map[string]Track, len(tracks))
	for _, t := range tracks {
		byID[t.ID] = t
	}
	return byID
}

// selectAudio 选择音轨：解说轨需求 > 语言偏好 > 默认标记 > 回退链。
func selectAudio(tracks []Track, prefs Preferences) *Track {
	var candidates []Track
	for _, t := range tracks {
		if t.Kind == KindAudio {
			candidates = append(candidates, t)
		}
	}
	if len(candidates) == 0 {
		return nil
	}
	langs := prefs.AudioLanguages
	if len(langs) == 0 {
		langs = prefs.Languages
	}
	best := -1
	bestScore := [3]int{1 << 30, 1 << 30, 1 << 30}
	for i, t := range candidates {
		typeRank := 0
		if prefs.WantCommentary {
			if t.Type == TypeCommentary {
				typeRank = 0
			} else {
				typeRank = 1
			}
		} else if t.Type == TypeCommentary {
			typeRank = 1
		}
		defRank := 1
		if t.Default {
			defRank = 0
		}
		score := [3]int{typeRank, langRank(t.Language, langs), defRank}
		if best == -1 || less(score, bestScore) {
			best, bestScore = i, score
		}
	}
	chosen := candidates[best]
	// 若最佳候选是某条回退链的中间节点，链首（默认轨）优先。
	byID := indexByID(tracks)
	for _, t := range fallbackChain(byID, chosen.ID) {
		if t.Kind == KindAudio && t.Default {
			return &t
		}
	}
	return &chosen
}

// selectSubtitle 选择主字幕轨：语言偏好 > SDH 场景权重 > 默认标记。
func selectSubtitle(tracks []Track, prefs Preferences, audio *Track) *Track {
	var candidates []Track
	for _, t := range tracks {
		if t.Kind == KindSubtitle && t.Type != TypeForced {
			candidates = append(candidates, t)
		}
	}
	if len(candidates) == 0 {
		return nil
	}
	best := -1
	bestScore := [3]int{1 << 30, 1 << 30, 1 << 30}
	for i, t := range candidates {
		typeRank := 0
		if prefs.PreferSDH {
			if t.Type == TypeSDH {
				typeRank = 0
			} else {
				typeRank = 1
			}
		} else if t.Type == TypeSDH {
			typeRank = 1
		}
		defRank := 1
		if t.Default {
			defRank = 0
		}
		score := [3]int{langRank(t.Language, prefs.Languages), typeRank, defRank}
		if best == -1 || less(score, bestScore) {
			best, bestScore = i, score
		}
	}
	chosen := candidates[best]
	// 语言完全未命中时，沿默认轨的回退链找一条可用字幕。
	if langRank(chosen.Language, prefs.Languages) == len(prefs.Languages) {
		byID := indexByID(tracks)
		for _, t := range tracks {
			if t.Kind == KindSubtitle && t.Default {
				for i, fb := range fallbackChain(byID, t.ID) {
					if i == 0 {
						continue // 链首即当前默认轨，已判定语言未命中
					}
					if fb.Kind == KindSubtitle && fb.Type != TypeForced {
						return &fb
					}
				}
			}
		}
	}
	return &chosen
}

// selectForced 当音轨语言与字幕语言不一致时，挑选与字幕同语言的强制叙述轨。
func selectForced(tracks []Track, prefs Preferences, audio, subtitle *Track) *Track {
	if audio == nil || subtitle == nil || audio.Language == subtitle.Language {
		return nil
	}
	var match *Track
	for _, t := range tracks {
		if t.Kind == KindSubtitle && t.Type == TypeForced && t.Language == subtitle.Language {
			t := t
			if match == nil || (t.Default && !match.Default) {
				match = &t
			}
		}
	}
	return match
}

// less 按字典序比较两个评分向量。
func less(a, b [3]int) bool {
	for i := range a {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return false
}

func describe(s Selection) string {
	r := "audio="
	if s.Audio != nil {
		r += fmt.Sprintf("%s(%s/%s)", s.Audio.ID, s.Audio.Language, s.Audio.Type)
	} else {
		r += "none"
	}
	r += " subtitle="
	if s.Subtitle != nil {
		r += fmt.Sprintf("%s(%s/%s)", s.Subtitle.ID, s.Subtitle.Language, s.Subtitle.Type)
	} else {
		r += "none"
	}
	if s.Forced != nil {
		r += fmt.Sprintf(" forced=%s(%s)", s.Forced.ID, s.Forced.Language)
	}
	return r
}
