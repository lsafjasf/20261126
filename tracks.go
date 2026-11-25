package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

// Track is the order-independent identity of one Matroska track.
type Track struct {
	Position         int    `json:"position"`
	TrackNumber      uint64 `json:"trackNumber"`
	TrackUID         uint64 `json:"trackUid"`
	TrackType        uint64 `json:"trackType"`
	CodecID          string `json:"codecId"`
	CodecPrivate     []byte `json:"-"`
	CodecPrivateB64  string `json:"codecPrivate,omitempty"`
	CodecPrivateHash string `json:"codecPrivateSha256"`
	Language         string `json:"language"`
	LanguageIETF     string `json:"languageIetf"`
	Default          bool   `json:"default"`
	DefaultExplicit  bool   `json:"defaultExplicit"`
}

// effectiveLanguage prefers the BCP-47 form when present.
func (t Track) effectiveLanguage() string {
	if t.LanguageIETF != "" {
		return t.LanguageIETF
	}
	return t.Language
}

// extractTracks parses the Tracks section of a Matroska file in container
// order and returns one identity per TrackEntry.
func extractTracks(buf []byte) ([]Track, error) {
	var tracks []Track
	err := walkElements(buf, 0, int64(len(buf)), nil, func(el Element, path []uint32) bool {
		if el.ID != idTrackEntry {
			return true
		}
		track := Track{Position: len(tracks) + 1, DefaultExplicit: false}
		_ = walkElements(buf, el.DataOffset, el.DataOffset+el.DataSize, nil, func(child Element, _ []uint32) bool {
			switch child.ID {
			case idTrackNumber:
				track.TrackNumber = uintValue(buf, child)
			case idTrackUID:
				track.TrackUID = uintValue(buf, child)
			case idTrackType:
				track.TrackType = uintValue(buf, child)
			case idFlagDefault:
				track.Default = uintValue(buf, child) != 0
				track.DefaultExplicit = true
			case idCodecID:
				track.CodecID = stringValue(buf, child)
			case idCodecPrivate:
				track.CodecPrivate = append([]byte(nil), buf[child.DataOffset:child.DataOffset+child.DataSize]...)
			case idLanguage:
				track.Language = stringValue(buf, child)
			case idLanguageIETF:
				track.LanguageIETF = stringValue(buf, child)
			}
			return false
		})
		sum := sha256.Sum256(track.CodecPrivate)
		track.CodecPrivateHash = hex.EncodeToString(sum[:])
		tracks = append(tracks, track)
		return false
	})
	if err != nil {
		return nil, fmt.Errorf("extract tracks: %w", err)
	}
	if len(tracks) == 0 {
		return nil, fmt.Errorf("no TrackEntry elements found")
	}
	return tracks, nil
}

// Roster is the baseline identity list recorded before the file was damaged.
type Roster struct {
	Tracks []Track `json:"tracks"`
}

func loadRoster(path string) ([]Track, error) {
	data, err := readFile(path)
	if err != nil {
		return nil, err
	}
	var roster Roster
	if err := json.Unmarshal(data, &roster); err != nil {
		return nil, fmt.Errorf("parse roster %s: %w", path, err)
	}
	if len(roster.Tracks) == 0 {
		return nil, fmt.Errorf("roster %s has no tracks", path)
	}
	return roster.Tracks, nil
}

// loadBaseline loads baseline identities either from a roster JSON file or
// directly from a reference Matroska file.
func loadBaseline(path string) ([]Track, error) {
	data, err := readFile(path)
	if err != nil {
		return nil, err
	}
	if len(data) > 0 && data[0] == '{' {
		return loadRoster(path)
	}
	return extractTracks(data)
}
