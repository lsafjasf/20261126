package cea608

import "time"

// Rows and Cols are the conventional CEA-608 character grid dimensions.
const (
	Rows = 15
	Cols = 32
)

// Mode is the active caption display mode.
type Mode int

const (
	ModeNone Mode = iota
	ModePopOn
	ModePaintOn
	ModeRollUp
	ModeText
)

// String returns the conventional mode name.
func (m Mode) String() string {
	switch m {
	case ModePopOn:
		return "pop-on"
	case ModePaintOn:
		return "paint-on"
	case ModeRollUp:
		return "roll-up"
	case ModeText:
		return "text"
	default:
		return "none"
	}
}

// ParityMode controls CEA-608 odd-parity validation.
type ParityMode int

const (
	// ParityAuto uses odd parity when the first pair is correctly protected,
	// and otherwise treats input as already-stripped seven-bit data.
	ParityAuto ParityMode = iota
	// ParityWire requires odd parity on every byte.
	ParityWire
	// ParityOff ignores the parity bit but still masks it during decoding.
	ParityOff
)

// Options configures a Decoder.
type Options struct {
	Parity ParityMode
	// RecordPackets retains every classified pair in Decoder.Packets.
	RecordPackets bool
	// AutoRecover rewinds an invalid control lead so that a missing second
	// byte does not permanently shift pair alignment.
	AutoRecover bool
	// DisableAutoRecover turns off one-byte rewind recovery. Explicit Resync
	// and DecodeSegment remain available.
	DisableAutoRecover bool
	// RewindOnBadLead assumes the second byte of an invalid low control pair
	// is actually the first byte of the next pair. It is off by default
	// because ordinary pair-framed streams should drop the whole bad pair.
	RewindOnBadLead bool
}

// Pair is one two-byte CEA-608 word.
type Pair struct {
	B1        byte `json:"b1"`
	B2        byte `json:"b2"`
	Raw1      byte `json:"raw1"`
	Raw2      byte `json:"raw2"`
	Offset    int  `json:"offset"`
	Index     int  `json:"index"`
	PairIndex int  `json:"pair_index"`
	ParityBad bool `json:"parity_bad,omitempty"`
}

// Code returns the normalized big-endian control code.
func (p Pair) Code() uint16 { return uint16(p.B1)<<8 | uint16(p.B2) }

// PacketKind identifies how one pair was interpreted.
type PacketKind string

const (
	KindNull       PacketKind = "null"
	KindCaption    PacketKind = "caption"
	KindControl    PacketKind = "control"
	KindPAC        PacketKind = "pac"
	KindStyle      PacketKind = "style"
	KindSpecial    PacketKind = "special"
	KindExtended   PacketKind = "extended"
	KindTab        PacketKind = "tab"
	KindXDS        PacketKind = "xds"
	KindXDSStart   PacketKind = "xds_start"
	KindXDSData    PacketKind = "xds_data"
	KindXDSEnd     PacketKind = "xds_end"
	KindBadControl PacketKind = "bad_control"
	KindBadParity  PacketKind = "bad_parity"
)

// Packet is a classified pair.
type Packet struct {
	Kind   PacketKind `json:"kind"`
	Pair   Pair       `json:"pair"`
	Name   string     `json:"name,omitempty"`
	Reason string     `json:"reason,omitempty"`
}

// BadControl describes the first invalid or unsupported control pair.
type BadControl struct {
	Code        uint16 `json:"code"`
	ControlCode uint16 `json:"control_code"`
	B1          byte   `json:"b1"`
	B2          byte   `json:"b2"`
	FirstByte   byte   `json:"first_byte"`
	SecondByte  byte   `json:"second_byte"`
	Offset      int    `json:"offset"`
	Index       int    `json:"index"`
	PairIndex   int    `json:"pair_index"`
	Reason      string `json:"reason"`
	ParityError bool   `json:"parity_error,omitempty"`
	Recovered   bool   `json:"recovered,omitempty"`
}

// XDSPacket is one completed Extended Data Services packet.
type XDSPacket struct {
	Class       int       `json:"class"`
	Type        int       `json:"type"`
	ClassCode   byte      `json:"class_code"`
	TypeCode    byte      `json:"type_code"`
	Content     []byte    `json:"content"`
	Raw         []byte    `json:"raw"`
	Checksum    byte      `json:"checksum"`
	ChecksumOK  bool      `json:"checksum_ok"`
	StartOffset int       `json:"start_offset"`
	EndOffset   int       `json:"end_offset"`
	ReceivedAt  time.Time `json:"-"`
}

// XDSState is state that never feeds the caption screen.
type XDSState struct {
	Active        bool        `json:"active"`
	HasHeader     bool        `json:"has_header"`
	Class         int         `json:"class"`
	Type          int         `json:"type"`
	ClassCode     byte        `json:"class_code"`
	TypeCode      byte        `json:"type_code"`
	Content       []byte      `json:"content"`
	Packets       []XDSPacket `json:"packets"`
	BadChecksums  int         `json:"bad_checksums"`
	Truncated     int         `json:"truncated_packets"`
	MaxPacketSize int         `json:"max_packet_size"`
	StartOffset   int         `json:"start_offset"`
	EndOffset     int         `json:"end_offset"`
	raw           []byte
}

// Cue is a completed pop-on caption. Text contains trimmed non-empty rows.
type Cue struct {
	Text  string    `json:"text"`
	Lines []string  `json:"lines"`
	At    time.Time `json:"at,omitempty"`
}

// CaptionState contains display-related state only.
type CaptionState struct {
	Mode     Mode   `json:"mode"`
	Text     string `json:"text"`
	Cues     []Cue  `json:"cues"`
	Row      int    `json:"row"`
	Col      int    `json:"col"`
	RollRows int    `json:"roll_rows"`
	BaseRow  int    `json:"base_row"`
}

// Diagnostics contains lossless parser recovery counters.
type Diagnostics struct {
	BadPairs            int         `json:"bad_pairs"`
	BadControls         int         `json:"bad_controls"`
	FirstBadCode        uint16      `json:"first_bad_code,omitempty"`
	FirstBadControlCode uint16      `json:"first_bad_control_code,omitempty"`
	FirstBadControl     *BadControl `json:"first_bad_control,omitempty"`
	TruncatedPairs      int         `json:"truncated_pairs"`
	Recoveries          int         `json:"recoveries"`
	ParityErrors        int         `json:"parity_errors"`
}
