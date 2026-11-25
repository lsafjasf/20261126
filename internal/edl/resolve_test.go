package edl

import (
	"path/filepath"
	"testing"
)

func fixturePaths(t *testing.T) (off, on, cl string) {
	t.Helper()
	base := filepath.Join("..", "..", "testdata")
	return filepath.Join(base, "offline_library.json"),
		filepath.Join(base, "online_library.json"),
		filepath.Join(base, "offline_changelist.json")
}

func statusBySeq(rep Report) map[string]Resolution {
	m := map[string]Resolution{}
	for _, bucket := range [][]Resolution{rep.Applied, rep.Pending, rep.Dead} {
		for _, r := range bucket {
			m[string(rune('0'+r.Seq))] = r
		}
	}
	return m
}

func loadAndResolve(t *testing.T) Report {
	t.Helper()
	offPath, onPath, clPath := fixturePaths(t)
	offline, err := LoadLibrary(offPath)
	if err != nil {
		t.Fatalf("load offline library: %v", err)
	}
	online, err := LoadLibrary(onPath)
	if err != nil {
		t.Fatalf("load online library: %v", err)
	}
	cl, err := LoadChangeList(clPath)
	if err != nil {
		t.Fatalf("load change list: %v", err)
	}
	return Resolve(offline, online, cl)
}

func TestBucketCounts(t *testing.T) {
	rep := loadAndResolve(t)
	if rep.AppliedCount != 3 || rep.PendingCount != 4 || rep.DeadCount != 2 {
		t.Fatalf("bucket counts = (%d,%d,%d), want (3,4,2)",
			rep.AppliedCount, rep.PendingCount, rep.DeadCount)
	}
	if len(rep.Applied)+len(rep.Pending)+len(rep.Dead) != 9 {
		t.Fatal("every offline op must appear in exactly one bucket")
	}
}

func TestAppliedResolutions(t *testing.T) {
	by := statusBySeq(loadAndResolve(t))

	r1 := by["1"]
	if r1.Status != StatusApplied || r1.Reason != ReasonIDHashMatch {
		t.Fatalf("seq1 = %s/%s, want applied/%s", r1.Status, r1.Reason, ReasonIDHashMatch)
	}

	// Clean hash relocation: ID changed, no conflicting dimension.
	r4 := by["4"]
	if r4.Status != StatusApplied || r4.Reason != ReasonHashRelocated {
		t.Fatalf("seq4 = %s/%s, want applied/hash_relocated", r4.Status, r4.Reason)
	}
	if r4.ResolvedTo == nil || r4.ResolvedTo.ClipID != "c14" {
		t.Fatalf("seq4 target = %+v, want c14", r4.ResolvedTo)
	}
}

func TestPendingResolutions(t *testing.T) {
	by := statusBySeq(loadAndResolve(t))

	// ID reuse: old c02 now names different content; the real content is c12.
	r2 := by["2"]
	if r2.Status != StatusPending || r2.Reason != ReasonHashRelocated {
		t.Fatalf("seq2 = %s/%s, want pending/hash_relocated", r2.Status, r2.Reason)
	}
	if r2.ResolvedTo == nil || r2.ResolvedTo.ClipID != "c12" {
		t.Fatalf("seq2 suggested target = %+v, want c12", r2.ResolvedTo)
	}

	// Same-name/different-content trap must force confirmation.
	r3 := by["3"]
	if r3.Status != StatusPending {
		t.Fatalf("seq3 = %s, want pending due to file name collision", r3.Status)
	}

	// Duplicate content on two clip IDs is ambiguous.
	r6 := by["6"]
	if r6.Status != StatusPending || r6.Reason != ReasonHashDuplicate {
		t.Fatalf("seq6 = %s/%s, want pending/hash_duplicate", r6.Status, r6.Reason)
	}
	if len(r6.Candidates) != 2 {
		t.Fatalf("seq6 candidates = %d, want 2", len(r6.Candidates))
	}

	// Offline manifest inconsistency must never auto-match.
	r9 := by["9"]
	if r9.Status != StatusPending || r9.Reason != ReasonOfflineMismatch {
		t.Fatalf("seq9 = %s/%s, want pending/offline_mismatch", r9.Status, r9.Reason)
	}
}

