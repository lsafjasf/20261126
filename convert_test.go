package pcmconv

import (
	"bytes"
	"errors"
	"fmt"
	"reflect"
	"testing"
)

func TestRoundTripAllFrameShapes(t *testing.T) {
	t.Parallel()

	for _, channels := range []int{1, 2, 3, 5, 8} {
		for _, sampleBytes := range []int{1, 2, 3, 4} {
			for _, fullFrames := range []int{0, 1, 7} {
				for partial := 0; partial < channels; partial++ {
					if fullFrames == 0 && partial == 0 {
						continue
					}
					name := testName(channels, sampleBytes, fullFrames, partial)
					t.Run(name, func(t *testing.T) {
						t.Parallel()

						format := Format{Channels: channels, BytesPerSample: sampleBytes}
						totalSamples := fullFrames*channels + partial
						interleaved := testBytes(totalSamples * sampleBytes)

						planar, err := InterleavedToPlanar(Interleaved{
							Format: format,
							Data:   interleaved,
						})
						if err != nil {
							t.Fatalf("InterleavedToPlanar: %v", err)
						}
						verifyCounts(t, planar.Counts, channels, fullFrames, partial)
						verifyPlanarSamples(t, planar, interleaved)

						back, err := PlanarToInterleaved(planar)
						if err != nil {
							t.Fatalf("PlanarToInterleaved: %v", err)
						}
						if !bytes.Equal(back.Data, interleaved) {
							t.Fatalf("round trip changed bytes\n got: %v\nwant: %v",
								back.Data, interleaved)
						}
						if !reflect.DeepEqual(back.Counts, planar.Counts) {
							t.Fatalf("round trip changed counts: got %v, want %v",
								back.Counts, planar.Counts)
						}
					})
				}
			}
		}
	}
}

func TestPlanarToInterleavedPartialFrame(t *testing.T) {
	t.Parallel()

	format := Format{Channels: 3, BytesPerSample: 2}
	planar := Planar{
		Format: format,
		Channels: [][]byte{
			{0x10, 0x11, 0x20, 0x21},
			{0x12, 0x13, 0x22, 0x23},
			{0x14, 0x15},
		},
	}

	got, err := PlanarToInterleaved(planar)
	if err != nil {
		t.Fatalf("PlanarToInterleaved: %v", err)
	}
	want := []byte{
		0x10, 0x11, 0x12, 0x13, 0x14, 0x15,
		0x20, 0x21, 0x22, 0x23,
	}
	if !bytes.Equal(got.Data, want) {
		t.Fatalf("interleaved data: got %v, want %v", got.Data, want)
	}
	if wantCounts := []int{2, 2, 1}; !reflect.DeepEqual(got.Counts, wantCounts) {
		t.Fatalf("counts: got %v, want %v", got.Counts, wantCounts)
	}
}

func TestInterleavedToPlanarOneSamplePerChannel(t *testing.T) {
	t.Parallel()

	format := Format{Channels: 5, BytesPerSample: 1}
	data := []byte{10, 11, 12, 13, 14}
	got, err := InterleavedToPlanar(Interleaved{Format: format, Data: data})
	if err != nil {
		t.Fatalf("InterleavedToPlanar: %v", err)
	}

	want := [][]byte{{10}, {11}, {12}, {13}, {14}}
	if !reflect.DeepEqual(got.Channels, want) {
		t.Fatalf("channels: got %v, want %v", got.Channels, want)
	}
	if wantCounts := []int{1, 1, 1, 1, 1}; !reflect.DeepEqual(got.Counts, wantCounts) {
		t.Fatalf("counts: got %v, want %v", got.Counts, wantCounts)
	}
}

func TestInterleavedToPlanarPartialFrame(t *testing.T) {
	t.Parallel()

	format := Format{Channels: 5, BytesPerSample: 2}
	data := []byte{
		0, 1, 2, 3, 4, 5, 6, 7, 8, 9,
		10, 11, 12, 13,
	}
	got, err := InterleavedToPlanar(Interleaved{Format: format, Data: data})
	if err != nil {
		t.Fatalf("InterleavedToPlanar: %v", err)
	}

	want := [][]byte{
		{0, 1, 10, 11},
		{2, 3, 12, 13},
		{4, 5},
		{6, 7},
		{8, 9},
	}
	if !reflect.DeepEqual(got.Channels, want) {
		t.Fatalf("channels: got %v, want %v", got.Channels, want)
	}
	if wantCounts := []int{2, 2, 1, 1, 1}; !reflect.DeepEqual(got.Counts, wantCounts) {
		t.Fatalf("counts: got %v, want %v", got.Counts, wantCounts)
	}
}

func TestEmptyInput(t *testing.T) {
	t.Parallel()

	format := Format{Channels: 3, BytesPerSample: 4}
	planar, err := InterleavedToPlanar(Interleaved{Format: format})
	if err != nil {
		t.Fatalf("InterleavedToPlanar: %v", err)
	}
	if len(planar.Channels) != 3 {
		t.Fatalf("got %d channels, want 3", len(planar.Channels))
	}
	for channel, data := range planar.Channels {
		if len(data) != 0 {
			t.Fatalf("channel %d has %d bytes, want 0", channel, len(data))
		}
	}
	if wantCounts := []int{0, 0, 0}; !reflect.DeepEqual(planar.Counts, wantCounts) {
		t.Fatalf("counts: got %v, want %v", planar.Counts, wantCounts)
	}

	interleaved, err := PlanarToInterleaved(planar)
	if err != nil {
		t.Fatalf("PlanarToInterleaved: %v", err)
	}
	if len(interleaved.Data) != 0 {
		t.Fatalf("got %d bytes, want 0", len(interleaved.Data))
	}
}

