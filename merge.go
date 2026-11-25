package clipmerge

import (
	"fmt"
	"sort"
	"strings"
)

// Merge evaluates concurrent property-level edits against a base clip and
// returns a reproducible report. It never applies last-write-wins: edits
// carry no timestamps and the outcome is a pure function of the request.
func Merge(req MergeRequest) (*Report, error) {
	if err := validateRequest(req); err != nil {
		return nil, err
	}

	applicable, ignored, err := partitionEdits(req)
	if err != nil {
		return nil, err
	}

	resolutions := resolveProperties(req.Base, applicable)

	rep := &Report{
		SchemaVersion:       SchemaVersion,
		ClipID:              req.Base.ID,
		SourceID:            req.Base.SourceID,
		BaseRevision:        req.Base.Revision,
		TimeUnit:            "ms",
		Source:              req.Source,
		Base:                Snapshot(req.Base),
		Edits:               nil,
		IgnoredEdits:        ignored,
		PropertyResolutions: resolutions,
		Conflicts:           []Conflict{},
	}

	unresolved := unresolvedProperties(resolutions)
	rep.Edits = editRecords(applicable, unresolved)

	switch {
	case len(unresolved) > 0:
		rep.Status = "conflict"
		rep.Conflict = true
		rep.Checks = skippedChecks()
		for _, property := range PropertyOrder {
			if !unresolved[property] {
				continue
			}
			res := resolutionOf(resolutions, property)
			rep.Conflicts = append(rep.Conflicts, Conflict{
				Kind:       KindSamePropertyDivergence,
				Properties: []string{property},
				Reason:     "divergent_property_values",
				Message: fmt.Sprintf("%s has %d divergent values. No value is selected by client or timestamp; an explicit manual resolution is required.",
					property, len(res.Candidates)),
				Candidates: res.Candidates,
			})
		}
		rep.Decision = Decision{
			AutoMerge:                false,
			Code:                     CodeManualResolutionRequired,
			Reason:                   "a property has divergent concurrent values; last-write-wins is intentionally disabled",
			RequiresManualResolution: true,
		}
	default:
		merged := req.Base
		for _, res := range resolutions {
			merged = WithValue(merged, res.Property, *res.EffectiveValue)
		}
		rep.Checks = runChecks(req.Source, merged)
		failed := failedChecks(rep.Checks)
		if len(failed) == 0 {
			rep.Status = "auto_merged"
			rep.Conflict = false
			rep.Decision = Decision{
				AutoMerge: true,
				Code:      CodeAutoMerged,
				Reason:    "all property candidates collapsed to one value and every media boundary check passed",
			}
			snap := Snapshot(merged)
			snap.Revision = req.Base.Revision + 1
			rep.MergedClip = &snap
		} else {
			rep.Status = "conflict"
			rep.Conflict = true
			names := make([]string, 0, len(failed))
			for _, chk := range failed {
				names = append(names, chk.Name)
				rep.Conflicts = append(rep.Conflicts, dependencyConflict(chk, applicable, merged))
			}
			rep.Decision = Decision{
				AutoMerge:                false,
				Code:                     CodeMediaBoundaryViolation,
				Reason:                   "combined property values violate media boundaries: " + strings.Join(names, ", "),
				RequiresManualResolution: true,
			}
		}
	}

	rep.AssignIDs()
	return rep, nil
}

// validateRequest rejects malformed input before any merge semantics run.
func validateRequest(req MergeRequest) error {
	if req.Base.ID == "" || req.Base.SourceID == "" {
		return fmt.Errorf("base clip requires id and sourceId")
	}
	if req.Source.ID == "" || req.Source.Duration <= 0 {
		return fmt.Errorf("source requires id and a positive duration")
	}
	if req.Source.MinTimelineDuration < 0 {
		return fmt.Errorf("source minTimelineDuration must not be negative")
	}
	seen := map[string]bool{}
	for _, e := range req.Edits {
		if e.ID == "" || e.ClientID == "" {
			return fmt.Errorf("edit requires id and clientId")
		}
		if seen[e.ID] {
			return fmt.Errorf("duplicate edit id %q", e.ID)
		}
		seen[e.ID] = true
		if _, err := ParseValue(e.Property, e.Value); err != nil {
			return fmt.Errorf("edit %s: %w", e.ID, err)
		}
	}
	return nil
}

// partitionEdits separates stale edits (wrong base revision) from applicable
// ones and sorts both deterministically by edit ID.
func partitionEdits(req MergeRequest) ([]Edit, []IgnoredEdit, error) {
	var applicable []Edit
	ignored := []IgnoredEdit{}
	for _, e := range req.Edits {
		v, err := ParseValue(e.Property, e.Value)
		if err != nil {
			return nil, nil, fmt.Errorf("edit %s: %w", e.ID, err)
		}
		rec := EditRecord{
			ID: e.ID, ClientID: e.ClientID, BaseRevision: e.BaseRevision,
			Property: e.Property, Value: v,
		}
		if e.BaseRevision != req.Base.Revision {
			rec.Disposition = DispositionStaleIgnored
			ignored = append(ignored, IgnoredEdit{
				EditRecord: rec,
				Reason: fmt.Sprintf("edit targets base revision %d but current base revision is %d",
					e.BaseRevision, req.Base.Revision),
			})
			continue
		}
		applicable = append(applicable, e)
	}
	sort.Slice(applicable, func(i, j int) bool { return applicable[i].ID < applicable[j].ID })
	sort.Slice(ignored, func(i, j int) bool { return ignored[i].ID < ignored[j].ID })
	return applicable, ignored, nil
}
