package imsc

import (
	"io"
	"strings"
)

// node is a generic XML element that keeps source location and namespace
// prefixes so later validation can report the exact offending element.
type node struct {
	Local    string
	Prefix   string
	Line     int
	Attrs    map[string]xmlAttr
	Children []*node
	Text     string

	parent *node
	index  int
	path   string
}

type xmlAttr struct {
	Prefix string
	Value  string
}

// attr returns the value of the attribute whose local name matches, ignoring
// the namespace prefix.
func (n *node) attr(local string) (string, bool) {
	a, ok := n.Attrs[local]
	return a.Value, ok
}

func (n *node) qName() string {
	if n.Prefix == "" {
		return n.Local
	}
	return n.Prefix + ":" + n.Local
}

func (n *node) walk(fn func(*node)) {
	fn(n)
	for _, child := range n.Children {
		child.walk(fn)
	}
}

func (n *node) collect(local string) []*node {
	var out []*node
	n.walk(func(c *node) {
		if c.Local == local {
			out = append(out, c)
		}
	})
	return out
}

func (n *node) child(local string) *node {
	for _, child := range n.Children {
		if child.Local == local {
			return child
		}
	}
	return nil
}

// fullText concatenates the character data of the element and descendants,
// normalising whitespace runs to single spaces.
func (n *node) fullText() string {
	var parts []string
	var gather func(*node)
	gather = func(c *node) {
		if strings.TrimSpace(c.Text) != "" {
			parts = append(parts, strings.TrimSpace(c.Text))
		}
		for _, child := range c.Children {
			gather(child)
		}
	}
	gather(n)
	return strings.Join(parts, " ")
}

// buildPath computes a stable positional path such as
// tt[1]/body[1]/div[1]/p[2], including xml:id when present.
func (n *node) buildPath() string {
	var segments []string
	for cur := n; cur != nil; cur = cur.parent {
		seg := cur.qName() + "[" + itoa(cur.index+1) + "]"
		if id, ok := cur.attr("id"); ok && strings.TrimSpace(id) != "" {
			seg += "[id=" + strings.TrimSpace(id) + "]"
		}
		segments = append([]string{seg}, segments...)
	}
	return strings.Join(segments, "/")
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

// lineReader records the line number at every byte offset so XML token
// offsets can be mapped back to source lines.
type lineReader struct {
	r       io.Reader
	offset  int
	lineOff []int
}

func newLineReader(r io.Reader) *lineReader {
	return &lineReader{r: r, lineOff: []int{0}}
}

func (l *lineReader) Read(p []byte) (int, error) {
	n, err := l.r.Read(p)
	for i := 0; i < n; i++ {
		l.offset++
		if p[i] == '\n' {
			l.lineOff = append(l.lineOff, l.offset)
		}
	}
	return n, err
}

// lineAt maps a byte offset to its 1-based source line.
func (l *lineReader) lineAt(offset int64) int {
	off := int(offset)
	lo, hi := 0, len(l.lineOff)
	for lo < hi {
		mid := (lo + hi) / 2
		if l.lineOff[mid] <= off {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	return lo
}
