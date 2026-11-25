package bwf

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

const (
	maxChunks    = 1_000_000
	maxMetaBytes = 64 << 20
	rf64DSSize   = 0xFFFFFFFF
)

type File struct {
	Path      string
	Container Container
	rf64Data  uint64
	rf64Riff  uint64
}

func Open(path string) (*File, error) {
	fh, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer fh.Close()
	info, err := fh.Stat()
	if err != nil {
		return nil, err
	}
	return readContainer(fh, path, info.Size())
}

func readContainer(r io.ReaderAt, path string, fileSize int64) (*File, error) {
	var head [12]byte
	if _, err := r.ReadAt(head[:], 0); err != nil {
		return nil, fmt.Errorf("read RIFF header: %w", err)
	}
	kind := string(head[0:4])
	if kind != "RIFF" && kind != "RF64" {
		return nil, fmt.Errorf("not RIFF/RF64: %q", kind)
	}
	form := string(head[8:12])
	declared := uint64(binary.LittleEndian.Uint32(head[4:8]))
	f := &File{
		Path: path,
		Container: Container{
			Kind:         kind,
			Form:         form,
			DeclaredSize: declared,
			FileSize:     fileSize,
		},
	}
	end := int64(8) + int64(declared)
	if declared == rf64DSSize && kind == "RF64" {
		end = fileSize
	}
	if end > fileSize {
		end = fileSize
	}
	off := int64(12)
	for n := 0; off+8 <= end && n < maxChunks; n++ {
		var hdr [8]byte
		if _, err := r.ReadAt(hdr[:], off); err != nil {
			return nil, fmt.Errorf("read chunk header at %d: %w", off, err)
		}
		id := chunkID(hdr[0:4])
		size := uint64(binary.LittleEndian.Uint32(hdr[4:8]))
		ch := Chunk{
			ID:           id,
			Offset:       off,
			DataOffset:   off + 8,
			DeclaredSize: size,
			Size:         size,
		}
		if id == "ds64" && kind == "RF64" && size >= 24 {
			var payload [24]byte
			if _, err := r.ReadAt(payload[:], off+8); err != nil {
				return nil, fmt.Errorf("read ds64 at %d: %w", off, err)
			}
			f.rf64Riff = binary.LittleEndian.Uint64(payload[0:8])
			f.rf64Data = binary.LittleEndian.Uint64(payload[8:16])
			if f.rf64Riff > 0 {
				f.Container.DeclaredSize = f.rf64Riff
			}
		}
		if id == "data" && kind == "RF64" && size == rf64DSSize {
			ch.Placeholder = true
			if f.rf64Data > 0 {
				ch.Size = f.rf64Data
			}
		}
		if id == "data" && kind == "RIFF" && size == rf64DSSize {
			ch.Placeholder = true
		}
		payloadEnd := ch.DataOffset + int64(ch.Size)
		if payloadEnd > fileSize || payloadEnd < ch.DataOffset {
			ch.Truncated = true
			if fileSize > ch.DataOffset {
				ch.Size = uint64(fileSize - ch.DataOffset)
			} else {
				ch.Size = 0
			}
			payloadEnd = ch.DataOffset + int64(ch.Size)
		}
		f.Container.Chunks = append(f.Container.Chunks, ch)
		next := payloadEnd
		if next&1 == 1 {
			next++
		}
		if next <= off {
			return nil, fmt.Errorf("non-advancing chunk %q at %d", id, off)
		}
		off = next
	}
	if off < end {
		f.Container.Chunks = append(f.Container.Chunks, Chunk{
			ID:     "<trailing>",
			Offset: off,
			Size:   uint64(end - off),
		})
	}
	return f, nil
}

func (f *File) Chunk(id string) *Chunk {
	for i := range f.Container.Chunks {
		if f.Container.Chunks[i].ID == id {
			return &f.Container.Chunks[i]
		}
	}
	return nil
}

func (f *File) Payload(chunk *Chunk, limit uint64) ([]byte, error) {
	fh, err := os.Open(f.Path)
	if err != nil {
		return nil, err
	}
	defer fh.Close()
	n := chunk.Size
	if limit > 0 && n > limit {
		n = limit
	}
	if n == 0 {
		return nil, nil
	}
	buf := make([]byte, n)
	if _, err := fh.ReadAt(buf, chunk.DataOffset); err != nil {
		return nil, err
	}
	return buf, nil
}

func (f *File) PayloadFull(chunk *Chunk) ([]byte, error) {
	if chunk.Size > maxMetaBytes {
		return nil, fmt.Errorf("metadata chunk %q is too large: %d bytes", chunk.ID, chunk.Size)
	}
	return f.Payload(chunk, 0)
}

func chunkID(b []byte) string {
	if len(b) != 4 {
		return fmt.Sprintf("%x", b)
	}
	var sb strings.Builder
	for _, c := range b {
		if c >= 0x20 && c <= 0x7e {
			sb.WriteByte(c)
		} else {
			fmt.Fprintf(&sb, "\\x%02x", c)
		}
	}
	return sb.String()
}

func checkChunkBounds(f *File) error {
	if f.Container.Form != "WAVE" {
		return errors.New("RIFF form is not WAVE")
	}
	return nil
}
