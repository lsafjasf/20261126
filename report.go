package mp3timeline

import (
	"fmt"
)

// Report is the machine-readable analysis emitted by the CLI.
type Report struct {
	File       string           `json:"file,omitempty"`
	FileSize   int64            `json:"file_size"`
	Container  ContainerReport  `json:"container"`
	Xing       *XingReport      `json:"xing,omitempty"`
	Scan       ScanReport       `json:"frame_scan"`
	Comparison ComparisonReport `json:"comparison"`
	Gapless    *GaplessReport   `json:"gapless,omitempty"`
	Mapping    MappingReport    `json:"stable_mapping"`
	Frames     []FrameReport    `json:"frames,omitempty"`
	Warnings   []string         `json:"warnings,omitempty"`
}

type ContainerReport struct {
	ID3v2Bytes      int64  `json:"id3v2_bytes"`
	ID3v1Bytes      int64  `json:"id3v1_bytes"`
	FirstMPEGOffset int64  `json:"first_mpeg_offset"`
	MetadataFrame   int64  `json:"metadata_frame_offset,omitempty"`
	AudioStart      int64  `json:"audio_start"`
	AudioEnd        int64  `json:"audio_end"`
	MPEGVersion     string `json:"mpeg_version"`
	Layer           int    `json:"layer"`
	SampleRate      int    `json:"sample_rate"`
	Channels        int    `json:"channels"`
	SamplesPerFrame int    `json:"samples_per_frame"`
}

type XingReport struct {
	Kind                  string `json:"kind"`
	Offset                int64  `json:"offset"`
	Frames                uint32 `json:"frames"`
	StreamBytes           uint32 `json:"stream_bytes"`
	HasTOC                bool   `json:"has_toc"`
	TOCEntries            int    `json:"toc_entries"`
	DeclaredRawSamples    int64  `json:"declared_raw_samples"`
	DeclaredDurationUS    int64  `json:"declared_duration_us"`
	CountsMetadataFrame   bool   `json:"counts_metadata_frame"`
	NormalizedAudioFrames int64  `json:"normalized_audio_frames"`
}

type ScanReport struct {
	Complete    bool  `json:"complete"`
	AudioFrames int64 `json:"audio_frames"`
	RawSamples  int64 `json:"raw_samples"`
	DurationUS  int64 `json:"duration_us"`
	Resyncs     int   `json:"resyncs"`
	Truncated   bool  `json:"truncated"`
}

type ComparisonReport struct {
	FrameDelta      int64  `json:"frame_delta"`
	RawSampleDelta  int64  `json:"raw_sample_delta"`
	DurationDeltaUS int64  `json:"duration_delta_us"`
	Relation        string `json:"relation"`
	CanonicalSource string `json:"canonical_source"`
}

type GaplessReport struct {
	Present             bool   `json:"present"`
	Encoder             string `json:"encoder,omitempty"`
	TagOffset           int64  `json:"tag_offset,omitempty"`
	EncoderDelaySamples int    `json:"encoder_delay_samples"`
	TailPaddingSamples  int    `json:"tail_padding_samples"`
	RawSamples          int64  `json:"raw_samples"`
	PresentationSamples int64  `json:"presentation_samples"`
	DurationUS          int64  `json:"presentation_duration_us"`
}

type MappingReport struct {
	Authority           string `json:"authority"`
	TimeUnit            string `json:"time_unit"`
	SeekOriginByte      int64  `json:"seek_origin_byte"`
	SampleRate          int    `json:"sample_rate"`
	SamplesPerFrame     int    `json:"samples_per_frame"`
	AudioFrames         int64  `json:"audio_frames"`
	RawSamples          int64  `json:"raw_samples"`
	EncoderDelaySamples int    `json:"encoder_delay_samples"`
	TailPaddingSamples  int    `json:"tail_padding_samples"`
	PresentationSamples int64  `json:"presentation_samples"`
	DurationUS          int64  `json:"duration_us"`
	FirstFrame          int    `json:"first_frame"`
	LastFrame           int    `json:"last_frame"`
	TimeToFrame         string `json:"time_to_frame"`
	FrameToTime         string `json:"frame_to_time"`
}

type FrameReport struct {
	Number         int   `json:"number"`
	Offset         int64 `json:"offset"`
	Size           int   `json:"size"`
	Bitrate        int   `json:"bitrate_bps"`
	RawStartSample int64 `json:"raw_start_sample"`
	StartUS        int64 `json:"presentation_start_us"`
}

