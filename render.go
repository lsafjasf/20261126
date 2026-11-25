package sdh

import "bytes"

// EntriesAsCues converts timed entries to renderable cues, renumbered in
// slice order.
func EntriesAsCues(entries []*Entry) []Cue {
	cues := make([]Cue, 0, len(entries))
	for i, entry := range entries {
		cues = append(cues, Cue{
			ID:    entry.ID,
			Index: i + 1,
			Start: entry.Start,
			End:   entry.End,
			Text:  entry.Text,
		})
	}
	return cues
}

// RenderEntriesSRT renders a set of entries as a standalone SRT document.
func RenderEntriesSRT(entries []*Entry) []byte {
	return RenderSRT(EntriesAsCues(entries))
}

// RenderLayerSRT renders one semantic layer as an SRT track.
func RenderLayerSRT(layer *Layer) []byte {
	if layer == nil {
		return nil
	}
	return RenderEntriesSRT(layer.Entries)
}

// LayerTracks returns a role -> SRT bytes map for every non-empty layer.
func (p *Project) LayerTracks() map[Role][]byte {
	out := map[Role][]byte{}
	for _, role := range p.LayerOrder {
		if data := RenderLayerSRT(p.Layers[role]); len(bytes.TrimSpace(data)) > 0 {
			out[role] = data
		}
	}
	return out
}
