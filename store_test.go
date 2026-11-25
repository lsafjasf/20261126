package timelinesnap

import (
	"context"
	"testing"
)

func ctx() context.Context { return context.Background() }

func mustBuild(t *testing.T, p *Project) *Snapshot {
	t.Helper()
	snap, _, err := Build(p)
	if err != nil {
		t.Fatalf("build failed: %v", err)
	}
	return snap
}

func TestCommitNoOpDoesNotGrowHistory(t *testing.T) {
	store := NewMemStore()
	p := sampleProject()

	first, err := SnapshotProject(ctx(), store, p, "v1", "alice")
	if err != nil {
		t.Fatal(err)
	}
	if !first.Created || first.Commit.Revision != 1 {
		t.Fatalf("unexpected first commit: %+v", first)
	}

	// Saving the same content again (resource table reshuffled) is a no-op.
	p.Resources[0], p.Resources[1] = p.Resources[1], p.Resources[0]
	second, err := SnapshotProject(ctx(), store, p, "v1 again", "bob")
	if err != nil {
		t.Fatal(err)
	}
	if second.Created {
		t.Fatal("identical content created a new commit")
	}
	commits, _ := store.List(ctx())
	if len(commits) != 1 {
		t.Fatalf("history grew on no-op save: %d", len(commits))
	}
}

func TestRevertIsNonDestructiveAndRestorable(t *testing.T) {
	store := NewMemStore()

	p1 := sampleProject()
	r1, err := SnapshotProject(ctx(), store, p1, "v1", "alice")
	if err != nil {
		t.Fatal(err)
	}

	p2 := sampleProject()
	p2.Name = "cut v2"
	p2.Tracks[0].Clips = p2.Tracks[0].Clips[:1]
	if _, err := SnapshotProject(ctx(), store, p2, "v2", "alice"); err != nil {
		t.Fatal(err)
	}

	revertCommit, err := RollbackRevision(ctx(), store, r1.Commit.Revision, "", "bob")
	if err != nil {
		t.Fatal(err)
	}
	if revertCommit.RevertOf != 1 || revertCommit.Revision != 3 {
		t.Fatalf("unexpected revert commit: %+v", revertCommit)
	}

	restored, err := RestoreCommit(ctx(), store, revertCommit.Revision)
	if err != nil {
		t.Fatal(err)
	}
	restoredSnap, _, _ := Build(restored)
	if restoredSnap.ID != r1.Commit.SnapshotID {
		t.Fatal("rollback did not restore revision 1 content")
	}

	// History is preserved, not rewritten.
	commits, _ := store.List(ctx())
	if len(commits) != 3 {
		t.Fatalf("revert must append, got %d commits", len(commits))
	}
}

func TestGCBoundsHistoryAndReapsOrphans(t *testing.T) {
	store := NewMemStore()

	for i := 0; i < 5; i++ {
		p := sampleProject()
		p.Name = "cut " + string(rune('a'+i))
		if _, err := SnapshotProject(ctx(), store, p, "msg", "alice"); err != nil {
			t.Fatal(err)
		}
	}
	commits, _ := store.List(ctx())
	if len(commits) != 5 {
		t.Fatalf("setup: want 5 commits, got %d", len(commits))
	}

	result, err := store.GC(ctx(), RetentionPolicy{MaxCommits: 2})
	if err != nil {
		t.Fatal(err)
	}
	if result.RemovedCommits != 3 || result.RemovedBlobs != 3 {
		t.Fatalf("unexpected GC result: %+v", result)
	}
	remaining, _ := store.List(ctx())
	if len(remaining) != 2 || remaining[0].Revision != 1 || remaining[1].Revision != 2 {
		t.Fatalf("retention/renumber failed: %+v", remaining)
	}

	// The store still resolves every retained snapshot.
	for _, c := range remaining {
		if _, err := store.Load(ctx(), c.SnapshotID); err != nil {
			t.Fatalf("retained blob missing: %v", err)
		}
	}
	if _, err := store.GetCommit(ctx(), 5); err != ErrNotFound {
		t.Fatalf("trimmed revision should be ErrNotFound, got %v", err)
	}
}

