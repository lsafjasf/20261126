package aiffmeta

import "fmt"

// LostItem is metadata that is absent from a rewritten or resaved file.
type LostItem struct {
	Kind      string `json:"kind"`
	ChunkID   string `json:"chunk_id,omitempty"`
	Text      string `json:"text,omitempty"`
	TimeStamp uint32 `json:"timestamp,omitempty"`
	MarkerID  int16  `json:"marker_id,omitempty"`
	Position  uint32 `json:"position,omitempty"`
}

func lostItemsForChunk(f *File, chunk Chunk) []LostItem {
	switch chunk.Kind {
	case KindText:
		for _, tc := range f.Text.Chunks {
			if tc.ChunkIndex == chunk.Index {
				return []LostItem{{Kind: textItemKind(chunk.ID), ChunkID: chunk.ID, Text: tc.Text}}
			}
		}
	case KindComments:
		items := make([]LostItem, 0, len(f.Text.Comments))
		for _, comment := range f.Text.Comments {
			items = append(items, LostItem{
				Kind:      "comment",
				ChunkID:   "COMT",
				Text:      comment.Text,
				TimeStamp: comment.TimeStamp,
				MarkerID:  comment.MarkerID,
			})
		}
		return items
	case KindMarkers:
		items := make([]LostItem, 0, len(f.Text.Markers))
		for _, marker := range f.Text.Markers {
			items = append(items, LostItem{
				Kind:     "marker",
				ChunkID:  "MARK",
				Text:     marker.Name,
				MarkerID: marker.ID,
				Position: marker.Position,
			})
		}
		return items
	}
	if chunk.Kind != KindAudio && chunk.Kind != KindCommon {
		return []LostItem{{Kind: "chunk", ChunkID: chunk.ID}}
	}
	return nil
}

func textItemKind(chunkID string) string {
	switch chunkID {
	case "NAME":
		return "name"
	case "AUTH":
		return "author"
	case "(c) ":
		return "copyright"
	case "ANNO":
		return "annotation"
	default:
		return "text_chunk"
	}
}

// MetadataDiff describes chunks and text present in Source but missing from
// Result. Added is informational and does not imply data loss.
type MetadataDiff struct {
	MissingChunks []Chunk    `json:"missing_chunks,omitempty"`
	AddedChunks   []Chunk    `json:"added_chunks,omitempty"`
	Lost          []LostItem `json:"lost,omitempty"`
}

// CompareMetadata compares two already-parsed files without touching audio.
func CompareMetadata(source, result *File) MetadataDiff {
	var diff MetadataDiff
	if source == nil || result == nil {
		return diff
	}
	diff.MissingChunks = missingChunks(source.Chunks, result.Chunks)
	diff.AddedChunks = missingChunks(result.Chunks, source.Chunks)
	diff.Lost = lostMetadata(source, result)
	return diff
}

func missingChunks(source, result []Chunk) []Chunk {
	counts := make(map[string]int, len(result))
	for _, chunk := range result {
		counts[chunk.ID]++
	}
	var missing []Chunk
	for _, chunk := range source {
		if counts[chunk.ID] > 0 {
			counts[chunk.ID]--
			continue
		}
		missing = append(missing, chunk)
	}
	return missing
}

func lostMetadata(source, result *File) []LostItem {
	var lost []LostItem
	remainingText := append([]TextChunk(nil), result.Text.Chunks...)
	for _, chunk := range source.Text.Chunks {
		index := indexOfTextChunk(remainingText, chunk)
		if index < 0 {
			lost = append(lost, LostItem{
				Kind:    textItemKind(chunk.ID),
				ChunkID: chunk.ID,
				Text:    chunk.Text,
			})
			continue
		}
		remainingText = append(remainingText[:index], remainingText[index+1:]...)
	}

	remainingComments := append([]Comment(nil), result.Text.Comments...)
	for _, comment := range source.Text.Comments {
		index := indexOfComment(remainingComments, comment)
		if index < 0 {
			lost = append(lost, LostItem{
				Kind:      "comment",
				ChunkID:   "COMT",
				Text:      comment.Text,
				TimeStamp: comment.TimeStamp,
				MarkerID:  comment.MarkerID,
			})
			continue
		}
		remainingComments = append(remainingComments[:index], remainingComments[index+1:]...)
	}

	remainingMarkers := append([]Marker(nil), result.Text.Markers...)
	for _, marker := range source.Text.Markers {
		index := indexOfMarker(remainingMarkers, marker)
		if index < 0 {
			lost = append(lost, LostItem{
				Kind:     "marker",
				ChunkID:  "MARK",
				Text:     marker.Name,
				MarkerID: marker.ID,
				Position: marker.Position,
			})
			continue
		}
		remainingMarkers = append(remainingMarkers[:index], remainingMarkers[index+1:]...)
	}
	return lost
}

func indexOfTextChunk(values []TextChunk, target TextChunk) int {
	for i, value := range values {
		if value.ID == target.ID && value.Text == target.Text {
			return i
		}
	}
	return -1
}

func indexOfComment(values []Comment, target Comment) int {
	for i, value := range values {
		if value.TimeStamp == target.TimeStamp && value.MarkerID == target.MarkerID &&
			value.Text == target.Text {
			return i
		}
	}
	return -1
}

func indexOfMarker(values []Marker, target Marker) int {
	for i, value := range values {
		if value.ID == target.ID && value.Position == target.Position && value.Name == target.Name {
			return i
		}
	}
	return -1
}

func (d MetadataDiff) String() string {
	if len(d.MissingChunks) == 0 && len(d.Lost) == 0 {
		return "metadata: unchanged"
	}
	return fmt.Sprintf("metadata: %d missing chunk(s), %d lost item(s)",
		len(d.MissingChunks), len(d.Lost))
}
