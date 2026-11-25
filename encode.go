package channelmap

import (
	"encoding/binary"
	"fmt"
	"math"
)

// ToneChannel creates a normalized sine channel with short edge fades.
func ToneChannel(frequencyHz float64, durationSeconds float64, sampleRate int, amplitude float64) []float64 {
	n := int(math.Round(durationSeconds * float64(sampleRate)))
	out := make([]float64, n)
	fade := sampleRate / 100
	if fade < 16 {
		fade = 16
	}
	if fade > n/4 {
		fade = n / 4
	}
	for i := 0; i < n; i++ {
		v := amplitude * math.Sin(2*math.Pi*frequencyHz*float64(i)/float64(sampleRate))
		if i < fade {
			v *= float64(i) / float64(fade)
		} else if i > n-fade {
			v *= float64(n-i) / float64(fade)
		}
		out[i] = v
	}
	return out
}

// EncodeWAVOptions controls the small WAV writer used by tests and the generator.
type EncodeWAVOptions struct {
	SampleRate    int
	BitsPerSample int
	IEEEFloat     bool
	ChannelMask   uint32
	Extensible    bool
}

// EncodeWAV interleaves normalized [-1,1] channels and writes a PCM/float WAV.
func EncodeWAV(channels [][]float64, opts EncodeWAVOptions) ([]byte, error) {
	if len(channels) == 0 {
		return nil, fmt.Errorf("encode: at least one channel is required")
	}
	frames := len(channels[0])
	for i, ch := range channels {
		if len(ch) != frames {
			return nil, fmt.Errorf("encode: channel %d has %d frames, want %d", i, len(ch), frames)
		}
	}
	if opts.SampleRate <= 0 {
		opts.SampleRate = 48000
	}
	if opts.BitsPerSample == 0 {
		opts.BitsPerSample = 16
	}
	if opts.IEEEFloat {
		opts.BitsPerSample = 32
	}
	bytesPerSample := opts.BitsPerSample / 8
	if opts.BitsPerSample%8 != 0 || (opts.BitsPerSample != 16 && opts.BitsPerSample != 24 && opts.BitsPerSample != 32) {
		return nil, fmt.Errorf("encode: unsupported bits %d", opts.BitsPerSample)
	}
	if opts.IEEEFloat && opts.BitsPerSample != 32 {
		return nil, fmt.Errorf("encode: only 32-bit float is supported by the helper")
	}

	blockAlign := len(channels) * bytesPerSample
	data := make([]byte, 0, frames*blockAlign)
	for frame := 0; frame < frames; frame++ {
		for _, ch := range channels {
			v := ch[frame]
			if v > 1 {
				v = 1
			} else if v < -1 {
				v = -1
			}
			data = appendEncodedSample(data, v, opts.BitsPerSample, opts.IEEEFloat)
		}
	}

	useExtensible := opts.Extensible || opts.ChannelMask != 0 || len(channels) > 2
	var fmtChunk []byte
	if useExtensible {
		fmtChunk = make([]byte, 40)
		binary.LittleEndian.PutUint16(fmtChunk[0:2], FormatExtensible)
		binary.LittleEndian.PutUint16(fmtChunk[2:4], uint16(len(channels)))
		binary.LittleEndian.PutUint32(fmtChunk[4:8], uint32(opts.SampleRate))
		binary.LittleEndian.PutUint32(fmtChunk[8:12], uint32(opts.SampleRate*blockAlign))
		binary.LittleEndian.PutUint16(fmtChunk[12:14], uint16(blockAlign))
		binary.LittleEndian.PutUint16(fmtChunk[14:16], uint16(opts.BitsPerSample))
		binary.LittleEndian.PutUint16(fmtChunk[16:18], 22)
		binary.LittleEndian.PutUint16(fmtChunk[18:20], uint16(opts.BitsPerSample))
		binary.LittleEndian.PutUint32(fmtChunk[20:24], opts.ChannelMask)
		guid := GUIDPCM
		if opts.IEEEFloat {
			guid = GUIDFloat
		}
		binary.LittleEndian.PutUint32(fmtChunk[24:28], guid.Data1)
		binary.LittleEndian.PutUint16(fmtChunk[28:30], guid.Data2)
		binary.LittleEndian.PutUint16(fmtChunk[30:32], guid.Data3)
		copy(fmtChunk[32:40], guid.Data4[:])
	} else {
		fmtChunk = make([]byte, 16)
		tag := FormatPCM
		if opts.IEEEFloat {
			tag = FormatIEEEFloat
		}
		binary.LittleEndian.PutUint16(fmtChunk[0:2], tag)
		binary.LittleEndian.PutUint16(fmtChunk[2:4], uint16(len(channels)))
		binary.LittleEndian.PutUint32(fmtChunk[4:8], uint32(opts.SampleRate))
		binary.LittleEndian.PutUint32(fmtChunk[8:12], uint32(opts.SampleRate*blockAlign))
		binary.LittleEndian.PutUint16(fmtChunk[12:14], uint16(blockAlign))
		binary.LittleEndian.PutUint16(fmtChunk[14:16], uint16(opts.BitsPerSample))
	}

	var b []byte
	b = append(b, "RIFF"...)
	riffSize := uint32(4 + 8 + len(fmtChunk) + 8 + len(data) + len(data)%2)
	var u32 [4]byte
	binary.LittleEndian.PutUint32(u32[:], riffSize)
	b = append(b, u32[:]...)
	b = append(b, "WAVE"...)
	b = appendChunk(b, "fmt ", fmtChunk)
	b = appendChunk(b, "data", data)
	return b, nil
}

