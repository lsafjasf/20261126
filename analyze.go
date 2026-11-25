package mp3timeline

import (
	"fmt"
	"io"
	"os"
)

// AnalyzeFile parses an MP3 file and builds a stable presentation timeline.
func AnalyzeFile(path string) (*Timeline, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	t, err := Analyze(f)
	if t != nil {
		t.Path = path
	}
	return t, err
}

// Analyze parses from the beginning of r. It seeks to determine the size and
// leaves the read position unspecified.
func Analyze(r io.ReadSeeker) (*Timeline, error) {
	size, err := r.Seek(0, io.SeekEnd)
	if err != nil {
		return nil, fmt.Errorf("mp3timeline: determine size: %w", err)
	}
	if size <= 0 {
		return nil, fmt.Errorf("mp3timeline: empty input")
	}
	t := &Timeline{FileSize: size}

	id3v2End, err := readID3v2End(r, size)
	if err != nil {
		return nil, err
	}
	t.ID3v2Bytes = id3v2End
	t.ID3v1Bytes = readID3v1Size(r, size)
	t.AudioEnd = size - t.ID3v1Bytes
	if t.AudioEnd <= id3v2End {
		return nil, ErrNoMPEGFrames
	}

	first, header, err := findFirstHeader(r, id3v2End, t.AudioEnd)
	if err != nil {
		return nil, err
	}
	t.FirstMPEGOffset = first
	t.Format = publicFormat(header)

	firstRead := maxInt(header.frameSize, 1024)
	firstData, _ := readBytes(r, first, minInt(firstRead, int(t.AudioEnd-first)))
	_, rel, hasVBR := findVBRHeader(firstData, header)
	if hasVBR {
		x, g := parseXingAt(firstData, rel, first+int64(rel))
		if x != nil {
			t.Xing = x
			t.Gapless = g
		}
		t.AudioStart = findNextAudioHeader(r, first+4, t.AudioEnd, header)
		if t.AudioStart <= 0 {
			t.AudioStart = t.AudioEnd
			t.Warnings = append(t.Warnings,
				"VBR header frame found but no following audio frame was located")
		}
	} else {
		t.AudioStart = first
	}

	frames, resyncs, truncated := scanFrames(r, t.AudioStart, t.AudioEnd, header)
	t.Frames = frames
	t.Resyncs = resyncs
	t.Truncated = truncated
	if truncated {
		t.Warnings = append(t.Warnings, "final MPEG frame extends past the audio data boundary")
	}
	if resyncs > 0 {
		t.Warnings = append(t.Warnings, fmt.Sprintf("resynchronized %d time(s) while scanning frames", resyncs))
	}

	t.finish()
	return t, nil
}

func (t *Timeline) finish() {
	spf := int64(t.Format.SamplesPerFrame)
	scanFrames := int64(len(t.Frames))
	scanRaw := scanFrames * spf

	if t.Xing != nil && t.Xing.HasFrames {
		declared := int64(t.Xing.FrameCount)
		if !t.Truncated && scanFrames > 0 && declared == scanFrames+1 {
			t.Warnings = append(t.Warnings,
				"Xing/Info frame count includes the metadata frame; using scanned audio count")
		} else if declared != scanFrames && scanFrames > 0 && !t.Truncated {
			t.Warnings = append(t.Warnings,
				fmt.Sprintf("Xing/Info declares %d frames, scan found %d", declared, scanFrames))
		}
	}

	scanUsable := scanFrames > 0 && !t.Truncated
	if scanUsable {
		t.Source = SourceFrameScan
		t.AudioFrames = scanFrames
		t.RawSamples = scanRaw
	} else if t.Xing != nil && t.Xing.HasFrames {
		declared := int64(t.Xing.FrameCount)
		t.Source = SourceXingDeclared
		if !t.Truncated && scanFrames > 0 && declared == scanFrames+1 {
			declared--
		}
		t.AudioFrames = declared
		t.RawSamples = declared * spf
		if scanFrames == 0 {
			t.Warnings = append(t.Warnings,
				"frame scan unavailable; seek offsets are unavailable, duration uses Xing/Info")
		}
	} else {
		t.AudioFrames = scanFrames
		t.RawSamples = scanRaw
		if t.Truncated {
			t.Source = SourceFrameScan
			t.Warnings = append(t.Warnings,
				"truncated frame scan and no Xing/Info frame count; duration is a lower bound")
		}
	}

	if t.Gapless != nil && t.Gapless.Present {
		t.EncoderDelay = t.Gapless.EncoderDelay
		t.TailPadding = t.Gapless.TailPadding
	}
	t.PresentationSamples = t.RawSamples - int64(t.EncoderDelay) - int64(t.TailPadding)
	if t.PresentationSamples < 0 {
		t.Warnings = append(t.Warnings,
			"encoder delay plus tail padding exceeds stream samples; presentation clamped to zero")
		t.PresentationSamples = 0
	}
}

func publicFormat(h mpegHeader) Format {
	return Format{
		MPEGVersion:     h.version.String(),
		Layer:           h.layer,
		SampleRate:      h.sampleRate,
		Channels:        h.channels,
		SamplesPerFrame: h.samplesPerFrame,
	}
}

