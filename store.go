package timelinesnap

import (
	"context"
	"errors"
	"time"
)

// ErrNotFound indicates a snapshot or commit revision does not exist.
var ErrNotFound = errors.New("timelinesnap: snapshot or commit not found")

// Commit is one entry in the snapshot history. It is a pointer to a
// content-addressed blob, so reverting never duplicates snapshot data.
type Commit struct {
	Revision       int       `json:"revision"`
	SnapshotID     string    `json:"snapshotId"`
	Message        string    `json:"message"`
	Author         string    `json:"author,omitempty"`
	CreatedAt      time.Time `json:"createdAt"`
	ParentRevision int       `json:"parentRevision,omitempty"`
	// RevertOf is set when this commit restores an earlier snapshot.
	RevertOf int `json:"revertOf,omitempty"`
}

// RetentionPolicy bounds snapshot history.
type RetentionPolicy struct {
	// MaxCommits keeps at most this many newest commits. Zero means keep all
	// commits; orphaned blobs are still collected.
	MaxCommits int
}

// GCResult reports what a garbage-collection pass removed.
type GCResult struct {
	RemovedCommits int
	RemovedBlobs   int
}

// Store persists snapshots and their commit history.
type Store interface {
	// Save stores a snapshot blob. It is idempotent: saving the same content
	// twice writes nothing new and returns the same ID.
	Save(ctx context.Context, snap *Snapshot) (string, error)
	// Load fetches a snapshot blob by content ID.
	Load(ctx context.Context, id string) (*Snapshot, error)
	// Commit records snap as the new head. If the head already holds the same
	// content, nothing is stored and created is false, which prevents the
	// history from growing on no-op saves.
	Commit(ctx context.Context, snap *Snapshot, message, author string) (c *Commit, created bool, err error)
	// Head returns the newest commit.
	Head(ctx context.Context) (*Commit, error)
	// GetCommit resolves a 1-based revision.
	GetCommit(ctx context.Context, revision int) (*Commit, error)
	// List returns commits oldest-first.
	List(ctx context.Context) ([]*Commit, error)
	// Revert creates a new head from the snapshot recorded at revision.
	Revert(ctx context.Context, revision int, message, author string) (*Commit, error)
	// GC enforces the retention policy and removes unreferenced blobs.
	GC(ctx context.Context, policy RetentionPolicy) (GCResult, error)
	// Close releases underlying resources.
	Close() error
}

// storeSink is the persistence backend used by the shared in-memory logic.
type storeSink interface {
	writeBlob(snap *Snapshot) error
	deleteBlob(id string) error
	writeCommit(c *Commit) error
	deleteCommit(revision int) error
}

type baseStore struct {
	blobs   map[string]*Snapshot
	commits []*Commit
}

func newBaseStore() baseStore {
	return baseStore{blobs: make(map[string]*Snapshot)}
}

func (b *baseStore) save(sink storeSink, snap *Snapshot) (string, error) {
	if snap == nil {
		return "", errInvalid("cannot save nil snapshot")
	}
	if err := verifyID(snap); err != nil {
		return "", err
	}
	if _, ok := b.blobs[snap.ID]; ok {
		return snap.ID, nil
	}
	if err := sink.writeBlob(snap.Clone()); err != nil {
		return "", err
	}
	b.blobs[snap.ID] = snap.Clone()
	return snap.ID, nil
}

func (b *baseStore) load(id string) (*Snapshot, error) {
	snap, ok := b.blobs[id]
	if !ok {
		return nil, ErrNotFound
	}
	return snap.Clone(), nil
}

func (b *baseStore) commit(sink storeSink, snap *Snapshot, message, author string, revertOf int) (*Commit, bool, error) {
	id, err := b.save(sink, snap)
	if err != nil {
		return nil, false, err
	}
	if head := b.head(); head != nil && head.SnapshotID == id {
		return head, false, nil
	}
	c := &Commit{
		Revision:   len(b.commits) + 1,
		SnapshotID: id,
		Message:    message,
		Author:     author,
		CreatedAt:  clockNow(),
		RevertOf:   revertOf,
	}
	if head := b.head(); head != nil {
		c.ParentRevision = head.Revision
	}
	if err := sink.writeCommit(c); err != nil {
		return nil, false, err
	}
	b.commits = append(b.commits, c)
	return c, true, nil
}

func (b *baseStore) revert(sink storeSink, revision int, message, author string) (*Commit, error) {
	c, err := b.getCommit(revision)
	if err != nil {
		return nil, err
	}
	snap, err := b.load(c.SnapshotID)
	if err != nil {
		return nil, err
	}
	if message == "" {
		message = "revert to revision " + itoa(revision)
	}
	created, _, err := b.commit(sink, snap, message, author, revision)
	return created, err
}

func (b *baseStore) head() *Commit {
	if len(b.commits) == 0 {
		return nil
	}
	return b.commits[len(b.commits)-1]
}

func (b *baseStore) getCommit(revision int) (*Commit, error) {
	if revision < 1 || revision > len(b.commits) {
		return nil, ErrNotFound
	}
	return b.commits[revision-1], nil
}

func (b *baseStore) list() []*Commit {
	out := make([]*Commit, len(b.commits))
	copy(out, b.commits)
	return out
}

func (b *baseStore) gc(sink storeSink, policy RetentionPolicy) (GCResult, error) {
	var result GCResult

	keepFrom := 0
	if policy.MaxCommits > 0 && policy.MaxCommits < len(b.commits) {
		keepFrom = len(b.commits) - policy.MaxCommits
	}
	if keepFrom > 0 {
		dropped := b.commits[:keepFrom]
		for _, c := range dropped {
			if err := sink.deleteCommit(c.Revision); err != nil {
				return result, err
			}
			result.RemovedCommits++
		}
		b.commits = b.commits[keepFrom:]
		// Renumber retained revisions so the chain stays 1-based and dense.
		for i, c := range b.commits {
			c.Revision = i + 1
			if i == 0 {
				c.ParentRevision = 0
			} else {
				c.ParentRevision = i
			}
			if c.RevertOf > keepFrom {
				// Re-numbered reference inside the retained window.
				c.RevertOf -= keepFrom
			} else {
				// Reference points at a pruned revision (or none).
				c.RevertOf = 0
			}
			if err := sink.writeCommit(c); err != nil {
				return result, err
			}
		}
	}

	referenced := make(map[string]struct{}, len(b.commits))
	for _, c := range b.commits {
		referenced[c.SnapshotID] = struct{}{}
	}
	for id := range b.blobs {
		if _, ok := referenced[id]; ok {
			continue
		}
		if err := sink.deleteBlob(id); err != nil {
			return result, err
		}
		delete(b.blobs, id)
		result.RemovedBlobs++
	}
	return result, nil
}

func verifyID(snap *Snapshot) error {
	body := snapshotBody{
		Name:      snap.Name,
		Resources: snap.Resources,
		Tracks:    snap.Tracks,
		Markers:   snap.Markers,
	}
	want, err := hashContent(body)
	if err != nil {
		return err
	}
	if want != snap.ID {
		return errInvalid("snapshot ID does not match content")
	}
	return nil
}

// clockNow is indirected so time-dependent code stays testable.
var clockNow = time.Now
