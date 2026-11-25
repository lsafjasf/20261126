package ass

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

func parseClip(value string) (Clip, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return Clip{}, fmt.Errorf("empty clip")
	}

	scale := 1.0
	body := value
	if comma := strings.IndexByte(value, ','); comma >= 0 {
		first := strings.TrimSpace(value[:comma])
		rest := strings.TrimSpace(value[comma+1:])
		if n, err := strconv.ParseFloat(first, 64); err == nil && looksLikeDrawing(rest) {
			scale, body = n, rest
		}
	}

	if looksLikeDrawing(body) {
		points, bounds, err := parseDrawing(body)
		if err != nil {
			return Clip{}, err
		}
		if scale != 1 {
			for i := range points {
				points[i].X *= scale
				points[i].Y *= scale
			}
			bounds = scaleBounds(bounds, scale)
		}
		return Clip{
			Mode:     ClipNormal,
			IsVector: true,
			Points:   points,
			Bounds:   bounds,
		}, nil
	}

	nums := parseTagNumbers(body)
	if len(nums) != 4 {
		return Clip{}, fmt.Errorf("expected 4 rectangle coordinates or a vector path, got %q", value)
	}
	bounds := sortedBounds(nums[0], nums[1], nums[2], nums[3])
	return Clip{
		Mode:   ClipNormal,
		Bounds: bounds,
		Points: []Point{
			{X: bounds.MinX, Y: bounds.MinY},
			{X: bounds.MaxX, Y: bounds.MinY},
			{X: bounds.MaxX, Y: bounds.MaxY},
			{X: bounds.MinX, Y: bounds.MaxY},
		},
	}, nil
}

func looksLikeDrawing(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	switch s[0] {
	case 'm', 'M', 'n', 'N', 'l', 'L', 'b', 'B', 's', 'S', 'p', 'P', 'c', 'C':
		return true
	default:
		return false
	}
}

type drawToken struct {
	command byte
	number  float64
	isNum   bool
}

func parseDrawing(value string) ([]Point, Bounds, error) {
	tokens, err := tokenizeDrawing(value)
	if err != nil {
		return nil, Bounds{}, err
	}
	var points []Point
	bounds := emptyBounds()
	seenCommand := false
	for i := 0; i < len(tokens); {
		if tokens[i].isNum {
			return nil, Bounds{}, fmt.Errorf("drawing starts with a number near token %d", i)
		}
		cmd := tokens[i].command
		seenCommand = true
		i++
		var nums []float64
		for i < len(tokens) && tokens[i].isNum {
			nums = append(nums, tokens[i].number)
			i++
		}
		switch cmd {
		case 'm', 'n', 'l', 's', 'p':
			if len(nums) < 2 || len(nums)%2 != 0 {
				return nil, Bounds{}, fmt.Errorf("%c expects coordinate pairs", cmd)
			}
		case 'b':
			if len(nums) == 0 || len(nums)%6 != 0 {
				return nil, Bounds{}, fmt.Errorf("b expects groups of 6 coordinates")
			}
		case 'c':
			if len(nums) != 0 {
				return nil, Bounds{}, fmt.Errorf("c takes no coordinates")
			}
		default:
			return nil, Bounds{}, fmt.Errorf("unsupported drawing command %q", cmd)
		}
		for j := 0; j+1 < len(nums); j += 2 {
			p := Point{X: nums[j], Y: nums[j+1]}
			points = append(points, p)
			expandBounds(&bounds, p)
		}
	}
	if !seenCommand || len(points) == 0 {
		return nil, Bounds{}, fmt.Errorf("drawing has no points")
	}
	return points, bounds, nil
}

func tokenizeDrawing(value string) ([]drawToken, error) {
	var tokens []drawToken
	for i := 0; i < len(value); {
		c := value[i]
		if c == ' ' || c == '\t' || c == '\r' || c == '\n' || c == ',' {
			i++
			continue
		}
		if isCommand(c) {
			tokens = append(tokens, drawToken{command: byte(lowerASCII(c))})
			i++
			continue
		}
		start := i
		if value[i] == '+' || value[i] == '-' {
			i++
		}
		for i < len(value) && (value[i] >= '0' && value[i] <= '9' || value[i] == '.') {
			i++
		}
		if i < len(value) && (value[i] == 'e' || value[i] == 'E') {
			i++
			if i < len(value) && (value[i] == '+' || value[i] == '-') {
				i++
			}
			for i < len(value) && value[i] >= '0' && value[i] <= '9' {
				i++
			}
		}
		if start == i {
			return nil, fmt.Errorf("invalid drawing character %q", c)
		}
		n, err := strconv.ParseFloat(value[start:i], 64)
		if err != nil {
			return nil, fmt.Errorf("invalid drawing number %q", value[start:i])
		}
		tokens = append(tokens, drawToken{number: n, isNum: true})
	}
	return tokens, nil
}

func isCommand(c byte) bool {
	switch lowerASCII(c) {
	case 'm', 'n', 'l', 'b', 's', 'p', 'c':
		return true
	default:
		return false
	}
}

func lowerASCII(c byte) byte {
	if c >= 'A' && c <= 'Z' {
		return c + ('a' - 'A')
	}
	return c
}

func emptyBounds() Bounds {
	return Bounds{
		MinX: math.Inf(1), MinY: math.Inf(1),
		MaxX: math.Inf(-1), MaxY: math.Inf(-1),
	}
}

func expandBounds(b *Bounds, p Point) {
	b.MinX = math.Min(b.MinX, p.X)
	b.MinY = math.Min(b.MinY, p.Y)
	b.MaxX = math.Max(b.MaxX, p.X)
	b.MaxY = math.Max(b.MaxY, p.Y)
}

func sortedBounds(x1, y1, x2, y2 float64) Bounds {
	return Bounds{
		MinX: math.Min(x1, x2),
		MinY: math.Min(y1, y2),
		MaxX: math.Max(x1, x2),
		MaxY: math.Max(y1, y2),
	}
}

func scaleBounds(b Bounds, scale float64) Bounds {
	return sortedBounds(b.MinX*scale, b.MinY*scale, b.MaxX*scale, b.MaxY*scale)
}
