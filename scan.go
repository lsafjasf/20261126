package ac3select

import (
	"errors"
	"io"
	"strings"
)

// Options controls scanning behavior.
type Options struct {
	// StrictCRC treats a frame with a non-zero CRC residual as a bad frame.
	// CRC metadata is still calculated and exposed when this is false.
	StrictCRC bool
}

// Read consumes r and scans all AC-3 frames. An I/O error is returned; AC-3
// frame errors are represented in Report.BadFrames.
func Read(r io.Reader) (Report, error) {
	return ReadWithOptions(r, Options{})
}

// ReadWithOptions consumes r and scans using opts.
func ReadWithOptions(r io.Reader, opts Options) (Report, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return Report{}, err
	}
	return ParseBytesWithOptions(data, opts), nil
}

// ParseBytes scans an in-memory AC-3 elementary stream with default options.
func ParseBytes(data []byte) Report {
	return ParseBytesWithOptions(data, Options{})
}

// ParseBytesWithOptions scans data using opts.
func ParseBytesWithOptions(data []byte, opts Options) Report {
	report := Report{
		Bytes:       int64(len(data)),
		Segments:    []ProgramSegment{},
		BadFrames:   []BadFrame{},
		Frames:      []Frame{},
		SampleCount: 0,
	}

	length := int64(len(data))
	pos := int64(0)
	regionStart := int64(0)
	lastReason := ""
	lastKind := ""

	flushGap := func(end int64, recovered int64) {
		if end <= regionStart {
			return
		}
		reason := lastReason
		if reason == "" {
			reason = "byte range is not positioned at an AC-3 synchronization frame"
		}
		kind := lastKind
		if kind == "" {
			kind = "unsynchronized_gap"
		}
		bad := BadFrame{
			Offset:      regionStart,
			EndOffset:   end,
			Length:      end - regionStart,
			Kind:        kind,
			Reason:      reason,
			RecoveredAt: recovered,
		}
		bad.Index = len(report.BadFrames) + 1
		report.BadFrames = append(report.BadFrames, bad)
		regionStart = end
		lastReason = ""
		lastKind = ""
	}

	for pos < length {
		off := FindSync(data, pos)
		if off < 0 {
			flushGap(length, -1)
			break
		}

		frame, err := ParseFrame(data[off:])
		if err == nil && opts.StrictCRC && frame.Header.CRC != nil && !frame.Header.CRC.Valid {
			err = errors.New("ac3: frame CRC residual is non-zero")
		}

		if err == nil {
			flushGap(off, off)
			frame.Index = len(report.Frames)
			frame.Header.Offset = off
			report.Frames = append(report.Frames, frame)

			next := off + int64(frame.Header.FrameSize)
			pos = next
			regionStart = next
			lastReason = ""
			lastKind = ""
			continue
		}

		size := frame.Header.FrameSize
		canTrustBoundary := size > 0 && off+int64(size) <= length
		if canTrustBoundary && isPostHeaderError(err) {
			flushGap(off, off)
			end := off + int64(size)
			bad := BadFrame{
				Index:        len(report.BadFrames) + 1,
				Offset:       off,
				EndOffset:    end,
				Length:       int64(size),
				Kind:         classifyError(err),
				Reason:       err.Error(),
				DeclaredSize: size,
				RecoveredAt:  end,
			}
			report.BadFrames = append(report.BadFrames, bad)
			pos = end
			regionStart = end
			lastReason = ""
			lastKind = ""
			continue
		}

		if lastReason == "" {
			lastReason = err.Error()
			lastKind = classifyError(err)
		}
		pos = off + 1
	}

	if regionStart < length {
		flushGap(length, -1)
	}

	report.FrameCount = len(report.Frames)
	report.BadCount = len(report.BadFrames)
	for _, bad := range report.BadFrames {
		report.BadBytes += bad.Length
	}
	report.Segments = buildSegments(report.Frames)
	report.SampleCount = int64(report.FrameCount) * SamplesPerFrame
	for _, segment := range report.Segments {
		report.DurationMS += segment.DurationMS
	}
	report.Selection = buildSelection(report.Segments)
	return report
}

func isPostHeaderError(err error) bool {
	return errors.Is(err, ErrCoupling) ||
		errors.Is(err, io.ErrUnexpectedEOF) ||
		strings.Contains(err.Error(), "frame CRC residual")
}

func classifyError(err error) string {
	switch {
	case errors.Is(err, ErrSyncWord):
		return "sync"
	case errors.Is(err, ErrSampleRateCode):
		return "sample_rate_code"
	case errors.Is(err, ErrFrameSizeCode):
		return "frame_size_code"
	case errors.Is(err, ErrUnsupportedEAC3):
		return "unsupported_eac3"
	case errors.Is(err, ErrBitstreamID):
		return "bitstream_id"
	case errors.Is(err, ErrFrameTruncated), errors.Is(err, io.ErrUnexpectedEOF):
		return "truncated"
	case errors.Is(err, ErrCoupling):
		return "coupling"
	case strings.Contains(err.Error(), "CRC"):
		return "crc"
	default:
		return "invalid_frame"
	}
}

// FindSync returns the offset of the next big-endian AC-3 sync word at or
// after start, or -1 if none exists.
func FindSync(data []byte, start int64) int64 {
	if start < 0 {
		start = 0
	}
	for i := int(start); i+1 < len(data); i++ {
		if data[i] == 0x0B && data[i+1] == 0x77 {
			return int64(i)
		}
	}
	return -1
}
