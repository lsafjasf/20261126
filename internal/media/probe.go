// Package media probes synthetic media files using only the standard
// library. Files are self-describing: a small MRL1 container stores the
// codec tag, duration, frame rate, raw frame payloads and raw audio
// samples. Identity signals (frame hash, audio fingerprint) are computed
// from the actual file bytes, never from the file name.
package media

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
)

// Magic identifies the MRL1 container. Probing sniffs these bytes, so a
// file whose extension changed after migration still probes correctly.
const Magic = "MRL1"

// Probe holds the metadata extracted from one media file.
type Probe struct {
	Path             string `json:"path"`
	Codec            string `json:"codec"`
	DurationMS       int64  `json:"duration_ms"`
	FPSNum           int    `json:"fps_num"`
	FPSDen           int    `json:"fps_den"`
	FrameCount       int    `json:"frame_count"`
	SampleRate       int    `json:"sample_rate"`
	FrameHash        string `json:"frame_hash"`
	AudioFingerprint string `json:"audio_fingerprint"`
	SizeBytes        int64  `json:"size_bytes"`
}

// ErrNotMedia marks files that are not MRL1 containers.
var ErrNotMedia = errors.New("not an MRL1 media file")

const headerLen = 4 + 4 + 8 + 4 + 4 + 4 + 4 + 4

// ProbeFile parses one media file and computes its identity signals.
func ProbeFile(path string) (Probe, error) {
	f, err := os.Open(path)
	if err != nil {
		return Probe{}, err
	}
	defer f.Close()

	header := make([]byte, headerLen)
	if _, err := io.ReadFull(f, header); err != nil {
		return Probe{}, fmt.Errorf("%s: %w", path, ErrNotMedia)
	}
	if string(header[0:4]) != Magic {
		return Probe{}, fmt.Errorf("%s: %w", path, ErrNotMedia)
	}
	le := binary.LittleEndian
	p := Probe{
		Path:       path,
		Codec:      string(header[4:8]),
		DurationMS: int64(le.Uint64(header[8:16])),
		FPSNum:     int(le.Uint32(header[16:20])),
		FPSDen:     int(le.Uint32(header[20:24])),
		FrameCount: int(le.Uint32(header[24:28])),
		SampleRate: int(le.Uint32(header[28:32])),
	}

	audioLen := int(le.Uint32(header[32:36]))
	audio := make([]byte, audioLen)
	if _, err := io.ReadFull(f, audio); err != nil {
		return Probe{}, fmt.Errorf("%s: truncated audio: %w", path, err)
	}
	p.AudioFingerprint = FingerprintHex(Fingerprint(audio))

	frameHash := sha256.New()
	var lenBuf [4]byte
	for i := 0; i < p.FrameCount; i++ {
		if _, err := io.ReadFull(f, lenBuf[:]); err != nil {
			return Probe{}, fmt.Errorf("%s: truncated frame table: %w", path, err)
		}
		frame := make([]byte, le.Uint32(lenBuf[:]))
		if _, err := io.ReadFull(f, frame); err != nil {
			return Probe{}, fmt.Errorf("%s: truncated frame %d: %w", path, i, err)
		}
		frameHash.Write(frame)
	}
	p.FrameHash = fmt.Sprintf("%x", frameHash.Sum(nil))

	if info, err := f.Stat(); err == nil {
		p.SizeBytes = info.Size()
	}
	return p, nil
}

// Encode writes one MRL1 media file. Used by the fixture generator.
func Encode(path string, codec string, durationMS int64, fpsNum, fpsDen int,
	sampleRate int, audio []byte, frames [][]byte) error {

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	le := binary.LittleEndian
	header := make([]byte, headerLen)
	copy(header[0:4], Magic)
	copy(header[4:8], codec)
	le.PutUint64(header[8:16], uint64(durationMS))
	le.PutUint32(header[16:20], uint32(fpsNum))
	le.PutUint32(header[20:24], uint32(fpsDen))
	le.PutUint32(header[24:28], uint32(len(frames)))
	le.PutUint32(header[28:32], uint32(sampleRate))
	le.PutUint32(header[32:36], uint32(len(audio)))
	if _, err := f.Write(header); err != nil {
		return err
	}
	if _, err := f.Write(audio); err != nil {
		return err
	}
	var lenBuf [4]byte
	for _, frame := range frames {
		le.PutUint32(lenBuf[:], uint32(len(frame)))
		if _, err := f.Write(lenBuf[:]); err != nil {
			return err
		}
		if _, err := f.Write(frame); err != nil {
			return err
		}
	}
	return nil
}
