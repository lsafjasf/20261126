// Package timelinesnap creates compact, content-addressed version snapshots
// of a collaborative video-editing timeline.
//
// Snapshots only contain resources that are actually reachable from the
// timeline. Resource and project-marker tables are order-insensitive and are
// canonicalized, so reordering them never produces a content change. Clips,
// effects and the tracks themselves are ordered sequences, where reordering
// is a real timeline change and is reported separately from field edits.
package timelinesnap

// ResourceType describes how a resource is used by the timeline.
type ResourceType string

const (
	ResourceVideo ResourceType = "video"
	ResourceAudio ResourceType = "audio"
	ResourceImage ResourceType = "image"
)

// Resource is an entry in the project's media table. Only resources reachable
// from at least one clip are persisted in a snapshot.
type Resource struct {
	ID       string            `json:"id"`
	Type     ResourceType      `json:"type"`
	URI      string            `json:"uri"`
	Checksum string            `json:"checksum,omitempty"`
	Size     int64             `json:"size,omitempty"`
	Meta     map[string]string `json:"meta,omitempty"`
}

// TrackState records the collaborative state of a track.
type TrackState string

const (
	TrackEnabled  TrackState = "enabled"
	TrackDisabled TrackState = "disabled"
	TrackLocked   TrackState = "locked"
)

// Effect applies to a clip. Effects form an ordered stack: order is
// semantically meaningful and is preserved in snapshots.
type Effect struct {
	ID     string         `json:"id"`
	Kind   string         `json:"kind"`
	Params map[string]any `json:"params,omitempty"`
}

// Clip is an ordered element of a track. The ResourceID must reference a
// resource reachable from the project.
type Clip struct {
	ID         string            `json:"id"`
	ResourceID string            `json:"resourceId"`
	StartNS    int64             `json:"startNs"`
	DurationNS int64             `json:"durationNs"`
	Effects    []Effect          `json:"effects,omitempty"`
	Meta       map[string]string `json:"meta,omitempty"`
}

// Track is an ordered collection of clips. Track order inside a project and
// clip order inside a track are both semantically meaningful.
type Track struct {
	ID    string            `json:"id"`
	Name  string            `json:"name"`
	State TrackState        `json:"state"`
	Clips []Clip            `json:"clips,omitempty"`
	Meta  map[string]string `json:"meta,omitempty"`
}

// Marker is an annotation attached to the project. Markers are an
// order-insensitive set keyed by ID; their table order is canonicalized.
type Marker struct {
	ID    string            `json:"id"`
	AtNS  int64             `json:"atNs"`
	Label string            `json:"label"`
	Color string            `json:"color,omitempty"`
	Meta  map[string]string `json:"meta,omitempty"`
}

// Project is the live, collaboratively edited timeline document. It may hold
// a large resource table containing entries that no clip references; such
// entries are ignored when building snapshots.
type Project struct {
	Version   int        `json:"version"`
	Name      string     `json:"name"`
	Resources []Resource `json:"resources,omitempty"`
	Tracks    []Track    `json:"tracks,omitempty"`
	Markers   []Marker   `json:"markers,omitempty"`
}

// ResourceView is a resource known to be reachable.
type ResourceView = Resource

// Snapshot is an immutable, content-addressed view of a project at one point.
// It contains reachable resources, markers, effects and full track state, and
// nothing else.
type Snapshot struct {
	// ID is the content address of the snapshot. Two builds of semantically
	// identical projects (including order-only reshuffles of resources and
	// markers) produce the same ID.
	ID   string `json:"id"`
	Name string `json:"name"`

	// Resources holds only reachable media, sorted by ID.
	Resources []ResourceView `json:"resources"`
	// Tracks preserve clip and effect ordering.
	Tracks []Track `json:"tracks"`
	// Markers are sorted by ID.
	Markers []Marker `json:"markers"`

	// ReachableResources is the number of referenced resource entries.
	ReachableResources int `json:"reachableResources"`
	// DroppedResources counts resource-table entries unreachable from clips.
	DroppedResources int `json:"droppedResources"`
}

// ReachStats summarizes the reachability analysis of a build.
type ReachStats struct {
	TotalResources     int
	ReachableResources int
	DroppedResources   int
	DanglingRefs       []DanglingRef
}

// DanglingRef reports a clip referencing a resource that does not exist.
type DanglingRef struct {
	TrackID    string
	ClipID     string
	ResourceID string
}

// ReachError aggregates reachability problems found while building. Build
// tolerates missing references (the clip is kept, no resource is emitted),
// but callers can treat this error as fatal if they prefer strict validation.
type ReachError struct {
	DanglingRefs []DanglingRef
}

func (e *ReachError) Error() string {
	return "timelinesnap: " + itoa(len(e.DanglingRefs)) + " dangling clip resource reference(s)"
}
