package sdh

import (
	"sort"
	"strings"
)

// musicNotes are the music-note runes that delimit sung lyrics.
var musicNotes = map[rune]bool{
	'♪': true, // U+266A eighth note
	'♫': true, // U+266B beamed eighth notes
	'♬': true, // U+266C beamed sixteenth notes
	'♩': true, // U+2669 quarter note
	'🎵': true,
	'🎶': true,
}

func containsMusicNote(s string) bool {
	for _, r := range s {
		if musicNotes[r] {
			return true
		}
	}
	return false
}

// bracketPairs maps an opening bracket rune to its closing rune.
var bracketPairs = map[rune]rune{
	'[': ']',
	'(': ')',
	'（': '）',
	'【': '】',
	'〔': '〕',
	'｛': '｝',
	'{': '}',
}

// normalizeLex lowercases and strips every non letter/digit rune so that
// "V.O.", "v o" and "VO" all compare equal to "vo".
func normalizeLex(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if isLexRune(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func isLexRune(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') ||
		(r >= 0x4e00 && r <= 0x9fff)
}

// modifierPhrases are parenthetical delivery modifiers that are not spoken
// text and not sound description. Keys are normalizeLex forms.
var modifierPhrases = map[string]modifierKind{
	"vo":           {voiceOver: true},
	"voiceover":    {voiceOver: true},
	"ov":           {voiceOver: true}, // off voice
	"os":           {offScreen: true},
	"offscreen":    {offScreen: true},
	"oc":           {offScreen: true}, // off camera
	"continued":    {},
	"contd":        {},
	"continuing":   {},
	"continuity":   {},
	"subtitled":    {},
	"dubbed":       {},
	"withsubtitle": {},
}

type modifierKind struct {
	voiceOver bool
	offScreen bool
}

// cjkModifierPhrases are substring markers for Chinese delivery modifiers.
var cjkModifierPhrases = []struct {
	phrase string
	kind   modifierKind
}{
	{"画外音", modifierKind{voiceOver: true}},
	{"画外", modifierKind{offScreen: true}},
	{"旁白", modifierKind{voiceOver: true}},
	{"内心独白", modifierKind{voiceOver: true}},
	{"心声", modifierKind{voiceOver: true}},
	{"解说", modifierKind{voiceOver: true}},
}

// modifierKeys returns modifier lexicon keys ordered longest-first so that
// e.g. "voiceover" is consumed before the short key "vo".
func modifierKeys() []string {
	keys := make([]string, 0, len(modifierPhrases))
	for k := range modifierPhrases {
		keys = append(keys, k)
	}
	sortStringsByLengthDesc(keys)
	return keys
}

func sortStringsByLengthDesc(s []string) {
	sort.Slice(s, func(i, j int) bool {
		if len(s[i]) != len(s[j]) {
			return len(s[i]) > len(s[j])
		}
		return s[i] < s[j]
	})
}

// parseModifier tests whether bracket content such as "V.O." / "画外音" is
// only a delivery modifier. It reports the voice-over/off-screen flags.
func parseModifier(content string) (vo, offScreen, ok bool) {
	raw := strings.TrimSpace(content)
	if raw == "" {
		return false, false, false
	}
	rest := normalizeLex(raw)
	if rest == "" {
		return false, false, false
	}
	found := false

	for _, mp := range cjkModifierPhrases {
		key := normalizeLex(mp.phrase)
		if key != "" && strings.Contains(rest, key) {
			found = true
			if mp.kind.voiceOver {
				vo = true
			}
			if mp.kind.offScreen {
				offScreen = true
			}
			rest = strings.ReplaceAll(rest, key, "")
		}
	}

	for _, key := range modifierKeys() {
		if strings.Contains(rest, key) {
			found = true
			mk := modifierPhrases[key]
			if mk.voiceOver {
				vo = true
			}
			if mk.offScreen {
				offScreen = true
			}
			rest = strings.ReplaceAll(rest, key, "")
		}
	}

	// Valid only when at least one modifier matched and every lexical rune
	// was consumed by modifier phrases.
	if found && rest == "" {
		return vo, offScreen, true
	}
	return false, false, false
}

// musicKeywords are normalized substrings marking a music description rather
// than an ordinary environmental sound.
var musicKeywords = []string{
	"music", "soundtrack", "score", "melody", "orchestra", "songplays",
	"playsong", "jazz", "piano", "guitar", "drum", "violin", "singing",
	"chorus", "choir", "anthem", "lullaby", "riff", "beatdrops", "bgm",
	"背景音乐", "音乐", "乐曲", "旋律", "主题曲", "插曲", "配乐", "歌声",
	"歌曲", "合唱", "哼唱", "奏起", "响起乐",
}

// soundKeywords are normalized substrings that strongly mark a bracket as a
// sound/action description rather than a person's name.
var soundKeywords = []string{
	"thunder", "rain", "wind", "door", "footstep", "steps", "knock",
	"bell", "phone", "ring", "gun", "shot", "explosion", "crash", "bang",
	"slam", "creak", "bark", "dog", "cat", "siren", "horn", "engine",
	"car", "train", "plane", "helicopter", "crowd", "cheer", "applause",
	"clap", "laugh", "laughter", "cough", "sigh", "gasp", "scream",
	"shout", "whisper", "cry", "weep", "sniff", "kiss", "punch", "hit",
	"break", "glass", "fire", "water", "splash", "clock", "tick",
	"keyboard", "typing", "camera", "shutter", "noise", "sound",
	"crackle", "static", "footfalls", "rustle", "rustling", "howl",
	"roar", "buzz", "hum", "beep", "chime", "echo",
	"雷声", "雷雨", "下雨", "雨声", "风声", "门", "敲门", "脚步", "铃",
	"电话", "枪", "爆炸", "撞", "碎", "狗", "猫", "警报", "喇叭", "引擎",
	"车", "火车", "飞机", "人群", "欢呼", "掌声", "鼓掌", "笑", "咳嗽",
	"叹气", "叹息", "喘", "尖叫", "喊", "低语", "哭", "吻", "拳", "玻璃",
	"火", "水", "钟", "滴答", "键盘", "打字", "相机", "快门", "声", "响",
	"鸣", "噪音", "广播声",
}

func containsAny(s string, keys []string) bool {
	for _, k := range keys {
		if strings.Contains(s, k) {
			return true
		}
	}
	return false
}

// classifyDescription distinguishes music descriptions from environmental
// sound/action descriptions.
func classifyDescription(content string) Role {
	norm := normalizeLex(content)
	if containsAny(norm, musicKeywords) {
		return RoleMusic
	}
	return RoleSound
}

// cjkNameBlocklist are words that may precede a full-width colon but are not
// speaker names.
var cjkLabelBlocklist = []string{
	"时间", "地点", "日期", "备注", "注", "字幕", "编辑", "翻译", "校对",
	"审核", "时长", "格式", "文件名", "标题", "简介", "剧情", "提示",
	"注意", "警告",
}

// latinRoleNouns are lowercase source/role nouns accepted as speaker labels.
var latinRoleNouns = map[string]bool{
	"narrator": true, "announcer": true, "speaker": true, "voice": true,
	"radio": true, "tv": true, "man": true, "woman": true, "boy": true,
	"girl": true, "child": true, "kid": true, "crowd": true, "chorus": true,
	"everyone": true, "somebody": true, "someone": true, "stranger": true,
	"operator": true, "officer": true, "doctor": true, "nurse": true,
	"waiter": true, "waitress": true, "clerk": true, "teacher": true,
	"student": true, "mother": true, "father": true, "mom": true, "dad": true,
	"grandma": true, "grandpa": true, "brother": true, "sister": true,
	"friend": true, "boss": true, "guard": true, "soldier": true, "driver": true,
	"passenger": true, "customer": true, "guest": true, "host": true,
	"reporter": true, "journalist": true, "singer": true,
}
