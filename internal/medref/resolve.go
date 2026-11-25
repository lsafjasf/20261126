package medref

import (
	"os"
	"path/filepath"
	"strings"
)

// Resolve maps a parsed reference onto an on-disk path relative to root.
// Windows drive and UNC paths need a root mapping; without one they cannot
// be probed and the second return value is false.
func (s *Store) Resolve(p Path) (string, bool) {
	switch p.Kind {
	case Relative:
		return filepath.Join(s.root, filepath.FromSlash(strings.TrimPrefix(p.Clean, "./"))), true
	case UnixRooted:
		return filepath.FromSlash(p.Clean), true
	case WindowsDrive:
		target, ok := s.manifest.RootMappings[p.Drive+":"]
		if !ok {
			return "", false
		}
		return filepath.Join(target, filepath.FromSlash(strings.TrimPrefix(p.Clean, "/"))), true
	case UNC:
		key := `\\` + p.Server + `\` + p.Share
		target, ok := s.manifest.RootMappings[key]
		if !ok {
			if alt, ok2 := s.manifest.RootMappings["//"+p.Server+"/"+p.Share]; ok2 {
				target, ok = alt, ok2
			}
		}
		if !ok {
			return "", false
		}
		return filepath.Join(target, filepath.FromSlash(strings.TrimPrefix(p.Clean, "/"))), true
	default:
		return "", false
	}
}

// evalAbs returns the absolute, symlink-resolved path when the target
// exists; otherwise the absolute lexical path and false.
func evalAbs(path string) (string, bool) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return path, false
	}
	if real, err := filepath.EvalSymlinks(abs); err == nil {
		return real, true
	}
	return abs, false
}

func fileExists(path string) bool {
	if path == "" {
		return false
	}
	_, err := os.Stat(path)
	return err == nil
}

// probeOnDisk returns an existing on-disk path that the reference may point
// at, allowing case and Unicode-normalization differences. Empty string
// means no candidate was found.
func (s *Store) probeOnDisk(p Path) string {
	literal, ok := s.Resolve(p)
	if !ok {
		return ""
	}
	abs, exists := evalAbs(literal)
	if exists {
		return abs
	}
	segments := splitSegments(p)
	var base string
	switch p.Kind {
	case Relative:
		base = s.root
	default:
		var okRoot bool
		base, okRoot = s.volumeRoot(p)
		if !okRoot {
			return ""
		}
	}
	return walkSegments(base, segments)
}

func (s *Store) volumeRoot(p Path) (string, bool) {
	switch p.Kind {
	case WindowsDrive:
		t, ok := s.manifest.RootMappings[p.Drive+":"]
		return t, ok
	case UNC:
		if t, ok := s.manifest.RootMappings[`\\`+p.Server+`\`+p.Share]; ok {
			return t, true
		}
		t, ok := s.manifest.RootMappings["//"+p.Server+"/"+p.Share]
		return t, ok
	default:
		return "", false
	}
}

func splitSegments(p Path) []string {
	clean := strings.TrimPrefix(p.Clean, "./")
	clean = strings.TrimPrefix(clean, "/")
	if clean == "" {
		return nil
	}
	return strings.Split(clean, "/")
}

// walkSegments descends from base, matching each segment first exactly and
// then loosely (ASCII case / Unicode normalization). A missing directory
// stops the walk.
func walkSegments(base string, segments []string) string {
	current := base
	for _, want := range segments {
		entries, err := os.ReadDir(current)
		if err != nil {
			return ""
		}
		next := ""
		for _, ent := range entries {
			name := ent.Name()
			if name == want {
				next = name
				break
			}
		}
		if next == "" {
			for _, ent := range entries {
				if LooseEqual(ent.Name(), want) {
					next = ent.Name()
					break
				}
			}
		}
		if next == "" {
			return ""
		}
		current = filepath.Join(current, next)
	}
	if _, err := os.Stat(current); err != nil {
		return ""
	}
	return current
}
