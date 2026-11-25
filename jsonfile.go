package audiotracks

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// SaveProjectJSON validates the complete project, then atomically replaces
// path. On any validation or I/O error the previous file is left untouched.
func SaveProjectJSONAtomic(path string, project Project) error {
	normalized, err := NormalizeProject(project)
	if err != nil {
		return err
	}
	return WriteJSONAtomic(path, normalized)
}

// WriteJSONAtomic writes to a temporary file in the destination directory and
// renames it into place only after the full document has been flushed.
func WriteJSONAtomic(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("encode JSON: %w", err)
	}
	data = append(data, '\n')

	dir := filepath.Dir(path)
	base := filepath.Base(path)
	temp, err := os.CreateTemp(dir, "."+base+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temporary output: %w", err)
	}
	tempName := temp.Name()
	committed := false
	defer func() {
		if !committed {
			_ = temp.Close()
			_ = os.Remove(tempName)
		}
	}()

	if err := temp.Chmod(0o644); err != nil {
		return fmt.Errorf("set output permissions: %w", err)
	}
	if _, err := temp.Write(data); err != nil {
		return fmt.Errorf("write output: %w", err)
	}
	if err := temp.Sync(); err != nil {
		return fmt.Errorf("flush output: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close output: %w", err)
	}
	if err := os.Rename(tempName, path); err != nil {
		return fmt.Errorf("commit output: %w", err)
	}
	committed = true

	if err := syncDirectory(dir); err != nil {
		return err
	}
	return nil
}

func syncDirectory(dir string) error {
	handle, err := os.Open(dir)
	if err != nil {
		return fmt.Errorf("open output directory: %w", err)
	}
	defer handle.Close()
	if err := handle.Sync(); err != nil && !errors.Is(err, os.ErrInvalid) {
		return fmt.Errorf("flush output directory: %w", err)
	}
	return nil
}
