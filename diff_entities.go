package timelinesnap

import "fmt"

// keyedDiff compares two keyed collections.
//
// ordered=true means index position is semantically meaningful (tracks,
// clips, effects): when the same key set exists but positions moved, a
// "reordered" change is reported. ordered=false (resources, markers) treats
// the collection as a set and only reports real value edits; pure shuffles
// surface as order-only via detectOrderOnly in DiffProjects.
func keyedDiff[T any](
	path string,
	old, newer []T,
	key func(T) string,
	ordered bool,
	entityDiff func(prefix string, a, b T) []Change,
) []Change {
	oldByID := make(map[string]T, len(old))
	newByID := make(map[string]T, len(newer))
	for _, v := range old {
		oldByID[key(v)] = v
	}
	for _, v := range newer {
		newByID[key(v)] = v
	}

	var changes []Change

	// Deletions and value edits, walking the old side in its own order.
	for _, v := range old {
		id := key(v)
		nv, ok := newByID[id]
		if !ok {
			changes = append(changes, Change{Type: ChangeDeleted, Path: joinPath(path, id)})
			continue
		}
		changes = append(changes, entityDiff(joinPath(path, id), v, nv)...)
	}

	// Additions, walking the new side in its own order.
	for _, v := range newer {
		if _, ok := oldByID[key(v)]; !ok {
			changes = append(changes, Change{Type: ChangeAdded, Path: joinPath(path, key(v))})
		}
	}

	if ordered && len(changes) == 0 && !samePosition(old, newer, key) {
		changes = append(changes, Change{
			Type:   ChangeReordered,
			Path:   path,
			Detail: "same elements in a different order",
		})
	}
	return changes
}

func samePosition[T any](old, newer []T, key func(T) string) bool {
	if len(old) != len(newer) {
		return false
	}
	for i := range old {
		if key(old[i]) != key(newer[i]) {
			return false
		}
	}
	return true
}

func diffResources(old, newer []Resource) []Change {
	return keyedDiff("resources", old, newer, func(r Resource) string { return r.ID }, false,
		func(prefix string, a, b Resource) []Change {
			var changes []Change
			if a.Type != b.Type {
				changes = append(changes, modChange(joinPath(prefix, "type"), a.Type, b.Type))
			}
			if a.URI != b.URI {
				changes = append(changes, modChange(joinPath(prefix, "uri"), a.URI, b.URI))
			}
			if a.Checksum != b.Checksum {
				changes = append(changes, modChange(joinPath(prefix, "checksum"), a.Checksum, b.Checksum))
			}
			if a.Size != b.Size {
				changes = append(changes, modChange(joinPath(prefix, "size"), a.Size, b.Size))
			}
			changes = append(changes, diffMeta(joinPath(prefix, "meta"), a.Meta, b.Meta)...)
			return changes
		})
}

func diffMarkers(old, newer []Marker) []Change {
	return keyedDiff("markers", old, newer, func(m Marker) string { return m.ID }, false,
		func(prefix string, a, b Marker) []Change {
			var changes []Change
			if a.AtNS != b.AtNS {
				changes = append(changes, modChange(joinPath(prefix, "atNs"), a.AtNS, b.AtNS))
			}
			if a.Label != b.Label {
				changes = append(changes, modChange(joinPath(prefix, "label"), a.Label, b.Label))
			}
			if a.Color != b.Color {
				changes = append(changes, modChange(joinPath(prefix, "color"), a.Color, b.Color))
			}
			changes = append(changes, diffMeta(joinPath(prefix, "meta"), a.Meta, b.Meta)...)
			return changes
		})
}

func diffTracks(old, newer []Track) []Change {
	return keyedDiff("tracks", old, newer, func(t Track) string { return t.ID }, true,
		func(prefix string, a, b Track) []Change {
			var changes []Change
			if a.Name != b.Name {
				changes = append(changes, modChange(joinPath(prefix, "name"), a.Name, b.Name))
			}
			if a.State != b.State {
				changes = append(changes, modChange(joinPath(prefix, "state"), a.State, b.State))
			}
			changes = append(changes, diffMeta(joinPath(prefix, "meta"), a.Meta, b.Meta)...)
			changes = append(changes, diffClips(joinPath(prefix, "clips"), a.Clips, b.Clips)...)
			return changes
		})
}

func diffClips(prefix string, old, newer []Clip) []Change {
	return keyedDiff(prefix, old, newer, func(c Clip) string { return c.ID }, true,
		func(p string, a, b Clip) []Change {
			var changes []Change
			if a.ResourceID != b.ResourceID {
				changes = append(changes, modChange(joinPath(p, "resourceId"), a.ResourceID, b.ResourceID))
			}
			if a.StartNS != b.StartNS {
				changes = append(changes, modChange(joinPath(p, "startNs"), a.StartNS, b.StartNS))
			}
			if a.DurationNS != b.DurationNS {
				changes = append(changes, modChange(joinPath(p, "durationNs"), a.DurationNS, b.DurationNS))
			}
			changes = append(changes, diffMeta(joinPath(p, "meta"), a.Meta, b.Meta)...)
			changes = append(changes, diffEffects(joinPath(p, "effects"), a.Effects, b.Effects)...)
			return changes
		})
}

func diffEffects(prefix string, old, newer []Effect) []Change {
	return keyedDiff(prefix, old, newer, func(e Effect) string { return e.ID }, true,
		func(p string, a, b Effect) []Change {
			var changes []Change
			if a.Kind != b.Kind {
				changes = append(changes, modChange(joinPath(p, "kind"), a.Kind, b.Kind))
			}
			if !canonicalEqual(a.Params, b.Params) {
				changes = append(changes, Change{
					Type: ChangeModified, Path: joinPath(p, "params"),
					Detail: "effect parameters changed",
				})
			}
			return changes
		})
}

func diffMeta(prefix string, old, newer map[string]string) []Change {
	var changes []Change
	for _, k := range sortedKeys(old) {
		nv, ok := newer[k]
		if !ok {
			changes = append(changes, Change{Type: ChangeDeleted, Path: joinPath(prefix, k)})
		} else if old[k] != nv {
			changes = append(changes, modChange(joinPath(prefix, k), old[k], nv))
		}
	}
	for _, k := range sortedKeys(newer) {
		if _, ok := old[k]; !ok {
			changes = append(changes, Change{Type: ChangeAdded, Path: joinPath(prefix, k)})
		}
	}
	return changes
}

func modChange(path string, old, newer any) Change {
	return Change{
		Type:   ChangeModified,
		Path:   path,
		Detail: fmt.Sprintf("%v -> %v", old, newer),
	}
}
