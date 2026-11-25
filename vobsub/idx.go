// Package vobsub parses VobSub (.idx/.sub) files and selects subtitle
// streams by angle (context) while preserving playback position.
package vobsub

import (
	"bufio"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Entry is one subtitle cue declared in the .idx file.
type Entry struct {
	Timestamp time.Duration
	FilePos   int64
}

// Stream is a single subtitle stream (one angle/context) declared by an
// "id: <lang>, index: <n>" line in the .idx file.
type Stream struct {
	Lang    string
	Index   int
	Entries []Entry // sorted by Timestamp
}

// IDX is the parsed result of a VobSub .idx file.
type IDX struct {
	Streams map[int]*Stream // keyed by stream index (context number)
	Size    string
	Org     string
}

// ParseIDX parses a VobSub .idx file. Entries are attached to the stream
// declared by the most recent "id:" line, mirroring the VobSub format.
func ParseIDX(r io.Reader) (*IDX, error) {
	idx := &IDX{Streams: make(map[int]*Stream)}
	var cur *Stream
	lineNo := 0

	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		lineNo++
		line := strings.TrimSpace(sc.Text())
		switch {
		case strings.HasPrefix(line, "id:"):
			st, err := parseIDLine(line)
			if err != nil {
				return nil, fmt.Errorf("idx line %d: %w", lineNo, err)
			}
			if _, dup := idx.Streams[st.Index]; dup {
				return nil, fmt.Errorf("idx line %d: duplicate stream index %d", lineNo, st.Index)
			}
			idx.Streams[st.Index] = st
			cur = st
		case strings.HasPrefix(line, "timestamp:"):
			if cur == nil {
				return nil, fmt.Errorf("idx line %d: timestamp before any id: declaration", lineNo)
			}
			e, err := parseTimestampLine(line)
			if err != nil {
				return nil, fmt.Errorf("idx line %d: %w", lineNo, err)
			}
			cur.Entries = append(cur.Entries, e)
		case strings.HasPrefix(line, "size:"):
			idx.Size = strings.TrimSpace(strings.TrimPrefix(line, "size:"))
		case strings.HasPrefix(line, "org:"):
			idx.Org = strings.TrimSpace(strings.TrimPrefix(line, "org:"))
		}
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("reading idx: %w", err)
	}
	if len(idx.Streams) == 0 {
		return nil, fmt.Errorf("idx declares no subtitle streams")
	}
	for _, st := range idx.Streams {
		sort.Slice(st.Entries, func(i, j int) bool {
			return st.Entries[i].Timestamp < st.Entries[j].Timestamp
		})
	}
	return idx, nil
}

// parseIDLine parses "id: zh, index: 0".
func parseIDLine(line string) (*Stream, error) {
	body := strings.TrimSpace(strings.TrimPrefix(line, "id:"))
	parts := strings.Split(body, ",")
	if len(parts) != 2 {
		return nil, fmt.Errorf("malformed id line %q", line)
	}
	lang := strings.TrimSpace(parts[0])
	kv := strings.SplitN(strings.TrimSpace(parts[1]), ":", 2)
	if len(kv) != 2 || strings.TrimSpace(kv[0]) != "index" {
		return nil, fmt.Errorf("malformed id line %q: missing index", line)
	}
	n, err := strconv.Atoi(strings.TrimSpace(kv[1]))
	if err != nil || n < 0 {
		return nil, fmt.Errorf("malformed stream index in %q", line)
	}
	return &Stream{Lang: lang, Index: n}, nil
}

// parseTimestampLine parses "timestamp: 00:01:02:500, filepos: 000001000".
func parseTimestampLine(line string) (Entry, error) {
	var e Entry
	body := strings.TrimSpace(strings.TrimPrefix(line, "timestamp:"))
	parts := strings.Split(body, ",")
	if len(parts) != 2 {
		return e, fmt.Errorf("malformed timestamp line %q", line)
	}
	ts, err := parseTimestamp(strings.TrimSpace(parts[0]))
	if err != nil {
		return e, err
	}
	kv := strings.SplitN(strings.TrimSpace(parts[1]), ":", 2)
	if len(kv) != 2 || strings.TrimSpace(kv[0]) != "filepos" {
		return e, fmt.Errorf("malformed timestamp line %q: missing filepos", line)
	}
	pos, err := strconv.ParseInt(strings.TrimSpace(kv[1]), 16, 64)
	if err != nil {
		return e, fmt.Errorf("malformed filepos in %q", line)
	}
	e.Timestamp = ts
	e.FilePos = pos
	return e, nil
}

// parseTimestamp parses "hh:mm:ss:mmm".
func parseTimestamp(s string) (time.Duration, error) {
	f := strings.Split(s, ":")
	if len(f) != 4 {
		return 0, fmt.Errorf("malformed timestamp %q", s)
	}
	n := make([]int, 4)
	for i, part := range f {
		v, err := strconv.Atoi(part)
		if err != nil {
			return 0, fmt.Errorf("malformed timestamp %q", s)
		}
		n[i] = v
	}
	return time.Duration(n[0])*time.Hour +
		time.Duration(n[1])*time.Minute +
		time.Duration(n[2])*time.Second +
		time.Duration(n[3])*time.Millisecond, nil
}

// Indices returns the declared stream indices (context numbers) in order.
func (idx *IDX) Indices() []int {
	out := make([]int, 0, len(idx.Streams))
	for i := range idx.Streams {
		out = append(out, i)
	}
	sort.Ints(out)
	return out
}
