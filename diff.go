package timelinesnap

import (
	"fmt"
	"sort"
	"strings"
)

// ChangeType classifies a single difference between two projects.
type ChangeType string

const (
	// ChangeAdded / ChangeDeleted / ChangeModified are real content changes.
	ChangeAdded    ChangeType = "added"
	ChangeDeleted  ChangeType = "deleted"
	ChangeModified ChangeType = "modified"

	// ChangeReordered is a real change of an ordered sequence (tracks, clips
	// or effects): the same elements appear in a different order, which
	// alters playback or rendering output.
	ChangeReordered ChangeType = "reordered"

	// ChangeOrderOnly is meaningless noise from an order-insensitive table
	// (resources or markers): the same entries, same values, shuffled only.
	// It is never counted as a content change.
	ChangeOrderOnly ChangeType = "order-only"
)

// Change is one path-addressed difference.
type Change struct {
	Type   ChangeType `json:"type"`
	Path   string     `json:"path"`
	Detail string     `json:"detail,omitempty"`
}

// Report is the result of comparing two projects or snapshots.
type Report struct {
	OldID   string   `json:"oldId,omitempty"`
	NewID   string   `json:"newId,omitempty"`
	Changes []Change `json:"changes"`
}

// HasContentChanges reports whether the two versions differ in any way that
// affects timeline output. Pure order-only reshuffles return false.
func (r *Report) HasContentChanges() bool {
	if r == nil {
		return false
	}
	for _, c := range r.Changes {
		if c.Type != ChangeOrderOnly {
			return true
		}
	}
	return false
}

// ChangesOf returns the changes of the given type.
func (r *Report) ChangesOf(t ChangeType) []Change {
	var out []Change
	if r == nil {
		return out
	}
	for _, c := range r.Changes {
		if c.Type == t {
			out = append(out, c)
		}
	}
	return out
}

// DiffSnapshots compares two normalized snapshots. Because snapshots
// canonicalize resources and markers, meaningless table shuffles cannot
// appear here; only genuine differences are reported.
func DiffSnapshots(old, newer *Snapshot) (*Report, error) {
	if old == nil || newer == nil {
		return nil, errInvalid("cannot diff nil snapshot")
	}
	r := &Report{OldID: old.ID, NewID: newer.ID}
	r.Changes = append(r.Changes, diffResources(old.Resources, newer.Resources)...)
	r.Changes = append(r.Changes, diffMarkers(old.Markers, newer.Markers)...)
	r.Changes = append(r.Changes, diffTracks(old.Tracks, newer.Tracks)...)
	if old.Name != newer.Name {
		r.Changes = append(r.Changes, Change{
			Type: ChangeModified, Path: "name",
			Detail: fmt.Sprintf("%q -> %q", old.Name, newer.Name),
		})
	}
	return r, nil
}

// DiffProjects compares two live projects. It builds both snapshots first, so
// unreachable-resource churn is ignored, then additionally detects pure
// shuffles of the resource and marker tables and labels them order-only.
func DiffProjects(old, newer *Project) (*Report, error) {
	oldSnap, _, errOld := Build(old)
	newSnap, _, errNew := Build(newer)
	if errOld != nil && !isReachError(errOld) {
		return nil, errOld
	}
	if errNew != nil && !isReachError(errNew) {
		return nil, errNew
	}
	r, err := DiffSnapshots(oldSnap, newSnap)
	if err != nil {
		return nil, err
	}
	r.Changes = append(r.Changes,
		detectOrderOnly("resources", old.Resources, newer.Resources, func(x Resource) string { return x.ID })...)
	r.Changes = append(r.Changes,
		detectOrderOnly("markers", old.Markers, newer.Markers, func(x Marker) string { return x.ID })...)
	if errOld != nil {
		return r, errOld
	}
	return r, errNew
}

func isReachError(err error) bool {
	_, ok := err.(*ReachError)
	return ok
}

// detectOrderOnly emits a single order-only change when both slices hold the
// same keyed entries with identical values but in a different order.
func detectOrderOnly[T any](path string, old, newer []T, key func(T) string) []Change {
	if len(old) != len(newer) || !sameKeySet(old, newer, key) {
		return nil
	}
	orderChanged := false
	for i := range old {
		if key(old[i]) != key(newer[i]) {
			orderChanged = true
			break
		}
	}
	if !orderChanged || !canonicalEqualSlice(old, newer) {
		return nil
	}
	return []Change{{Type: ChangeOrderOnly, Path: path,
		Detail: "entries unchanged; only table order differs"}}
}

func sameKeySet[T any](old, newer []T, key func(T) string) bool {
	seen := make(map[string]int, len(old))
	for _, v := range old {
		seen[key(v)]++
	}
	for _, v := range newer {
		k := key(v)
		if seen[k] == 0 {
			return false
		}
		seen[k]--
	}
	return true
}

func canonicalEqualSlice[T any](old, newer []T) bool {
	if len(old) != len(newer) {
		return false
	}
	// Compare as multisets of canonical encodings, which is order-insensitive.
	oldEnc := multisetEnc(old)
	newEnc := multisetEnc(newer)
	if len(oldEnc) != len(newEnc) {
		return false
	}
	for enc, n := range oldEnc {
		if newEnc[enc] != n {
			return false
		}
	}
	return true
}

func multisetEnc[T any](in []T) map[string]int {
	out := make(map[string]int, len(in))
	for _, v := range in {
		b, err := canonicalJSON(v)
		if err != nil {
			return nil
		}
		out[string(b)]++
	}
	return out
}

func canonicalEqual(a, b any) bool {
	ba, errA := canonicalJSON(a)
	bb, errB := canonicalJSON(b)
	return errA == nil && errB == nil && string(ba) == string(bb)
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func joinPath(parts ...string) string {
	return strings.Join(parts, "/")
}
