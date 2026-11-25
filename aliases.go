package cea608

// NewCEA608 is a domain-prefixed constructor alias.
func NewCEA608() *Decoder { return New() }

// NewCEA608Parser is a domain-prefixed constructor alias.
func NewCEA608Parser() *Decoder { return New() }

// ParseCEA608 is a domain-prefixed parse alias.
func ParseCEA608(data []byte) *Result { return Parse(data) }

// DecodeCEA608 is a domain-prefixed decode alias.
func DecodeCEA608(data []byte) *Result { return Parse(data) }

// ParseXDS decodes only XDS packets from a CEA-608 byte stream.
func ParseXDS(data []byte) []XDSPacket {
	d := NewWithOptions(Options{Parity: ParityAuto, RecordPackets: false})
	d.Decode(data)
	d.Finish()
	return d.XDSPackets()
}

// StartXDS forces the XDS state machine active; the next pair is the header.
func (d *Decoder) StartXDS() {
	d.XDS.Active = true
	d.XDS.HasHeader = false
}

// Parse decodes data and returns a snapshot. The receiver remains usable;
// call Finish after the final chunk to record a trailing incomplete pair.
func (d *Decoder) Parse(data []byte) *Result {
	d.Decode(data)
	return d.Result()
}

// ProcessPair feeds one normalized or wire byte pair.
func (d *Decoder) ProcessPair(b1, b2 byte) { d.Decode([]byte{b1, b2}) }

// DecodePair is an alias for ProcessPair.
func (d *Decoder) DecodePair(b1, b2 byte) { d.ProcessPair(b1, b2) }

// Feed is a streaming alias for Decode.
func (d *Decoder) Feed(data []byte) { d.Decode(data) }

// XDSPackets returns completed XDS packets.
func (d *Decoder) XDSPackets() []XDSPacket { return d.XDS.Completed() }

// XDSStatePtr returns the independent XDS state.
func (d *Decoder) XDSStatePtr() *XDSState { return &d.XDS }

// CaptionStatePtr returns the independent caption state.
func (d *Decoder) CaptionStatePtr() *CaptionState { return &d.Caption }

// CaptionText returns the accumulated clean caption text.
func (d *Decoder) CaptionText() string { return d.Caption.Text }

// FirstBad is a short alias for the first bad-control diagnostic.
func (d *Decoder) FirstBad() *BadControl { return d.Diagnostics.FirstBadControl }

// BadControl returns the first bad-control diagnostic. It is a method alias;
// the exported field remains available as FirstBadControl.
func (d *Decoder) BadControl() *BadControl { return d.Diagnostics.FirstBadControl }

// GetFirstBadControl returns the first bad-control diagnostic.
func (d *Decoder) GetFirstBadControl() *BadControl { return d.Diagnostics.FirstBadControl }

// GetFirstBadControlCode returns the first bad-control code, or zero.
func (d *Decoder) GetFirstBadControlCode() uint16 {
	return d.Diagnostics.FirstBadControlCode
}
