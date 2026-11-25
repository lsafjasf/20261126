package vorbiscomment

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"sort"
)

var (
	ErrNotOgg              = errors.New("ogg: not an Ogg stream")
	ErrNoVorbisComment     = errors.New("ogg: no Vorbis comment packet found")
	ErrUnsupportedOgg      = errors.New("ogg: unsupported page layout")
	ErrMalformedOggPage    = errors.New("ogg: malformed page")
	ErrIncompleteOggPacket = errors.New("ogg: incomplete packet at end of stream")
)

type oggPage struct {
	index       int
	fileOffset  int64
	raw         []byte
	flags       byte
	granule     uint64
	serial      uint32
	sequence    uint32
	checksum    uint32
	segments    []byte
	payload     []byte
	headerBytes int
}

type packetSpan struct {
	logicalStart  int
	physicalStart int64
	length        int
}

type oggPacket struct {
	data        []byte
	spans       []packetSpan
	startPage   int
	endPage     int
	startOffset int64
}

type oggStream struct {
	serial  uint32
	pages   []int
	packets []oggPacket
}

type oggDocument struct {
	data    []byte
	pages   []oggPage
	streams map[uint32]*oggStream
	order   []uint32
}

func parseOgg(data []byte) (*oggDocument, error) {
	if !bytes.HasPrefix(data, []byte("OggS")) {
		return nil, ErrNotOgg
	}
	doc := &oggDocument{
		data:    data,
		streams: make(map[uint32]*oggStream),
	}
	for offset := 0; offset < len(data); {
		page, err := parseOggPage(data, offset, len(doc.pages))
		if err != nil {
			return nil, err
		}
		doc.pages = append(doc.pages, page)
		stream := doc.streams[page.serial]
		if stream == nil {
			stream = &oggStream{serial: page.serial}
			doc.streams[page.serial] = stream
			doc.order = append(doc.order, page.serial)
		}
		stream.pages = append(stream.pages, page.index)
		offset += len(page.raw)
	}
	for serial, stream := range doc.streams {
		if err := assemblePackets(doc, stream); err != nil {
			return nil, fmt.Errorf("serial %d: %w", serial, err)
		}
	}
	return doc, nil
}

func parseOggPage(data []byte, offset, index int) (oggPage, error) {
	if len(data)-offset < 27 {
		return oggPage{}, fmt.Errorf("%w at %d: header is truncated", ErrMalformedOggPage, offset)
	}
	raw := data[offset:]
	if !bytes.Equal(raw[:4], []byte("OggS")) {
		return oggPage{}, fmt.Errorf("%w at %d: missing capture pattern", ErrMalformedOggPage, offset)
	}
	if raw[4] != 0 {
		return oggPage{}, fmt.Errorf("%w at %d: version %d", ErrMalformedOggPage, offset, raw[4])
	}
	segmentCount := int(raw[26])
	headerBytes := 27 + segmentCount
	if len(raw) < headerBytes {
		return oggPage{}, fmt.Errorf("%w at %d: segment table is truncated", ErrMalformedOggPage, offset)
	}
	segments := raw[27:headerBytes]
	payloadLength := 0
	for _, segment := range segments {
		payloadLength += int(segment)
	}
	if len(raw) < headerBytes+payloadLength {
		return oggPage{}, fmt.Errorf("%w at %d: payload is truncated", ErrMalformedOggPage, offset)
	}
	pageLength := headerBytes + payloadLength
	return oggPage{
		index:       index,
		fileOffset:  int64(offset),
		raw:         raw[:pageLength],
		flags:       raw[5],
		granule:     binary.LittleEndian.Uint64(raw[6:14]),
		serial:      binary.LittleEndian.Uint32(raw[14:18]),
		sequence:    binary.LittleEndian.Uint32(raw[18:22]),
		checksum:    binary.LittleEndian.Uint32(raw[22:26]),
		segments:    segments,
		payload:     raw[headerBytes : headerBytes+payloadLength],
		headerBytes: headerBytes,
	}, nil
}

