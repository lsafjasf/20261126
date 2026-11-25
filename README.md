# pcmconv

`pcmconv` converts packed PCM data between planar (channel-major) and
interleaved (frame-major) layouts using only the Go standard library.

The converter works one frame at a time. It supports any positive channel count
and any positive whole-byte sample width, including partial final frames. Each
result carries `Counts`, the exact number of samples for every channel.

```go
format := pcmconv.Format{Channels: 3, BytesPerSample: 2}

interleaved, counts, err := pcmconv.Interleave(format, [][]byte{
	{0x10, 0x11, 0x20, 0x21}, // channel 0: 2 samples
	{0x12, 0x13, 0x22, 0x23}, // channel 1: 2 samples
	{0x14, 0x15},             // channel 2: 1 sample, partial frame
})

planar, counts, err := pcmconv.Deinterleave(format, interleaved)
```

For a stream containing `N` complete frames followed by a partial frame with
`R` samples, the leading `R` channels have `N+1` samples and the remaining
channels have `N`. For example, a 5-channel stream ending after the first two
samples of a frame records `[]int{N+1, N+1, N, N, N}`.

The conversion rejects byte tails that are smaller than one sample rather than
silently discarding them. Planar inputs must also have a valid frame shape;
arbitrary unequal channel lengths return `ErrInconsistentChannels`.
