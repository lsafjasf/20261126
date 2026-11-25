package vobsub

import (
	"encoding/binary"
	"fmt"
	"io"
)

const (
	packStartCode   = 0x000001BA // MPEG-PS pack header
	privateStream1  = 0x000001BD // PES private_stream_1 (carries VobSub)
	subPictureBase  = 0x20       // subpicture sub-stream ids are 0x20+index
	maxPackScanSize = 256 * 1024 // bound for scanning a pack for its PES header
)

// ValidateOffset checks that the .sub data at pos is a plausible VobSub
// packet for the given stream index (context number). It verifies the
// MPEG-PS pack start code and the subpicture sub-stream id inside the
// first PES packet, guarding against stale or misaligned idx offsets.
func ValidateOffset(r io.ReaderAt, pos int64, streamIndex int) error {
	if pos < 0 {
		return fmt.Errorf("sub offset %d is negative", pos)
	}
	var head [4]byte
	if _, err := r.ReadAt(head[:], pos); err != nil {
		return fmt.Errorf("sub offset %d unreadable: %w", pos, err)
	}
	if binary.BigEndian.Uint32(head[:]) != packStartCode {
		return fmt.Errorf("sub offset %#x: missing pack start code 0x000001BA (got %#x)",
			pos, binary.BigEndian.Uint32(head[:]))
	}

	// Scan forward within the pack for the private_stream_1 PES header.
	chunk, err := readUpTo(r, pos+4, maxPackScanSize)
	if err != nil {
		return fmt.Errorf("sub offset %#x: %w", pos, err)
	}
	off := -1
	for i := 0; i+4 <= len(chunk); i++ {
		if binary.BigEndian.Uint32(chunk[i:]) == privateStream1 {
			off = i
			break
		}
	}
	if off < 0 {
		return fmt.Errorf("sub offset %#x: no PES private_stream_1 header within pack", pos)
	}

	// PES: 4B start code, 2B length, then header. For private_stream_1 the
	// MPEG-2 PES header is: 2B flags, 1B header_data_length, then payload.
	if off+9 > len(chunk) {
		return fmt.Errorf("sub offset %#x: truncated PES header", pos)
	}
	payloadOff := off + 9 + int(chunk[off+8])
	if payloadOff >= len(chunk) {
		return fmt.Errorf("sub offset %#x: truncated subpicture payload", pos)
	}
	subID := chunk[payloadOff]
	want := byte(subPictureBase + streamIndex)
	if subID != want {
		return fmt.Errorf("sub offset %#x: sub-stream id %#02x does not match stream index %d (want %#02x)",
			pos, subID, streamIndex, want)
	}
	return nil
}

// readUpTo reads up to n bytes starting at pos; short reads at EOF are fine.
func readUpTo(r io.ReaderAt, pos int64, n int) ([]byte, error) {
	buf := make([]byte, n)
	m, err := r.ReadAt(buf, pos)
	if err != nil && err != io.EOF {
		return nil, err
	}
	return buf[:m], nil
}
