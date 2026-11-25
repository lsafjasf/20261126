package audio

// Anchor records the sample position at the moment the stream was
// (re)negotiated, so playback position stays continuous and auditable
// across device reconfigurations.
type Anchor struct {
	Seq          int   // renegotiation sequence number
	SourceFrame  int64 // canonical position in source-rate frames
	DeviceFrame  int64 // frames rendered on the device config starting here
	SourceRate   int
	DeviceRate   int
	DeviceLayout ChannelLayout
}

// Position tracks the canonical playback position in source-rate frames
// while the render side advances in device-rate frames. Conversion uses an
// exact rational accumulator so no rounding drift accumulates over time.
type Position struct {
	sourceRate   int
	deviceRate   int
	sourceFrames int64
	deviceFrames int64 // frames rendered under the current device config
	totalDevice  int64 // frames rendered across all configs
	rem          int64 // rational remainder, numerator over deviceRate
	anchors      []Anchor
}

func NewPosition(sourceRate, deviceRate int, layout ChannelLayout) *Position {
	p := &Position{sourceRate: sourceRate, deviceRate: deviceRate}
	p.anchors = append(p.anchors, Anchor{
		Seq:          0,
		SourceRate:   sourceRate,
		DeviceRate:   deviceRate,
		DeviceLayout: layout,
	})
	return p
}

// Advance records that n device-rate frames have been rendered and folds
// them into the canonical source-frame position.
func (p *Position) Advance(n int64) {
	if n <= 0 {
		return
	}
	p.deviceFrames += n
	p.totalDevice += n
	num := p.rem + n*int64(p.sourceRate)
	p.sourceFrames += num / int64(p.deviceRate)
	p.rem = num % int64(p.deviceRate)
}

// Retune is called when the negotiated device parameters change. It closes
// the current anchor and opens a new one at the exact current position, so
// subsequent Advance calls convert with the new rate and the position
// remains sample-continuous (no crackle-inducing jumps).
func (p *Position) Retune(deviceRate int, layout ChannelLayout) {
	p.anchors = append(p.anchors, Anchor{
		Seq:          len(p.anchors),
		SourceFrame:  p.sourceFrames,
		DeviceFrame:  0,
		SourceRate:   p.sourceRate,
		DeviceRate:   deviceRate,
		DeviceLayout: layout,
	})
	p.deviceRate = deviceRate
	p.deviceFrames = 0
	p.rem = 0
}

// SourceFrame is the canonical playback position in source-rate frames.
func (p *Position) SourceFrame() int64 { return p.sourceFrames }

// DeviceFrames is the number of frames rendered under the current device
// configuration since the last Retune.
func (p *Position) DeviceFrames() int64 { return p.deviceFrames }

// TotalDeviceFrames is the number of frames rendered across all configs.
func (p *Position) TotalDeviceFrames() int64 { return p.totalDevice }

// Anchors returns the recorded renegotiation anchors.
func (p *Position) Anchors() []Anchor {
	out := make([]Anchor, len(p.anchors))
	copy(out, p.anchors)
	return out
}
