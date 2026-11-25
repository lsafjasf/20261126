package cea608

import "strings"

// Decoder consumes raw CEA-608 byte pairs. Decode may be called with arbitrary
// chunk boundaries; an incomplete pair is retained until the next call.
type Decoder struct {
	Options
	Diagnostics

	Caption CaptionState
	XDS     XDSState

	displayed screen
	stored    screen
	state     modeState

	pending      []byte
	inputLen     int
	pairIndex    int
	parityChosen bool
	parityWire   bool
	lastCode     uint16
	lastKind     PacketKind

	// Packets contains classified pairs when Options.RecordPackets is true.
	Packets []Packet
}

// New returns a decoder with automatic parity and recovery defaults.
func New() *Decoder {
	d := &Decoder{state: noneState{}}
	d.Options = Options{
		Parity:        ParityAuto,
		AutoRecover:   true,
		RecordPackets: true,
	}
	d.Caption.Mode = ModeNone
	return d
}

// NewDecoder is a descriptive alias for New.
func NewDecoder() *Decoder { return New() }

// NewWithOptions returns a decoder using caller-supplied options.
func NewWithOptions(options Options) *Decoder {
	d := New()
	d.Options = options
	if d.Options.Parity == 0 {
		d.Options.Parity = ParityAuto
	}
	if d.Options.DisableAutoRecover {
		d.Options.AutoRecover = false
	} else {
		d.Options.AutoRecover = true
	}
	return d
}

func (d *Decoder) ensure() {
	if d.state == nil {
		d.state = noneState{}
	}
}

// Reset clears caption, XDS, diagnostics and pending bytes. Options survive.
func (d *Decoder) Reset() {
	options := d.Options
	*d = Decoder{Options: options, state: noneState{}}
	d.Caption.Mode = ModeNone
}

// Mode returns the active caption mode.
func (d *Decoder) Mode() Mode {
	if d.state == nil {
		return ModeNone
	}
	return d.state.mode()
}

// Lines returns the 15 currently displayed rows.
func (d *Decoder) Lines() []string { return d.displayed.lines() }

// VisibleLines returns only non-empty displayed rows.
func (d *Decoder) VisibleLines() []string { return nonEmptyLines(d.Lines()) }

// Text returns trimmed non-empty displayed rows joined by newlines.
func (d *Decoder) Text() string { return strings.Join(d.VisibleLines(), "\n") }

// String renders the full 15-row display including blank rows.
func (d *Decoder) String() string { return strings.Join(d.Lines(), "\n") }

// Decode appends and processes as many complete pairs as possible.
func (d *Decoder) Decode(data []byte) {
	d.ensure()
	d.inputLen += len(data)
	d.pending = append(d.pending, data...)

	for len(d.pending) >= 2 {
		offset := d.inputLen - len(d.pending)
		raw1, raw2 := d.pending[0], d.pending[1]
		pair := normalizedPair(raw1, raw2)
		pair.Offset = offset
		pair.Index = d.pairIndex
		pair.PairIndex = d.pairIndex
		pair.ParityBad = !oddParity(raw1) || !oddParity(raw2)

		if d.Options.Parity == ParityAuto && !d.parityChosen {
			d.parityChosen = true
			d.parityWire = !pair.ParityBad
		}
		validateParity := d.Options.Parity == ParityWire ||
			(d.Options.Parity == ParityAuto && d.parityWire)

		if validateParity && pair.ParityBad {
			d.rejectBadParity(pair)
			d.advancePair(2)
			continue
		}

		if d.handlePair(pair) == recoverConsumedOne {
			d.advancePair(1)
			d.Diagnostics.Recoveries++
			continue
		}
		d.advancePair(2)
	}
}

type pairAction int

const (
	pairConsumed pairAction = iota
	recoverConsumedOne
)

func (d *Decoder) advancePair(n int) {
	if len(d.pending) < n {
		n = len(d.pending)
	}
	d.pending = d.pending[n:]
	if n == 2 {
		d.pairIndex++
	}
}

// Resync discards a lone leading byte left by a truncated two-byte pair.
func (d *Decoder) Resync() bool {
	if len(d.pending) == 1 {
		d.pending = d.pending[:0]
		d.Diagnostics.TruncatedPairs++
		d.Diagnostics.Recoveries++
		return true
	}
	return false
}

// Recover is an alias for Resync.
func (d *Decoder) Recover() bool { return d.Resync() }

// SkipPending is an alias for Resync.
func (d *Decoder) SkipPending() bool { return d.Resync() }

// DecodeSegment starts at a known pair boundary. It discards a partial pair
// left by an interrupted segment before decoding data.
func (d *Decoder) DecodeSegment(data []byte) {
	d.Resync()
	d.Decode(data)
}

// PendingByte returns the incomplete first byte and true when one is buffered.
func (d *Decoder) PendingByte() (byte, bool) {
	if len(d.pending) == 1 {
		return d.pending[0], true
	}
	return 0, false
}

// Finish records a trailing incomplete pair as truncation and discards it.
func (d *Decoder) Finish() {
	if d.XDS.Active {
		d.XDS.Abort()
	}
	if len(d.pending) == 1 {
		d.Diagnostics.TruncatedPairs++
	}
	d.pending = nil
}