func appendChunk(dst []byte, id string, data []byte) []byte {
	dst = append(dst, id...)
	var size [4]byte
	binary.LittleEndian.PutUint32(size[:], uint32(len(data)))
	dst = append(dst, size[:]...)
	dst = append(dst, data...)
	if len(data)%2 == 1 {
		dst = append(dst, 0)
	}
	return dst
}

func appendEncodedSample(dst []byte, v float64, bits int, float bool) []byte {
	var buf [8]byte
	if float {
		binary.LittleEndian.PutUint32(buf[:4], math.Float32bits(float32(v)))
		return append(dst, buf[:4]...)
	}
	switch bits {
	case 16:
		x := int16(math.Round(v * 32767))
		binary.LittleEndian.PutUint16(buf[:2], uint16(x))
		return append(dst, buf[:2]...)
	case 24:
		x := int32(math.Round(v * 8388607))
		buf[0] = byte(x)
		buf[1] = byte(x >> 8)
		buf[2] = byte(x >> 16)
		return append(dst, buf[:3]...)
	case 32:
		x := int32(math.Round(v * 2147483647))
		binary.LittleEndian.PutUint32(buf[:4], uint32(x))
		return append(dst, buf[:4]...)
	default:
		return dst
	}
}

// Standard51ToneChannels returns calibration tones in mask order.
func Standard51ToneChannels(duration float64, rate int) [][]float64 {
	freqs := []float64{320, 400, 1000, 70, 520, 640}
	out := make([][]float64, len(freqs))
	for i, hz := range freqs {
		amp := 0.22
		if i == 3 {
			amp = 0.28
		}
		out[i] = ToneChannel(hz, duration, rate, amp)
	}
	return out
}

// SwapCenterBackLeft reproduces one concrete mixer wiring mistake in sample order.
func SwapCenterBackLeft(channels [][]float64) [][]float64 {
	out := append([][]float64(nil), channels...)
	out[2], out[4] = out[4], out[2]
	return out
}

// SwapCenterWithEitherSurround makes both surround signatures indistinguishable.
func AmbiguousCenterSurroundChannels(duration float64, rate int) [][]float64 {
	channels := Standard51ToneChannels(duration, rate)
	channels[4] = ToneChannel(580, duration, rate, 0.22)
	channels[5] = ToneChannel(580, duration, rate, 0.22)
	return SwapCenterBackLeft(channels)
}
