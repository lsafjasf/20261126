package clipmerge

import "sort"

// resolveProperties performs the per-property three-way merge. For each
// property, every distinct value proposed relative to the base becomes one
// candidate. A property resolves only when all concurrent proposals agree
// (zero or one distinct candidate); two or more distinct candidates make it
// unresolved. Arrival order and timestamps play no role.
func resolveProperties(base Clip, edits []Edit) []PropertyResolution {
	resolutions := make([]PropertyResolution, 0, len(PropertyOrder))
	for _, property := range PropertyOrder {
		baseValue := ValueOf(base, property)
		res := PropertyResolution{
			Property:   property,
			BaseValue:  baseValue,
			Candidates: []Candidate{},
		}
		for _, e := range edits {
			if e.Property != property {
				continue
			}
			v, err := ParseValue(e.Property, e.Value)
			if err != nil {
				continue // guaranteed valid by validateRequest
			}
			if v.Equal(baseValue) {
				continue // no-op edit: proposes nothing new
			}
			res.Candidates = addCandidate(res.Candidates, v, e)
		}
		sort.Slice(res.Candidates, func(i, j int) bool {
			return res.Candidates[i].Value.Less(res.Candidates[j].Value)
		})
		switch len(res.Candidates) {
		case 0:
			v := baseValue
			res.EffectiveValue = &v
			res.State = "resolved"
		case 1:
			v := res.Candidates[0].Value
			res.EffectiveValue = &v
			res.State = "resolved"
		default:
			res.EffectiveValue = nil
			res.State = "unresolved"
		}
		resolutions = append(resolutions, res)
	}
	return resolutions
}

// addCandidate records an edit under its distinct value, creating the
// candidate bucket on first sight.
func addCandidate(candidates []Candidate, v Value, e Edit) []Candidate {
	for i := range candidates {
		if candidates[i].Value.Equal(v) {
			candidates[i].EditIDs = insertSorted(candidates[i].EditIDs, e.ID)
			candidates[i].ClientIDs = insertSortedUnique(candidates[i].ClientIDs, e.ClientID)
			return candidates
		}
	}
	return append(candidates, Candidate{
		Value:     v,
		EditIDs:   []string{e.ID},
		ClientIDs: []string{e.ClientID},
	})
}

// unresolvedProperties indexes which properties failed to resolve.
func unresolvedProperties(resolutions []PropertyResolution) map[string]bool {
	out := map[string]bool{}
	for _, res := range resolutions {
		if res.State == "unresolved" {
			out[res.Property] = true
		}
	}
	return out
}

// resolutionOf finds the resolution record for one property.
func resolutionOf(resolutions []PropertyResolution, property string) PropertyResolution {
	for _, res := range resolutions {
		if res.Property == property {
			return res
		}
	}
	return PropertyResolution{Property: property, Candidates: []Candidate{}}
}

// editRecords renders applicable edits in ID order with their dispositions.
func editRecords(edits []Edit, unresolved map[string]bool) []EditRecord {
	records := make([]EditRecord, 0, len(edits))
	for _, e := range edits {
		v, _ := ParseValue(e.Property, e.Value)
		disposition := DispositionPropertyResolved
		if unresolved[e.Property] {
			disposition = DispositionUnresolved
		}
		records = append(records, EditRecord{
			ID: e.ID, ClientID: e.ClientID, BaseRevision: e.BaseRevision,
			Property: e.Property, Value: v, Disposition: disposition,
		})
	}
	return records
}

// failedChecks returns the failing checks in canonical order.
func failedChecks(checks []Check) []Check {
	var failed []Check
	for _, chk := range checks {
		if chk.Status == checkStatusFail {
			failed = append(failed, chk)
		}
	}
	return failed
}

// dependencyConflict builds a dependency_violation conflict from a failed
// boundary check: the properties the check depends on, the edits that
// proposed those properties, and the combined values that broke the rule.
func dependencyConflict(chk Check, edits []Edit, merged Clip) Conflict {
	involved := map[string]bool{}
	for _, p := range chk.Properties {
		involved[p] = true
	}
	editIDs := []string{}
	for _, e := range edits {
		if involved[e.Property] {
			editIDs = append(editIDs, e.ID)
		}
	}
	sort.Strings(editIDs)
	observed := make([]ObservedValue, 0, len(chk.Properties))
	for _, p := range chk.Properties {
		observed = append(observed, ObservedValue{Property: p, Value: ValueOf(merged, p)})
	}
	return Conflict{
		Kind:       KindDependencyViolation,
		Properties: chk.Properties,
		Reason:     chk.Name,
		Message:    chk.Message,
		EditIDs:    editIDs,
		Observed:   observed,
	}
}

func insertSorted(xs []string, x string) []string {
	i := sort.SearchStrings(xs, x)
	return append(xs[:i], append([]string{x}, xs[i:]...)...)
}

func insertSortedUnique(xs []string, x string) []string {
	i := sort.SearchStrings(xs, x)
	if i < len(xs) && xs[i] == x {
		return xs
	}
	return append(xs[:i], append([]string{x}, xs[i:]...)...)
}
