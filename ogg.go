package opusalign

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
)

const (
	oggPageHeaderSize   = 27
	oggFlagContinuation = 0x01
	oggFlagBOS          = 0x02
	oggFlagEOS          = 0x04
)

type OggPage struct {
	Continuation    bool
	BOS             bool
	EOS             bool
	GranulePosition int64
	Serial          uint32
	Sequence        uint32
	SegmentTable    []byte
	Data            []byte
}

type OggPacket struct {
	Index           int
	Data            []byte
	GranulePosition *int64
	EndOfStream     bool
	FirstOnStream   bool
}

func ParseOggPages(reader io.Reader) ([]OggPage, error) {
	buffered := bufio.NewReader(reader)
	pages := make([]OggPage, 0)
	for {
		var header [oggPageHeaderSize]byte
		_, err := io.ReadFull(buffered, header[:])
		if err == io.EOF {
			return pages, nil
		}
		if err != nil {
			return nil, fmt.Errorf("read Ogg page header: %w", err)
		}
		if string(header[0:4]) != "OggS" {
			return nil, fmt.Errorf("bad Ogg capture pattern at page %d", len(pages))
		}
		if header[4] != 0 {
			return nil, fmt.Errorf("unsupported Ogg version %d", header[4])
		}

		segmentCount := int(header[26])
		segmentTable := make([]byte, segmentCount)
		if _, err := io.ReadFull(buffered, segmentTable); err != nil {
			return nil, fmt.Errorf("read Ogg segment table: %w", err)
		}
		dataLength := 0
		for _, size := range segmentTable {
			dataLength += int(size)
		}
		data := make([]byte, dataLength)
		if _, err := io.ReadFull(buffered, data); err != nil {
			return nil, fmt.Errorf("read Ogg page data: %w", err)
		}

		headerType := header[5]
		pages = append(pages, OggPage{
			Continuation:    headerType&oggFlagContinuation != 0,
			BOS:             headerType&oggFlagBOS != 0,
			EOS:             headerType&oggFlagEOS != 0,
			GranulePosition: int64(binary.LittleEndian.Uint64(header[6:14])),
			Serial:          binary.LittleEndian.Uint32(header[14:18]),
			Sequence:        binary.LittleEndian.Uint32(header[18:22]),
			SegmentTable:    segmentTable,
			Data:            data,
		})
	}
}

func ReadOggPackets(reader io.Reader) ([]OggPacket, error) {
	pages, err := ParseOggPages(reader)
	if err != nil {
		return nil, err
	}

	packets := make([]OggPacket, 0)
	var pending []byte
	packetIndex := 0
	previousGranule := int64(0)

	for pageIndex, page := range pages {
		lastCompletion := -1
		for i, lacing := range page.SegmentTable {
			if lacing < 255 {
				lastCompletion = i
			}
		}

		completedIndices := make([]int, 0)
		dataOffset := 0
		for segmentIndex, lacing := range page.SegmentTable {
			segmentEnd := dataOffset + int(lacing)
			if segmentEnd > len(page.Data) {
				return nil, fmt.Errorf("ogg page %d segment %d exceeds page data", pageIndex, segmentIndex)
			}
			pending = append(pending, page.Data[dataOffset:segmentEnd]...)
			dataOffset = segmentEnd

			if lacing == 255 {
				continue
			}

			currentIndex := packetIndex
			packet := OggPacket{
				Index:         currentIndex,
				Data:          append([]byte(nil), pending...),
				FirstOnStream: page.BOS && currentIndex == 0,
			}
			if segmentIndex == lastCompletion {
				packet.EndOfStream = page.EOS
			}
			packets = append(packets, packet)
			completedIndices = append(completedIndices, currentIndex)
			packetIndex++
			pending = nil
		}

		if page.GranulePosition < 0 || lastCompletion < 0 || len(completedIndices) == 0 {
			continue
		}

		if page.EOS && assignEOSGranules(page, packets, completedIndices, previousGranule) {
			previousGranule = page.GranulePosition
			continue
		}

		end := page.GranulePosition
		for i := len(completedIndices) - 1; i >= 0; i-- {
			packetIndex := completedIndices[i]
			granule := end
			packets[packetIndex].GranulePosition = &granule
			duration, _, err := PacketDurationSamples(packets[packetIndex].Data)
			if err != nil {
				break
			}
			end -= int64(duration)
		}
		previousGranule = page.GranulePosition
	}

	if len(pending) != 0 {
		return nil, fmt.Errorf("truncated Ogg packet with %d trailing bytes", len(pending))
	}
	return packets, nil
}

func assignEOSGranules(page OggPage, packets []OggPacket, completedIndices []int, previousGranule int64) bool {
	durations := make([]int64, len(completedIndices))
	total := int64(0)
	for i, packetIndex := range completedIndices {
		duration, _, err := PacketDurationSamples(packets[packetIndex].Data)
		if err != nil {
			return false
		}
		durations[i] = int64(duration)
		total += durations[i]
	}
	expectedEnd := previousGranule + total
	if page.GranulePosition < previousGranule || page.GranulePosition >= expectedEnd {
		return false
	}

	cursor := previousGranule
	last := len(completedIndices) - 1
	for i, packetIndex := range completedIndices {
		cursor += durations[i]
		granule := cursor
		if i == last {
			granule = page.GranulePosition
		}
		packets[packetIndex].GranulePosition = &granule
	}
	return true
}

func AnalyzeOgg(reader io.Reader) (Report, OpusHead, error) {
	oggPackets, err := ReadOggPackets(reader)
	if err != nil {
		return Report{}, OpusHead{}, err
	}

	var head OpusHead
	haveHead := false
	timeline := TimelineInput{}
	for _, oggPacket := range oggPackets {
		switch {
		case len(oggPacket.Data) >= 8 && string(oggPacket.Data[:8]) == opusHeadMagic:
			head, err = ParseOpusHead(oggPacket.Data)
			if err != nil {
				return Report{}, OpusHead{}, err
			}
			haveHead = true
			timeline.PreSkip = int(head.PreSkip)
		case len(oggPacket.Data) >= 8 && string(oggPacket.Data[:8]) == "OpusTags":
			continue
		default:
			timeline.Packets = append(timeline.Packets, Packet{
				ID:              fmt.Sprintf("ogg-packet-%d", oggPacket.Index),
				Data:            oggPacket.Data,
				GranulePosition: oggPacket.GranulePosition,
				EndOfStream:     oggPacket.EndOfStream,
			})
		}
	}
	if !haveHead {
		return Report{}, OpusHead{}, fmt.Errorf("OpusHead packet not found")
	}
	report, err := BuildTimeline(timeline)
	return report, head, err
}
