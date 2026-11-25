package smptett

import (
	"fmt"
	"image/color"
	"math"
	"strconv"
	"strings"
)

type styleSheet struct {
	styles map[string]map[string]string
}

func newStyleSheet() *styleSheet {
	return &styleSheet{styles: make(map[string]map[string]string)}
}

func (s *styleSheet) addStyle(n *xmlNode) {
	raw := attrLocalMap(n)
	id := raw["id"]
	if id == "" {
		return
	}
	s.styles[id] = raw
}

func (s *styleSheet) resolve(id string) map[string]string {
	return s.resolveWithStack(id, nil)
}

func (s *styleSheet) resolveWithStack(id string, stack []string) map[string]string {
	raw, ok := s.styles[id]
	if !ok {
		return nil
	}
	for _, active := range stack {
		if active == id {
			return map[string]string{"__warning": fmt.Sprintf("style reference cycle through %q", id)}
		}
	}
	stack = append(stack, id)
	merged := make(map[string]string)
	for _, ref := range strings.Fields(raw["style"]) {
		for key, value := range s.resolveWithStack(ref, stack) {
			merged[key] = value
		}
	}
	for key, value := range raw {
		merged[key] = value
	}
	return merged
}

func (s *styleSheet) rawForNode(n *xmlNode) map[string]string {
	merged := make(map[string]string)
	inline := attrLocalMap(n)
	for _, id := range strings.Fields(inline["style"]) {
		for key, value := range s.resolve(id) {
			merged[key] = value
		}
	}
	for key, value := range inline {
		merged[key] = value
	}
	return merged
}

func applyRawStyle(dst *Style, raw map[string]string, canvasWidth, canvasHeight int) {
	apply := func(key, value string) {
		switch key {
		case "color":
			if c, ok := parseColor(value); ok {
				dst.Color = c
			}
		case "backgroundColor":
			if c, ok := parseColor(value); ok {
				dst.Background = c
			}
		case "fontSize":
			if n, ok := parseFontSize(value, canvasHeight); ok {
				dst.FontSize = n
			}
		case "lineHeight":
			if n, ok := parseLineHeight(value, dst.FontSize); ok {
				dst.LineHeight = n
			}
		case "textAlign":
			switch value {
			case "start", "left":
				dst.TextAlign = "left"
			case "center":
				dst.TextAlign = "center"
			case "end", "right":
				dst.TextAlign = "right"
			}
		case "displayAlign":
			switch value {
			case "before", "center", "after":
				dst.DisplayAlign = value
			}
		case "writingMode":
			if value != "" {
				dst.WritingMode = value
			}
		case "display":
			if value != "" {
				dst.Display = value
			}
		case "visibility":
			if value != "" {
				dst.Visibility = value
			}
		case "showBackground":
			if value != "" {
				dst.ShowBackground = value
			}
		case "opacity":
			if n, err := strconv.ParseFloat(strings.TrimSuffix(value, "%"), 64); err == nil {
				if math.IsNaN(n) || math.IsInf(n, 0) {
					return
				}
				if strings.HasSuffix(value, "%") {
					n /= 100
				}
				dst.Opacity = math.Max(0, math.Min(1, n))
			}
		case "padding":
			if vals, ok := parseBox(value); ok {
				dst.Padding = vals
			}
		case "origin":
			if a, unitA, b, unitB, ok := parsePairMixed(value); ok {
				dst.OriginX, dst.OriginY = a, b
				dst.OriginXUnit, dst.OriginYUnit = unitA, unitB
				dst.HasOrigin = true
			}
		case "extent":
			if a, unitA, b, unitB, ok := parsePairMixed(value); ok {
				dst.ExtentW, dst.ExtentH = a, b
				dst.ExtentXUnit, dst.ExtentYUnit = unitA, unitB
				dst.HasExtent = true
			}
		}
	}
	order := []string{
		"fontSize", "lineHeight", "color", "backgroundColor", "textAlign",
		"displayAlign", "writingMode", "display", "visibility",
		"showBackground", "opacity", "padding", "origin", "extent",
	}
	for _, key := range order {
		if value, ok := raw[key]; ok {
			apply(key, strings.TrimSpace(value))
		}
	}
}

