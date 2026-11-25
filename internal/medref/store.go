package medref

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// Entry is one media reference. Every observed spelling of the same
// reference is kept verbatim in Aliases.
type Entry struct {
	ID        string   `json:"id"`
	Kind      string   `json:"kind"`
	Canonical string   `json:"canonical"`
	Drive     string   `json:"drive,omitempty"`
	Server    string   `json:"server,omitempty"`
	Share     string   `json:"share,omitempty"`
	Aliases   []string `json:"aliases"`
	Resolved  string   `json:"resolved,omitempty"`
	Missing   bool     `json:"missing"`
}

// Manifest is the on-disk reference catalog.
type Manifest struct {
	Version      int               `json:"version"`
	Project      string            `json:"project"`
	Platform     string            `json:"platform"`
	RootMappings map[string]string `json:"rootMappings,omitempty"`
	Entries      []*Entry          `json:"entries"`
}

// Store owns a manifest and the project root it belongs to.
type Store struct {
	root     string
	platform Platform
	manifest *Manifest
	byID     map[string]*Entry
}

const manifestVersion = 1

// NewStore creates an empty store for the given project root.
func NewStore(root string, plat Platform) (*Store, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	return &Store{
		root:     abs,
		platform: plat,
		manifest: &Manifest{
			Version:      manifestVersion,
			Project:      abs,
			Platform:     platName(plat),
			RootMappings: map[string]string{},
			Entries:      []*Entry{},
		},
		byID: map[string]*Entry{},
	}, nil
}

// LoadStore reads a manifest from path.
func LoadStore(path string) (*Store, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}
	plat, ok := ParsePlatform(m.Platform)
	if !ok {
		return nil, fmt.Errorf("unknown platform %q in manifest", m.Platform)
	}
	s := &Store{
		root:     m.Project,
		platform: plat,
		manifest: &m,
		byID:     map[string]*Entry{},
	}
	if s.manifest.RootMappings == nil {
		s.manifest.RootMappings = map[string]string{}
	}
	for _, e := range s.manifest.Entries {
		s.byID[e.ID] = e
	}
	return s, nil
}

// Save writes the manifest as indented JSON, preserving UTF-8 characters.
func (s *Store) Save(path string) error {
	s.sortEntries()
	buf, err := json.MarshalIndent(s.manifest, "", "  ")
	if err != nil {
		return err
	}
	buf = append(buf, '\n')
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, buf, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Root returns the absolute project root.
func (s *Store) Root() string { return s.root }

// Platform returns the configured platform.
func (s *Store) Platform() Platform { return s.platform }

// Manifest returns the underlying manifest (read-modify through Store).
func (s *Store) Manifest() *Manifest { return s.manifest }

// SetRootMapping records where a Windows/UNC volume lives on this machine,
// for example {"D:": "/mnt/d"}.
func (s *Store) SetRootMapping(volume, target string) {
	s.manifest.RootMappings[volume] = target
}

func (s *Store) sortEntries() {
	sort.Slice(s.manifest.Entries, func(i, j int) bool {
		return s.manifest.Entries[i].Canonical < s.manifest.Entries[j].Canonical
	})
}

func entryID(canonical string) string {
	sum := sha256.Sum256([]byte(canonical))
	return hex.EncodeToString(sum[:])[:12]
}

// ErrDuplicateAlias is returned when an exact raw string already exists.
var ErrDuplicateAlias = errors.New("reference already registered")

func platName(p Platform) string {
	switch p {
	case Windows:
		return "windows"
	case Darwin:
		return "darwin"
	default:
		return "linux"
	}
}
