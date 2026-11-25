package cea608

import "strings"

type screen struct {
	cells [Rows][Cols]rune
}

func (s *screen) clear() { *s = screen{} }

func (s *screen) put(r, c int, ch rune) {
	if r >= 0 && r < Rows && c >= 0 && c < Cols {
		s.cells[r][c] = ch
	}
}

func (s *screen) backspace(r, c int) {
	if r < 0 || r >= Rows {
		return
	}
	if c > 0 && c <= Cols {
		s.cells[r][c-1] = 0
	}
}

func (s *screen) deleteToEnd(r, c int) {
	if r < 0 || r >= Rows {
		return
	}
	if c < 0 {
		c = 0
	}
	for col := c; col < Cols; col++ {
		s.cells[r][col] = 0
	}
}

func (s *screen) scrollUp(top, bottom int) {
	if top < 0 {
		top = 0
	}
	if bottom >= Rows {
		bottom = Rows - 1
	}
	for r := top; r < bottom; r++ {
		s.cells[r] = s.cells[r+1]
	}
	if bottom >= top {
		s.cells[bottom] = [Cols]rune{}
	}
}

func (s *screen) line(r int) string {
	if r < 0 || r >= Rows {
		return ""
	}
	row := make([]rune, Cols)
	for c, ch := range s.cells[r] {
		if ch == 0 {
			row[c] = ' '
		} else {
			row[c] = ch
		}
	}
	return strings.TrimRight(string(row), " ")
}

func (s *screen) lines() []string {
	out := make([]string, Rows)
	for r := range out {
		out[r] = s.line(r)
	}
	return out
}

func nonEmptyLines(lines []string) []string {
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}
