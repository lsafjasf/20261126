package cea608

import (
	"io"
	"os"
)

// Result is an immutable-ish snapshot of a complete parse.
type Result struct {
	Text                string      `json:"text"`
	DisplayText         string      `json:"display_text"`
	Lines               []string    `json:"lines"`
	VisibleLines        []string    `json:"visible_lines"`
	Mode                Mode        `json:"mode"`
	Cues                []Cue       `json:"cues"`
	XDSPackets          []XDSPacket `json:"xds_packets"`
	XDS                 XDSState    `json:"xds"`
	Packets             []Packet    `json:"packets,omitempty"`
	FirstBadCode        uint16      `json:"first_bad_code,omitempty"`
	FirstBadControlCode uint16      `json:"first_bad_control_code,omitempty"`
	FirstBadControl     *BadControl `json:"first_bad_control,omitempty"`
	BadPairs            int         `json:"bad_pairs"`
	BadControls         int         `json:"bad_controls"`
	ParityErrors        int         `json:"parity_errors"`
	TruncatedPairs      int         `json:"truncated_pairs"`
	Recoveries          int         `json:"recoveries"`
}

// Parser is the streaming CEA-608 decoder.
type Parser = Decoder

// NewParser returns a streaming decoder.
func NewParser() *Parser { return New() }

// Result returns a snapshot of decoder output and diagnostics.
func (d *Decoder) Result() *Result {
	lines := d.Lines()
	visible := nonEmptyLines(lines)
	cues := append([]Cue(nil), d.Caption.Cues...)
	packets := append([]Packet(nil), d.Packets...)
	xdsPackets := d.XDS.Completed()
	for i := range xdsPackets {
		xdsPackets[i].Content = append([]byte(nil), xdsPackets[i].Content...)
		xdsPackets[i].Raw = append([]byte(nil), xdsPackets[i].Raw...)
	}
	xdsState := d.XDS
	xdsState.Packets = append([]XDSPacket(nil), xdsPackets...)
	xdsState.Content = append([]byte(nil), d.XDS.Content...)
	xdsState.raw = nil
	var firstBad *BadControl
	if d.Diagnostics.FirstBadControl != nil {
		bad := *d.Diagnostics.FirstBadControl
		firstBad = &bad
	}
	return &Result{
		Text:                d.Caption.Text,
		DisplayText:         d.Text(),
		Lines:               lines,
		VisibleLines:        visible,
		Mode:                d.Mode(),
		Cues:                cues,
		XDSPackets:          xdsPackets,
		XDS:                 xdsState,
		Packets:             packets,
		FirstBadCode:        d.Diagnostics.FirstBadCode,
		FirstBadControlCode: d.Diagnostics.FirstBadControlCode,
		FirstBadControl:     firstBad,
		BadPairs:            d.Diagnostics.BadPairs,
		BadControls:         d.Diagnostics.BadControls,
		ParityErrors:        d.Diagnostics.ParityErrors,
		TruncatedPairs:      d.Diagnostics.TruncatedPairs,
		Recoveries:          d.Diagnostics.Recoveries,
	}
}

// Parse decodes a complete CEA-608 byte stream. Malformed pairs are reported
// in the result rather than returned as a fatal error because recovery is a
// core parser requirement.
func Parse(data []byte) *Result {
	d := NewWithOptions(Options{RecordPackets: true})
	d.Decode(data)
	d.Finish()
	return d.Result()
}

// ParseString is Parse for string input.
func ParseString(data string) *Result { return Parse([]byte(data)) }

// ParseChecked behaves like Parse but preserves the conventional
// (result, error) call shape. The error is always nil for byte parsing because
// malformed CEA-608 is recoverable and represented in Result.
func ParseChecked(data []byte) (*Result, error) {
	return Parse(data), nil
}

// DecodeBytes is a descriptive alias for Parse.
func DecodeBytes(data []byte) *Result { return Parse(data) }

// ParseBytes is a descriptive alias for Parse.
func ParseBytes(data []byte) *Result { return Parse(data) }

// ParseReader reads all CEA-608 bytes from r and parses them.
func ParseReader(r io.Reader) (*Result, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	return Parse(data), nil
}

// ParseFile reads and parses a raw CEA-608 byte file.
func ParseFile(path string) (*Result, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return ParseReader(f)
}

// ParsePairs decodes already-framed pairs.
func ParsePairs(pairs ...Pair) *Result {
	d := NewWithOptions(Options{RecordPackets: true, Parity: ParityOff})
	var data []byte
	for _, pair := range pairs {
		data = append(data, pair.B1, pair.B2)
	}
	d.Decode(data)
	d.Finish()
	return d.Result()
}
