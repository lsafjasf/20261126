package ssastyle

import (
	"fmt"
	"strconv"
	"strings"
)

// Color 是归一化后的 RGBA 颜色；A 采用常规约定（255=不透明，0=全透明）。
// 注意 ASS/SSA 的 &HAABBGGRR 中 AA 语义相反（00=不透明，FF=全透明），
// 解析时会翻转。
type Color struct {
	R, G, B, A uint8
}

func (c Color) canonical() string {
	return fmt.Sprintf("#%02X%02X%02X%02X", c.R, c.G, c.B, c.A)
}

// ParseColor 支持 ASS/SSA 的 &H[AA]BBGGRR[&] 与常见 #RRGGBB[AA]。
func ParseColor(raw string) (Color, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return Color{}, fmt.Errorf("empty color")
	}
	if strings.HasPrefix(s, "&H") || strings.HasPrefix(s, "&h") {
		h := strings.TrimPrefix(strings.TrimPrefix(s, "&H"), "&h")
		h = strings.TrimSuffix(h, "&")
		v, err := strconv.ParseUint(h, 16, 32)
		if err != nil {
			return Color{}, fmt.Errorf("invalid ASS color %q: %w", raw, err)
		}
		switch len(h) {
		case 6: // BBGGRR，默认不透明
			return Color{R: uint8(v & 0xff), G: uint8((v >> 8) & 0xff), B: uint8((v >> 16) & 0xff), A: 255}, nil
		case 8: // AABBGGRR，AA 中 00 为不透明
			aa := uint8((v >> 24) & 0xff)
			return Color{R: uint8(v & 0xff), G: uint8((v >> 8) & 0xff), B: uint8((v >> 16) & 0xff), A: 255 - aa}, nil
		default:
			return Color{}, fmt.Errorf("unsupported ASS color width %q", raw)
		}
	}
	if strings.HasPrefix(s, "#") {
		h := strings.TrimPrefix(s, "#")
		v, err := strconv.ParseUint(h, 16, 32)
		if err != nil {
			return Color{}, fmt.Errorf("invalid hex color %q: %w", raw, err)
		}
		switch len(h) {
		case 6:
			return Color{R: uint8((v >> 16) & 0xff), G: uint8((v >> 8) & 0xff), B: uint8(v & 0xff), A: 255}, nil
		case 8:
			return Color{R: uint8((v >> 24) & 0xff), G: uint8((v >> 16) & 0xff), B: uint8((v >> 8) & 0xff), A: uint8(v & 0xff)}, nil
		}
	}
	return Color{}, fmt.Errorf("unrecognized color %q", raw)
}

// ssaAlignmentToASS 把旧 SSA 对齐码换算为 ASS 小键盘 1..9。
// 旧 SSA：底行 1/2/3，顶行 5/6/7，中行 9/10/11；4、8 留空。
var ssaAlignmentToASS = map[int]int{
	1: 1, 2: 2, 3: 3,
	5: 7, 6: 8, 7: 9,
	9: 4, 10: 5, 11: 6,
}

func normalizeBool(raw string) (string, error) {
	s := strings.TrimSpace(strings.ToLower(raw))
	switch s {
	case "-1", "1", "true", "yes", "y", "on":
		return "true", nil
	case "0", "false", "no", "n", "off":
		return "false", nil
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		if f != 0 {
			return "true", nil
		}
		return "false", nil
	}
	return "", fmt.Errorf("invalid bool %q", raw)
}

func normalizeNumber(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return "", fmt.Errorf("invalid number %q", raw)
	}
	// -0 与 0 视为相同；用最短表示消除 20 / 20.0 差异。
	if f == 0 {
		f = 0
	}
	return strconv.FormatFloat(f, 'f', -1, 64), nil
}

// NormalizeValue 按字段类型把原始值归一为可比较的规范字符串。
// dialect 为 "ssa" 时会执行旧对齐码到 ASS 小键盘码的换算。
// 未知字段退化为去空白后的字符串比较。
func NormalizeValue(canonicalKey, raw, dialect string) (string, error) {
	spec, known := LookupField(canonicalKey)
	if !known {
		return strings.TrimSpace(raw), nil
	}
	switch spec.Kind {
	case KindBool:
		return normalizeBool(raw)
	case KindInt, KindFloat:
		return normalizeNumber(raw)
	case KindColor:
		c, err := ParseColor(raw)
		if err != nil {
			return "", err
		}
		return c.canonical(), nil
	case KindAlignment:
		n, err := normalizeNumber(raw)
		if err != nil {
			return "", err
		}
		f, _ := strconv.ParseFloat(n, 64)
		code := int(f)
		if dialect == DialectSSA {
			if mapped, ok := ssaAlignmentToASS[code]; ok {
				code = mapped
			}
		}
		return strconv.Itoa(code), nil
	default:
		return strings.TrimSpace(raw), nil
	}
}

// 方言常量。
const (
	DialectSSA  = "ssa"
	DialectASS  = "ass"
	DialectJSON = "json"
)
