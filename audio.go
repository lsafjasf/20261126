package aiffmeta

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
)

// CopyAudio streams the sound bytes from SSND to w. The SSND offset/blockSize
// parameter block is not included; use Repack to retain it.
func (f *File) CopyAudio(w io.Writer) (int64, error) {
	r, err := f.AudioReader()
	if err != nil {
		return 0, err
	}
	if w == nil {
		return 0, errors.New("nil destination")
	}
	n, err := io.Copy(w, r)
	if err != nil {
		return n, fmt.Errorf("copy audio: %w", err)
	}
	return n, nil
}

// AudioReader returns a section reader over exactly the SSND sound bytes.
func (f *File) AudioReader() (*io.SectionReader, error) {
	if f == nil || f.SSND == nil {
		return nil, errors.New("file has no parseable SSND chunk")
	}
	if f.source == nil {
		return nil, errors.New("source reader is unavailable")
	}
	return io.NewSectionReader(f.source, f.SSND.AudioOffset, f.SSND.AudioSize), nil
}

// AudioDigest computes a streaming SHA-256 of the sound bytes.
func (f *File) AudioDigest() (string, int64, error) {
	if f == nil || f.SSND == nil {
		return "", 0, errors.New("file has no parseable SSND chunk")
	}
	hash := sha256.New()
	n, err := f.CopyAudio(hash)
	if err != nil {
		return "", n, err
	}
	return hex.EncodeToString(hash.Sum(nil)), n, nil
}
