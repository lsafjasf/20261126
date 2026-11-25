// Package medref normalizes media-path references, keeps the original
// strings, and relocates references after a project moves.
//
// It only uses the Go standard library.
package medref

import (
	"path"
	"strings"
)

// Kind classifies a media reference.
type Kind int

const (
	// Relative paths are resolved against the project root.
	Relative Kind = iota
	// UnixRooted is an absolute POSIX path (for example /mnt/media/a.png).
	UnixRooted
	// WindowsDrive is a drive-letter path (for example D:\media\a.png).
	WindowsDrive
	// UNC is a Windows \\server\share\... path.
	UNC
)

// Platform selects filesystem case semantics.
type Platform int

const (
	// Linux is case-sensitive.
	Linux Platform = iota
	// Windows is case-insensitive and case-preserving.
	Windows
	// Darwin is case-insensitive by default.
	Darwin
)

// ParsePlatform parses a platform name. An empty name means Linux.
func ParsePlatform(name string) (Platform, bool) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", "linux":
		return Linux, true
	case "windows", "win", "win32":
		return Windows, true
	case "darwin", "macos", "mac":
		return Darwin, true
	default:
		return Linux, false
	}
}

// Path is a parsed media reference. The original string is always retained.
type Path struct {
	// Raw is the exact string supplied by the user.
	Raw string
	// Kind is the path classification.
	Kind Kind
	// Drive is the upper-case drive letter ("D") or empty.
	Drive string
	// Server and Share identify a UNC path, both empty otherwise.
	Server string
	Share  string
	// Clean is the slash-separated, path.Clean-normalized body.
	// It starts with "./" for relative paths and "/" for rooted ones.
	Clean string
	// Canonical is the stable identity key (see CanonicalKey).
	Canonical string
}

func isDrivePrefix(s string) (letter string, rest string, ok bool) {
	if len(s) >= 2 && s[1] == ':' {
		c := s[0]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') {
			return strings.ToUpper(s[:1]), s[2:], true
		}
	}
	return "", "", false
}

// Parse normalizes a media reference without touching the filesystem.
// It accepts both "/" and "\" separators for Windows-style paths, while
// relative and POSIX paths use "/" (and keep backslashes as bytes, as a
// POSIX filesystem allows them inside names).
func Parse(raw string) Path {
	p := Path{Raw: raw}
	s := raw

	// UNC: \\server\share\... or //server/share/...
	if len(s) >= 2 && (s[0] == '\\' && s[1] == '\\' || s[0] == '/' && s[1] == '/') {
		sep := s[0]
		body := s[2:]
		parts := strings.Split(body, string(sep))
		// Some references mix separators; normalize each part later anyway.
		if len(parts) >= 2 && parts[0] != "" {
			p.Kind = UNC
			p.Server = parts[0]
			p.Share = parts[1]
			rest := strings.Join(parts[2:], "/")
			p.Clean = "/" + path.Clean(rest)
			if rest == "" {
				p.Clean = "/"
			}
			p.Canonical = "unc://" + strings.ToLower(p.Server) + "/" +
				strings.ToLower(p.Share) + canonicalCase(p.Clean, true)
			return p
		}
	}

	// Windows drive path, possibly with separators replaced by slashes.
	if _, _, ok := isDrivePrefix(s); ok {
		letter, body, _ := isDrivePrefix(s)
		p.Kind = WindowsDrive
		p.Drive = letter
		body = strings.ReplaceAll(body, "\\", "/")
		if body == "" || body == "/" {
			p.Clean = "/"
		} else {
			p.Clean = path.Clean("/" + body)
		}
		p.Canonical = "drive://" + letter + canonicalCase(p.Clean, true)
		return p
	}

	// POSIX absolute path.
	if strings.HasPrefix(s, "/") {
		p.Kind = UnixRooted
		p.Clean = path.Clean(s)
		p.Canonical = "posix://" + p.Clean
		return p
	}

	// Relative path: keep it relative (an explicit ./ prefix makes that
	// visible in the canonical key) and lexically resolve . and ..
	p.Kind = Relative
	cleaned := path.Clean(s)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		p.Clean = cleaned
	} else {
		p.Clean = "./" + strings.TrimPrefix(cleaned, "./")
	}
	p.Canonical = "rel://" + p.Clean
	return p
}

// canonicalCase folds ASCII case when the volume is case-insensitive.
// Non-ASCII bytes are left untouched on purpose: full Unicode case folding
// cannot be done correctly with only the standard library's tables, and a
// wrong fold would merge two legal, distinct paths.
func canonicalCase(clean string, fold bool) string {
	if !fold {
		return clean
	}
	var b strings.Builder
	b.Grow(len(clean))
	for _, r := range clean {
		if r >= 'a' && r <= 'z' {
			r -= 'a' - 'A'
		}
		b.WriteRune(r)
	}
	return b.String()
}
