package wavfix

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
)

// IsG711 reports whether the effective format tag is A-law or μ-law.
func (f *FormatChunk) IsG711() bool {
	return f != nil && (f.EffectiveTag == FormatALaw || f.EffectiveTag == FormatMuLaw)
}

// CalculatedBlockAlign is channels * bitsPerSample / 8. For G.711 this is
// normally the channel count, because each channel has one 8-bit code per frame.
func (f *FormatChunk) CalculatedBlockAlign() int {
	if f == nil || f.BitsPerSample == 0 || f.BitsPerSample%8 != 0 {
		return 0
	}
	return int(f.Channels) * int(f.BitsPerSample/8)
}

// BuildReport derives the true sample-frame count from fmt and data and compares
// it with fact.
func (f *File) BuildReport() *Report {
	if f.Format == nil {
		return &Report{
			InputSize: len(f.Raw),
			Issues: append([]Issue(nil), append(f.Reports, Issue{
				Code:    "missing_fmt",
				Chunk:   "fmt ",
				Message: "WAVE file has no fmt chunk",
			})...),
		}
	}

	format := f.Format
	data := f.Data
	fmtDataOffset := 0
	if format.ChunkIndex < len(f.Chunks) {
		fmtDataOffset = f.Chunks[format.ChunkIndex].DataOffset
	}
	frameBytes := int(format.BlockAlign)
	if frameBytes <= 0 {
		frameBytes = format.CalculatedBlockAlign()
	}

	dataBytes := 0
	if data != nil {
		dataBytes = len(data.Payload)
	}
	dataFrames := uint64(0)
	remainder := 0
	if frameBytes > 0 {
		dataFrames = uint64(dataBytes / frameBytes)
		remainder = dataBytes % frameBytes
	}

	var audioPayload []byte
	if data != nil {
		audioPayload = data.Payload
	}
	sum := sha256.Sum256(audioPayload)
	report := &Report{
		InputSize:          len(f.Raw),
		RIFFDeclaredSize:   f.RIFFSize,
		RIFFActualSize:     len(f.Raw) - 8,
		FormatTag:          format.Tag,
		EffectiveFormatTag: format.EffectiveTag,
		Channels:           format.Channels,
		SampleRate:         format.SampleRate,
		BlockAlign:         format.BlockAlign,
		BitsPerSample:      format.BitsPerSample,
		DataBytes:          dataBytes,
		FrameBytes:         frameBytes,
		DataFrames:         dataFrames,
		DataRemainderBytes: remainder,
		AudioSHA256:        hex.EncodeToString(sum[:]),
		Issues:             append([]Issue(nil), f.Reports...),
	}

	for _, chunk := range f.Chunks {
		switch chunk.ID {
		case "fmt ", "fact", "data":
		default:
			report.UnknownChunks = append(report.UnknownChunks, chunk.ID)
		}
	}

	if f.Fact != nil {
		report.FactPresent = true
		report.FactDeclaredCount = f.Fact.SampleCount
		report.FactValid = f.Fact.Valid
		if f.Fact.Valid {
			report.DurationFromFact = float64(f.Fact.SampleCount) / float64(format.SampleRate)
		}
	} else {
		report.AddIssue("missing_fact", "fact", 0, "non-PCM/G.711 WAV should contain a fact chunk")
	}

	if report.FrameBytes <= 0 {
		report.AddIssue("invalid_block_align", "fmt ", 0, "fmt chunk has no usable nBlockAlign or bits-per-sample")
	} else {
		report.DurationFromData = float64(dataFrames) / float64(format.SampleRate)
		report.NaiveByteCountSeconds = float64(dataBytes) / float64(format.SampleRate)
		report.FloatValues = decodedFloatValueCount(dataBytes, format)
		if format.Channels > 0 {
			report.FloatDurationSeconds = float64(report.FloatValues) / float64(format.Channels) / float64(format.SampleRate)
		}
	}

	if expected := format.CalculatedBlockAlign(); expected > 0 && int(format.BlockAlign) != expected {
		report.AddIssue("block_align_mismatch", "fmt ", fmtDataOffset+12,
			fmt.Sprintf("nBlockAlign=%d but channels*bits/8=%d", format.BlockAlign, expected))
	}
	if format.AvgBytesPerSec != 0 && frameBytes > 0 {
		expectedRate := uint64(format.SampleRate) * uint64(frameBytes)
		if uint64(format.AvgBytesPerSec) != expectedRate && expectedRate < uint64(^uint32(0)) {
			report.AddIssue("byte_rate_mismatch", "fmt ", fmtDataOffset+8,
				fmt.Sprintf("nAvgBytesPerSec=%d but sampleRate*blockAlign=%d", format.AvgBytesPerSec, expectedRate))
		}
	}
	if format.IsG711() && format.BitsPerSample != 8 {
		report.AddIssue("g711_bits_mismatch", "fmt ", fmtDataOffset+14,
			fmt.Sprintf("G.711 wire format should use 8 bits per code, got %d", format.BitsPerSample))
	}
	if remainder != 0 && data != nil {
		report.AddIssue("partial_frame", "data", data.Offset,
			fmt.Sprintf("data size %d leaves %d byte(s) after %d-byte frames", dataBytes, remainder, frameBytes))
	}
	if f.Fact != nil && f.Fact.Valid && uint64(f.Fact.SampleCount) != dataFrames {
		factOffset := 0
		if f.Fact.ChunkIndex < len(f.Chunks) {
			factOffset = f.Chunks[f.Fact.ChunkIndex].DataOffset
		}
		report.AddIssue("fact_sample_count_mismatch", "fact", factOffset,
			fmt.Sprintf("fact says %d frames; fmt/data imply %d frames", f.Fact.SampleCount, dataFrames))
	}
	if f.Fact != nil && !f.Fact.Valid {
		factOffset := 0
		if f.Fact.ChunkIndex < len(f.Chunks) {
			factOffset = f.Chunks[f.Fact.ChunkIndex].DataOffset
		}
		report.AddIssue("fact_too_short", "fact", factOffset, "fact chunk does not contain dwSampleLength")
	}

	if report.DataFrames <= math.MaxUint32 {
		report.RepairedFactCount = uint32(report.DataFrames)
	}
	report.FactInserted = f.Fact == nil
	if f.Fact == nil {
		report.FactChanged = true
	} else {
		report.FactChanged = !f.Fact.Valid ||
			f.Fact.Size != 4 ||
			uint64(f.Fact.SampleCount) != report.DataFrames
	}

	return report
}

// AddIssue appends a diagnostic to the report.
func (r *Report) AddIssue(code, chunk string, offset int, message string) {
	r.Issues = append(r.Issues, Issue{Code: code, Chunk: chunk, Offset: offset, Message: message})
}

func decodedFloatValueCount(dataBytes int, format *FormatChunk) int {
	if format.IsG711() || format.BitsPerSample == 8 || format.BitsPerSample == 0 {
		return dataBytes
	}
	bytesPerValue := int(format.BitsPerSample / 8)
	if format.BitsPerSample%8 != 0 || bytesPerValue == 0 {
		return 0
	}
	return dataBytes / bytesPerValue
}

// Factor returns naive byte-count duration divided by the data-derived duration.
// For stereo 8-bit G.711 it is 2.
func (r *Report) DurationErrorFactor() float64 {
	if r.DurationFromData == 0 {
		return 0
	}
	return r.NaiveByteCountSeconds / r.DurationFromData
}