func assemblePackets(doc *oggDocument, stream *oggStream) error {
	var current []byte
	var spans []packetSpan
	var startPage int
	var startOffset int64

	for _, pageIndex := range stream.pages {
		page := &doc.pages[pageIndex]
		payloadAt := 0
		for _, segment := range page.segments {
			length := int(segment)
			if len(current) == 0 {
				startPage = page.index
				startOffset = page.fileOffset + int64(page.headerBytes+payloadAt)
			}
			if length > 0 {
				chunk := page.payload[payloadAt : payloadAt+length]
				if n := len(spans); n > 0 && spans[n-1].physicalStart+int64(spans[n-1].length) ==
					page.fileOffset+int64(page.headerBytes+payloadAt) {
					spans[n-1].length += length
				} else {
					spans = append(spans, packetSpan{
						logicalStart:  len(current),
						physicalStart: page.fileOffset + int64(page.headerBytes+payloadAt),
						length:        length,
					})
				}
				current = append(current, chunk...)
			}
			payloadAt += length
			if segment == 255 {
				continue
			}
			stream.packets = append(stream.packets, oggPacket{
				data:        append([]byte(nil), current...),
				spans:       spans,
				startPage:   startPage,
				endPage:     page.index,
				startOffset: startOffset,
			})
			current = nil
			spans = nil
		}
	}
	if len(current) > 0 {
		return ErrIncompleteOggPacket
	}
	return nil
}

// OggComment identifies one Vorbis comment packet inside an Ogg file.
type OggComment struct {
	Serial       uint32   `json:"serial"`
	PacketIndex  int      `json:"packet_index"`
	PacketOffset int64    `json:"packet_offset"`
	Comment      *Comment `json:"comment"`
}

// OggResult contains every Vorbis comment packet found in an Ogg file.
type OggResult struct {
	Comments []OggComment `json:"comments"`
}

// InspectOgg leniently parses Vorbis comment packets in an Ogg file.
func InspectOgg(data []byte, opts Options) (*OggResult, error) {
	doc, err := parseOgg(data)
	if err != nil {
		return nil, err
	}
	refs, err := findVorbisComments(doc, opts)
	if err != nil {
		return nil, err
	}
	result := &OggResult{}
	for _, ref := range refs {
		result.Comments = append(result.Comments, OggComment{
			Serial:       ref.stream.serial,
			PacketIndex:  ref.packetIndex,
			PacketOffset: ref.packet.startOffset + opts.BaseOffset,
			Comment:      ref.comment,
		})
	}
	sort.Slice(result.Comments, func(i, j int) bool {
		return result.Comments[i].PacketOffset < result.Comments[j].PacketOffset
	})
	return result, nil
}

type vorbisCommentRef struct {
	stream      *oggStream
	packetIndex int
	packet      oggPacket
	comment     *Comment
}

func findVorbisComments(doc *oggDocument, opts Options) ([]vorbisCommentRef, error) {
	var refs []vorbisCommentRef
	for _, serial := range doc.order {
		stream := doc.streams[serial]
		if len(stream.packets) < 2 {
			continue
		}
		if !bytes.HasPrefix(stream.packets[0].data, []byte{0x01, 'v', 'o', 'r', 'b', 'i', 's'}) {
			continue
		}
		packet := stream.packets[1]
		if !bytes.HasPrefix(packet.data, packetMagic) {
			continue
		}
		comment, err := Parse(packet.data, Options{MaxValueBytes: opts.MaxValueBytes})
		if err != nil {
			return nil, fmt.Errorf("serial %d comment packet: %w", serial, err)
		}
		remapCommentOffsets(comment, packet, opts.BaseOffset)
		refs = append(refs, vorbisCommentRef{
			stream:      stream,
			packetIndex: 1,
			packet:      packet,
			comment:     comment,
		})
	}
	if len(refs) == 0 {
		return nil, ErrNoVorbisComment
	}
	return refs, nil
}

func (p oggPacket) fileOffset(logical int) int64 {
	if len(p.spans) == 0 {
		return p.startOffset + int64(logical)
	}
	for i := len(p.spans) - 1; i >= 0; i-- {
		span := p.spans[i]
		if logical >= span.logicalStart {
			return span.physicalStart + int64(logical-span.logicalStart)
		}
	}
	return p.startOffset
}

func remapCommentOffsets(comment *Comment, packet oggPacket, base int64) {
	mapper := func(logical int64) int64 { return base + packet.fileOffset(int(logical)) }
	comment.HeaderOffset = mapper(0)
	for i := range comment.Entries {
		entry := &comment.Entries[i]
		entry.Offset = mapper(entry.Offset)
		entry.KeyOffset = mapper(entry.KeyOffset)
		entry.ValueOffset = mapper(entry.ValueOffset)
	}
	for i := range comment.Issues {
		comment.Issues[i].Offset = mapper(comment.Issues[i].Offset)
	}
}
