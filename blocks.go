package main

// BlockLayout describes how the payload splits into per-channel blocks,
// including a trailing partial block when DataSize is not an exact
// multiple of BlockSizePerChannel*ChannelNum.
type BlockLayout struct {
	FullBlocks        uint64 // complete block-sets across all channels
	RemainderBytes    uint64 // payload bytes after the last full block-set
	PartialPerChannel uint64 // whole bytes each channel gets in the tail
	StrayBytes        uint64 // tail bytes not divisible by channel count
}

// Layout computes the block structure of the audio payload.
func (i *Info) Layout() BlockLayout {
	set := uint64(i.BlockSizePerChannel) * uint64(i.ChannelNum)
	if set == 0 {
		return BlockLayout{}
	}
	l := BlockLayout{
		FullBlocks:     i.DataSize / set,
		RemainderBytes: i.DataSize % set,
	}
	if i.ChannelNum > 0 {
		l.PartialPerChannel = l.RemainderBytes / uint64(i.ChannelNum)
		l.StrayBytes = l.RemainderBytes % uint64(i.ChannelNum)
	}
	return l
}

// ActualSampleCount derives the true per-channel sample count from the
// payload size, crediting the trailing partial block that a naive
// block-aligned reader would silently drop.
func (i *Info) ActualSampleCount() uint64 {
	l := i.Layout()
	if i.BitsPerSample == 8 {
		// 8-bit variant: one byte per sample.
		return l.FullBlocks*uint64(i.BlockSizePerChannel) + l.PartialPerChannel
	}
	return (l.FullBlocks*uint64(i.BlockSizePerChannel) + l.PartialPerChannel) * 8
}

// DurationNanos is the header-declared play time in nanoseconds.
func (i *Info) DurationNanos() uint64 {
	if i.SampleFreq == 0 {
		return 0
	}
	return i.SampleCount * 1e9 / uint64(i.SampleFreq)
}

// ActualDurationNanos is the play time derived from the real payload,
// including the trailing partial block.
func (i *Info) ActualDurationNanos() uint64 {
	if i.SampleFreq == 0 {
		return 0
	}
	return i.ActualSampleCount() * 1e9 / uint64(i.SampleFreq)
}
