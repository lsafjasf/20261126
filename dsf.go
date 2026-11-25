// Package main implements dsfmeta: a DSF (DSD Stream File) chunk parser
// plus a sample-accurate time axis for DSD64/DSD128 content, so tags and
// chapters land on real timestamps instead of a PCM-misread timeline.
//
// Only the DSD header bookkeeping fields (file size, metadata pointer)
// and the trailing ID3 metadata region are ever written; the DSD audio
// payload is treated as immutable.
package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// DSF chunk layout constants (all integers little-endian).
const (
	dsdChunkSize = 28 // "DSD " chunk is fixed at 28 bytes
	fmtChunkSize = 52 // "fmt " chunk is fixed at 52 bytes
)

// Info describes one parsed DSF file.
type Info struct {
	// DSD chunk
	DeclaredFileSize uint64 // total file size recorded in the DSD chunk
	MetadataOffset   uint64 // absolute offset of the ID3 tag, 0 = none

	// fmt chunk
	FormatVersion       uint32
	FormatID            uint32
	ChannelType         uint32
	ChannelNum          uint32
	SampleFreq          uint32 // DSD rate in Hz, e.g. 2822400 (DSD64)
	BitsPerSample       uint32 // 1 or 8
	SampleCount         uint64 // per-channel sample count from the header
	BlockSizePerChannel uint32 // bytes per channel per block, typically 4096

	// data chunk
	DataOffset uint64 // absolute offset of the first audio payload byte
	DataSize   uint64 // audio payload bytes (data chunk size minus 12-byte header)

	// RealFileSize is the actual on-disk size, used to validate bookkeeping.
	RealFileSize int64
}

// Parse reads the DSD, fmt and data chunk headers from r.
func Parse(r io.ReaderAt, realSize int64) (*Info, error) {
	info := &Info{RealFileSize: realSize}

	var dsd [dsdChunkSize]byte
	if _, err := r.ReadAt(dsd[:], 0); err != nil {
		return nil, fmt.Errorf("read DSD chunk: %w", err)
	}
	if string(dsd[0:4]) != "DSD " {
		return nil, errors.New(`not a DSF file: missing "DSD " chunk`)
	}
	if sz := binary.LittleEndian.Uint64(dsd[4:12]); sz != dsdChunkSize {
		return nil, fmt.Errorf("unexpected DSD chunk size %d", sz)
	}
	info.DeclaredFileSize = binary.LittleEndian.Uint64(dsd[12:20])
	info.MetadataOffset = binary.LittleEndian.Uint64(dsd[20:28])

	var fm [fmtChunkSize]byte
	if _, err := r.ReadAt(fm[:], dsdChunkSize); err != nil {
		return nil, fmt.Errorf("read fmt chunk: %w", err)
	}
	if string(fm[0:4]) != "fmt " {
		return nil, errors.New(`missing "fmt " chunk after DSD chunk`)
	}
	if sz := binary.LittleEndian.Uint64(fm[4:12]); sz != fmtChunkSize {
		return nil, fmt.Errorf("unexpected fmt chunk size %d", sz)
	}
	info.FormatVersion = binary.LittleEndian.Uint32(fm[12:16])
	info.FormatID = binary.LittleEndian.Uint32(fm[16:20])
	info.ChannelType = binary.LittleEndian.Uint32(fm[20:24])
	info.ChannelNum = binary.LittleEndian.Uint32(fm[24:28])
	info.SampleFreq = binary.LittleEndian.Uint32(fm[28:32])
	info.BitsPerSample = binary.LittleEndian.Uint32(fm[32:36])
	info.SampleCount = binary.LittleEndian.Uint64(fm[36:44])
	info.BlockSizePerChannel = binary.LittleEndian.Uint32(fm[44:48])

	var hdr [12]byte
	if _, err := r.ReadAt(hdr[:], dsdChunkSize+fmtChunkSize); err != nil {
		return nil, fmt.Errorf("read data chunk header: %w", err)
	}
	if string(hdr[0:4]) != "data" {
		return nil, errors.New(`missing "data" chunk after fmt chunk`)
	}
	dataChunkSize := binary.LittleEndian.Uint64(hdr[4:12])
	if dataChunkSize < 12 {
		return nil, fmt.Errorf("invalid data chunk size %d", dataChunkSize)
	}
	info.DataOffset = dsdChunkSize + fmtChunkSize + 12
	info.DataSize = dataChunkSize - 12
	if end := info.DataOffset + info.DataSize; int64(end) > realSize {
		return nil, fmt.Errorf("data chunk overruns file: end=%d size=%d", end, realSize)
	}
	if info.MetadataOffset != 0 && info.MetadataOffset < info.DataOffset+info.DataSize {
		return nil, fmt.Errorf("metadata offset %d overlaps audio payload", info.MetadataOffset)
	}
	return info, nil
}
