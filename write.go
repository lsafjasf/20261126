package main

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// payloadHash hashes exactly the DSD audio payload (data chunk content).
func payloadHash(f *os.File, info *Info) ([32]byte, error) {
	var sum [32]byte
	h := sha256.New()
	if _, err := f.Seek(int64(info.DataOffset), io.SeekStart); err != nil {
		return sum, err
	}
	if _, err := io.CopyN(h, f, int64(info.DataSize)); err != nil {
		return sum, err
	}
	copy(sum[:], h.Sum(nil))
	return sum, nil
}

// WriteMetadata replaces the trailing ID3 region with tag and fixes the
// two bookkeeping fields in the DSD chunk (file size, metadata pointer).
// Bytes [fmtEnd, DataOffset+DataSize) — the data chunk header and the
// entire DSD payload — are copied verbatim and hash-verified.
func WriteMetadata(path string, info *Info, tag []byte) error {
	src, err := os.Open(path)
	if err != nil {
		return err
	}
	defer src.Close()
	st, err := src.Stat()
	if err != nil {
		return err
	}

	before, err := payloadHash(src, info)
	if err != nil {
		return fmt.Errorf("hash payload before write: %w", err)
	}

	tmp, err := os.CreateTemp(filepath.Dir(path), ".dsfmeta-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() {
		tmp.Close()
		if _, statErr := os.Stat(tmpName); statErr == nil {
			os.Remove(tmpName)
		}
	}()

	// Header: copy [0, DataOffset) then patch the DSD chunk fields.
	if _, err := src.Seek(0, io.SeekStart); err != nil {
		return err
	}
	head := make([]byte, info.DataOffset)
	if _, err := io.ReadFull(src, head); err != nil {
		return fmt.Errorf("read header: %w", err)
	}
	metaOff := info.DataOffset + info.DataSize
	binary.LittleEndian.PutUint64(head[12:20], metaOff+uint64(len(tag)))
	binary.LittleEndian.PutUint64(head[20:28], metaOff)
	if _, err := tmp.Write(head); err != nil {
		return err
	}

	// Payload: stream verbatim.
	if _, err := io.CopyN(tmp, src, int64(info.DataSize)); err != nil {
		return fmt.Errorf("copy payload: %w", err)
	}
	// New metadata region replaces anything from the old MetadataOffset on.
	if _, err := tmp.Write(tag); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Chmod(st.Mode()); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}

	// Verify the payload survived byte-identical before committing.
	check, err := os.Open(tmpName)
	if err != nil {
		return err
	}
	after, err := payloadHash(check, info)
	check.Close()
	if err != nil {
		return fmt.Errorf("hash payload after write: %w", err)
	}
	if before != after {
		return fmt.Errorf("payload hash mismatch: refusing to replace %s", path)
	}
	return os.Rename(tmpName, path)
}