func parseFontSize(value string, canvasHeight int) (float64, bool) {
	n, unit, ok := parseLength(value)
	if !ok || n < 0 {
		return 0, false
	}
	switch unit {
	case "%":
		return float64(canvasHeight) * n / 100, true
	case "c":
		return float64(canvasHeight) / 15 * n, true
	case "em":
		return 32 * n, true
	default:
		return n, true
	}
}

func parseLineHeight(value string, fontSize float64) (float64, bool) {
	if value == "normal" {
		return 0, true
	}
	n, unit, ok := parseLength(value)
	if !ok || n < 0 {
		return 0, false
	}
	switch unit {
	case "%":
		return fontSize * n / 100, true
	case "":
		if n > 0 && n < 10 {
			return fontSize * n, true
		}
		return n, true
	default:
		return n, true
	}
}

func parseLength(value string) (float64, string, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, "", false
	}
	units := []string{"px", "em", "%", "c", "rh", "rw"}
	for _, unit := range units {
		if strings.HasSuffix(value, unit) {
			raw := strings.TrimSpace(strings.TrimSuffix(value, unit))
			n, err := strconv.ParseFloat(raw, 64)
			if err != nil || math.IsNaN(n) || math.IsInf(n, 0) {
				return 0, "", false
			}
			return n, unit, true
		}
	}
	n, err := strconv.ParseFloat(value, 64)
	if err != nil || math.IsNaN(n) || math.IsInf(n, 0) {
		return 0, "", false
	}
	return n, "", true
}

func parsePair(value string) (float64, float64, string, bool) {
	a, unitA, b, unitB, ok := parsePairMixed(value)
	if !ok || unitA != unitB {
		return 0, 0, "", false
	}
	return a, b, unitA, true
}

func parsePairMixed(value string) (float64, string, float64, string, bool) {
	parts := strings.Fields(value)
	if len(parts) != 2 {
		return 0, "", 0, "", false
	}
	a, unitA, okA := parseLength(parts[0])
	b, unitB, okB := parseLength(parts[1])
	if !okA || !okB {
		return 0, "", 0, "", false
	}
	return a, unitA, b, unitB, true
}

func parseBox(value string) ([4]float64, bool) {
	parts := strings.Fields(value)
	var out [4]float64
	if len(parts) == 0 || len(parts) > 4 {
		return out, false
	}
	vals := make([]float64, len(parts))
	for i, part := range parts {
		n, _, ok := parseLength(part)
		if !ok {
			return out, false
		}
		vals[i] = n
	}
	switch len(vals) {
	case 1:
		out = [4]float64{vals[0], vals[0], vals[0], vals[0]}
	case 2:
		out = [4]float64{vals[0], vals[1], vals[0], vals[1]}
	case 3:
		out = [4]float64{vals[0], vals[1], vals[2], vals[1]}
	case 4:
		out = [4]float64{vals[0], vals[1], vals[2], vals[3]}
	}
	return out, true
}

