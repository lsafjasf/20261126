package asskaraoke

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	kTagRE  = regexp.MustCompile(`\\k([of]?)(\d+)`)
	ktRE    = regexp.MustCompile(`\\kt(\d+)`)
	colorRE = regexp.MustCompile(`&H([0-9A-Fa-f]{0,8})&?`)
)

// ParseASSTime 解析 H:MM:SS.cc（厘秒）形式的 ASS 时间。
func ParseASSTime(s string) (time.Duration, error) {
	parts := strings.Split(s, ":")
	if len(parts) != 3 {
		return 0, &time.ParseError{Layout: "H:MM:SS.cc", Value: s}
	}
	h, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
	m, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
	secParts := strings.SplitN(parts[2], ".", 2)
	sec, err3 := strconv.Atoi(strings.TrimSpace(secParts[0]))
	if err1 != nil || err2 != nil || err3 != nil || m < 0 || m > 59 || sec < 0 || sec > 59 {
		return 0, &time.ParseError{Layout: "H:MM:SS.cc", Value: s}
	}
	cs := 0
	if len(secParts) == 2 {
		cs, _ = strconv.Atoi(strings.TrimSpace(secParts[1]))
	}
	centis := h*360000 + m*6000 + sec*100 + cs
	return time.Duration(centis) * 10 * time.Millisecond, nil
}

// parseASSColor 解析 &HAABBGGRR（可省略前导 0 或 & 尾缀）。
func parseASSColor(s string) (Color, bool) {
	m := colorRE.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return Color{}, false
	}
	hex := m[1]
	if len(hex) > 8 {
		hex = hex[:8]
	}
	v, err := strconv.ParseUint(hex, 16, 32)
	if err != nil {
		return Color{}, false
	}
	switch len(hex) {
	case 6: // BBGGRR，默认不透明
		return Color{R: uint8(v), G: uint8(v >> 8), B: uint8(v >> 16), A: 0}, true
	case 8: // AABBGGRR
		return Color{R: uint8(v), G: uint8(v >> 8), B: uint8(v >> 16), A: uint8(v >> 24)}, true
	default:
		return Color{}, false
	}
}

// tokenizeKaraoke 把一条 Dialogue 的 Text 切成音节，base 是该事件的绝对媒体起点。
// 返回去标签纯文本与音节切片；音节 Start/End 已换算为绝对媒体时间。
func tokenizeKaraoke(text string, base time.Duration) (string, []Syllable) {
	type chunk struct {
		tags string
		text string
	}
	var chunks []chunk
	var cur chunk
	flush := func() {
		if cur.tags != "" || cur.text != "" {
			chunks = append(chunks, cur)
		}
		cur = chunk{}
	}

	// emit 同时写入音节文本与纯文本缓冲。
	var plain strings.Builder
	emit := func(s string) {
		cur.text += s
		plain.WriteString(s)
	}
	for i := 0; i < len(text); {
		switch {
		case text[i] == '{':
			close := strings.IndexByte(text[i:], '}')
			if close < 0 {
				emit(text[i:])
				i = len(text)
				continue
			}
			// 新标签块属于其后的文本：先把前一音节落盘。
			flush()
			cur.tags += text[i+1 : i+close]
			i += close + 1
		case text[i] == '\\' && i+1 < len(text) && (text[i+1] == 'n' || text[i+1] == 'N'):
			emit("\n")
			i += 2
		case text[i] == '\\' && i+1 < len(text) && text[i+1] == 'h':
			emit(" ") // ASS 硬空格 \h
			i += 2
		default:
			emit(string(text[i]))
			i++
		}
	}
	flush()

	var syls []Syllable
	cursor := base
	for _, ch := range chunks {
		kind, durCS, hasK := parseKTag(ch.tags)
		if !hasK {
			// 无 \k 的引导/间隔文本：零时长占位，不参与高亮计时。
			syls = append(syls, Syllable{
				Text: ch.text, Tags: ch.tags, Kind: KindNone,
				Start: cursor, End: cursor, Index: len(syls),
			})
			continue
		}
		if off, ok := parseKT(ch.tags); ok {
			cursor = base + time.Duration(off)*10*time.Millisecond
		}
		dur := time.Duration(durCS) * 10 * time.Millisecond
		syls = append(syls, Syllable{
			Text: ch.text, Tags: ch.tags, Kind: kind,
			Start: cursor, End: cursor + dur, Duration: dur, Index: len(syls),
		})
		cursor += dur
	}
	return plain.String(), syls
}

func parseKTag(tags string) (KaraokeKind, int, bool) {
	m := kTagRE.FindStringSubmatch(tags)
	if m == nil {
		return KindNone, 0, false
	}
	cs, _ := strconv.Atoi(m[2])
	switch m[1] {
	case "f":
		return KindKF, cs, true
	case "o":
		return KindKO, cs, true
	default:
		return KindK, cs, true
	}
}

func parseKT(tags string) (int, bool) {
	m := ktRE.FindStringSubmatch(tags)
	if m == nil {
		return 0, false
	}
	v, _ := strconv.Atoi(m[1])
	return v, true
}
