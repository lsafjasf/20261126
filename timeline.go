package mp3timeline

import (
	"time"
)

// Format is the constant MPEG audio format shared by the mapped frames.
type Format struct {
	MPEGVersion     string `json:"mpeg_version"`
	Layer           int    `json:"layer"`
	SampleRate      int    `json:"sample_rate"`
	Channels        int    `json:"channels"`
	SamplesPerFrame int    `json:"samples_per_frame"`
}

// Frame is one physical audio frame. The Xing/Info metadata frame is excluded.
type Frame struct {
	Number         int   `json:"number"`
	Offset         int64 `json:"offset"`
	Size           int   `json:"size"`
	Bitrate        int   `json:"bitrate_bps"`
	SampleRate     int   `json:"sample_rate"`
	Samples        int   `json:"samples"`
	Padding        bool  `json:"padding_bit"`
	RawStartSample int64 `json:"raw_start_sample"`
}

// Source identifies which time source produced the canonical mapping.
const (
	SourceFrameScan    = "frame_scan"
	SourceXingDeclared = "xing_declared"
)

// Timeline is the parsed MP3 time model.
type Timeline struct {
	Path            string       `json:"-"`
	FileSize        int64        `json:"file_size"`
	ID3v2Bytes      int64        `json:"id3v2_bytes"`
	ID3v1Bytes      int64        `json:"id3v1_bytes"`
	FirstMPEGOffset int64        `json:"first_mpeg_offset"`
	AudioStart      int64        `json:"audio_start"`
	AudioEnd        int64        `json:"audio_end"`
	Format          Format       `json:"format"`
	Xing            *XingHeader  `json:"xing,omitempty"`
	Gapless         *GaplessInfo `json:"gapless,omitempty"`
	Frames          []Frame      `json:"-"`

	Source              string   `json:"source"`
	AudioFrames         int64    `json:"audio_frames"`
	RawSamples          int64    `json:"raw_samples"`
	EncoderDelay        int      `json:"encoder_delay"`
	TailPadding         int      `json:"tail_padding"`
	PresentationSamples int64    `json:"presentation_samples"`
	Truncated           bool     `json:"truncated"`
	Resyncs             int      `json:"resyncs"`
	Warnings            []string `json:"warnings,omitempty"`
}

// Duration is the stable presentation duration. Frame-padding samples are
// removed once, and every frame timestamp is derived from integer samples.
func (t *Timeline) Duration() time.Duration {
	return samplesToDuration(t.PresentationSamples, t.Format.SampleRate)
}

// DeclaredDuration returns the raw duration stated by Xing/Info/VBRI, before
// encoder delay and tail padding are removed.
func (t *Timeline) DeclaredDuration() (time.Duration, bool) {
	if t.Xing == nil || !t.Xing.HasFrames || t.Format.SampleRate <= 0 {
		return 0, false
	}
	samples := int64(t.Xing.FrameCount) * int64(t.Format.SamplesPerFrame)
	return samplesToDuration(samples, t.Format.SampleRate), true
}

// ScannedDuration returns the raw duration accumulated from physical audio
// frames. The metadata frame is not included.
func (t *Timeline) ScannedDuration() time.Duration {
	samples := int64(len(t.Frames)) * int64(t.Format.SamplesPerFrame)
	return samplesToDuration(samples, t.Format.SampleRate)
}

// FrameCount returns the number of mapped audio frames.
func (t *Timeline) FrameCount() int {
	return len(t.Frames)
}

// TimeAtFrame returns the presentation timestamp for the first sample of frame
// index. The encoder delay is removed before the timestamp is clamped to [0,
// Duration()].
func (t *Timeline) TimeAtFrame(index int) (time.Duration, bool) {
	if index < 0 || index >= len(t.Frames) {
		return 0, false
	}
	start := t.Frames[index].RawStartSample - int64(t.EncoderDelay)
	if start < 0 {
		start = 0
	}
	if start > t.PresentationSamples {
		start = t.PresentationSamples
	}
	return samplesToDuration(start, t.Format.SampleRate), true
}

// FrameAt returns the audio frame that contains presentation time. Times on or
// after the duration resolve to the final frame.
func (t *Timeline) FrameAt(at time.Duration) (int, bool) {
	if len(t.Frames) == 0 {
		return 0, false
	}
	if at < 0 {
		at = 0
	}
	if at >= t.Duration() {
		return len(t.Frames) - 1, true
	}
	raw := int64(t.EncoderDelay) + durationToSamples(at, t.Format.SampleRate)
	if raw < 0 {
		raw = 0
	}
	index := int(raw / int64(t.Format.SamplesPerFrame))
	first := t.firstPresentationFrame()
	if index < first {
		index = first
	}
	if index >= len(t.Frames) {
		index = len(t.Frames) - 1
	}
	return index, true
}

func (t *Timeline) firstPresentationFrame() int {
	if len(t.Frames) == 0 {
		return 0
	}
	first := 0
	if t.Format.SamplesPerFrame > 0 {
		first = t.EncoderDelay / t.Format.SamplesPerFrame
	}
	if first >= len(t.Frames) {
		first = len(t.Frames) - 1
	}
	return first
}

// SeekPoint is byte- and frame-accurate seek data for a player.
type SeekPoint struct {
	RequestedUS int64 `json:"requested_us"`
	Frame       int   `json:"frame"`
	Offset      int64 `json:"byte_offset"`
	Size        int   `json:"size"`
	FrameTimeUS int64 `json:"frame_time_us"`
	AtEnd       bool  `json:"at_end"`
}

// SeekPointAt maps presentation time to a physical frame offset. Display time
// must subsequently come from TimeAtFrame or the same sample timeline; mixing
// it with the Xing header duration will reintroduce drift.
func (t *Timeline) SeekPointAt(at time.Duration) (SeekPoint, bool) {
	index, ok := t.FrameAt(at)
	if !ok {
		return SeekPoint{}, false
	}
	f := t.Frames[index]
	ft, _ := t.TimeAtFrame(index)
	requested := at
	if requested < 0 {
		requested = 0
	}
	return SeekPoint{
		RequestedUS: durationToMicros(requested),
		Frame:       index,
		Offset:      f.Offset,
		Size:        f.Size,
		FrameTimeUS: durationToMicros(ft),
		AtEnd:       at >= t.Duration(),
	}, true
}

func samplesToDuration(samples int64, sampleRate int) time.Duration {
	if sampleRate <= 0 || samples <= 0 {
		return 0
	}
	seconds := samples / int64(sampleRate)
	remainder := samples % int64(sampleRate)
	return time.Duration(seconds)*time.Second +
		time.Duration(remainder*int64(time.Second)/int64(sampleRate))
}

func durationToSamples(d time.Duration, sampleRate int) int64 {
	if d <= 0 || sampleRate <= 0 {
		return 0
	}
	seconds := int64(d / time.Second)
	remainder := int64(d % time.Second)
	samples := remainder*int64(sampleRate) + int64(time.Second)/2
	return seconds*int64(sampleRate) + samples/int64(time.Second)
}

func durationToMicros(d time.Duration) int64 {
	if d <= 0 {
		return 0
	}
	return int64(d / time.Microsecond)
}
