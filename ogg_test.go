package vorbiscomment

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func pageWithPackets(serial, sequence uint32, flags byte, granule uint64, packets ...[]byte) []byte {
	var laces []laceSegment
	for index, packet := range packets {
		for offset := 0; ; {
			length := len(packet) - offset
			if length > 255 {
				length = 255
			}
			laces = append(laces, laceSegment{packetIndex: index, offset: offset, length: length})
			offset += length
			if length < 255 {
				break
			}
		}
	}
	return makeOggPage(serial, sequence, flags, granule, laces, packets)
}

func makeVorbisFile(commentPacket, audioPacket []byte) []byte {
	ident := append([]byte{0x01}, []byte("vorbis")...)
	setup := append([]byte{0x05}, []byte("vorbis")...)
	var out []byte
	out = append(out, pageWithPackets(7, 0, 0x02, 0, ident)...)
	out = append(out, pageWithPackets(7, 1, 0, 0, commentPacket)...)
	out = append(out, pageWithPackets(7, 2, 0, 0, setup)...)
	out = append(out, pageWithPackets(7, 3, 0, 1234, audioPacket)...)
	return out
}

func makeCommentPacketNoFraming(vendor string, entries ...string) []byte {
	return append(append([]byte(nil), packetMagic...), makeBody(vendor, entries...)...)
}

func TestInspectOggMapsOffsetsToFile(t *testing.T) {
	payload := append(makeBody("encoder", "TITLE=\xff"), 1)
	packet := append(append([]byte(nil), packetMagic...), payload...)
	file := makeVorbisFile(packet, []byte{0x00, 'a'})

	result, err := InspectOgg(file, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Comments) != 1 {
		t.Fatalf("comments = %d", len(result.Comments))
	}
	comment := result.Comments[0].Comment
	issue := findIssue(t, comment, CodeInvalidUTF8)
	badOffset := int(issue.Offset)
	if badOffset < 0 || badOffset >= len(file) || file[badOffset] != 0xff {
		t.Fatalf("offset %d does not point at invalid byte", badOffset)
	}
	if result.Comments[0].PacketOffset < 27 ||
		result.Comments[0].PacketOffset >= int64(len(file)) {
		t.Fatalf("packet offset = %d", result.Comments[0].PacketOffset)
	}
}

func TestRewriteOggPreservesAudioAndCommentBytes(t *testing.T) {
	commentPacket := makeCommentPacketNoFraming("encoder",
		"ARTIST=one", "artist=two", "X-UNKNOWN=\xff", "LONG="+string(bytes.Repeat([]byte{'z'}, 40)))
	audioPacket := []byte{0x00, 1, 2, 3, 4, 5}
	file := makeVorbisFile(commentPacket, audioPacket)

	out, report, err := RewriteOgg(file, Options{MaxValueBytes: 20}, RewriteOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Comments) != 1 {
		t.Fatalf("report comments = %d", len(report.Comments))
	}
	if !report.Comments[0].Comment.RequiresManualConfirmation() {
		t.Fatal("expected manual findings")
	}

	original, err := parseOgg(file)
	if err != nil {
		t.Fatal(err)
	}
	rewritten, err := parseOgg(out)
	if err != nil {
		t.Fatal(err)
	}
	before := original.streams[7].packets
	after := rewritten.streams[7].packets
	if !bytes.Equal(before[1].data, after[1].data) {
		t.Fatal("conservative rewrite changed comment packet")
	}
	if !bytes.Equal(before[3].data, after[3].data) {
		t.Fatal("rewrite changed audio packet")
	}
	verifyPageCRCs(t, out)
	verifySequences(t, rewritten, 7)
}

func TestRewriteOggSafeRepairsFraming(t *testing.T) {
	commentPacket := makeCommentPacketNoFraming("encoder", "TITLE=hello")
	file := makeVorbisFile(commentPacket, []byte{0x00, 'a'})
	inspected, err := InspectOgg(file, Options{})
	if err != nil {
		t.Fatal(err)
	}
	findIssue(t, inspected.Comments[0].Comment, CodeMissingFramingBit)

	out, _, err := RewriteOgg(file, Options{}, RewriteOptions{Mode: RewriteSafe})
	if err != nil {
		t.Fatal(err)
	}
	rewritten, err := parseOgg(out)
	if err != nil {
		t.Fatal(err)
	}
	packet := rewritten.streams[7].packets[1].data
	if len(packet) != len(commentPacket)+1 || packet[len(packet)-1] != 1 {
		t.Fatalf("framing bit was not restored: %x", packet)
	}
	verifyPageCRCs(t, out)
	verifySequences(t, rewritten, 7)
}

