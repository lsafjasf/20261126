package fcpxml

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

func (d *Document) Validate() error {
	if len(d.issues) == 0 {
		return nil
	}
	issues := append([]MappingIssue(nil), d.issues...)
	return &LossySaveError{Issues: issues}
}

// Save writes the imported document only when every parameter can be
// represented without ambiguity. It never silently drops an unmapped value.
func (d *Document) Save(w io.Writer) error {
	if err := d.Validate(); err != nil {
		return err
	}
	if _, err := w.Write(d.input); err != nil {
		return fmt.Errorf("write FCPXML: %w", err)
	}
	return nil
}

// SaveFile validates before touching the destination. The replacement is
// committed with a same-directory rename, so a lossy document cannot truncate
// or partially overwrite an existing file.
func (d *Document) SaveFile(path string) error {
	if err := d.Validate(); err != nil {
		return err
	}
	dir := filepath.Dir(path)
	mode := os.FileMode(0o644)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	}
	tmp, err := os.CreateTemp(dir, ".fcpxml-save-*")
	if err != nil {
		return fmt.Errorf("create temporary file for %q: %w", path, err)
	}
	tmpName := tmp.Name()
	cleanup := func() {
		tmp.Close()
		os.Remove(tmpName)
	}
	if _, err := tmp.Write(d.input); err != nil {
		cleanup()
		return fmt.Errorf("write temporary file for %q: %w", path, err)
	}
	if err := tmp.Chmod(mode); err != nil {
		cleanup()
		return fmt.Errorf("set mode on temporary file for %q: %w", path, err)
	}
	if err := tmp.Sync(); err != nil {
		cleanup()
		return fmt.Errorf("sync temporary file for %q: %w", path, err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("close temporary file for %q: %w", path, err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("replace %q: %w", path, err)
	}
	return nil
}
