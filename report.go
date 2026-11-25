package clipmerge

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

// SchemaVersion identifies the report schema emitted by this service.
const SchemaVersion = "clip-merge/v1"

// Dispositions assigned to edits by the merge engine.
const (
	DispositionPropertyResolved = "property_resolved"
	DispositionUnresolved       = "unresolved"
	DispositionStaleIgnored     = "stale_ignored"
)

// Decision codes.
const (
	CodeAutoMerged               = "auto_merged"
	CodeManualResolutionRequired = "manual_resolution_required"
	CodeMediaBoundaryViolation   = "media_boundary_violation"
)

// Conflict kinds.
const (
	KindSamePropertyDivergence = "same_property_divergence"
	KindDependencyViolation    = "dependency_violation"
)

// ClipSnapshot is a clip plus its derived, exactly-computed fields.
type ClipSnapshot struct {
	ID               string `json:"id"`
	SourceID         string `json:"sourceId"`
	Revision         int64  `json:"revision"`
	InPoint          int64  `json:"inPoint"`
	OutPoint         int64  `json:"outPoint"`
	Speed            Speed  `json:"speed"`
	SourceSpan       int64  `json:"sourceSpan"`
	TimelineDuration Speed  `json:"timelineDuration"`
}

// Snapshot derives the deterministic snapshot of a clip.
func Snapshot(c Clip) ClipSnapshot {
	return ClipSnapshot{
		ID: c.ID, SourceID: c.SourceID, Revision: c.Revision,
		InPoint: c.InPoint, OutPoint: c.OutPoint, Speed: c.Speed.Normalized(),
		SourceSpan: c.SourceSpan(), TimelineDuration: c.TimelineDuration(),
	}
}

// EditRecord is an edit plus the disposition decided by the engine.
type EditRecord struct {
	ID           string `json:"id"`
	ClientID     string `json:"clientId"`
	BaseRevision int64  `json:"baseRevision"`
	Property     string `json:"property"`
	Value        Value  `json:"value"`
	Disposition  string `json:"disposition"`
}

// IgnoredEdit is a stale edit excluded from merging, with the reason.
type IgnoredEdit struct {
	EditRecord
	Reason string `json:"reason"`
}

// Candidate is one distinct proposed value and the edits/clients behind it.
type Candidate struct {
	Value     Value    `json:"value"`
	EditIDs   []string `json:"editIds"`
	ClientIDs []string `json:"clientIds"`
}

// PropertyResolution is the per-property three-way merge outcome.
type PropertyResolution struct {
	Property       string      `json:"property"`
	BaseValue      Value       `json:"baseValue"`
	Candidates     []Candidate `json:"candidates"`
	EffectiveValue *Value      `json:"effectiveValue"`
	State          string      `json:"state"`
}

// Check is one media-boundary validation result.
type Check struct {
	Name       string   `json:"name"`
	Properties []string `json:"properties"`
	Rule       string   `json:"rule"`
	Status     string   `json:"status"`
	Observed   string   `json:"observed"`
	Expected   string   `json:"expected"`
	Message    string   `json:"message"`
}

// ObservedValue pairs a property with its effective value in a conflict.
type ObservedValue struct {
	Property string `json:"property"`
	Value    Value  `json:"value"`
}

// Conflict is a property-level conflict record. Exactly one of Candidates
// (same_property_divergence) or EditIDs/Observed (dependency_violation) is set.
type Conflict struct {
	ConflictID string          `json:"conflictId,omitempty"`
	Kind       string          `json:"kind"`
	Properties []string        `json:"properties"`
	Reason     string          `json:"reason"`
	Message    string          `json:"message"`
	Candidates []Candidate     `json:"candidates,omitempty"`
	EditIDs    []string        `json:"editIds,omitempty"`
	Observed   []ObservedValue `json:"observed,omitempty"`
}

// Decision is the final merge gate outcome.
type Decision struct {
	AutoMerge                bool   `json:"autoMerge"`
	Code                     string `json:"code"`
	Reason                   string `json:"reason"`
	RequiresManualResolution bool   `json:"requiresManualResolution"`
}

// Report is the reproducible output of one merge evaluation.
type Report struct {
	SchemaVersion       string               `json:"schemaVersion"`
	ReportID            string               `json:"reportId,omitempty"`
	Status              string               `json:"status"`
	Conflict            bool                 `json:"conflict"`
	ClipID              string               `json:"clipId"`
	SourceID            string               `json:"sourceId"`
	BaseRevision        int64                `json:"baseRevision"`
	TimeUnit            string               `json:"timeUnit"`
	Source              Source               `json:"source"`
	Base                ClipSnapshot         `json:"base"`
	Edits               []EditRecord         `json:"edits"`
	IgnoredEdits        []IgnoredEdit        `json:"ignoredEdits"`
	PropertyResolutions []PropertyResolution `json:"propertyResolutions"`
	Checks              []Check              `json:"checks"`
	Conflicts           []Conflict           `json:"conflicts"`
	Decision            Decision             `json:"decision"`
	MergedClip          *ClipSnapshot        `json:"mergedClip,omitempty"`
}

// canonicalJSON is the deterministic byte form used for hashing and for
// report IDs: compact JSON, schema field order, no HTML escaping.
func canonicalJSON(v interface{}) []byte {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		panic(err)
	}
	return bytes.TrimRight(buf.Bytes(), "\n")
}

// contentHash hashes the canonical form of a value whose ID field is empty
// (and therefore omitted), making the ID a pure function of the content.
func contentHash(v interface{}) string {
	sum := sha256.Sum256(canonicalJSON(v))
	return "sha256:" + hex.EncodeToString(sum[:])
}

// AssignIDs computes deterministic content IDs for the report and its
// conflicts. No wall-clock data is involved: identical inputs always yield
// identical IDs, which is what makes the report reproducible.
func (r *Report) AssignIDs() {
	for i := range r.Conflicts {
		r.Conflicts[i].ConflictID = ""
		r.Conflicts[i].ConflictID = contentHash(r.Conflicts[i])
	}
	r.ReportID = ""
	r.ReportID = contentHash(*r)
}

// Marshal renders the report as indented JSON with a trailing newline.
func (r *Report) Marshal() []byte {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(r); err != nil {
		panic(err)
	}
	return buf.Bytes()
}
