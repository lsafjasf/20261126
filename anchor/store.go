package anchor

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

// Load reads and validates a project file.
func Load(path string) (*Project, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var p Project
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if err := p.Validate(); err != nil {
		return nil, fmt.Errorf("validate %s: %w", path, err)
	}
	return &p, nil
}

// Save validates and writes the project atomically. The JSON encoding is
// deterministic (indentation is fixed, encoding/json sorts map keys), so
// identical project content always produces identical bytes and an identical
// checksum, even across save/reload cycles.
func (p *Project) Save(path string) error {
	if err := p.Validate(); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')

	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".project-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(raw); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return err
	}
	return nil
}

// Checksum returns the SHA-256 of the canonical JSON encoding of the project.
// It is the stability guard: save -> load -> Checksum must be constant.
func (p *Project) Checksum() (string, error) {
	if err := p.Validate(); err != nil {
		return "", err
	}
	raw, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

// FormatTime renders ticks as HH:MM:SS.mmm using the project timebase.
func (p *Project) FormatTime(t Ticks) string {
	if p.TicksPerSecond <= 0 {
		return strconv.FormatInt(int64(t), 10)
	}
	totalMillis := t * 1000 / Ticks(p.TicksPerSecond)
	millis := totalMillis % 1000
	totalSec := totalMillis / 1000
	secs := totalSec % 60
	mins := (totalSec / 60) % 60
	hours := totalSec / 3600
	return fmt.Sprintf("%02d:%02d:%02d.%03d", hours, mins, secs, millis)
}