func TestRewriteOggRejectsSharedAudioPage(t *testing.T) {
	ident := append([]byte{0x01}, []byte("vorbis")...)
	setup := append([]byte{0x05}, []byte("vorbis")...)
	comment := makePacket("encoder", "TITLE=hello")
	audio := []byte{0x00, 'a'}
	var file []byte
	file = append(file, pageWithPackets(9, 0, 0x02, 0, ident)...)
	file = append(file, pageWithPackets(9, 1, 0, 0, comment, setup, audio)...)

	_, _, err := RewriteOgg(file, Options{}, RewriteOptions{})
	if err == nil {
		t.Fatal("expected unsupported shared-page error")
	}
}

func TestOggCommentPacketSpanningPagesKeepsOffsetsAndAudio(t *testing.T) {
	comment := makePacket("encoder",
		"LONG="+string(bytes.Repeat([]byte{'x'}, 220)),
		"TARGET=second-page")
	if len(comment) <= 255 {
		t.Fatal("test packet is too short to span pages")
	}
	firstPart := comment[:255]
	secondPart := comment[255:]
	ident := append([]byte{0x01}, []byte("vorbis")...)
	setup := append([]byte{0x05}, []byte("vorbis")...)
	audio := []byte{0x00, 'a', 'b'}

	var file []byte
	file = append(file, pageWithPackets(11, 0, 0x02, 0, ident)...)
	file = append(file, makeOggPage(11, 1, 0, 0,
		[]laceSegment{{packetIndex: 0, offset: 0, length: len(firstPart)}},
		[][]byte{firstPart})...)
	file = append(file, makeOggPage(11, 2, 0x01, 0,
		[]laceSegment{{packetIndex: 0, offset: 0, length: len(secondPart)}},
		[][]byte{secondPart})...)
	file = append(file, pageWithPackets(11, 3, 0, 0, setup)...)
	file = append(file, pageWithPackets(11, 4, 0, 999, audio)...)

	result, err := InspectOgg(file, Options{})
	if err != nil {
		t.Fatal(err)
	}
	entries := result.Comments[0].Comment.Entries
	if len(entries) != 2 {
		t.Fatalf("entries = %d", len(entries))
	}
	if entries[1].Offset <= 255 || file[entries[1].Offset] != 'T' {
		t.Fatalf("entry offset %d was not mapped to the second page", entries[1].Offset)
	}

	out, _, err := RewriteOgg(file, Options{}, RewriteOptions{Mode: RewriteSafe})
	if err != nil {
		t.Fatal(err)
	}
	rewritten, err := parseOgg(out)
	if err != nil {
		t.Fatal(err)
	}
	packets := rewritten.streams[11].packets
	if !bytes.Equal(packets[1].data, comment) {
		t.Fatal("comment packet changed")
	}
	if !bytes.Equal(packets[3].data, audio) {
		t.Fatal("audio packet changed")
	}
	verifyPageCRCs(t, out)
	verifySequences(t, rewritten, 11)
}

func verifyPageCRCs(t *testing.T, data []byte) {
	t.Helper()
	doc, err := parseOgg(data)
	if err != nil {
		t.Fatal(err)
	}
	for _, page := range doc.pages {
		want := binary.LittleEndian.Uint32(page.raw[22:26])
		copyPage := append([]byte(nil), page.raw...)
		binary.LittleEndian.PutUint32(copyPage[22:26], 0)
		if got := oggCRC(copyPage); got != want {
			t.Fatalf("page %d CRC = %08x, want %08x", page.index, want, got)
		}
	}
}

func verifySequences(t *testing.T, doc *oggDocument, serial uint32) {
	t.Helper()
	stream := doc.streams[serial]
	for i, pageIndex := range stream.pages {
		if got := doc.pages[pageIndex].sequence; got != uint32(i) {
			t.Fatalf("page %d sequence = %d, want %d", i, got, i)
		}
	}
}
