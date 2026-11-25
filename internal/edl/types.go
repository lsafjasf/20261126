// Package edl models offline edit decision lists, the media libraries they
// point at, and the rules for re-anchoring offline edits onto an online
// project whose clip IDs and files have changed.
//
// Identity of a piece of media is the pair (clip ID, media hash). A file
// name is never treated as identity: the test fixtures deliberately contain
// same-name/different-content and different-name/same-content traps.
package edl

// Asset is one media library entry. MediaHash is the authoritative content
// hash (SHA-256 of the media bytes, hex encoded). Path is the current file
// location and may be reused by unrelated content, so it is never a key.
type Asset struct {
	ClipID    string `json:"clip_id"`
	Path      string `json:"path"`
	MediaHash string `json:"media_hash"`
	SizeBytes int64  `json:"size_bytes"`
}

// Library is a versioned media library, e.g. the old offline project or the
// current online project.
type Library struct {
	SchemaVersion string  `json:"schema_version"`
	ProjectID     string  `json:"project_id"`
	Assets        []Asset `json:"assets"`
}

// Op is one line of the offline change list. It references media as it was
// identified in the old project.
type Op struct {
	Seq        int    `json:"seq"`
	Action     string `json:"action"` // trim | move | rename | set_note
	OldClipID  string `json:"old_clip_id"`
	MediaHash  string `json:"media_hash"` // content hash captured offline
	OldPath    string `json:"old_path"`   // recorded for evidence only
	InFrame    int64  `json:"in_frame,omitempty"`
	OutFrame   int64  `json:"out_frame,omitempty"`
	NewTrack   string `json:"new_track,omitempty"`
	NewStartMs int64  `json:"new_start_ms,omitempty"`
	Note       string `json:"note,omitempty"`
}

// ChangeList is the offline editor's edit decision list.
type ChangeList struct {
	SchemaVersion string `json:"schema_version"`
	ProjectID     string `json:"project_id"`
	BasedOn       string `json:"based_on"`
	Ops           []Op   `json:"ops"`
}

// Classification buckets.
const (
	StatusApplied = "applied" // re-anchored on exactly one online asset
	StatusPending = "pending" // ambiguous or blocked; a human must decide
	StatusDead    = "dead"    // the referenced content no longer exists online
)

// Reason codes explain why an op landed in its bucket.
const (
	ReasonIDHashMatch       = "id_and_hash_match"
	ReasonHashRelocated     = "hash_relocated_to_new_id"
	ReasonHashDuplicate     = "hash_present_on_multiple_assets"
	ReasonIDReused          = "clip_id_reused_by_different_content"
	ReasonFileNameCollision = "same_file_name_different_content"
	ReasonHashMissing       = "media_hash_not_found_online"
	ReasonOfflineMismatch   = "offline_id_hash_pair_inconsistent"
	ReasonOnlineHashVerify  = "online_media_hash_verification_failed"
)

// Candidate records one online asset that matched on a dimension, plus which
// dimensions matched, so reviewers can see why a decision was ambiguous.
type Candidate struct {
	ClipID    string `json:"clip_id"`
	Path      string `json:"path"`
	MediaHash string `json:"media_hash"`
	MatchID   bool   `json:"match_clip_id"`
	MatchHash bool   `json:"match_media_hash"`
	MatchName bool   `json:"match_file_name"`
}

// Resolution is the verdict for one offline op.
type Resolution struct {
	Seq        int         `json:"seq"`
	Action     string      `json:"action"`
	Status     string      `json:"status"`
	Reason     string      `json:"reason"`
	Offline    Op          `json:"offline"`
	Candidates []Candidate `json:"candidates"`
	ResolvedTo *Asset      `json:"resolved_to,omitempty"`
	Warnings   []string    `json:"warnings,omitempty"`
}

// Report is the full three-way classification of a change list.
type Report struct {
	ProjectID     string       `json:"project_id"`
	OnlineVersion string       `json:"online_schema_version"`
	Applied       []Resolution `json:"applied"`
	Pending       []Resolution `json:"pending"`
	Dead          []Resolution `json:"dead"`
	AppliedCount  int          `json:"applied_count"`
	PendingCount  int          `json:"pending_count"`
	DeadCount     int          `json:"dead_count"`
}
