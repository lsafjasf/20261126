package pcmconv

import "fmt"

// PlanarToInterleaved converts channel-major PCM to frame-major PCM. Every
// sample is copied exactly once. Channels may have different lengths only when
// they differ by one sample and the longer channels are the leading channels,
// which is the shape produced by a partial final frame.
func PlanarToInterleaved(src Planar) (Interleaved, error) {
	if err := src.Format.Validate(); err != nil {
		return Interleaved{}, err
	}
	channels := src.Format.Channels
	if len(src.Channels) != channels {
		return Interleaved{}, fmt.Errorf("%w: format has %d, input has %d",
			ErrChannelCount, channels, len(src.Channels))
	}

	counts := make([]int, channels)
	for channel, data := range src.Channels {
		if len(data)%src.Format.BytesPerSample != 0 {
			return Interleaved{}, fmt.Errorf("%w: channel %d has %d bytes",
				ErrPartialSample, channel, len(data))
		}
		counts[channel] = len(data) / src.Format.BytesPerSample
	}
	if len(src.Counts) != 0 {
		if len(src.Counts) != channels {
			return Interleaved{}, fmt.Errorf("%w: format has %d, counts have %d",
				ErrChannelCount, channels, len(src.Counts))
		}
		for channel := range counts {
			if src.Counts[channel] != counts[channel] {
				return Interleaved{}, fmt.Errorf("%w: channel %d has %d samples, count says %d",
					ErrCountsMismatch, channel, counts[channel], src.Counts[channel])
			}
		}
	}

	fullFrames, partial, err := canonicalCounts(counts)
	if err != nil {
		return Interleaved{}, err
	}
	if fullFrames > (maxInt()-partial)/channels {
		return Interleaved{}, fmt.Errorf("pcmconv: interleaved size overflows int")
	}
	totalSamples := fullFrames*channels + partial
	if totalSamples > maxInt()/src.Format.BytesPerSample {
		return Interleaved{}, fmt.Errorf("pcmconv: interleaved size overflows int")
	}

	data := make([]byte, totalSamples*src.Format.BytesPerSample)
	offset := 0
	write := func(channel, frame int) {
		sampleOffset := frame * src.Format.BytesPerSample
		copy(data[offset:offset+src.Format.BytesPerSample],
			src.Channels[channel][sampleOffset:sampleOffset+src.Format.BytesPerSample])
		offset += src.Format.BytesPerSample
	}

	for frame := 0; frame < fullFrames; frame++ {
		for channel := 0; channel < channels; channel++ {
			write(channel, frame)
		}
	}
	for channel := 0; channel < partial; channel++ {
		write(channel, fullFrames)
	}

	return Interleaved{
		Format: src.Format,
		Data:   data,
		Counts: append([]int(nil), counts...),
	}, nil
}

// InterleavedToPlanar converts frame-major PCM to channel-major PCM. A final
// frame that contains fewer than Channels samples is preserved: its leading
// channels receive one extra sample, and Counts records that distribution.
func InterleavedToPlanar(src Interleaved) (Planar, error) {
	if err := src.Format.Validate(); err != nil {
		return Planar{}, err
	}
	sampleBytes := src.Format.BytesPerSample
	if len(src.Data)%sampleBytes != 0 {
		return Planar{}, fmt.Errorf("%w: %d bytes, sample size %d",
			ErrPartialSample, len(src.Data), sampleBytes)
	}

	channels := src.Format.Channels
	counts := make([]int, channels)
	for offset := 0; offset < len(src.Data); {
		for channel := 0; channel < channels && offset < len(src.Data); channel++ {
			counts[channel]++
			offset += sampleBytes
		}
	}
	if len(src.Counts) != 0 {
		if len(src.Counts) != channels {
			return Planar{}, fmt.Errorf("%w: format has %d, counts have %d",
				ErrChannelCount, channels, len(src.Counts))
		}
		for channel := range counts {
			if src.Counts[channel] != counts[channel] {
				return Planar{}, fmt.Errorf("%w: channel %d has %d samples, count says %d",
					ErrCountsMismatch, channel, counts[channel], src.Counts[channel])
			}
		}
	}

	planar := make([][]byte, channels)
	for channel := range planar {
		if counts[channel] > maxInt()/sampleBytes {
			return Planar{}, fmt.Errorf("pcmconv: channel %d size overflows int", channel)
		}
		planar[channel] = make([]byte, counts[channel]*sampleBytes)
	}

	offset := 0
	frame := 0
	for offset < len(src.Data) {
		for channel := 0; channel < channels && offset < len(src.Data); channel++ {
			sampleOffset := frame * sampleBytes
			copy(planar[channel][sampleOffset:sampleOffset+sampleBytes],
				src.Data[offset:offset+sampleBytes])
			offset += sampleBytes
		}
		frame++
	}

	return Planar{
		Format:   src.Format,
		Channels: planar,
		Counts:   counts,
	}, nil
}

// Interleave is a convenience wrapper that returns only the interleaved bytes
// and per-channel sample counts.
func Interleave(format Format, channels [][]byte) ([]byte, []int, error) {
	converted, err := PlanarToInterleaved(Planar{
		Format:   format,
		Channels: channels,
	})
	if err != nil {
		return nil, nil, err
	}
	return converted.Data, converted.Counts, nil
}

// Deinterleave is a convenience wrapper that returns the planar channels and
// per-channel sample counts.
func Deinterleave(format Format, data []byte) ([][]byte, []int, error) {
	converted, err := InterleavedToPlanar(Interleaved{
		Format: format,
		Data:   data,
	})
	if err != nil {
		return nil, nil, err
	}
	return converted.Channels, converted.Counts, nil
}

// canonicalCounts validates the count shape of complete frames followed by at
// most one partial frame. It returns the number of complete frames and the
// number of samples in the partial frame.
func canonicalCounts(counts []int) (int, int, error) {
	if len(counts) == 0 {
		return 0, 0, fmt.Errorf("%w: no channels", ErrInvalidChannels)
	}
	fullFrames := counts[0]
	for channel, count := range counts {
		if count < 0 {
			return 0, 0, fmt.Errorf("%w: channel %d has negative count %d",
				ErrInconsistentChannels, channel, count)
		}
		if count < fullFrames {
			fullFrames = count
		}
	}
	partial := 0
	shortSeen := false
	for _, count := range counts {
		switch count {
		case fullFrames:
			shortSeen = true
		case fullFrames + 1:
			if shortSeen {
				return 0, 0, fmt.Errorf("%w: counts %v do not describe frames",
					ErrInconsistentChannels, counts)
			}
			partial++
		default:
			return 0, 0, fmt.Errorf("%w: counts %v do not describe frames",
				ErrInconsistentChannels, counts)
		}
	}
	return fullFrames, partial, nil
}
