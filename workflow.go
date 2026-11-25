package timelinesnap

import "context"

// SnapshotResult bundles a commit with the build statistics.
type SnapshotResult struct {
	Commit *Commit
	// Created is false when the head already had identical content, meaning
	// the history was not extended (anti-bloat no-op).
	Created bool
	Stats   ReachStats
}

// SnapshotProject builds the reachable-only snapshot of p and records it.
// Dangling references are reported through SnapshotResult.Stats but do not
// block the commit; pass a stricter policy at the caller if needed.
func SnapshotProject(ctx context.Context, store Store, p *Project, message, author string) (*SnapshotResult, error) {
	snap, stats, _ := Build(p)
	c, created, err := store.Commit(ctx, snap, message, author)
	if err != nil {
		return nil, err
	}
	return &SnapshotResult{Commit: c, Created: created, Stats: stats}, nil
}

// RollbackRevision reverts the store head to the snapshot recorded at
// revision. The original history is preserved: a new commit pointing at the
// old snapshot is appended instead of deleting anything.
func RollbackRevision(ctx context.Context, store Store, revision int, message, author string) (*Commit, error) {
	return store.Revert(ctx, revision, message, author)
}

// RestoreCommit returns an editable Project for the snapshot of a commit.
func RestoreCommit(ctx context.Context, store Store, revision int) (*Project, error) {
	c, err := store.GetCommit(ctx, revision)
	if err != nil {
		return nil, err
	}
	snap, err := store.Load(ctx, c.SnapshotID)
	if err != nil {
		return nil, err
	}
	return snap.Restore(), nil
}

// CompareRevisions diffs two stored commits. Revisions are 1-based.
func CompareRevisions(ctx context.Context, store Store, oldRev, newRev int) (*Report, error) {
	oldProject, err := RestoreCommit(ctx, store, oldRev)
	if err != nil {
		return nil, err
	}
	newProject, err := RestoreCommit(ctx, store, newRev)
	if err != nil {
		return nil, err
	}
	report, err := DiffProjects(oldProject, newProject)
	if err != nil {
		return nil, err
	}
	oldCommit, err := store.GetCommit(ctx, oldRev)
	if err != nil {
		return nil, err
	}
	newCommit, err := store.GetCommit(ctx, newRev)
	if err != nil {
		return nil, err
	}
	report.OldID = oldCommit.SnapshotID
	report.NewID = newCommit.SnapshotID
	return report, nil
}
