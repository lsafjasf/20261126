package aiffmeta

import (
	"encoding/binary"
	"fmt"
	"io"
	"strings"
)

func parseTextChunk(r io.ReaderAt, chunk Chunk, text *TextMetadata, f *File) error {
	payload, err := readPayload(r, chunk)
	if err != nil {
		return err
	}
	switch chunk.ID {
	case "NAME", "AUTH", "ANNO", "(c) ":
		value := cleanText(payload)
		text.Chunks = append(text.Chunks, TextChunk{
			ChunkIndex: chunk.Index,
			ID:         chunk.ID,
			DataOffset: chunk.DataOffset,
			Text:       value,
		})
		switch chunk.ID {
		case "NAME":
			if countTextChunk(text.Chunks, "NAME") > 1 {
				f.addIssue(chunk.HeaderOffset, chunk.ID, "duplicate NAME chunk; first value retained")
			} else {
				text.Name = value
			}
		case "AUTH":
			if countTextChunk(text.Chunks, "AUTH") > 1 {
				f.addIssue(chunk.HeaderOffset, chunk.ID, "duplicate AUTH chunk; first value retained")
			} else {
				text.Author = value
			}
		case "(c) ":
			if countTextChunk(text.Chunks, "(c) ") > 1 {
				f.addIssue(chunk.HeaderOffset, chunk.ID, "duplicate copyright chunk; first value retained")
			} else {
				text.Copyright = value
			}
		case "ANNO":
			text.Annotations = append(text.Annotations, value)
		}
	case "COMT":
		parseCOMT(payload, chunk, text, f)
	case "MARK":
		parseMARK(payload, chunk, text, f)
	default:
		return fmt.Errorf("unsupported text chunk %q", chunk.ID)
	}
	return nil
}

func countTextChunk(chunks []TextChunk, id string) int {
	count := 0
	for _, chunk := range chunks {
		if chunk.ID == id {
			count++
		}
	}
	return count
}

func parseCOMT(payload []byte, chunk Chunk, text *TextMetadata, f *File) {
	if len(payload) < 2 {
		f.addIssue(chunk.DataOffset, chunk.ID, "payload is too short for comment count")
		return
	}
	count := int(binary.BigEndian.Uint16(payload[0:2]))
	pos := 2
	for i := 0; i < count; i++ {
		if len(payload)-pos < 8 {
			f.addIssue(chunk.DataOffset+int64(pos), chunk.ID,
				"comment %d: need 8-byte header, only %d byte(s) remain", i, len(payload)-pos)
			return
		}
		timestamp := binary.BigEndian.Uint32(payload[pos : pos+4])
		markerID := int16(binary.BigEndian.Uint16(payload[pos+4 : pos+6]))
		textSize := int(binary.BigEndian.Uint16(payload[pos+6 : pos+8]))
		pos += 8
		if len(payload)-pos < textSize {
			f.addIssue(chunk.DataOffset+int64(pos), chunk.ID,
				"comment %d: declares %d text byte(s), only %d remain",
				i, textSize, len(payload)-pos)
			return
		}
		commentText := cleanText(payload[pos : pos+textSize])
		text.Comments = append(text.Comments, Comment{
			TimeStamp: timestamp,
			MarkerID:  markerID,
			Text:      commentText,
		})
		pos += textSize
		if textSize&1 != 0 && pos < len(payload) {
			pos++
		}
	}
	if pos < len(payload) {
		f.addIssue(chunk.DataOffset+int64(pos), chunk.ID,
			"%d trailing byte(s) after %d comment(s)", len(payload)-pos, count)
	}
}

func parseMARK(payload []byte, chunk Chunk, text *TextMetadata, f *File) {
	if len(payload) < 2 {
		f.addIssue(chunk.DataOffset, chunk.ID, "payload is too short for marker count")
		return
	}
	count := int(binary.BigEndian.Uint16(payload[0:2]))
	pos := 2
	seen := make(map[int16]struct{}, count)
	for i := 0; i < count; i++ {
		if len(payload)-pos < 7 {
			f.addIssue(chunk.DataOffset+int64(pos), chunk.ID,
				"marker %d: need id/position/name, only %d byte(s) remain", i, len(payload)-pos)
			return
		}
		id := int16(binary.BigEndian.Uint16(payload[pos : pos+2]))
		position := binary.BigEndian.Uint32(payload[pos+2 : pos+6])
		name, consumed, err := parsePString(payload[pos+6:])
		if err != nil {
			f.addIssue(chunk.DataOffset+int64(pos+6), chunk.ID,
				"marker %d name: %v", i, err)
			return
		}
		if id == 0 {
			f.addIssue(chunk.DataOffset+int64(pos), chunk.ID, "marker %d uses reserved id 0", i)
		}
		if _, duplicate := seen[id]; duplicate {
			f.addIssue(chunk.DataOffset+int64(pos), chunk.ID, "duplicate marker id %d", id)
		}
		seen[id] = struct{}{}
		text.Markers = append(text.Markers, Marker{
			ID:       id,
			Position: position,
			Name:     name,
		})
		pos += 6 + consumed
	}
	if pos < len(payload) {
		f.addIssue(chunk.DataOffset+int64(pos), chunk.ID,
			"%d trailing byte(s) after %d marker(s)", len(payload)-pos, count)
	}
}

func cleanText(b []byte) string {
	return strings.TrimRight(string(b), "\x00")
}
