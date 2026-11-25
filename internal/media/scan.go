package media

import (
	"io/fs"
	"path/filepath"
	"sort"
)

// Scan walks root and probes every regular file. Files that are not MRL1
// containers are skipped and reported by name so the operator sees them.
func Scan(root string) (probes []Probe, skipped []string, err error) {
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		p, perr := ProbeFile(path)
		if perr != nil {
			skipped = append(skipped, path)
			return nil
		}
		probes = append(probes, p)
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	sort.Slice(probes, func(i, j int) bool { return probes[i].Path < probes[j].Path })
	sort.Strings(skipped)
	return probes, skipped, nil
}
