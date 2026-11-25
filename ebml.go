package main

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
)

// EBML element IDs needed to rebuild Matroska track identity.
const (
	idEBML            = 0x1A45DFA3
	idSegment         = 0x18538067
	idTracks          = 0x1654AE6B
	idTrackEntry      = 0xAE
	idTrackNumber     = 0xD7
	idTrackUID        = 0x73C5
	idTrackType       = 0x83
	idFlagDefault     = 0x88
	idCodecID         = 0x86
	idCodecPrivate    = 0x63A2
	idLanguage        = 0x22B59C
	idLanguageIETF    = 0x22B59D
	idTags            = 0x1254C367
	idTag             = 0x7373
	idTargets         = 0x63C0
	idTagTrackUID     = 0x63C5
	idChapters        = 0x1043A770
	idEditionEntry    = 0x45B9
	idChapterAtom     = 0xB6
	idChapterTrack    = 0x8F
	idChapterTrackUID = 0x89
	idCues            = 0x1C53BB6B
	idCuePoint        = 0xBB
	idCueTrackPos     = 0xB7
	idCueTrack        = 0xF7
	idCluster         = 0x1F43B675
	idSimpleBlock     = 0xA3
	idBlockGroup      = 0xA0
	idBlock           = 0xA1
)

// masterIDs are elements whose payload is a sequence of child elements.
var masterIDs = map[uint32]bool{
	idEBML: true, idSegment: true, idTracks: true, idTrackEntry: true,
	idTags: true, idTag: true, idTargets: true,
	idChapters: true, idEditionEntry: true, idChapterAtom: true, idChapterTrack: true,
	idCues: true, idCuePoint: true, idCueTrackPos: true,
	idCluster: true, idBlockGroup: true,
}

// Element describes one parsed EBML element header.
type Element struct {
	ID           uint32
	HeaderOffset int64 // offset of the first ID byte
	DataOffset   int64 // offset of the payload
	DataSize     int64 // payload size in bytes
}

// readVINT reads an EBML variable-length integer. When strip is false the
// length marker bit is kept (element IDs, block track numbers); when true it
// is removed (element sizes). It returns the value and the byte width.
func readVINT(buf []byte, off int64, strip bool) (uint64, int, error) {
	if off >= int64(len(buf)) {
		return 0, 0, io.ErrUnexpectedEOF
	}
	first := buf[off]
	if first == 0 {
		return 0, 0, fmt.Errorf("invalid VINT at offset %d", off)
	}
	width := 1
	for mask := byte(0x80); mask > 0 && first&mask == 0; mask >>= 1 {
		width++
	}
	if width > 8 || off+int64(width) > int64(len(buf)) {
		return 0, 0, fmt.Errorf("invalid VINT width %d at offset %d", width, off)
	}
	value := uint64(first)
	if strip {
		value &= 0xFF >> uint(width)
	}
	for i := 1; i < width; i++ {
		value = value<<8 | uint64(buf[off+int64(i)])
	}
	return value, width, nil
}

// isUnknownSize reports whether a stripped size VINT is all ones.
func isUnknownSize(value uint64, width int) bool {
	return value == (uint64(1)<<(uint(width)*7))-1
}

// WalkFunc is invoked for every element. Returning false skips the element's
// children (only meaningful for master elements).
type WalkFunc func(el Element, path []uint32) bool

// walkElements iterates the EBML elements in buf[start:end], recursing into
// known master elements.
func walkElements(buf []byte, start, end int64, path []uint32, fn WalkFunc) error {
	off := start
	for off < end {
		rawID, idWidth, err := readVINT(buf, off, false)
		if err != nil {
			return err
		}
		id := uint32(rawID)
		sizeOff := off + int64(idWidth)
		size, sizeWidth, err := readVINT(buf, sizeOff, true)
		if err != nil {
			return err
		}
		dataOff := sizeOff + int64(sizeWidth)
		dataSize := int64(size)
		if isUnknownSize(size, sizeWidth) || dataOff+dataSize > end {
			dataSize = end - dataOff
		}
		el := Element{ID: id, HeaderOffset: off, DataOffset: dataOff, DataSize: dataSize}
		recurse := fn(el, path)
		if recurse && masterIDs[id] && dataSize > 0 {
			if err := walkElements(buf, dataOff, dataOff+dataSize, append(path, id), fn); err != nil {
				return err
			}
		}
		off = dataOff + dataSize
	}
	return nil
}

// uintValue decodes an unsigned-integer element payload.
func uintValue(buf []byte, el Element) uint64 {
	var value uint64
	for i := int64(0); i < el.DataSize && i < 8; i++ {
		value = value<<8 | uint64(buf[el.DataOffset+i])
	}
	return value
}

// stringValue decodes a string element payload.
func stringValue(buf []byte, el Element) string {
	return string(buf[el.DataOffset : el.DataOffset+el.DataSize])
}

// writeUint rewrites an unsigned-integer payload in place, big-endian,
// keeping the original byte length.
func writeUint(buf []byte, offset int64, length int, value uint64) error {
	if length > 8 {
		return fmt.Errorf("uint payload too wide: %d bytes", length)
	}
	if length < 8 && value >= uint64(1)<<(uint(length)*8) {
		return fmt.Errorf("value %d does not fit in %d bytes", value, length)
	}
	var tmp [8]byte
	binary.BigEndian.PutUint64(tmp[:], value)
	copy(buf[offset:offset+int64(length)], tmp[8-length:])
	return nil
}

// writeVINTSameWidth rewrites a VINT in place keeping its byte width and
// marker bit (used for block track numbers).
func writeVINTSameWidth(buf []byte, offset int64, width int, value uint64) error {
	if width != 1 {
		return fmt.Errorf("unsupported VINT rewrite width %d", width)
	}
	if value == 0 || value > 0x7F {
		return fmt.Errorf("track number %d does not fit in %d-byte VINT", value, width)
	}
	buf[offset] = 0x80 | byte(value)
	return nil
}

func readFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}

func writeFile(path string, data []byte) error {
	return os.WriteFile(path, data, 0o644)
}
