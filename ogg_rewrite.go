package vorbiscomment

import (
	"encoding/binary"
	"fmt"
)

// RewriteOgg rewrites every Vorbis comment packet while preserving all audio
// packets and unrelated logical streams byte-for-byte. It fails rather than
// guessing when a header packet shares an Ogg page with audio.
func RewriteOgg(data []byte, parseOpts Options, rewriteOpts RewriteOptions) ([]byte, *OggResult, error) {
	result, err := InspectOgg(data, parseOpts)
	if err != nil {
		return nil, nil, err
	}
	serials := make([]uint32, 0, len(result.Comments))
	seen := make(map[uint32]bool)
	for _, item := range result.Comments {
		if !seen[item.Serial] {
			seen[item.Serial] = true
			serials = append(serials, item.Serial)
		}
	}

	current := data
	for _, serial := range serials {
		current, err = rewriteOggStream(current, serial, parseOpts, rewriteOpts)
		if err != nil {
			return nil, result, err
		}
	}
	return current, result, nil
}

func rewriteOggStream(data []byte, serial uint32, parseOpts Options, rewriteOpts RewriteOptions) ([]byte, error) {
	doc, err := parseOgg(data)
	if err != nil {
		return nil, err
	}
	refs, err := findVorbisComments(doc, parseOpts)
	if err != nil {
		return nil, err
	}
	var ref *vorbisCommentRef
	for i := range refs {
		if refs[i].stream.serial == serial {
			ref = &refs[i]
			break
		}
	}
	if ref == nil {
		return nil, fmt.Errorf("%w: serial %d", ErrNoVorbisComment, serial)
	}
	stream := ref.stream
	if len(stream.packets) < 3 {
		return nil, fmt.Errorf("%w: serial %d has no setup header", ErrUnsupportedOgg, serial)
	}
	setup := stream.packets[2]
	for i := 3; i < len(stream.packets); i++ {
		if stream.packets[i].startPage <= setup.endPage {
			return nil, fmt.Errorf("%w: serial %d has audio on header page %d",
				ErrUnsupportedOgg, serial, setup.endPage)
		}
	}

	commentPacket, err := ref.comment.Marshal(rewriteOpts)
	if err != nil {
		return nil, fmt.Errorf("serial %d comment packet: %w", serial, err)
	}
	headerPackets := [][]byte{
		stream.packets[0].data,
		commentPacket,
		stream.packets[2].data,
	}
	firstPage := doc.pages[stream.pages[0]]
	headerPages := buildHeaderPages(serial, firstPage.sequence, headerPackets)
	headerFirst := stream.pages[0]
	headerLast := stream.packets[2].endPage

	capHint := len(data) + len(commentPacket) - len(ref.packet.data)
	if capHint < 0 {
		capHint = 0
	}
	out := make([]byte, 0, capHint)
	nextSequence := firstPage.sequence + uint32(len(headerPages))
	wroteHeader := false
	for _, page := range doc.pages {
		if page.serial == serial && page.index >= headerFirst && page.index <= headerLast {
			if !wroteHeader {
				for _, headerPage := range headerPages {
					out = append(out, headerPage...)
				}
				wroteHeader = true
			}
			continue
		}
		if page.serial == serial && page.index > headerLast {
			out = append(out, rewriteOggPageSequence(page, nextSequence)...)
			nextSequence++
			continue
		}
		out = append(out, page.raw...)
	}
	if !wroteHeader {
		return nil, fmt.Errorf("%w: serial %d header pages missing", ErrUnsupportedOgg, serial)
	}
	return out, nil
}

type laceSegment struct {
	packetIndex int
	offset      int
	length      int
}

func buildHeaderPages(serial, sequence uint32, packets [][]byte) [][]byte {
	var pages [][]byte
	first := buildPacketPages(serial, sequence, packets[:1], true)
	pages = append(pages, first...)
	sequence += uint32(len(first))
	pages = append(pages, buildPacketPages(serial, sequence, packets[1:], false)...)
	return pages
}

func buildPacketPages(serial, sequence uint32, packets [][]byte, bos bool) [][]byte {
	var laces []laceSegment
	for packetIndex, packet := range packets {
		for offset := 0; ; {
			length := len(packet) - offset
			if length > 255 {
				length = 255
			}
			laces = append(laces, laceSegment{packetIndex: packetIndex, offset: offset, length: length})
			offset += length
			if length < 255 {
				break
			}
		}
	}

	var pages [][]byte
	for len(laces) > 0 {
		count := len(laces)
		if count > 255 {
			count = 255
		}
		chunk := laces[:count]
		laces = laces[count:]
		flags := byte(0)
		if len(pages) == 0 && bos {
			flags |= 0x02
		}
		if chunk[0].offset > 0 {
			flags |= 0x01
		}
		granule := uint64(0)
		if last := chunk[len(chunk)-1]; last.length == 255 {
			granule = ^uint64(0)
		}
		pages = append(pages, makeOggPage(serial, sequence, flags, granule, chunk, packets))
		sequence++
	}
	return pages
}

func makeOggPage(serial, sequence uint32, flags byte, granule uint64, laces []laceSegment, packets [][]byte) []byte {
	payloadLength := 0
	for _, lace := range laces {
		payloadLength += lace.length
	}
	page := make([]byte, 27+len(laces)+payloadLength)
	copy(page, "OggS")
	page[4] = 0
	page[5] = flags
	binary.LittleEndian.PutUint64(page[6:14], granule)
	binary.LittleEndian.PutUint32(page[14:18], serial)
	binary.LittleEndian.PutUint32(page[18:22], sequence)
	page[26] = byte(len(laces))
	segmentAt := 27
	payloadAt := 27 + len(laces)
	for _, lace := range laces {
		page[segmentAt] = byte(lace.length)
		segmentAt++
		if lace.length > 0 {
			copy(page[payloadAt:], packets[lace.packetIndex][lace.offset:lace.offset+lace.length])
			payloadAt += lace.length
		}
	}
	putOggChecksum(page)
	return page
}

func rewriteOggPageSequence(page oggPage, sequence uint32) []byte {
	out := append([]byte(nil), page.raw...)
	binary.LittleEndian.PutUint32(out[18:22], sequence)
	putOggChecksum(out)
	return out
}

func putOggChecksum(page []byte) {
	binary.LittleEndian.PutUint32(page[22:26], 0)
	binary.LittleEndian.PutUint32(page[22:26], oggCRC(page))
}
