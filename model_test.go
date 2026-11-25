package clipfade

import (
	"errors"
	"sync"
	"testing"
	"time"
)

func addClipOK(t *testing.T, m *Model, c Clip) {
	t.Helper()
	if err := m.AddClip(c); err != nil {
		t.Fatalf("AddClip(%s): %v", c.ID, err)
	}
}

func sinePair(t *testing.T, m *Model, group GroupID, cut int64, wl, wr func(int64) float64) {
	t.Helper()
	left, right := adjacentPair(cut, group, wl, wr)
	addClipOK(t, m, left)
	addClipOK(t, m, right)
}

func linkedScene(t *testing.T) (*Model, FadeID, FadeReference) {
	t.Helper()
	m, err := NewModel(testSR)
	if err != nil {
		t.Fatal(err)
	}
	addClipOK(t, m, Clip{
		ID: "V1", Kind: KindVideo, Group: "av",
		Start: 0, VisibleLen: testCut,
	})
	sinePair(t, m, "av", testCut, sineAt(1000, 0.6), sineAt(1000, 0.6))

	fid, ref, err := m.AddCrossfade(CrossfadeConfig{
		Left: "A", Right: "B",
		Duration: 200 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("AddCrossfade: %v", err)
	}
	return m, fid, ref
}

func TestCrossfadeBuilds200msAutomation(t *testing.T) {
	m, fid, ref := linkedScene(t)

	if ref.Revision != m.CurrentRevision() {
		t.Fatal("initial reference revision mismatch")
	}
	curve, err := m.Resolve(ref)
	if err != nil {
		t.Fatal(err)
	}
	if curve.Plan.DurationSamples != testFadeD {
		t.Fatalf("duration = %d, want %d", curve.Plan.DurationSamples, testFadeD)
	}
	if curve.Plan.RegionStart != testCut-testHalfD {
		t.Fatalf("region start = %d", curve.Plan.RegionStart)
	}
	if curve.Plan.Phase.Relation != PhaseInPhase {
		t.Fatalf("phase = %s, want in_phase", curve.Plan.Phase.Relation)
	}
	if curve.Plan.Law != LawEqualGain {
		t.Fatalf("law = %s, want equal_gain for in-phase", curve.Plan.Law)
	}

	points := m.Points(fid)
	const wantPoints = 2 * (16 + 1)
	if len(points) != wantPoints {
		t.Fatalf("automation points = %d, want %d", len(points), wantPoints)
	}
	if points[0].Lane != LaneFadeOut || points[0].Value != 1 {
		t.Fatalf("first out point wrong: %+v", points[0])
	}
	if points[16].Lane != LaneFadeOut || absFloat(points[16].Value) > 1e-12 {
		t.Fatalf("last out point wrong: %+v", points[16])
	}
	if points[17].Lane != LaneFadeIn || absFloat(points[17].Value) > 1e-12 {
		t.Fatalf("first in point wrong: %+v", points[17])
	}
	if points[len(points)-1].Value != 1 {
		t.Fatalf("last in point wrong: %+v", points[len(points)-1])
	}

	rep, err := m.LastReport(fid)
	if err != nil {
		t.Fatal(err)
	}
	if !rep.Peak.Pass || rep.Peak.Peak > 0.601 {
		t.Fatalf("peak check unexpected: %+v", rep.Peak)
	}
	if rep.ZeroCrossings.Count == 0 {
		t.Fatal("expected zero crossings in rendered fade")
	}
	editZC := rep.EditZeroCrossing
	if !editZC.LeftFound || !editZC.RightFound {
		t.Fatalf("edit zero-crossing check incomplete: %+v", editZC)
	}
	if editZC.LeftDistance > 2 || editZC.RightDistance > 2 {
		t.Fatalf("1kHz cut should be within 2 samples of a crossing: %+v", editZC)
	}
}

// Regression test for the reported bug: moving the linked picture clip moves
// the cut; the old curve reference must die and rebuilt points must live at
// the new position. No old automation point may remain in the store.
func TestMovePictureClipInvalidatesAndRebuilds(t *testing.T) {
	m, fid, ref1 := linkedScene(t)
	rev1 := m.CurrentRevision()
	oldPoints := m.Points(fid)
	if len(oldPoints) == 0 {
		t.Fatal("expected initial automation points")
	}
	oldSamples := make(map[int64]bool, len(oldPoints))
	for _, p := range oldPoints {
		oldSamples[p.Sample] = true
	}

	const delta = int64(testSR) // move picture + linked audio by 1 s
	rev2, err := m.MoveClipBy("V1", delta)
	if err != nil {
		t.Fatal(err)
	}
	if rev2 != rev1+1 || m.CurrentRevision() != rev2 {
		t.Fatalf("revision after move = %d/%d, want %d", rev2, m.CurrentRevision(), rev1+1)
	}

	if _, err := m.Resolve(ref1); !errors.Is(err, ErrStaleReference) {
		t.Fatalf("old reference resolve error = %v, want ErrStaleReference", err)
	}

	// The store must contain zero points from the old generation.
	for storedFade, pts := range m.points {
		for _, p := range pts {
			if p.Revision != rev2 {
				t.Fatalf("fade %s retains old automation point rev %d: %+v", storedFade, p.Revision, p)
			}
		}
	}

	ref2, err := m.Reference(fid)
	if err != nil {
		t.Fatal(err)
	}
	if ref2.Revision != rev2 || ref2.CutSample != testCut+delta {
		t.Fatalf("new reference = %+v, want rev %d cut %d", ref2, rev2, testCut+delta)
	}
	curve2, err := m.Resolve(ref2)
	if err != nil {
		t.Fatalf("new reference must resolve: %v", err)
	}
	if curve2.Plan.RegionStart != testCut-testHalfD+delta {
		t.Fatalf("new region start = %d", curve2.Plan.RegionStart)
	}

	newPoints := m.Points(fid)
	if len(newPoints) != len(oldPoints) {
		t.Fatalf("point count changed: %d vs %d", len(newPoints), len(oldPoints))
	}
	for i, np := range newPoints {
		op := oldPoints[i]
		if np.Lane != op.Lane || np.Order != op.Order {
			t.Fatalf("point %d identity changed: old %+v new %+v", i, op, np)
		}
		if np.Sample != op.Sample+delta {
			t.Fatalf("point %d sample = %d, want shifted %d", i, np.Sample, op.Sample+delta)
		}
		if np.Value != op.Value {
			t.Fatalf("point %d value changed: %.6f vs %.6f", i, np.Value, op.Value)
		}
		if oldSamples[np.Sample] {
			t.Fatalf("rebuilt point %+v reuses an old absolute sample", np)
		}
	}

	// End-to-end render at the new location still reconstructs in-phase audio.
	mixed, rep, err := m.RenderCurrent(fid)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Plan.Phase.Relation != PhaseInPhase || !rep.Peak.Pass {
		t.Fatalf("post-move checks bad: phase=%s peak=%+v", rep.Plan.Phase.Relation, rep.Peak)
	}
	left := m.clips["A"]
	for i, got := range mixed {
		timeline := curve2.Plan.RegionStart + int64(i)
		want, ok := left.sampleAt(timeline)
		if !ok {
			t.Fatalf("left sample unavailable at %d", timeline)
		}
		if d := absFloat(got - want); d > 1e-12 {
			t.Fatalf("mixed[%d] = %.12f, want %.12f (err %.2e)", i, got, want, d)
		}
	}
}

func TestMoveClipToMovesLinkGroup(t *testing.T) {
	m, fid, oldRef := linkedScene(t)
	if m.SampleRate() != testSR {
		t.Fatalf("sample rate = %d", m.SampleRate())
	}
	rev, err := m.MoveClipTo("V1", testCut)
	if err != nil {
		t.Fatal(err)
	}
	if m.clips["V1"].Start != testCut || m.clips["A"].Start != testCut || m.clips["B"].Start != 2*testCut {
		t.Fatalf("group geometry wrong: V=%d A=%d B=%d",
			m.clips["V1"].Start, m.clips["A"].Start, m.clips["B"].Start)
	}
	if _, err := m.Resolve(oldRef); !errors.Is(err, ErrStaleReference) {
		t.Fatalf("old ref = %v, want stale", err)
	}
	ref, err := m.Reference(fid)
	if err != nil {
		t.Fatal(err)
	}
	curve, err := m.Resolve(ref)
	if err != nil {
		t.Fatal(err)
	}
	if curve.Plan.Cut != 2*testCut || ref.Revision != rev {
		t.Fatalf("cut=%d refRev=%d modelRev=%d", curve.Plan.Cut, ref.Revision, rev)
	}
}

func TestSetCutPointInvalidatesAllReferences(t *testing.T) {
	m, err := NewModel(testSR)
	if err != nil {
		t.Fatal(err)
	}
	const cut2 = 144000
	l1, r1 := adjacentPair(testCut, "g1", sineAt(440, 0.5), sineAt(440, 0.5))
	l1.ID, r1.ID = "L1", "R1"
	l2, r2 := adjacentPair(cut2, "g2", sineAt(660, 0.5), sineAt(660, 0.5))
	l2.ID, r2.ID = "L2", "R2"
	for _, c := range []Clip{l1, r1, l2, r2} {
		addClipOK(t, m, c)
	}
	id1, ref1, err := m.AddCrossfade(CrossfadeConfig{ID: "xf1", Left: "L1", Right: "R1", Duration: 200 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	id2, ref2, err := m.AddCrossfade(CrossfadeConfig{ID: "xf2", Left: "L2", Right: "R2", Duration: 200 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	revBefore := m.CurrentRevision()

	const newCut = testCut - 100
	newRef1, err := m.SetCutPoint(id1, newCut)
	if err != nil {
		t.Fatal(err)
	}

	for name, old := range map[string]FadeReference{"xf1": ref1, "xf2": ref2} {
		if _, err := m.Resolve(old); !errors.Is(err, ErrStaleReference) {
			t.Fatalf("%s old reference: %v, want ErrStaleReference", name, err)
		}
	}
	if m.CurrentRevision() != revBefore+1 {
		// revBefore was captured after both adds; SetCutPoint bumps once.
		t.Fatalf("revision = %d, want %d", m.CurrentRevision(), revBefore+1)
	}
	if newRef1.CutSample != newCut || newRef1.Revision != m.CurrentRevision() {
		t.Fatalf("bad new ref: %+v rev %d", newRef1, m.CurrentRevision())
	}
	if _, err := m.Resolve(newRef1); err != nil {
		t.Fatalf("new xf1 ref: %v", err)
	}

	ref2New, err := m.Reference(id2)
	if err != nil {
		t.Fatal(err)
	}
	curve2, err := m.Resolve(ref2New)
	if err != nil {
		t.Fatal(err)
	}
	if curve2.Plan.Cut != cut2 {
		t.Fatalf("unrelated fade cut moved to %d", curve2.Plan.Cut)
	}

	// Every stored point, including the unrelated fade's, belongs to the
	// newest generation; nothing old is retained.
	for fid, pts := range m.points {
		if len(pts) == 0 {
			t.Fatalf("fade %s has no rebuilt points", fid)
		}
		for _, p := range pts {
			if p.Revision != m.CurrentRevision() {
				t.Fatalf("stale point retained for %s: rev %d", fid, p.Revision)
			}
		}
	}
}

func TestFailedRebuildPurgesOldAutomation(t *testing.T) {
	m, err := NewModel(testSR)
	if err != nil {
		t.Fatal(err)
	}
	// Deliberately unlinked pair.
	left, right := adjacentPair(testCut, "", sineAt(440, 0.5), sineAt(440, 0.5))
	addClipOK(t, m, left)
	addClipOK(t, m, right)
	fid, oldRef, err := m.AddCrossfade(CrossfadeConfig{Left: "A", Right: "B", Duration: 200 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Points(fid)) == 0 {
		t.Fatal("expected points before break")
	}

	// Moving only A breaks adjacency; the curve can no longer be derived.
	if _, err := m.MoveClipBy("A", 100); err != nil {
		t.Fatal(err)
	}

	if pts := m.Points(fid); len(pts) != 0 {
		t.Fatalf("expected old points purged, got %d", len(pts))
	}
	if _, retained := m.points[fid]; retained {
		t.Fatal("point table still contains generation for unbuildable fade")
	}
	if _, err := m.Resolve(oldRef); !errors.Is(err, ErrStaleReference) {
		t.Fatalf("old ref err = %v, want stale", err)
	}
	freshRef, refErr := m.Reference(fid)
	if !errors.Is(refErr, ErrClipsNotAdjacent) {
		t.Fatalf("Reference err = %v, want ErrClipsNotAdjacent", refErr)
	}
	if _, err := m.Resolve(freshRef); !errors.Is(err, ErrClipsNotAdjacent) {
		t.Fatalf("Resolve fresh ref err = %v, want ErrClipsNotAdjacent", err)
	}
}

func TestSnapCutToZeroCrossing(t *testing.T) {
	m, err := NewModel(testSR)
	if err != nil {
		t.Fatal(err)
	}
	const offCut = int64(48011)
	left, right := adjacentPair(offCut, "", zcPattern, zcPattern)
	addClipOK(t, m, left)
	addClipOK(t, m, right)
	fid, _, err := m.AddCrossfade(CrossfadeConfig{Left: "A", Right: "B", Duration: 200 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}

	if _, _, _, err := m.SnapCutToZeroCrossing(fid, 0); !errors.Is(err, ErrNoZeroCrossing) {
		t.Fatalf("window-0 snap err = %v, want ErrNoZeroCrossing", err)
	}

	cut, moved, ref, err := m.SnapCutToZeroCrossing(fid, 64)
	if err != nil {
		t.Fatal(err)
	}
	if !moved || cut != 48012 {
		t.Fatalf("snap = (%d,moved=%v), want (48012,true)", cut, moved)
	}
	curve, err := m.Resolve(ref)
	if err != nil {
		t.Fatal(err)
	}
	if curve.Plan.Cut != 48012 {
		t.Fatalf("resolved cut = %d", curve.Plan.Cut)
	}
	if !curve.Report.EditZeroCrossing.OnCut {
		t.Fatalf("snapped cut not reported as on zero crossing: %+v", curve.Report.EditZeroCrossing)
	}
	if m.clips["A"].End() != 48012 || m.clips["B"].Start != 48012 {
		t.Fatal("clip geometry did not follow snapped cut")
	}

	// Snapping again is a no-op (already on a joint crossing).
	cut2, moved2, _, err := m.SnapCutToZeroCrossing(fid, 64)
	if err != nil || moved2 || cut2 != 48012 {
		t.Fatalf("second snap = (%d,%v,%v), want no move", cut2, moved2, err)
	}
}

func TestPhaseDrivenReports(t *testing.T) {
	t.Run("anti-phase warning and polarity correction", func(t *testing.T) {
		m, err := NewModel(testSR)
		if err != nil {
			t.Fatal(err)
		}
		l, r := adjacentPair(testCut, "", sineAt(330, 0.7), anti(sineAt(330, 0.7)))
		addClipOK(t, m, l)
		addClipOK(t, m, r)
		fid, _, err := m.AddCrossfade(CrossfadeConfig{Left: "A", Right: "B", Duration: 200 * time.Millisecond})
		if err != nil {
			t.Fatal(err)
		}
		rep, err := m.LastReport(fid)
		if err != nil {
			t.Fatal(err)
		}
		if rep.Plan.Phase.Relation != PhaseAntiPhase {
			t.Fatalf("relation = %s rho=%.2f", rep.Plan.Phase.Relation, rep.Plan.Phase.Correlation)
		}
		if len(rep.Plan.Warnings) == 0 || rep.CancellationDepthDB > -12 {
			t.Fatalf("expected cancellation warning/deep notch, depth=%.1f warnings=%v",
				rep.CancellationDepthDB, rep.Plan.Warnings)
		}

		fidFlip, _, err := m.AddCrossfade(CrossfadeConfig{
			ID: "xf-flip", Left: "A", Right: "B",
			Duration: 200 * time.Millisecond, Polarity: PolarityCorrectAntiPhase,
		})
		if err != nil {
			t.Fatal(err)
		}
		repFlip, err := m.LastReport(fidFlip)
		if err != nil {
			t.Fatal(err)
		}
		if !repFlip.Plan.FlipRight || repFlip.Plan.Law != LawEqualGain {
			t.Fatalf("expected polarity-corrected equal-gain plan: %+v", repFlip.Plan)
		}
		if repFlip.CancellationDepthDB < -3 {
			t.Fatalf("corrected fade still has notch depth %.1f dB", repFlip.CancellationDepthDB)
		}
	})

	t.Run("equal-power overshoot caught by peak check", func(t *testing.T) {
		m, err := NewModel(testSR)
		if err != nil {
			t.Fatal(err)
		}
		l, r := adjacentPair(testCut, "", sineAt(1000, 0.8), sineAt(1000, 0.8))
		addClipOK(t, m, l)
		addClipOK(t, m, r)
		fid, _, err := m.AddCrossfade(CrossfadeConfig{
			Left: "A", Right: "B", Duration: 200 * time.Millisecond, Law: LawEqualPower,
		})
		if err != nil {
			t.Fatal(err)
		}
		rep, err := m.LastReport(fid)
		if err != nil {
			t.Fatal(err)
		}
		if rep.Peak.Pass || rep.Peak.Peak <= 1.0 {
			t.Fatalf("expected peak failure from +3dB midpoint, peak=%.3f", rep.Peak.Peak)
		}
		if rep.Peak.Peak > 0.8*1.4142+0.02 {
			t.Fatalf("peak implausibly high: %.3f", rep.Peak.Peak)
		}
	})

	t.Run("uncorrelated chooses equal power", func(t *testing.T) {
		m, err := NewModel(testSR)
		if err != nil {
			t.Fatal(err)
		}
		l, r := adjacentPair(testCut, "", noiseWave(11), noiseWave(22))
		addClipOK(t, m, l)
		addClipOK(t, m, r)
		fid, _, err := m.AddCrossfade(CrossfadeConfig{Left: "A", Right: "B", Duration: 200 * time.Millisecond})
		if err != nil {
			t.Fatal(err)
		}
		rep, err := m.LastReport(fid)
		if err != nil {
			t.Fatal(err)
		}
		if rep.Plan.Phase.Relation != PhaseUncorrelated {
			t.Fatalf("relation = %s rho=%.2f", rep.Plan.Phase.Relation, rep.Plan.Phase.Correlation)
		}
		if rep.Plan.Law != LawEqualPower {
			t.Fatalf("law = %s, want equal_power", rep.Plan.Law)
		}
	})
}

func TestConcurrentReferenceResolve(t *testing.T) {
	m, fid, ref := linkedScene(t)
	var wg sync.WaitGroup

	for reader := 0; reader < 4; reader++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				r, err := m.Reference(fid)
				if err != nil {
					t.Errorf("Reference: %v", err)
					return
				}
				if _, err := m.Resolve(r); err != nil && !errors.Is(err, ErrStaleReference) {
					t.Errorf("Resolve: %v", err)
					return
				}
				_ = m.AllPoints()
			}
		}()
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			delta := int64(1024)
			if i%2 == 1 {
				delta = -1024
			}
			if _, err := m.MoveClipBy("V1", delta); err != nil {
				t.Errorf("MoveClipBy: %v", err)
				return
			}
		}
	}()

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("concurrent test timed out")
	}

	// Original reference is from an old revision regardless of interleaving.
	if _, err := m.Resolve(ref); !errors.Is(err, ErrStaleReference) {
		t.Fatalf("original ref err = %v, want stale", err)
	}
}

func absFloat(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}