func TestDeadResolutionsIgnoreFileName(t *testing.T) {
	rep := loadAndResolve(t)
	by := statusBySeq(rep)

	r5 := by["5"]
	if r5.Status != StatusDead || r5.Reason != ReasonHashMissing {
		t.Fatalf("seq5 = %s/%s, want dead/hash_missing", r5.Status, r5.Reason)
	}

	// c07's content was deleted, but a decoy keeps the same file name.
	// Matching by name would wrongly "succeed"; the hash rule must win.
	r7 := by["7"]
	if r7.Status != StatusDead {
		t.Fatalf("seq7 = %s, want dead despite same-name decoy", r7.Status)
	}
	foundDecoy := false
	for _, c := range r7.Candidates {
		if c.MatchName && !c.MatchHash {
			foundDecoy = true
		}
	}
	if !foundDecoy {
		t.Fatal("seq7 should surface the same-name decoy as evidence only")
	}
}

func TestAppliedChangeListOnlyContainsApplied(t *testing.T) {
	rep := loadAndResolve(t)
	acl, err := BuildAppliedChangeList(rep)
	if err != nil {
		t.Fatalf("build applied change list: %v", err)
	}
	if len(acl.Ops) != rep.AppliedCount {
		t.Fatalf("applied ops = %d, want %d", len(acl.Ops), rep.AppliedCount)
	}
	wantSeq := map[int]bool{1: true, 4: true, 8: true}
	for _, op := range acl.Ops {
		if !wantSeq[op.Seq] {
			t.Fatalf("unexpected applied seq %d", op.Seq)
		}
	}
}

func TestOnDiskHashVerification(t *testing.T) {
	online, err := LoadLibrary(filepath.Join("..", "..", "testdata", "online_library.json"))
	if err != nil {
		t.Fatalf("load online library: %v", err)
	}
	root := filepath.Join("..", "..", "testdata", "online_media")
	if err := VerifyLibrary(online, root); err != nil {
		t.Fatalf("verify online media hashes: %v", err)
	}
}

func TestOnDiskHashVerificationDetectsTamper(t *testing.T) {
	lib := Library{
		SchemaVersion: "test",
		ProjectID:     "p",
		Assets: []Asset{{
			ClipID:    "x1",
			Path:      "media/intro.mov", // real fixture file
			MediaHash: "deadbeef",        // deliberately wrong
		}},
	}
	root := filepath.Join("..", "..", "testdata", "online_media")
	if err := VerifyLibrary(lib, root); err == nil {
		t.Fatal("expected hash mismatch error, got nil")
	}
}

func TestFileNameNeverActsAsIdentity(t *testing.T) {
	// Minimal libraries: same file name on both versions, different content
	// and a different clip ID. No hash match may exist, so the op is dead.
	offline := Library{
		SchemaVersion: "old", ProjectID: "p",
		Assets: []Asset{{ClipID: "a", Path: "media/shot.mov", MediaHash: "h1"}},
	}
	online := Library{
		SchemaVersion: "new", ProjectID: "p",
		Assets: []Asset{{ClipID: "b", Path: "media/shot.mov", MediaHash: "h2"}},
	}
	cl := ChangeList{
		SchemaVersion: "old", ProjectID: "p", BasedOn: "old",
		Ops: []Op{{Seq: 1, Action: "trim", OldClipID: "a",
			MediaHash: "h1", OldPath: "media/shot.mov"}},
	}
	rep := Resolve(offline, online, cl)
	if rep.DeadCount != 1 || rep.AppliedCount != 0 {
		t.Fatalf("counts = applied %d dead %d, want 0 applied / 1 dead",
			rep.AppliedCount, rep.DeadCount)
	}
}