func TestGCRenumbersRevertReferenceAtWindowBoundary(t *testing.T) {
	store := NewMemStore()
	p1 := sampleProject()
	r1, err := SnapshotProject(ctx(), store, p1, "v1", "alice")
	if err != nil {
		t.Fatal(err)
	}
	p2 := sampleProject()
	p2.Name = "cut v2"
	if _, err := SnapshotProject(ctx(), store, p2, "v2", "alice"); err != nil {
		t.Fatal(err)
	}
	revertCommit, err := store.Revert(ctx(), r1.Commit.Revision, "", "bob")
	if err != nil {
		t.Fatal(err)
	}

	// Keep only the newest two commits: old rev 2 becomes rev 1, old rev 3
	// becomes rev 2, and its revert target (old rev 1 = pruned) must be
	// dropped rather than pointing at an unrelated retained revision.
	if _, err := store.GC(ctx(), RetentionPolicy{MaxCommits: 2}); err != nil {
		t.Fatal(err)
	}
	remaining, _ := store.List(ctx())
	if len(remaining) != 2 {
		t.Fatalf("want 2 retained commits, got %d", len(remaining))
	}
	if remaining[1].RevertOf != 0 {
		t.Fatalf("pruned revert reference not cleared: %+v", remaining[1])
	}

	// Re-pointing inside the retained window is re-numbered correctly.
	revertCommit, err = store.Revert(ctx(), remaining[0].Revision, "", "bob")
	if err != nil {
		t.Fatal(err)
	}
	if revertCommit.RevertOf != 1 || revertCommit.Revision != 3 {
		t.Fatalf("unexpected revert metadata: %+v", revertCommit)
	}
}

func TestGCDeduplicatesBlobsAcrossCommits(t *testing.T) {
	store := NewMemStore()
	p := sampleProject()

	c1, _, err := store.Commit(ctx(), mustBuild(t, p), "v1", "alice")
	if err != nil {
		t.Fatal(err)
	}
	// A no-op commit shares the blob; GC must not remove the shared blob.
	if _, _, err := store.Commit(ctx(), mustBuild(t, p), "v1 duplicate", "alice"); err != nil {
		t.Fatal(err)
	}
	result, err := store.GC(ctx(), RetentionPolicy{MaxCommits: 0})
	if err != nil {
		t.Fatal(err)
	}
	if result.RemovedBlobs != 0 {
		t.Fatalf("shared blob reaped: %+v", result)
	}
	if _, err := store.Load(ctx(), c1.SnapshotID); err != nil {
		t.Fatalf("blob unavailable after GC: %v", err)
	}
}

func TestSaveRejectsTamperedID(t *testing.T) {
	store := NewMemStore()
	snap := mustBuild(t, sampleProject())
	snap.ID = "deadbeef"
	if _, err := store.Save(ctx(), snap); err == nil {
		t.Fatal("tampered snapshot ID was accepted")
	}
}

func TestFileStorePersistenceAndReopen(t *testing.T) {
	dir := t.TempDir()

	store, err := OpenFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	result, err := SnapshotProject(ctx(), store, sampleProject(), "v1", "alice")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := OpenFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()

	commits, err := reopened.List(ctx())
	if err != nil {
		t.Fatal(err)
	}
	if len(commits) != 1 || commits[0].SnapshotID != result.Commit.SnapshotID {
		t.Fatalf("history not persisted: %+v", commits)
	}
	snap, err := reopened.Load(ctx(), result.Commit.SnapshotID)
	if err != nil {
		t.Fatal(err)
	}
	if snap.ReachableResources != 2 {
		t.Fatalf("blob content mismatch: %+v", snap)
	}

	// Reopening again and committing continues the chain at revision 2.
	c, created, err := reopened.Commit(ctx(), mustBuild(t, sampleProject()), "same", "alice")
	if err != nil {
		t.Fatal(err)
	}
	if created || c.Revision != 1 {
		t.Fatalf("reopened store lost dedupe state: rev=%d created=%v", c.Revision, created)
	}
}

func TestCompareRevisionsRoundTrip(t *testing.T) {
	store := NewMemStore()

	p1 := sampleProject()
	r1, err := SnapshotProject(ctx(), store, p1, "v1", "alice")
	if err != nil {
		t.Fatal(err)
	}
	p2 := sampleProject()
	p2.Tracks[1].State = TrackDisabled
	r2, err := SnapshotProject(ctx(), store, p2, "v2", "alice")
	if err != nil {
		t.Fatal(err)
	}

	report, err := CompareRevisions(ctx(), store, r1.Commit.Revision, r2.Commit.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if !report.HasContentChanges() || report.OldID != r1.Commit.SnapshotID {
		t.Fatalf("unexpected report: %+v", report)
	}
}
