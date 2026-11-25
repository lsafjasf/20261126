package medref

// AddResult reports what Add did.
type AddResult struct {
	Entry   *Entry
	Created bool
	// Merged is true when the raw string was attached to an existing entry
	// instead of creating a new one.
	Merged bool
	// Duplicate is true when the exact raw string was already an alias.
	Duplicate bool
}

// Add registers a media reference. The raw string is always preserved.
// References are merged only when they are the same path under the
// platform's case policy, or when they resolve to one file on disk.
func (s *Store) Add(raw string) (AddResult, error) {
	p := Parse(raw)

	// 1. Exact raw string already known: nothing to do.
	for _, e := range s.manifest.Entries {
		if containsAlias(e, raw) {
			return AddResult{Entry: e, Duplicate: true}, nil
		}
	}

	// 2. Lexical identity under the platform case policy (drive letters,
	//    case-insensitive volumes).
	for _, e := range s.manifest.Entries {
		other := Parse(e.Aliases[0])
		if LexicallyEqual(p, other, s.platform) {
			e.Aliases = append(e.Aliases, raw)
			s.refreshEntry(e)
			return AddResult{Entry: e, Merged: true}, nil
		}
	}

	// 3. On-disk identity. Case or Unicode variants that point at the same
	//    existing file are the same reference; distinct files (for example
	//    two differently-cased files on Linux) stay separate.
	probed := s.probeOnDisk(p)
	if probed != "" {
		for _, e := range s.manifest.Entries {
			if e.Resolved == probed {
				e.Aliases = append(e.Aliases, raw)
				return AddResult{Entry: e, Merged: true}, nil
			}
		}
	}

	// 4. Genuinely new reference.
	e := &Entry{
		ID:        entryID(p.Canonical),
		Kind:      kindName(p.Kind),
		Canonical: p.Canonical,
		Drive:     p.Drive,
		Server:    p.Server,
		Share:     p.Share,
		Aliases:   []string{raw},
	}
	s.refreshEntry(e)
	s.manifest.Entries = append(s.manifest.Entries, e)
	s.byID[e.ID] = e
	return AddResult{Entry: e, Created: true}, nil
}

// refreshEntry re-probes an entry from its first alias and records whether
// the referenced file currently exists.
func (s *Store) refreshEntry(e *Entry) {
	p := Parse(e.Aliases[0])
	resolved := s.probeOnDisk(p)
	if resolved == "" {
		if literal, ok := s.Resolve(p); ok {
			abs, _ := evalAbs(literal)
			resolved = abs
		}
	}
	e.Resolved = resolved
	e.Missing = resolved == "" || !fileExists(resolved)
}

func containsAlias(e *Entry, raw string) bool {
	for _, a := range e.Aliases {
		if a == raw {
			return true
		}
	}
	return false
}

func kindName(k Kind) string {
	switch k {
	case UnixRooted:
		return "unix"
	case WindowsDrive:
		return "drive"
	case UNC:
		return "unc"
	default:
		return "relative"
	}
}
