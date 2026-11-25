package adts

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

var (
	ErrTruncatedFrame = errors.New("adts: frame extends beyond input")
	ErrFrameOverlap   = errors.New("adts: frame length overlaps a later frame")
)

// Segment is a contiguous run of valid frames with the same Stream State.
type Segment struct {
	Index             int   `json:"index"`
	StartOffset       int64 `json:"startOffset"`
	EndOffset         int64 `json:"endOffset"`
	State             State `json:"state"`
	FrameCount        int   `json:"frameCount"`
	SampleCount       int64 `json:"sampleCount"`
	CumulativeSamples int64 `json:"cumulativeSamples"`
}

// BadFrame describes a range that could not be parsed as a complete, valid
// ADTS frame at a frame boundary. It can contain more than one corrupt frame
// if no valid frame boundary can be found before Length bytes are consumed.
type BadFrame struct {
	Offset int64  `json:"offset"`
	Length int64  `json:"length"`
	Reason string `json:"reason"`
}

// Report is the result of scanning an ADTS byte stream.
type Report struct {
	Segments     []Segment  `json:"segments"`
	BadFrames    []BadFrame `json:"badFrames"`
	Bytes        int64      `json:"bytes"`
	FrameCount   int        `json:"frameCount"`
	BadBytes     int64      `json:"badBytes"`
	TotalSamples int64      `json:"totalSamples"`
}

// Read consumes r and scans all ADTS frames. Malformed input is represented in
// Report.BadFrames; only an I/O error from r is returned.
func Read(r io.Reader) (Report, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return Report{}, err
	}
	return ParseBytes(data), nil
}

// ParseBytes scans an in-memory ADTS byte stream.
func ParseBytes(data []byte) Report {
	p := parser{
		data: data,
		report: Report{
			Segments:  make([]Segment, 0),
			BadFrames: make([]BadFrame, 0),
			Bytes:     int64(len(data)),
		},
	}
	p.scan()
	p.report.FrameCount = p.frameCount
	p.report.TotalSamples = p.totalSamples
	return p.report
}

// WriteJSON writes a report as indented JSON using the standard library.
func WriteJSON(w io.Writer, report Report) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(report)
}

type parser struct {
	data         []byte
	report       Report
	current      *segmentBuilder
	frameCount   int
	totalSamples int64
}

type segmentBuilder struct {
	start       int
	state       State
	frameCount  int
	sampleCount int64
}

type frameInfo struct {
	header Header
	end    int
}

func (p *parser) scan() {
	offset := 0
	for offset < len(p.data) {
		frame, err := p.frameAt(offset)
		if err == nil {
			p.acceptFrame(offset, frame)
			offset = frame.end
			continue
		}

		next := p.nextFrame(offset + 1)
		p.flushSegment(offset)
		badEnd := len(p.data)
		if next >= 0 {
			badEnd = next
		}
		p.recordBad(offset, badEnd, err.Error())
		offset = badEnd
	}
	p.flushSegment(len(p.data))
}

func (p *parser) frameAt(offset int) (frameInfo, error) {
	frame, err := p.rawFrameAt(offset)
	if err != nil {
		return frameInfo{}, err
	}

	end := frame.end
	if remaining := len(p.data) - end; remaining < 2 || SyncWordAt(p.data[end:]) {
		return frame, nil
	}

	// The declared end is not a frame boundary. If another structurally valid
	// frame starts inside this frame, frame_length is definitively wrong.
	// Otherwise the bytes after this frame are a bad region, not this frame.
	if p.hasFrameBefore(offset+1, end) {
		return frameInfo{}, fmt.Errorf(
			"%w: offset %d declares bytes through %d",
			ErrFrameOverlap, offset, end,
		)
	}
	return frame, nil
}

func (p *parser) rawFrameAt(offset int) (frameInfo, error) {
	header, err := ParseHeader(p.data[offset:])
	if err != nil {
		return frameInfo{}, err
	}

	end := offset + header.FrameLength
	if end > len(p.data) {
		return frameInfo{}, fmt.Errorf(
			"%w: offset %d declares %d bytes, only %d remain",
			ErrTruncatedFrame, offset, header.FrameLength, len(p.data)-offset,
		)
	}

	return frameInfo{header: header, end: end}, nil
}

func (p *parser) nextFrame(start int) int {
	for search := start; search+1 < len(p.data); {
		index := bytes.IndexByte(p.data[search:], 0xff)
		if index < 0 {
			return -1
		}
		candidate := search + index
		if _, err := p.rawFrameAt(candidate); err == nil {
			return candidate
		}
		search = candidate + 1
	}
	return -1
}

func (p *parser) hasFrameBefore(start, end int) bool {
	for search := start; search < end; {
		index := bytes.IndexByte(p.data[search:end], 0xff)
		if index < 0 {
			return false
		}
		candidate := search + index
		if _, err := p.rawFrameAt(candidate); err == nil {
			return true
		}
		search = candidate + 1
	}
	return false
}

func (p *parser) acceptFrame(offset int, frame frameInfo) {
	if p.current != nil && p.current.state != frame.header.State {
		p.flushSegment(offset)
	}
	if p.current == nil {
		p.current = &segmentBuilder{
			start: offset,
			state: frame.header.State,
		}
	}
	p.current.frameCount++
	p.current.sampleCount += frame.header.Samples()
	p.frameCount++
}

func (p *parser) flushSegment(end int) {
	if p.current == nil {
		return
	}
	p.totalSamples += p.current.sampleCount
	p.report.Segments = append(p.report.Segments, Segment{
		Index:             len(p.report.Segments),
		StartOffset:       int64(p.current.start),
		EndOffset:         int64(end),
		State:             p.current.state,
		FrameCount:        p.current.frameCount,
		SampleCount:       p.current.sampleCount,
		CumulativeSamples: p.totalSamples,
	})
	p.current = nil
}

func (p *parser) recordBad(start, end int, reason string) {
	length := end - start
	if length <= 0 {
		return
	}
	p.report.BadFrames = append(p.report.BadFrames, BadFrame{
		Offset: int64(start),
		Length: int64(length),
		Reason: reason,
	})
	p.report.BadBytes += int64(length)
}
