package safereflow

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
)

// LoadSafeDocument reads a safe-area JSON document.
func LoadSafeDocument(r io.Reader) (SafeDocument, error) {
	var doc SafeDocument
	dec := json.NewDecoder(r)
	if err := dec.Decode(&doc); err != nil {
		return doc, fmt.Errorf("safereflow: decode safe areas: %w", err)
	}
	return doc, nil
}

// LoadSafeFile reads and prepares a safe track from a JSON file.
func LoadSafeFile(path string) (*SafeTrack, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	doc, err := LoadSafeDocument(f)
	if err != nil {
		return nil, err
	}
	return NewSafeTrack(doc)
}

// LoadCueDocument reads a cue JSON document.
func LoadCueDocument(r io.Reader) (*CueDocument, error) {
	var doc CueDocument
	if err := json.NewDecoder(r).Decode(&doc); err != nil {
		return nil, fmt.Errorf("safereflow: decode cues: %w", err)
	}
	if doc.Canvas.Width <= 0 || doc.Canvas.Height <= 0 {
		return nil, fmt.Errorf("safereflow: cue document canvas width/height must be positive")
	}
	for i, c := range doc.Cues {
		if c == nil {
			return nil, fmt.Errorf("safereflow: cues[%d] is null", i)
		}
		if c.ID == "" {
			c.ID = fmt.Sprintf("cue-%d", i+1)
		}
		if c.End <= c.Start {
			return nil, fmt.Errorf("safereflow: cue %q end must be greater than start", c.ID)
		}
	}
	return &doc, nil
}

// LoadCueFile reads a cue JSON file.
func LoadCueFile(path string) (*CueDocument, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return LoadCueDocument(f)
}

// WriteJSON writes v as indented JSON to path (or stdout when path is "-").
func WriteJSON(path string, v any) error {
	w, closeFn, err := openOutput(path)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		closeFn()
		return err
	}
	return closeFn()
}

func openOutput(path string) (io.Writer, func() error, error) {
	if path == "" || path == "-" {
		return os.Stdout, func() error { return nil }, nil
	}
	f, err := os.Create(path)
	if err != nil {
		return nil, nil, err
	}
	return f, f.Close, nil
}
