package aiffmeta

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
)

// RepackOptions controls which top-level chunks are omitted. COMM and SSND
// cannot be dropped because that would either remove required format data or
// change the audio payload.
type RepackOptions struct {
	Drop []string
}

// RepackReport describes a rewrite. Lost contains the decoded text, comment
// and marker values belonging to explicitly dropped chunks.
type RepackReport struct {
	InputBytes           int64      `json:"input_bytes"`
	OutputBytes          int64      `json:"output_bytes"`
	KeptChunks           int        `json:"kept_chunks"`
	DroppedChunks        []Chunk    `json:"dropped_chunks,omitempty"`
	Lost                 []LostItem `json:"lost,omitempty"`
	AudioBytes           int64      `json:"audio_bytes"`
	AudioSHA256          string     `json:"audio_sha256,omitempty"`
	TrailingBytesOmitted int64      `json:"trailing_bytes_omitted,omitempty"`
	Warnings             []Issue    `json:"warnings,omitempty"`
}

// Repack rewrites a valid FORM container in its original chunk order. Chunk
// payloads are streamed verbatim, so SSND audio and the SSND parameter block
// keep their exact bytes.
func (f *File) Repack(w io.Writer, opts RepackOptions) (*RepackReport, error) {
	if f == nil || f.source == nil {
		return nil, errors.New("source reader is unavailable")
	}
	if w == nil {
		return nil, errors.New("nil destination")
	}
	if f.FormTruncated {
		return nil, errors.New("cannot repack a truncated FORM container")
	}
	dropped, err := dropSet(opts.Drop)
	if err != nil {
		return nil, err
	}
	if _, ok := dropped["COMM"]; ok {
		return nil, errors.New("refusing to drop COMM: it is required by AIFF/AIFC")
	}
	if _, ok := dropped["SSND"]; ok {
		return nil, errors.New("refusing to drop SSND: the audio payload must be preserved")
	}
	if f.Format == FormatAIFC {
		if _, ok := dropped["FVER"]; ok {
			return nil, errors.New("refusing to drop FVER from an AIFC container")
		}
	}

	report := &RepackReport{
		InputBytes:           f.Size,
		TrailingBytesOmitted: f.TrailingBytes,
		Warnings:             append([]Issue(nil), f.Warnings...),
	}
	if f.SSND != nil {
		report.AudioBytes = f.SSND.AudioSize
		report.AudioSHA256, _, err = f.AudioDigest()
		if err != nil {
			return nil, err
		}
	}

	kept, removed, err := partitionChunks(f.Chunks, dropped)
	if err != nil {
		return nil, err
	}
	formSize, err := formSizeFor(kept)
	if err != nil {
		return nil, err
	}
	report.KeptChunks = len(kept)
	report.DroppedChunks = removed
	for _, chunk := range removed {
		report.Lost = append(report.Lost, lostItemsForChunk(f, chunk)...)
	}

	cw := &countingWriter{w: w}
	header := make([]byte, 12)
	copy(header[0:4], "FORM")
	binary.BigEndian.PutUint32(header[4:8], formSize)
	copy(header[8:12], string(f.Format))
	if err := writeFull(cw, header); err != nil {
		return nil, err
	}
	for _, chunk := range kept {
		if err := f.copyWholeChunk(cw, chunk); err != nil {
			return nil, err
		}
	}
	report.OutputBytes = cw.n
	return report, nil
}

// RepackToFile writes atomically through a temporary file in the destination
// directory, so an error or a drop refusal never replaces an existing file.
func (f *File) RepackToFile(path string, opts RepackOptions) (*RepackReport, error) {
	dir := filepath.Dir(path)
	temp, err := os.CreateTemp(dir, ".aiffmeta-*")
	if err != nil {
		return nil, err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)

	report, err := f.Repack(temp, opts)
	closeErr := temp.Close()
	if err != nil {
		return nil, err
	}
	if closeErr != nil {
		return nil, closeErr
	}
	if err := os.Rename(tempPath, path); err != nil {
		return nil, err
	}
	return report, nil
}

func dropSet(ids []string) (map[string]struct{}, error) {
	set := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if len(id) != 4 {
			return nil, fmt.Errorf("chunk id %q must be exactly four bytes", id)
		}
		set[id] = struct{}{}
	}
	return set, nil
}

func partitionChunks(chunks []Chunk, dropped map[string]struct{}) ([]Chunk, []Chunk, error) {
	kept := make([]Chunk, 0, len(chunks))
	removed := make([]Chunk, 0)
	for _, chunk := range chunks {
		if chunk.Truncated || chunk.StoredSize != int64(chunk.Size) {
			return nil, nil, fmt.Errorf("chunk %d %q is truncated; cannot repack", chunk.Index, chunk.ID)
		}
		if _, ok := dropped[chunk.ID]; ok {
			removed = append(removed, chunk)
			continue
		}
		kept = append(kept, chunk)
	}
	return kept, removed, nil
}

func formSizeFor(chunks []Chunk) (uint32, error) {
	size := int64(4) // form type
	for _, chunk := range chunks {
		size += 8 + chunk.PaddedSize()
	}
	if size > math.MaxUint32 {
		return 0, fmt.Errorf("FORM payload would be %d bytes, beyond the format limit", size)
	}
	return uint32(size), nil
}

func (f *File) copyWholeChunk(w io.Writer, chunk Chunk) error {
	header := make([]byte, 8)
	copy(header[0:4], chunk.ID)
	binary.BigEndian.PutUint32(header[4:8], chunk.Size)
	if err := writeFull(w, header); err != nil {
		return err
	}
	payload := io.NewSectionReader(f.source, chunk.DataOffset, int64(chunk.Size))
	if _, err := io.CopyN(w, payload, int64(chunk.Size)); err != nil {
		return fmt.Errorf("copy chunk %d %q: %w", chunk.Index, chunk.ID, err)
	}
	if chunk.Size&1 != 0 {
		pad := []byte{0}
		if chunk.DataEnd < f.Size {
			if _, err := f.source.ReadAt(pad, chunk.DataEnd); err != nil {
				return fmt.Errorf("read padding for chunk %q: %w", chunk.ID, err)
			}
		}
		if err := writeFull(w, pad); err != nil {
			return err
		}
	}
	return nil
}

type countingWriter struct {
	w io.Writer
	n int64
}

func (w *countingWriter) Write(p []byte) (int, error) {
	n, err := w.w.Write(p)
	w.n += int64(n)
	return n, err
}

func writeFull(w io.Writer, p []byte) error {
	for len(p) > 0 {
		n, err := w.Write(p)
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
		p = p[n:]
	}
	return nil
}
