package timelinesnap

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strconv"
)

// canonicalJSON serializes a value deterministically.
//
// Go's encoding/json emits struct fields in declaration order and map keys in
// sorted order, so a fixed-shape struct always produces identical bytes. All
// hashed values in this package are plain structs (or slices of them), never
// raw maps with nested struct values, which keeps the encoding stable.
func canonicalJSON(v any) ([]byte, error) {
	buf, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return buf, nil
}

// hashContent returns the hex SHA-256 of the canonical encoding of v.
func hashContent(v any) (string, error) {
	buf, err := canonicalJSON(v)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(buf)
	return hex.EncodeToString(sum[:]), nil
}

func itoa(i int) string {
	return strconv.Itoa(i)
}

func cloneMeta(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func cloneParams(in map[string]any) map[string]any {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = cloneJSONValue(v)
	}
	return out
}

// cloneJSONValue deep-copies values that JSON round-trips (numbers are kept
// as json.Number-free float64, matching encoding/json behavior).
func cloneJSONValue(v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, sub := range t {
			out[k] = cloneJSONValue(sub)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, sub := range t {
			out[i] = cloneJSONValue(sub)
		}
		return out
	default:
		return v
	}
}

func cloneEffect(e Effect) Effect {
	return Effect{
		ID:     e.ID,
		Kind:   e.Kind,
		Params: cloneParams(e.Params),
	}
}

func cloneClip(c Clip) Clip {
	out := Clip{
		ID:         c.ID,
		ResourceID: c.ResourceID,
		StartNS:    c.StartNS,
		DurationNS: c.DurationNS,
		Meta:       cloneMeta(c.Meta),
	}
	if len(c.Effects) > 0 {
		out.Effects = make([]Effect, len(c.Effects))
		for i, e := range c.Effects {
			out.Effects[i] = cloneEffect(e)
		}
	}
	return out
}

func cloneTrack(t Track) Track {
	out := Track{
		ID:    t.ID,
		Name:  t.Name,
		State: t.State,
		Meta:  cloneMeta(t.Meta),
	}
	if len(t.Clips) > 0 {
		out.Clips = make([]Clip, len(t.Clips))
		for i, c := range t.Clips {
			out.Clips[i] = cloneClip(c)
		}
	}
	return out
}

func cloneResource(r Resource) Resource {
	return Resource{
		ID:       r.ID,
		Type:     r.Type,
		URI:      r.URI,
		Checksum: r.Checksum,
		Size:     r.Size,
		Meta:     cloneMeta(r.Meta),
	}
}

func cloneMarker(m Marker) Marker {
	return Marker{
		ID:    m.ID,
		AtNS:  m.AtNS,
		Label: m.Label,
		Color: m.Color,
		Meta:  cloneMeta(m.Meta),
	}
}
