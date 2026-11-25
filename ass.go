package asskaraoke

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Color 是 ASS 的 &HAABBGGRR 颜色（Alpha=0 不透明，255 全透明）。
type Color struct {
	R, G, B, A uint8
}

// Style 对应 [V4+ Styles] 中的一行，只保留卡拉 OK 渲染需要的字段。
type Style struct {
	Name           string
	FontName       string
	FontSize       float64
	Primary        Color // 普通填充色（未唱）
	Secondary      Color // 卡拉 OK 已唱填充色
	Outline        Color
	Shadow         Color
	Bold, Italic   bool
	OutlineWidth   float64
	ShadowWidth    float64
	Spacing        float64
	ScaleX, ScaleY float64
	HasPrimary     bool
	HasSecondary   bool
}

// KaraokeKind 是音节高亮方式。
type KaraokeKind int

const (
	KindNone KaraokeKind = iota
	KindK                // \k ：音节开始瞬间整体变色
	KindKF               // \kf ：音节持续期间从左到右扫过
	KindKO               // \ko ：先描边变色，开始瞬间整体填充变色
)

// Syllable 是一个卡拉 OK 音节。Start/End 为相对于 Dialogue 起点的媒体时间。
type Syllable struct {
	Text     string
	Tags     string // 作用于该音节的原始 override 标签块（不含花括号）
	Kind     KaraokeKind
	Start    time.Duration
	End      time.Duration
	Duration time.Duration // \k 给出的时长（厘秒换算）
	Index    int
}

// Dialogue 是一条对话事件及其解析出的音节。
type Dialogue struct {
	Layer   int
	Start   time.Duration // 媒体时间轴上的绝对起点
	End     time.Duration
	Style   string
	Name    string
	MarginL int
	MarginR int
	MarginV int
	Text    string // 原始 Text（含标签）
	Plain   string // 去掉所有标签、还原换行/硬空格后的纯文本
	Syls    []Syllable
}

// Document 是解析后的 ASS 字幕。
type Document struct {
	Styles   map[string]*Style
	Events   []*Dialogue
	PlayResX int
	PlayResY int
}

func NewDocument() *Document {
	return &Document{Styles: make(map[string]*Style)}
}

// ParseASS 解析完整 ASS 脚本文本。不依赖任何第三方字幕库。
func ParseASS(data string) (*Document, error) {
	doc := NewDocument()
	section := ""
	var styleCols []string
	var eventCols []string

	for _, raw := range strings.Split(normalizeBOM(data), "\n") {
		line := strings.TrimRight(raw, "\r")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, ";") {
			continue
		}
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			section = strings.ToLower(trimmed[1 : len(trimmed)-1])
			continue
		}
		colon := strings.Index(line, ":")
		if colon < 0 {
			continue
		}
		kind := strings.TrimSpace(line[:colon])
		payload := line[colon+1:]

		switch {
		case section == "script info" && kind == "PlayResX":
			doc.PlayResX = atoiSafe(strings.TrimSpace(payload))
		case section == "script info" && kind == "PlayResY":
			doc.PlayResY = atoiSafe(strings.TrimSpace(payload))
		case section == "v4+ styles" && kind == "Format":
			styleCols = splitCols(payload)
		case section == "v4+ styles" && kind == "Style":
			if err := doc.parseStyle(splitPayload(payload, styleCols)); err != nil {
				return nil, err
			}
		case section == "events" && kind == "Format":
			eventCols = splitCols(payload)
		case section == "events" && (kind == "Dialogue" || kind == "Comment"):
			if kind == "Comment" {
				continue
			}
			d, err := parseDialogue(splitPayload(payload, eventCols))
			if err != nil {
				return nil, err
			}
			doc.Events = append(doc.Events, d)
		}
	}
	return doc, nil
}

func (d *Document) parseStyle(fields map[string]string) error {
	s := &Style{
		Name:         strings.TrimSpace(fields["Name"]),
		FontName:     strings.TrimSpace(fields["Fontname"]),
		FontSize:     atofSafe(fields["Fontsize"]),
		OutlineWidth: atofSafe(fields["Outline"]),
		ShadowWidth:  atofSafe(fields["Shadow"]),
		Spacing:      atofSafe(fields["Spacing"]),
		ScaleX:       atofDefault(fields["Scalex"], 100),
		ScaleY:       atofDefault(fields["Scaley"], 100),
		Bold:         atoiSafe(fields["Bold"]) != 0,
		Italic:       atoiSafe(fields["Italic"]) != 0,
	}
	if c, ok := parseASSColor(fields["PrimaryColour"]); ok {
		s.Primary, s.HasPrimary = c, true
	}
	if c, ok := parseASSColor(fields["SecondaryColour"]); ok {
		s.Secondary, s.HasSecondary = c, true
	}
	s.Outline, _ = parseASSColor(fields["OutlineColour"])
	s.Shadow, _ = parseASSColor(fields["BackColour"])
	if s.ScaleX == 0 {
		s.ScaleX = 100
	}
	if s.ScaleY == 0 {
		s.ScaleY = 100
	}
	d.Styles[s.Name] = s
	return nil
}

func parseDialogue(fields map[string]string) (*Dialogue, error) {
	start, err := ParseASSTime(strings.TrimSpace(fields["Start"]))
	if err != nil {
		return nil, fmt.Errorf("bad Start: %w", err)
	}
	end, err := ParseASSTime(strings.TrimSpace(fields["End"]))
	if err != nil {
		return nil, fmt.Errorf("bad End: %w", err)
	}
	text := fields["Text"]
	d := &Dialogue{
		Layer:   atoiSafe(fields["Layer"]),
		Start:   start,
		End:     end,
		Style:   strings.TrimSpace(fields["Style"]),
		Name:    strings.TrimSpace(fields["Name"]),
		MarginL: atoiSafe(fields["MarginL"]),
		MarginR: atoiSafe(fields["MarginR"]),
		MarginV: atoiSafe(fields["MarginV"]),
		Text:    text,
	}
	d.Plain, d.Syls = tokenizeKaraoke(text, start)
	return d, nil
}

func splitCols(payload string) []string {
	cols := strings.Split(payload, ",")
	for i := range cols {
		cols[i] = strings.TrimSpace(cols[i])
	}
	return cols
}

// splitPayload 按 Format 列名映射字段；Text 是最后一列，需保留其中所有逗号。
func splitPayload(payload string, cols []string) map[string]string {
	out := make(map[string]string, len(cols))
	if len(cols) == 0 {
		return out
	}
	lastCol := len(cols) - 1
	parts := strings.SplitN(payload, ",", len(cols))
	for i, col := range cols {
		if i < len(parts) {
			out[col] = parts[i]
		}
	}
	_ = lastCol
	return out
}

func normalizeBOM(s string) string {
	return strings.TrimPrefix(s, "\ufeff")
}

func atoiSafe(s string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(s))
	return n
}

func atofSafe(s string) float64 {
	f, _ := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return f
}

func atofDefault(s string, def float64) float64 {
	f := atofSafe(s)
	if f == 0 {
		return def
	}
	return f
}