func parseColor(value string) (color.NRGBA, bool) {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" || value == "transparent" || value == "none" {
		return color.NRGBA{}, true
	}
	named := map[string]color.NRGBA{
		"black":   {A: 255},
		"white":   {R: 255, G: 255, B: 255, A: 255},
		"red":     {R: 255, A: 255},
		"green":   {G: 128, A: 255},
		"lime":    {G: 255, A: 255},
		"blue":    {B: 255, A: 255},
		"yellow":  {R: 255, G: 255, A: 255},
		"cyan":    {G: 255, B: 255, A: 255},
		"aqua":    {G: 255, B: 255, A: 255},
		"magenta": {R: 255, B: 255, A: 255},
		"fuchsia": {R: 255, B: 255, A: 255},
		"gray":    {R: 128, G: 128, B: 128, A: 255},
		"grey":    {R: 128, G: 128, B: 128, A: 255},
		"silver":  {R: 192, G: 192, B: 192, A: 255},
		"maroon":  {R: 128, A: 255},
		"navy":    {B: 128, A: 255},
		"olive":   {R: 128, G: 128, A: 255},
		"purple":  {R: 128, B: 128, A: 255},
		"teal":    {G: 128, B: 128, A: 255},
	}
	if c, ok := named[value]; ok {
		return c, true
	}
	if strings.HasPrefix(value, "#") {
		hex := strings.TrimPrefix(value, "#")
		switch len(hex) {
		case 3:
			hex = string([]byte{hex[0], hex[0], hex[1], hex[1], hex[2], hex[2]})
			fallthrough
		case 4:
			if len(hex) == 4 {
				hex = string([]byte{
					hex[0], hex[0], hex[1], hex[1],
					hex[2], hex[2], hex[3], hex[3],
				})
			} else {
				hex += "ff"
			}
		case 6:
			hex += "ff"
		case 8:
		default:
			return color.NRGBA{}, false
		}
		n, err := strconv.ParseUint(hex, 16, 32)
		if err != nil {
			return color.NRGBA{}, false
		}
		return color.NRGBA{
			R: uint8(n >> 24),
			G: uint8(n >> 16),
			B: uint8(n >> 8),
			A: uint8(n),
		}, true
	}
	for _, prefix := range []string{"rgba(", "rgb("} {
		if !strings.HasPrefix(value, prefix) || !strings.HasSuffix(value, ")") {
			continue
		}
		body := strings.TrimSuffix(strings.TrimPrefix(value, prefix), ")")
		parts := strings.FieldsFunc(body, func(r rune) bool {
			return r == ',' || r == ' ' || r == '/'
		})
		if len(parts) != 3 && len(parts) != 4 {
			return color.NRGBA{}, false
		}
		var channels [4]float64
		channels[3] = 1
		for i, p := range parts {
			p = strings.TrimSpace(p)
			if strings.HasSuffix(p, "%") {
				n, err := strconv.ParseFloat(strings.TrimSuffix(p, "%"), 64)
				if err != nil || math.IsNaN(n) || math.IsInf(n, 0) {
					return color.NRGBA{}, false
				}
				channels[i] = n / 100
			} else {
				n, err := strconv.ParseFloat(p, 64)
				if err != nil || math.IsNaN(n) || math.IsInf(n, 0) {
					return color.NRGBA{}, false
				}
				if i < 3 {
					channels[i] = n / 255
				} else {
					channels[i] = n
				}
			}
		}
		return color.NRGBA{
			R: clampByte(channels[0] * 255),
			G: clampByte(channels[1] * 255),
			B: clampByte(channels[2] * 255),
			A: clampByte(channels[3] * 255),
		}, true
	}
	return color.NRGBA{}, false
}

func clampByte(v float64) uint8 {
	if math.IsNaN(v) {
		return 0
	}
	return uint8(math.Max(0, math.Min(255, math.Round(v))))
}

func hiddenReasonsForStyle(style Style, text string) []string {
	var reasons []string
	if style.Display == "none" {
		reasons = append(reasons, "display-none")
	}
	if style.Visibility == "hidden" || style.Visibility == "collapse" {
		reasons = append(reasons, "visibility-hidden")
	}
	if style.Opacity <= 0 {
		reasons = append(reasons, "zero-opacity")
	}
	if style.WritingMode != "" && !strings.HasPrefix(style.WritingMode, "lr") && !strings.HasPrefix(style.WritingMode, "rl") {
		reasons = append(reasons, "unsupported-writing-mode")
	}
	if strings.TrimSpace(text) == "" {
		reasons = append(reasons, "empty-content")
	}
	return uniqueStrings(reasons)
}
