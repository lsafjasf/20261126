package vobsub

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"time"
)

// MissingAngleError is returned when the requested angle (context number)
// is not declared in the .idx file. It lists the available angles so the
// caller can surface a clear message instead of showing empty subtitles.
type MissingAngleError struct {
	Requested int
	Available []int
}

func (e *MissingAngleError) Error() string {
	avail := make([]string, len(e.Available))
	for i, a := range e.Available {
		avail[i] = fmt.Sprintf("%d", a)
	}
	return fmt.Sprintf("subtitle angle %d not available (declared angles: %s)",
		e.Requested, strings.Join(avail, ", "))
}

// EmptyStreamError is returned when the requested angle is declared but
// contains no cues.
type EmptyStreamError struct {
	Angle int
	Lang  string
}

func (e *EmptyStreamError) Error() string {
	return fmt.Sprintf("subtitle angle %d (%s) is declared but contains no cues", e.Angle, e.Lang)
}

// Switcher selects subtitle streams by angle and preserves playback
// position across switches.
type Switcher struct {
	idx *IDX
	sub io.ReaderAt // may be nil to skip offset validation

	angle   int
	current *Stream
}

// NewSwitcher builds a Switcher on the parsed idx. sub may be nil; when
// provided, every selected cue offset is validated against the .sub data.
func NewSwitcher(idx *IDX, sub io.ReaderAt) *Switcher {
	return &Switcher{idx: idx, sub: sub}
}

// Angle returns the currently selected angle, or -1 before the first switch.
func (s *Switcher) Angle() int { return s.angle }

// SwitchAngle selects the stream for angle (context number) and returns the
// cue active at position, so playback continues from the same timestamp in
// the new stream. The cue's filepos is validated against the .sub data when
// a ReaderAt was supplied.
//
// If the angle is not declared, a *MissingAngleError is returned; callers
// must surface it to the user rather than rendering empty subtitles.
func (s *Switcher) SwitchAngle(angle int, position time.Duration) (Entry, error) {
	st, ok := s.idx.Streams[angle]
	if !ok {
		return Entry{}, &MissingAngleError{Requested: angle, Available: s.idx.Indices()}
	}
	if len(st.Entries) == 0 {
		return Entry{}, &EmptyStreamError{Angle: angle, Lang: st.Lang}
	}
	s.angle = angle
	s.current = st
	return s.CueAt(position)
}

// CueAt returns the cue of the current stream active at position: the last
// cue whose timestamp is <= position (or the first cue if position precedes
// all cues). The cue offset is validated against the .sub data.
func (s *Switcher) CueAt(position time.Duration) (Entry, error) {
	if s.current == nil {
		return Entry{}, fmt.Errorf("no angle selected; call SwitchAngle first")
	}
	entries := s.current.Entries
	i := sort.Search(len(entries), func(i int) bool {
		return entries[i].Timestamp > position
	})
	if i == 0 {
		i = 1 // position before first cue: clamp to first
	}
	e := entries[i-1]
	if s.sub != nil {
		if err := ValidateOffset(s.sub, e.FilePos, s.current.Index); err != nil {
			return Entry{}, fmt.Errorf("angle %d cue at %s: %w",
				s.current.Index, formatTS(e.Timestamp), err)
		}
	}
	return e, nil
}

func formatTS(d time.Duration) string {
	ms := d.Milliseconds()
	return fmt.Sprintf("%02d:%02d:%02d:%03d",
		ms/3600000, ms/60000%60, ms/1000%60, ms%1000)
}
