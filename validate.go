package trackselect

import (
	"fmt"
	"sort"
	"strings"
)

// IssueKind 标识校验发现的问题类型。
type IssueKind int

const (
	IssueForcedConflict   IssueKind = iota // 同语言出现两条默认强制叙述轨
	IssueFallbackCycle                     // 回退链存在循环引用
	IssueMissingLanguage                   // 偏好语言没有任何可用轨
	IssueDanglingFallback                  // 回退目标 ID 不存在
)

func (k IssueKind) String() string {
	switch k {
	case IssueForcedConflict:
		return "forced-conflict"
	case IssueFallbackCycle:
		return "fallback-cycle"
	case IssueMissingLanguage:
		return "missing-language"
	default:
		return "dangling-fallback"
	}
}

// Issue 是一条校验告警，TrackIDs 按字典序排列以保证输出稳定。
type Issue struct {
	Kind     IssueKind
	Message  string
	TrackIDs []string
}

// Validate 对轨道集合与偏好做静态检查，返回全部问题。
func Validate(tracks []Track, prefs Preferences) []Issue {
	var issues []Issue
	issues = append(issues, detectForcedConflicts(tracks)...)
	issues = append(issues, detectFallbackCycles(tracks)...)
	issues = append(issues, detectMissingLanguages(tracks, prefs)...)
	return issues
}

// detectForcedConflicts 找出同一语言下被同时标记为默认的多条强制叙述轨。
func detectForcedConflicts(tracks []Track) []Issue {
	byLang := map[string][]Track{}
	for _, t := range tracks {
		if t.Kind == KindSubtitle && t.Type == TypeForced && t.Default {
			byLang[t.Language] = append(byLang[t.Language], t)
		}
	}
	var issues []Issue
	for lang, group := range byLang {
		if len(group) < 2 {
			continue
		}
		ids := make([]string, 0, len(group))
		for _, t := range group {
			ids = append(ids, t.ID)
		}
		sort.Strings(ids)
		issues = append(issues, Issue{
			Kind:     IssueForcedConflict,
			Message:  fmt.Sprintf("语言 %q 存在 %d 条默认强制叙述轨冲突: %s", lang, len(group), strings.Join(ids, ", ")),
			TrackIDs: ids,
		})
	}
	return issues
}

// detectFallbackCycles 沿 Fallback 链做三色 DFS，报告所有环与悬空引用。
func detectFallbackCycles(tracks []Track) []Issue {
	byID := map[string]Track{}
	for _, t := range tracks {
		byID[t.ID] = t
	}
	const (
		white = 0 // 未访问
		gray  = 1 // 在当前栈上
		black = 2 // 已完成
	)
	color := map[string]int{}
	var issues []Issue
	reported := map[string]bool{}

	var visit func(id string, stack []string)
	visit = func(id string, stack []string) {
		switch color[id] {
		case black:
			return
		case gray:
			// 找到环：截取栈中从 id 开始的部分。
			start := 0
			for i, s := range stack {
				if s == id {
					start = i
					break
				}
			}
			cycle := append(append([]string{}, stack[start:]...), id)
			key := strings.Join(cycle, "->")
			if !reported[key] {
				reported[key] = true
				issues = append(issues, Issue{
					Kind:     IssueFallbackCycle,
					Message:  fmt.Sprintf("默认轨回退链存在循环引用: %s", key),
					TrackIDs: append([]string{}, stack[start:]...),
				})
			}
			return
		}
		t, ok := byID[id]
		if !ok {
			return
		}
		color[id] = gray
		if t.Fallback != "" {
			if _, ok := byID[t.Fallback]; !ok {
				issues = append(issues, Issue{
					Kind:     IssueDanglingFallback,
					Message:  fmt.Sprintf("轨道 %q 的回退目标 %q 不存在", t.ID, t.Fallback),
					TrackIDs: []string{t.ID},
				})
			} else {
				visit(t.Fallback, append(stack, id))
			}
		}
		color[id] = black
	}

	for _, t := range tracks {
		if color[t.ID] == white {
			visit(t.ID, nil)
		}
	}
	return issues
}

// detectMissingLanguages 检查偏好语言是否缺少对应字幕轨或音轨。
func detectMissingLanguages(tracks []Track, prefs Preferences) []Issue {
	subLangs := map[string]bool{}
	audioLangs := map[string]bool{}
	for _, t := range tracks {
		if t.Kind == KindSubtitle {
			subLangs[t.Language] = true
		} else {
			audioLangs[t.Language] = true
		}
	}
	var issues []Issue
	for _, lang := range prefs.Languages {
		if !subLangs[lang] {
			issues = append(issues, Issue{
				Kind:    IssueMissingLanguage,
				Message: fmt.Sprintf("偏好字幕语言 %q 没有任何可用字幕轨", lang),
			})
		}
	}
	audioPrefs := prefs.AudioLanguages
	if len(audioPrefs) == 0 {
		audioPrefs = prefs.Languages
	}
	for _, lang := range audioPrefs {
		if !audioLangs[lang] {
			issues = append(issues, Issue{
				Kind:    IssueMissingLanguage,
				Message: fmt.Sprintf("偏好音轨语言 %q 没有任何可用音频轨", lang),
			})
		}
	}
	return issues
}
