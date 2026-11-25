package vtt

import "fmt"

// Block is one top-level WebVTT block: a cue, a STYLE block, a REGION
// block or a NOTE block. Document.Blocks keeps the original file order so
// the file round-trips faithfully.
type Block interface {
	blockMarker()
}

// Cue is a single WebVTT cue.
//
// Identity is deliberately carried on three independent layers:
//
//   - ID: the cue identifier line, which is not required to be unique
//   - the time interval (Start/End), stored as milliseconds
//   - Text: the cue payload
//
// Two cues are true duplicates only when all three layers match.
type Cue struct {
	ID       string
	Start    int64
	End      int64
	Settings string
	Text     string
}

func (*Cue) blockMarker() {}

// Style holds a STYLE block body (without the "STYLE" marker line).
type Style struct {
	Text string
}

func (*Style) blockMarker() {}

// Region holds a REGION block body (without the "REGION" marker line).
type Region struct {
	Text string
}

func (*Region) blockMarker() {}

// Note holds a NOTE block body, including the leading "NOTE" token.
type Note struct {
	Text string
}

func (*Note) blockMarker() {}

// Document is a parsed WebVTT file.
type Document struct {
	// Header is the text following the mandatory "WEBVTT" signature on the
	// first line (usually empty, optionally a descriptive title).
	Header string
	Blocks []Block
}

// Cues returns every cue block in document order.
func (d *Document) Cues() []*Cue {
	cues := make([]*Cue, 0, len(d.Blocks))
	for _, block := range d.Blocks {
		if cue, ok := block.(*Cue); ok {
			cues = append(cues, cue)
		}
	}
	return cues
}

// Identity is the three-layer fingerprint of a cue. Two cues with the same
// Identity are genuine duplicates. Repeated cue identifiers alone never
// collapse into the same Identity, because time interval and text are part
// of the key.
type Identity struct {
	ID    string
	Start int64
	End   int64
	Text  string
}

// Identity returns the three-layer fingerprint of the cue. Settings are
// intentionally excluded: positioning/styling directives do not change
// what segment the cue represents.
func (c *Cue) Identity() Identity {
	return Identity{ID: c.ID, Start: c.Start, End: c.End, Text: c.Text}
}

func (c *Cue) String() string {
	return fmt.Sprintf("Cue(id=%q, %d-%d, %q)", c.ID, c.Start, c.End, c.Text)
}
