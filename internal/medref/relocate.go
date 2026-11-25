package medref

import "path/filepath"

// RelocateReport summarizes a batch relocation.
type RelocateReport struct {
	OldRoot   string   `json:"oldRoot"`
	NewRoot   string   `json:"newRoot"`
	Updated   []string `json:"updated"`
	Missing   []string `json:"missing"`
	Unmapped  []string `json:"unmapped"`
	TotalRefs int      `json:"totalRefs"`
}

// Relocate moves the catalog to newRoot. Relative references are rebased
// onto the new project root. External references (drive/UNC/POSIX) are
// probed through the current root mappings; references whose volume has no
// mapping go to Unmapped instead of being guessed. Missing files are
// listed separately but still rebased.
func (s *Store) Relocate(newRoot string) (*RelocateReport, error) {
	abs, err := filepath.Abs(newRoot)
	if err != nil {
		return nil, err
	}
	report := &RelocateReport{
		OldRoot:   s.root,
		NewRoot:   abs,
		Updated:   []string{},
		Missing:   []string{},
		Unmapped:  []string{},
		TotalRefs: len(s.manifest.Entries),
	}

	oldRoot := s.root
	s.root = abs
	s.manifest.Project = abs

	for _, e := range s.manifest.Entries {
		p := Parse(e.Aliases[0])
		if p.Kind == WindowsDrive || p.Kind == UNC {
			if _, ok := s.volumeRoot(p); !ok {
				report.Unmapped = append(report.Unmapped, e.Aliases[0])
				e.Missing = true
				continue
			}
		}
		s.refreshEntry(e)
		report.Updated = append(report.Updated, e.Aliases[0])
		if e.Missing || !fileExists(e.Resolved) {
			e.Missing = true
			report.Missing = append(report.Missing, e.Aliases[0])
		}
	}

	_ = oldRoot
	return report, nil
}

// Recheck re-probes every entry against the current root and returns the
// raw aliases whose referenced files cannot be found.
func (s *Store) Recheck() []string {
	missing := []string{}
	for _, e := range s.manifest.Entries {
		s.refreshEntry(e)
		if e.Missing {
			missing = append(missing, e.Aliases[0])
		}
	}
	return missing
}

// MissingEntries returns the entries currently flagged as missing.
func (s *Store) MissingEntries() []*Entry {
	out := []*Entry{}
	for _, e := range s.manifest.Entries {
		if e.Missing {
			out = append(out, e)
		}
	}
	return out
}
