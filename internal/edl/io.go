package edl

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// LoadLibrary reads a versioned media library JSON document.
func LoadLibrary(path string) (Library, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Library{}, fmt.Errorf("read library %q: %w", path, err)
	}
	var lib Library
	if err := json.Unmarshal(data, &lib); err != nil {
		return Library{}, fmt.Errorf("parse library %q: %w", path, err)
	}
	if len(lib.Assets) == 0 {
		return Library{}, fmt.Errorf("library %q contains no assets", path)
	}
	return lib, nil
}

// LoadChangeList reads an offline edit decision list JSON document.
func LoadChangeList(path string) (ChangeList, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ChangeList{}, fmt.Errorf("read change list %q: %w", path, err)
	}
	var cl ChangeList
	if err := json.Unmarshal(data, &cl); err != nil {
		return ChangeList{}, fmt.Errorf("parse change list %q: %w", path, err)
	}
	if len(cl.Ops) == 0 {
		return ChangeList{}, fmt.Errorf("change list %q contains no ops", path)
	}
	return cl, nil
}

// HashFile returns the lowercase hex SHA-256 of the file at path.
func HashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// VerifyLibrary recomputes every media hash from disk (paths are resolved
// relative to mediaRoot) and returns the first mismatch found. A missing
// file is also an error. A nil result means every on-disk hash agrees with
// the manifest.
func VerifyLibrary(lib Library, mediaRoot string) error {
	for _, a := range lib.Assets {
		got, err := HashFile(filepath.Join(mediaRoot, filepath.FromSlash(a.Path)))
		if err != nil {
			return fmt.Errorf("clip %s: hash media: %w", a.ClipID, err)
		}
		if got != a.MediaHash {
			return fmt.Errorf("clip %s: manifest hash %s != on-disk hash %s",
				a.ClipID, a.MediaHash, got)
		}
	}
	return nil
}
