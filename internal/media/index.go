package media

import (
	"encoding/json"
	"fmt"
	"os"
)

// LoadIndex reads a media index JSON file.
func LoadIndex(path string) (Index, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Index{}, fmt.Errorf("read index: %w", err)
	}
	var ix Index
	if err := json.Unmarshal(data, &ix); err != nil {
		return Index{}, fmt.Errorf("parse index: %w", err)
	}
	for i, m := range ix.Media {
		if m.ID == "" {
			return Index{}, fmt.Errorf("media[%d]: missing id", i)
		}
		if m.Kind != "proxy" && m.Kind != "original" {
			return Index{}, fmt.Errorf("media[%d] (%s): kind must be proxy or original", i, m.ID)
		}
	}
	return ix, nil
}
