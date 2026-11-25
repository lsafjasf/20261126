package timelinesnap

import "sort"

// snapshotBody is the exact shape whose hash becomes Snapshot.ID. Stats and
// the ID itself are excluded: they are derived from the content below.
type snapshotBody struct {
	Name      string     `json:"name"`
	Resources []Resource `json:"resources"`
	Tracks    []Track    `json:"tracks"`
	Markers   []Marker   `json:"markers"`
}

// Build computes a compact snapshot of p.
//
// Only media referenced by a clip is retained; the rest of the (potentially
// huge) resource table is dropped. Resources and project markers are sorted
// by ID so their table order cannot influence the content hash, while tracks,
// clips and effects keep their meaningful ordering.
//
// A non-nil ReachError indicates dangling clip references; the snapshot is
// still returned and remains usable, letting strict callers reject it.
func Build(p *Project) (*Snapshot, ReachStats, error) {
	if p == nil {
		return nil, ReachStats{}, errInvalid("project is nil")
	}

	resourceByID := make(map[string]Resource, len(p.Resources))
	for _, r := range p.Resources {
		resourceByID[r.ID] = r
	}

	reachable := make(map[string]struct{})
	var dangling []DanglingRef
	for _, track := range p.Tracks {
		for _, clip := range track.Clips {
			if _, ok := resourceByID[clip.ResourceID]; ok {
				reachable[clip.ResourceID] = struct{}{}
			} else {
				dangling = append(dangling, DanglingRef{
					TrackID:    track.ID,
					ClipID:     clip.ID,
					ResourceID: clip.ResourceID,
				})
			}
		}
	}

	resources := make([]Resource, 0, len(reachable))
	for id := range reachable {
		resources = append(resources, cloneResource(resourceByID[id]))
	}
	sort.Slice(resources, func(i, j int) bool { return resources[i].ID < resources[j].ID })

	markers := make([]Marker, 0, len(p.Markers))
	for _, m := range p.Markers {
		markers = append(markers, cloneMarker(m))
	}
	sort.Slice(markers, func(i, j int) bool { return markers[i].ID < markers[j].ID })

	tracks := make([]Track, 0, len(p.Tracks))
	for _, t := range p.Tracks {
		tracks = append(tracks, cloneTrack(t))
	}

	body := snapshotBody{
		Name:      p.Name,
		Resources: resources,
		Tracks:    tracks,
		Markers:   markers,
	}
	id, err := hashContent(body)
	if err != nil {
		return nil, ReachStats{}, err
	}

	snap := &Snapshot{
		ID:                 id,
		Name:               p.Name,
		Resources:          resources,
		Tracks:             tracks,
		Markers:            markers,
		ReachableResources: len(resources),
		DroppedResources:   len(p.Resources) - len(resources),
	}
	stats := ReachStats{
		TotalResources:     len(p.Resources),
		ReachableResources: len(resources),
		DroppedResources:   len(p.Resources) - len(resources),
		DanglingRefs:       dangling,
	}
	var reachErr error
	if len(dangling) > 0 {
		reachErr = &ReachError{DanglingRefs: dangling}
	}
	return snap, stats, reachErr
}

// Restore rebuilds an editable Project from a snapshot. Unreachable resources
// cannot come back: they were never recorded. The returned project starts at
// version 1; callers may renumber it against their own history.
func (s *Snapshot) Restore() *Project {
	if s == nil {
		return nil
	}
	p := &Project{
		Version: 1,
		Name:    s.Name,
	}
	p.Resources = make([]Resource, len(s.Resources))
	for i, r := range s.Resources {
		p.Resources[i] = cloneResource(r)
	}
	p.Tracks = make([]Track, len(s.Tracks))
	for i, t := range s.Tracks {
		p.Tracks[i] = cloneTrack(t)
	}
	p.Markers = make([]Marker, len(s.Markers))
	for i, m := range s.Markers {
		p.Markers[i] = cloneMarker(m)
	}
	return p
}

// Clone returns an independent deep copy of the snapshot.
func (s *Snapshot) Clone() *Snapshot {
	if s == nil {
		return nil
	}
	out := &Snapshot{
		ID:                 s.ID,
		Name:               s.Name,
		ReachableResources: s.ReachableResources,
		DroppedResources:   s.DroppedResources,
	}
	out.Resources = make([]Resource, len(s.Resources))
	for i, r := range s.Resources {
		out.Resources[i] = cloneResource(r)
	}
	out.Tracks = make([]Track, len(s.Tracks))
	for i, t := range s.Tracks {
		out.Tracks[i] = cloneTrack(t)
	}
	out.Markers = make([]Marker, len(s.Markers))
	for i, m := range s.Markers {
		out.Markers[i] = cloneMarker(m)
	}
	return out
}

type validationError struct{ msg string }

func (e *validationError) Error() string { return "timelinesnap: " + e.msg }

func errInvalid(msg string) error { return &validationError{msg: msg} }