func readID3v2End(r io.ReadSeeker, size int64) (int64, error) {
	var header [10]byte
	n, err := readAt(r, 0, header[:])
	if err != nil && n < 10 {
		return 0, nil
	}
	if string(header[0:3]) != "ID3" {
		return 0, nil
	}
	if header[3] < 2 || header[3] > 4 {
		return 0, fmt.Errorf("mp3timeline: unsupported ID3v2 version 2.%d", header[3])
	}
	sz := int64(header[6]&0x7f)<<21 |
		int64(header[7]&0x7f)<<14 |
		int64(header[8]&0x7f)<<7 |
		int64(header[9]&0x7f)
	total := int64(10) + sz
	if header[3] == 4 && header[5]&0x10 != 0 {
		total += 10
	}
	if total > size {
		return 0, fmt.Errorf("mp3timeline: ID3v2 tag extends past file")
	}
	return total, nil
}

func readID3v1Size(r io.ReadSeeker, size int64) int64 {
	if size < 128 {
		return 0
	}
	var tag [3]byte
	if n, _ := readAt(r, size-128, tag[:]); n == 3 && string(tag[:]) == "TAG" {
		return 128
	}
	return 0
}

func findFirstHeader(r io.ReadSeeker, start, end int64) (int64, mpegHeader, error) {
	if start < 0 {
		start = 0
	}
	if h, err := parseHeaderAt(r, start); err == nil && start+int64(h.frameSize) <= end {
		return start, h, nil
	}
	pos, h, ok := findCompatibleHeader(r, start, minInt64(end, start+1024*1024), nil)
	if !ok {
		return 0, mpegHeader{}, ErrNoMPEGFrames
	}
	return pos, h, nil
}

func findNextAudioHeader(r io.ReadSeeker, start, end int64, base mpegHeader) int64 {
	limit := minInt64(end, start+1024*1024)
	pos, _, ok := findCompatibleHeader(r, start, limit, &base)
	if !ok {
		return 0
	}
	return pos
}

func findCompatibleHeader(r io.ReadSeeker, start, end int64, base *mpegHeader) (int64, mpegHeader, bool) {
	if start < 4 {
		start = 4
	}
	const chunkSize = 64 * 1024
	for chunkStart := start; chunkStart < end; chunkStart += chunkSize {
		chunkEnd := minInt64(chunkStart+chunkSize+3, end)
		buf, _ := readBytes(r, chunkStart, int(chunkEnd-chunkStart))
		for i := 0; i+4 <= len(buf); i++ {
			h, err := parseMPEGHeader(buf[i : i+4])
			if err != nil || chunkStart+int64(i)+int64(h.frameSize) > end {
				continue
			}
			if base != nil && !base.compatible(h) {
				continue
			}
			pos := chunkStart + int64(i)
			if base != nil && !hasFollowingHeader(r, pos, h, end) {
				continue
			}
			return pos, h, true
		}
	}
	return 0, mpegHeader{}, false
}

func hasFollowingHeader(r io.ReadSeeker, pos int64, h mpegHeader, end int64) bool {
	next := pos + int64(h.frameSize)
	if next >= end {
		return true
	}
	n, err := parseHeaderAt(r, next)
	return err == nil && h.compatible(n)
}

func parseHeaderAt(r io.ReadSeeker, pos int64) (mpegHeader, error) {
	var b [4]byte
	n, err := readAt(r, pos, b[:])
	if err != nil || n < 4 {
		return mpegHeader{}, fmt.Errorf("mp3timeline: short read at %d", pos)
	}
	return parseMPEGHeader(b[:])
}

func scanFrames(r io.ReadSeeker, start, end int64, base mpegHeader) ([]Frame, int, bool) {
	if start < 0 || start >= end {
		return nil, 0, false
	}
	var frames []Frame
	pos := start
	resyncs := 0
	truncated := false
	for pos+4 <= end {
		h, err := parseHeaderAt(r, pos)
		if err != nil || !base.compatible(h) {
			next, nh, ok := findCompatibleHeader(r, pos+1, minInt64(end, pos+1024*1024), &base)
			if !ok {
				truncated = pos < end
				break
			}
			pos = next
			h = nh
			resyncs++
		}
		if pos+int64(h.frameSize) > end {
			truncated = true
			break
		}
		frames = append(frames, Frame{
			Number:         len(frames),
			Offset:         pos,
			Size:           h.frameSize,
			Bitrate:        h.bitrate,
			SampleRate:     h.sampleRate,
			Samples:        h.samplesPerFrame,
			Padding:        h.padding,
			RawStartSample: int64(len(frames)) * int64(h.samplesPerFrame),
		})
		pos += int64(h.frameSize)
	}
	return frames, resyncs, truncated
}

func readBytes(r io.ReadSeeker, pos int64, length int) ([]byte, error) {
	if length <= 0 {
		return nil, nil
	}
	buf := make([]byte, length)
	n, err := readAt(r, pos, buf)
	return buf[:n], err
}

func readAt(r io.ReadSeeker, pos int64, buf []byte) (int, error) {
	if _, err := r.Seek(pos, io.SeekStart); err != nil {
		return 0, err
	}
	return io.ReadFull(r, buf)
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}
