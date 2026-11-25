package aiffmeta

import (
	"fmt"
	"math"
)

type AudioComparison struct {
	Comparable   bool   `json:"comparable"`
	Equal        bool   `json:"equal"`
	SourceBytes  int64  `json:"source_bytes"`
	ResultBytes  int64  `json:"result_bytes"`
	SourceSHA256 string `json:"source_sha256,omitempty"`
	ResultSHA256 string `json:"result_sha256,omitempty"`
	Note         string `json:"note,omitempty"`
}

type SSNDComparison struct {
	SourcePresent    bool `json:"source_present"`
	ResultPresent    bool `json:"result_present"`
	Comparable       bool `json:"comparable"`
	OffsetChanged    bool `json:"offset_changed"`
	BlockSizeChanged bool `json:"block_size_changed"`
}

type COMMComparison struct {
	Comparable  bool     `json:"comparable"`
	Changed     bool     `json:"changed"`
	Differences []string `json:"differences,omitempty"`
}

type CompareReport struct {
	SourcePath    string          `json:"source_path,omitempty"`
	ResultPath    string          `json:"result_path,omitempty"`
	FormatChanged bool            `json:"format_changed"`
	Audio         AudioComparison `json:"audio"`
	COMM          COMMComparison  `json:"comm"`
	SSND          SSNDComparison  `json:"ssnd"`
	Metadata      MetadataDiff    `json:"metadata"`
}

// Compare checks actual audio bytes (not SSND offset bytes or padding) and
// lists source metadata missing from result.
func Compare(source, result *File) (*CompareReport, error) {
	if source == nil || result == nil {
		return nil, fmt.Errorf("both source and result files are required")
	}
	report := &CompareReport{
		SourcePath:    source.Path,
		ResultPath:    result.Path,
		FormatChanged: source.Format != result.Format,
		Metadata:      CompareMetadata(source, result),
	}
	report.COMM = compareCOMM(source.COMM, result.COMM)
	report.SSND = SSNDComparison{
		SourcePresent: source.SSND != nil,
		ResultPresent: result.SSND != nil,
		Comparable:    source.SSND != nil && result.SSND != nil,
	}
	if report.SSND.Comparable {
		report.SSND.OffsetChanged = source.SSND.Offset != result.SSND.Offset
		report.SSND.BlockSizeChanged = source.SSND.BlockSize != result.SSND.BlockSize
	}
	if source.SSND == nil || result.SSND == nil {
		report.Audio.Note = "audio comparison unavailable: one file has no parseable SSND"
		return report, nil
	}
	report.Audio.Comparable = true
	sourceHash, sourceBytes, err := source.AudioDigest()
	if err != nil {
		return nil, err
	}
	resultHash, resultBytes, err := result.AudioDigest()
	if err != nil {
		return nil, err
	}
	report.Audio.SourceBytes = sourceBytes
	report.Audio.ResultBytes = resultBytes
	report.Audio.SourceSHA256 = sourceHash
	report.Audio.ResultSHA256 = resultHash
	report.Audio.Equal = sourceBytes == resultBytes && sourceHash == resultHash
	return report, nil
}

func compareCOMM(source, result *COMM) COMMComparison {
	comparison := COMMComparison{}
	if source == nil || result == nil {
		comparison.Changed = source != nil || result != nil
		if comparison.Changed {
			comparison.Differences = []string{"COMM presence changed"}
		}
		return comparison
	}
	comparison.Comparable = true
	if source.Channels != result.Channels {
		comparison.Differences = append(comparison.Differences,
			fmt.Sprintf("channels: %d -> %d", source.Channels, result.Channels))
	}
	if source.SampleFrames != result.SampleFrames {
		comparison.Differences = append(comparison.Differences,
			fmt.Sprintf("sample frames: %d -> %d", source.SampleFrames, result.SampleFrames))
	}
	if source.SampleSize != result.SampleSize {
		comparison.Differences = append(comparison.Differences,
			fmt.Sprintf("sample size: %d -> %d", source.SampleSize, result.SampleSize))
	}
	if math.Abs(source.SampleRate-result.SampleRate) > 1e-9 {
		comparison.Differences = append(comparison.Differences,
			fmt.Sprintf("sample rate: %g -> %g", source.SampleRate, result.SampleRate))
	}
	if source.CompressionType != result.CompressionType {
		comparison.Differences = append(comparison.Differences,
			fmt.Sprintf("compression type: %q -> %q",
				source.CompressionType, result.CompressionType))
	}
	if source.CompressionName != result.CompressionName {
		comparison.Differences = append(comparison.Differences,
			fmt.Sprintf("compression name: %q -> %q",
				source.CompressionName, result.CompressionName))
	}
	comparison.Changed = len(comparison.Differences) > 0
	return comparison
}