func TestInterleaveConvenienceWrapper(t *testing.T) {
	t.Parallel()

	data, counts, err := Interleave(
		Format{Channels: 3, BytesPerSample: 1},
		[][]byte{{1, 4}, {2, 5}, {3}},
	)
	if err != nil {
		t.Fatalf("Interleave: %v", err)
	}
	if want := []byte{1, 2, 3, 4, 5}; !bytes.Equal(data, want) {
		t.Fatalf("data: got %v, want %v", data, want)
	}
	if want := []int{2, 2, 1}; !reflect.DeepEqual(counts, want) {
		t.Fatalf("counts: got %v, want %v", counts, want)
	}
}

func TestRejectsMalformedInput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		run  func() error
		want error
	}{
		{
			name: "zero channels",
			run: func() error {
				_, err := InterleavedToPlanar(Interleaved{
					Format: Format{Channels: 0, BytesPerSample: 2},
				})
				return err
			},
			want: ErrInvalidChannels,
		},
		{
			name: "zero sample bytes",
			run: func() error {
				_, err := PlanarToInterleaved(Planar{
					Format: Format{Channels: 2, BytesPerSample: 0},
				})
				return err
			},
			want: ErrInvalidSampleBytes,
		},
		{
			name: "planar channel count",
			run: func() error {
				_, err := PlanarToInterleaved(Planar{
					Format:   Format{Channels: 2, BytesPerSample: 1},
					Channels: [][]byte{{1}, {2}, {3}},
				})
				return err
			},
			want: ErrChannelCount,
		},
		{
			name: "planar partial sample",
			run: func() error {
				_, err := PlanarToInterleaved(Planar{
					Format:   Format{Channels: 2, BytesPerSample: 2},
					Channels: [][]byte{{1, 2}, {3}},
				})
				return err
			},
			want: ErrPartialSample,
		},
		{
			name: "interleaved partial sample",
			run: func() error {
				_, err := InterleavedToPlanar(Interleaved{
					Format: Format{Channels: 2, BytesPerSample: 2},
					Data:   []byte{1},
				})
				return err
			},
			want: ErrPartialSample,
		},
		{
			name: "noncanonical planar lengths",
			run: func() error {
				_, err := PlanarToInterleaved(Planar{
					Format:   Format{Channels: 3, BytesPerSample: 1},
					Channels: [][]byte{{1, 2, 3}, {4}, {5, 6}},
				})
				return err
			},
			want: ErrInconsistentChannels,
		},
		{
			name: "stale planar counts",
			run: func() error {
				_, err := PlanarToInterleaved(Planar{
					Format:   Format{Channels: 2, BytesPerSample: 1},
					Channels: [][]byte{{1}, {2}},
					Counts:   []int{1, 2},
				})
				return err
			},
			want: ErrCountsMismatch,
		},
		{
			name: "stale interleaved counts",
			run: func() error {
				_, err := InterleavedToPlanar(Interleaved{
					Format: Format{Channels: 2, BytesPerSample: 1},
					Data:   []byte{1, 2, 3},
					Counts: []int{1, 1},
				})
				return err
			},
			want: ErrCountsMismatch,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			err := test.run()
			if !errors.Is(err, test.want) {
				t.Fatalf("got error %v, want %v", err, test.want)
			}
		})
	}
}

func verifyCounts(t *testing.T, got []int, channels, fullFrames, partial int) {
	t.Helper()
	if len(got) != channels {
		t.Fatalf("got %d counts, want %d", len(got), channels)
	}
	for channel, count := range got {
		want := fullFrames
		if channel < partial {
			want++
		}
		if count != want {
			t.Fatalf("channel %d count: got %d, want %d", channel, count, want)
		}
	}
}

func verifyPlanarSamples(t *testing.T, planar Planar, interleaved []byte) {
	t.Helper()
	sampleBytes := planar.Format.BytesPerSample
	for frame := 0; frame < maxCount(planar.Counts); frame++ {
		for channel, count := range planar.Counts {
			if frame >= count {
				continue
			}
			sample := planar.Channels[channel][frame*sampleBytes : (frame+1)*sampleBytes]
			if !bytes.Equal(sample, interleaved[:sampleBytes]) {
				t.Fatalf("frame %d channel %d: got %v, want %v",
					frame, channel, sample, interleaved[:sampleBytes])
			}
			interleaved = interleaved[sampleBytes:]
		}
	}
	if len(interleaved) != 0 {
		t.Fatalf("%d interleaved bytes were not verified", len(interleaved))
	}
}

func maxCount(counts []int) int {
	max := 0
	for _, count := range counts {
		if count > max {
			max = count
		}
	}
	return max
}

func testBytes(n int) []byte {
	data := make([]byte, n)
	for i := range data {
		data[i] = byte(i*37 + 11)
	}
	return data
}

func testName(channels, sampleBytes, fullFrames, partial int) string {
	return fmt.Sprintf("channels=%d/sample=%d/full=%d/partial=%d",
		channels, sampleBytes, fullFrames, partial)
}
