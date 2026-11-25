package wavfix

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
)

// Options controls conservative container repair. The zero value inserts a
// missing fact chunk and repairs G.711/fixed-frame WAV files.
type Options struct {
	// NoInsertMissingFact leaves a file without fact when fact is absent.
	NoInsertMissingFact bool

	// AllowOtherFixedFormats permits format tags beyond PCM, IEEE float,
	// A-law and μ-law when nBlockAlign is usable.
	AllowOtherFixedFormats bool

	// AllowPartialFrames uses floor(dataBytes/frameBytes) when data is not
	// frame aligned. The trailing bytes remain in data but cannot be timed by
	// fact; this is normally left false.
	AllowPartialFrames bool

	// StrictTruncation rejects chunks whose declared size exceeds the bytes
	// actually available. By default the chunk header is corrected to the
	// available payload and all available bytes are preserved.
	StrictTruncation bool
}

// Repair parses raw and returns a container-repaired copy.
func Repair(raw []byte) ([]byte, *Report, error) {
	return RepairWithOptions(raw, Options{})
}

// RepairWithOptions is Repair with explicit options.
func RepairWithOptions(raw []byte, options Options) ([]byte, *Report, error) {
	file, parseErr := Parse(raw)
	var report *Report
	if file != nil {
		report = file.BuildReport()
	}
	if parseErr != nil {
		repairableTruncation := errors.Is(parseErr, ErrTruncated) && !options.StrictTruncation
		if !repairableTruncation {
			return nil, report, parseErr
		}
	}

	if report.FrameBytes <= 0 {
		return nil, report, ErrBadFormat
	}
	if report.DataRemainderBytes != 0 && !options.AllowPartialFrames {
		return nil, report, fmt.Errorf("%w: %d remainder byte(s)", ErrMisaligned, report.DataRemainderBytes)
	}
	if !isKnownFixedFrameFormat(file.Format) && !options.AllowOtherFixedFormats {
		return nil, report, fmt.Errorf("%w: format tag 0x%04x", ErrUnsupported, file.Format.EffectiveTag)
	}
	if report.DataFrames > math.MaxUint32 {
		return nil, report, ErrFactOverflow
	}

	correctCount := uint32(report.DataFrames)
	factInserted := file.Fact == nil && !options.NoInsertMissingFact
	if file.Fact == nil && options.NoInsertMissingFact {
		return nil, report, fmt.Errorf("wavfix: fact is missing and insertion is disabled")
	}
	factChanged := factInserted || file.Fact == nil || !file.Fact.Valid || file.Fact.SampleCount != correctCount || file.Fact.Size != 4

	output, err := rewrite(file, correctCount, factInserted, factChanged)
	if err != nil {
		return nil, report, err
	}

	report.OutputSize = len(output)
	report.RepairedFactCount = correctCount
	report.FactInserted = factInserted
	report.FactChanged = factChanged
	return output, report, nil
}

func isKnownFixedFrameFormat(format *FormatChunk) bool {
	switch format.EffectiveTag {
	case FormatPCM, FormatFloat, FormatALaw, FormatMuLaw:
		return true
	default:
		return false
	}
}

func rewrite(file *File, count uint32, insertFact bool, replaceFact bool) ([]byte, error) {
	var output bytes.Buffer
	output.Grow(len(file.Raw) + 12)

	output.WriteString("RIFF")
	var riffSize [4]byte
	output.Write(riffSize[:])
	output.WriteString("WAVE")

	factDone := false
	writeFact := func() error {
		var header [8]byte
		copy(header[0:4], "fact")
		binary.LittleEndian.PutUint32(header[4:8], 4)
		if _, err := output.Write(header[:]); err != nil {
			return err
		}
		var body [4]byte
		binary.LittleEndian.PutUint32(body[:], count)
		_, err := output.Write(body[:])
		factDone = true
		return err
	}

	for _, chunk := range file.Chunks {
		switch chunk.ID {
		case "fact":
			if replaceFact {
				if err := writeFact(); err != nil {
					return nil, err
				}
			} else {
				output.Write(chunk.Raw)
				factDone = true
			}
		case "fmt ":
			output.Write(chunk.Raw)
			if insertFact && !factDone {
				if err := writeFact(); err != nil {
					return nil, err
				}
			}
		default:
			if chunk.Truncated {
				if err := writeChunkBytes(&output, chunk.ID, chunk.Payload); err != nil {
					return nil, err
				}
			} else {
				output.Write(chunk.Raw)
			}
		}
	}

	if insertFact && !factDone {
		if err := writeFact(); err != nil {
			return nil, err
		}
	}
	output.Write(file.Trailing)

	if uint64(output.Len()) > math.MaxUint32+8 {
		return nil, fmt.Errorf("wavfix: output is too large for standard RIFF")
	}
	binary.LittleEndian.PutUint32(output.Bytes()[4:8], uint32(output.Len()-8))
	return output.Bytes(), nil
}

func writeChunkBytes(output *bytes.Buffer, id string, payload []byte) error {
	var header [8]byte
	copy(header[0:4], id)
	binary.LittleEndian.PutUint32(header[4:8], uint32(len(payload)))
	if _, err := output.Write(header[:]); err != nil {
		return err
	}
	if _, err := output.Write(payload); err != nil {
		return err
	}
	if len(payload)%2 == 1 {
		return output.WriteByte(0)
	}
	return nil
}

// RepairFile reads input, repairs it, and atomically writes output.
func RepairFile(inputPath, outputPath string, options Options) (*Report, error) {
	raw, err := os.ReadFile(inputPath)
	if err != nil {
		return nil, err
	}
	fixed, report, err := RepairWithOptions(raw, options)
	if err != nil {
		return report, err
	}
	if outputPath == "" {
		return report, fmt.Errorf("wavfix: output path is required")
	}
	if err := writeFileAtomic(outputPath, fixed); err != nil {
		return report, err
	}
	return report, nil
}

func writeFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".wavfix-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	mode := os.FileMode(0o644)
	if info, statErr := os.Stat(path); statErr == nil {
		mode = info.Mode().Perm()
	}
	if err := os.Chmod(tmpName, mode); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
