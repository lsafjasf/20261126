package subselect

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

// Catalog is the immutable set of subtitle tracks available for one title.
// A Catalog is safe for concurrent use and is the intended shared dependency
// between browser sessions: selection state never lives here.
type Catalog struct {
	mu     sync.RWMutex // only guards against accidental post-build mutation helpers
	tracks []Track
	byID   map[string]Track
}

// NewCatalog validates tracks (unique non-empty IDs, nonzero time bases) and
// returns a catalog that keeps its own copy of the input slice.
func NewCatalog(tracks ...Track) (*Catalog, error) {
	c := &Catalog{byID: make(map[string]Track, len(tracks))}
	seen := make(map[string]struct{}, len(tracks))
	for i := range tracks {
		t := tracks[i]
		if strings.TrimSpace(t.ID) == "" {
			return nil, fmt.Errorf("subselect: track at index %d has empty ID", i)
		}
		if _, dup := seen[t.ID]; dup {
			return nil, fmt.Errorf("subselect: duplicate track ID %q", t.ID)
		}
		seen[t.ID] = struct{}{}
		if t.TimeBase.Timescale == 0 {
			return nil, fmt.Errorf("%w: track %q has zero timescale", ErrInvalidTimeBase, t.ID)
		}
		if t.Cues != nil {
			cp := append([]Cue(nil), t.Cues...)
			t.Cues = cp
		}
		c.tracks = append(c.tracks, t)
		c.byID[t.ID] = t
	}
	return c, nil
}

// Tracks returns a sorted copy of all tracks ordered by ID.
func (c *Catalog) Tracks() []Track {
	if c == nil {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := append([]Track(nil), c.tracks...)
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// Get returns a track by ID.
func (c *Catalog) Get(id string) (Track, bool) {
	if c == nil {
		return Track{}, false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	t, ok := c.byID[id]
	return t, ok
}

// Len reports the number of tracks including unavailable ones.
func (c *Catalog) Len() int {
	if c == nil {
		return 0
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.tracks)
}