// Report builds a report. When frames is true, every frame mapping is included.
func (t *Timeline) Report(frames bool) Report {
	spf := int64(t.Format.SamplesPerFrame)
	scanFrames := int64(len(t.Frames))
	scanRaw := scanFrames * spf

	r := Report{
		File:     t.Path,
		FileSize: t.FileSize,
		Container: ContainerReport{
			ID3v2Bytes:      t.ID3v2Bytes,
			ID3v1Bytes:      t.ID3v1Bytes,
			FirstMPEGOffset: t.FirstMPEGOffset,
			AudioStart:      t.AudioStart,
			AudioEnd:        t.AudioEnd,
			MPEGVersion:     t.Format.MPEGVersion,
			Layer:           t.Format.Layer,
			SampleRate:      t.Format.SampleRate,
			Channels:        t.Format.Channels,
			SamplesPerFrame: t.Format.SamplesPerFrame,
		},
		Warnings: append([]string(nil), t.Warnings...),
	}
	if t.Xing != nil {
		r.Container.MetadataFrame = t.FirstMPEGOffset
	}

	if t.Xing != nil {
		declared := int64(t.Xing.FrameCount)
		countsMetadata := !t.Truncated && scanFrames > 0 && declared == scanFrames+1
		normalized := declared
		if countsMetadata {
			normalized--
		}
		r.Xing = &XingReport{
			Kind:                  t.Xing.Kind,
			Offset:                t.Xing.Offset,
			Frames:                t.Xing.FrameCount,
			StreamBytes:           t.Xing.StreamBytes,
			HasTOC:                t.Xing.HasTOC,
			TOCEntries:            t.Xing.TOCEntries,
			DeclaredRawSamples:    declared * spf,
			DeclaredDurationUS:    samplesMicros(declared*spf, t.Format.SampleRate),
			CountsMetadataFrame:   countsMetadata,
			NormalizedAudioFrames: normalized,
		}
		if t.Xing.HasFrames {
			r.Comparison.FrameDelta = declared - scanFrames
			r.Comparison.RawSampleDelta = (declared - scanFrames) * spf
			r.Comparison.DurationDeltaUS = samplesMicros((declared-scanFrames)*spf, t.Format.SampleRate)
			switch {
			case declared == scanFrames:
				r.Comparison.Relation = "same"
			case countsMetadata:
				r.Comparison.Relation = "xing_counts_metadata_frame"
			default:
				r.Comparison.Relation = "mismatch"
			}
		} else {
			r.Comparison.Relation = "xing_without_frame_count"
		}
	} else {
		r.Comparison.Relation = "xing_absent"
	}
	r.Comparison.CanonicalSource = t.Source

	r.Scan = ScanReport{
		Complete:    !t.Truncated && scanFrames > 0,
		AudioFrames: scanFrames,
		RawSamples:  scanRaw,
		DurationUS:  samplesMicros(scanRaw, t.Format.SampleRate),
		Resyncs:     t.Resyncs,
		Truncated:   t.Truncated,
	}

	if t.Gapless != nil && t.Gapless.Present {
		r.Gapless = &GaplessReport{
			Present:             true,
			Encoder:             t.Gapless.Encoder,
			TagOffset:           t.Gapless.TagOffset,
			EncoderDelaySamples: t.EncoderDelay,
			TailPaddingSamples:  t.TailPadding,
			RawSamples:          t.RawSamples,
			PresentationSamples: t.PresentationSamples,
			DurationUS:          samplesMicros(t.PresentationSamples, t.Format.SampleRate),
		}
	}

	r.Mapping = MappingReport{
		Authority:           t.Source,
		TimeUnit:            "microsecond",
		SeekOriginByte:      t.AudioStart,
		SampleRate:          t.Format.SampleRate,
		SamplesPerFrame:     t.Format.SamplesPerFrame,
		AudioFrames:         t.AudioFrames,
		RawSamples:          t.RawSamples,
		EncoderDelaySamples: t.EncoderDelay,
		TailPaddingSamples:  t.TailPadding,
		PresentationSamples: t.PresentationSamples,
		DurationUS:          samplesMicros(t.PresentationSamples, t.Format.SampleRate),
		FirstFrame:          t.firstPresentationFrame(),
		LastFrame:           len(t.Frames) - 1,
		TimeToFrame: "frame = floor((encoder_delay_samples + round(time_us * sample_rate / 1000000)) / " +
			"samples_per_frame), clamped to [first_frame, last_frame]",
		FrameToTime: "time_us = clamp(frame * samples_per_frame - encoder_delay_samples, 0, " +
			"presentation_samples) * 1000000 / sample_rate",
	}
	if len(t.Frames) == 0 {
		r.Mapping.FirstFrame = -1
		r.Mapping.LastFrame = -1
	}
	if frames {
		r.Frames = make([]FrameReport, len(t.Frames))
		for i, f := range t.Frames {
			start := f.RawStartSample - int64(t.EncoderDelay)
			if start < 0 {
				start = 0
			}
			if start > t.PresentationSamples {
				start = t.PresentationSamples
			}
			r.Frames[i] = FrameReport{
				Number:         f.Number,
				Offset:         f.Offset,
				Size:           f.Size,
				Bitrate:        f.Bitrate,
				RawStartSample: f.RawStartSample,
				StartUS:        samplesMicros(start, t.Format.SampleRate),
			}
		}
	}
	return r
}

func samplesMicros(samples int64, sampleRate int) int64 {
	if samples <= 0 || sampleRate <= 0 {
		return 0
	}
	return durationToMicros(samplesToDuration(samples, sampleRate))
}

func (r Report) String() string {
	return fmt.Sprintf("%d audio frames, %d presentation samples",
		r.Mapping.AudioFrames, r.Mapping.PresentationSamples)
}
