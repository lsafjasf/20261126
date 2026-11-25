package clipfade

import (
	"fmt"
	"hash/fnv"
	"io"
	"math"
	"sort"
	"sync"
	"time"
)

// Revision is the model-wide geometry generation. Every cut/clip mutation
// increments it; references carry the revision they were built from.
type Revision uint64

// Lane identifies which side of the crossfade an automation lane drives.
type Lane string

const (
	LaneFadeOut Lane = "fade_out"
	LaneFadeIn  Lane = "fade_in"
)

// AutomationPoint is a derived breakpoint on a fade gain lane. Points are
// never the source of truth: they are regenerated wholesale from the
// ClipFade and current clip geometry and replaced atomically.
type AutomationPoint struct {
	FadeID   FadeID
	Lane     Lane
	Order    int
	Sample   int64
	Value    float64
	Revision Revision
}

// FadeReference is an opaque, storable handle to a derived curve. Resolve
// rejects it as soon as the model revision or the fade fingerprint changes.
type FadeReference struct {
	FadeID      FadeID
	Revision    Revision
	CutSample   int64
	Fingerprint uint64
}

// FadeCurve is a valid generation of a fade: resolved plan, checks and the
// complete automation point set.
type FadeCurve struct {
	Ref    FadeReference
	Plan   FadePlan
	Report FadeReport
	Points []AutomationPoint
}

// CrossfadeConfig configures a new clip-level crossfade.
type CrossfadeConfig struct {
	ID              FadeID
	Left            ClipID
	Right           ClipID
	Duration        time.Duration
	DurationSamples int64
	Law             Law
	Anchor          Anchor
	Segments        int
	Ceiling         float64
	Polarity        PolarityMode
	Phase           PhaseConfig
}

// Model owns clips, clip-level fades and the current automation generation.
// It is safe for concurrent use.
type Model struct {
	mu         sync.RWMutex
	sampleRate int

	clips map[ClipID]*Clip
	fades map[FadeID]*ClipFade

	// points/curves/buildErrs only ever hold the CURRENT revision. Geometry
	// changes replace these maps, so no old automation points survive.
	points    map[FadeID][]AutomationPoint
	curves    map[FadeID]*FadeCurve
	buildErrs map[FadeID]error

	revision Revision
	fadeSeq  int
}

// NewModel creates an empty session operating at sampleRate Hz.
func NewModel(sampleRate int) (*Model, error) {
	if sampleRate <= 0 {
		return nil, ErrInvalidSampleRate
	}
	return &Model{
		sampleRate: sampleRate,
		clips:      make(map[ClipID]*Clip),
		fades:      make(map[FadeID]*ClipFade),
		points:     make(map[FadeID][]AutomationPoint),
		curves:     make(map[FadeID]*FadeCurve),
		buildErrs:  make(map[FadeID]error),
	}, nil
}

// SampleRate returns the model sample rate.
func (m *Model) SampleRate() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.sampleRate
}

// CurrentRevision returns the model-wide geometry revision.
func (m *Model) CurrentRevision() Revision {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.revision
}

// AddClip registers a clip. The sample buffer is copied so later caller
// mutation cannot desynchronize the fingerprint.
func (m *Model) AddClip(c Clip) error {
	if c.ID == "" {
		return fmt.Errorf("%w: empty clip id", ErrClipNotFound)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.clips[c.ID]; exists {
		return fmt.Errorf("%w: %s", ErrClipExists, c.ID)
	}
	stored := c
	if c.Samples != nil {
		stored.Samples = append([]float64(nil), c.Samples...)
		stored.contentHash = hashSamples(stored.Samples)
	}
	if stored.Kind == "" {
		stored.Kind = KindAudio
	}
	m.clips[c.ID] = &stored
	return nil
}

// AddCrossfade binds a crossfade to two adjacent clips and builds the first
// automation generation.
func (m *Model) AddCrossfade(cfg CrossfadeConfig) (FadeID, FadeReference, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.clips[cfg.Left]; !ok {
		return "", FadeReference{}, fmt.Errorf("%w: %s", ErrClipNotFound, cfg.Left)
	}
	if _, ok := m.clips[cfg.Right]; !ok {
		return "", FadeReference{}, fmt.Errorf("%w: %s", ErrClipNotFound, cfg.Right)
	}

	m.fadeSeq++
	id := cfg.ID
	if id == "" {
		id = FadeID(fmt.Sprintf("fade-%d", m.fadeSeq))
	}
	if _, exists := m.fades[id]; exists {
		return "", FadeReference{}, fmt.Errorf("%w: fade id %s", ErrClipExists, id)
	}

	f := &ClipFade{
		ID:              id,
		Left:            cfg.Left,
		Right:           cfg.Right,
		Law:             cfg.Law,
		Anchor:          cfg.Anchor,
		Duration:        cfg.Duration,
		DurationSamples: cfg.DurationSamples,
		Segments:        cfg.Segments,
		Ceiling:         cfg.Ceiling,
		Polarity:        cfg.Polarity,
		Phase:           cfg.Phase,
	}
	f.defaults(m.sampleRate)
	m.fades[id] = f

	m.revision++
	m.recomputeAllLocked()
	ref, err := m.referenceLocked(id)
	return id, ref, err
}

// MoveClipBy moves one clip by delta samples. When the clip belongs to a
// link group (for example a picture clip linked to audio), every clip in the
// group moves by the same delta. All fade references are invalidated and all
// automation points are rebuilt.
func (m *Model) MoveClipBy(id ClipID, delta int64) (Revision, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.moveByLocked(id, delta); err != nil {
		return m.revision, err
	}
	return m.revision, nil
}

// MoveClipTo places a clip (and its link group) at an absolute Start sample.
func (m *Model) MoveClipTo(id ClipID, start int64) (Revision, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.clips[id]
	if !ok {
		return m.revision, fmt.Errorf("%w: %s", ErrClipNotFound, id)
	}
	delta := start - c.Start
	if err := m.moveByLocked(id, delta); err != nil {
		return m.revision, err
	}
	return m.revision, nil
}

// SetCutPoint changes the shared cut coordinate of a bound pair. The left
// clip is re-trimmed to the new cut and the right clip starts there. This is
// the explicit "change cut point" path: every outstanding reference becomes
// stale and no old automation point is retained.
func (m *Model) SetCutPoint(fadeID FadeID, cut int64) (FadeReference, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.setCutPointLocked(fadeID, cut)
}

// SnapCutToZeroCrossing moves the cut to the nearest joint zero-crossing
// edge within maxWindowSamples. It goes through the same invalidation path
// as any other cut edit. moved is false when the cut already sits on the
// nearest joint crossing (no revision change).
func (m *Model) SnapCutToZeroCrossing(fadeID FadeID, maxWindowSamples int64) (cut int64, moved bool, ref FadeReference, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	f, ok := m.fades[fadeID]
	if !ok {
		return 0, false, FadeReference{}, fmt.Errorf("%w: %s", ErrFadeNotFound, fadeID)
	}
	left := m.clips[f.Left]
	right := m.clips[f.Right]
	current := left.End()
	target, found := FindJointZeroCrossing(left, right, current, maxWindowSamples)
	if !found {
		return current, false, FadeReference{}, fmt.Errorf("%w: fade %s around %d",
			ErrNoZeroCrossing, fadeID, current)
	}
	if target == current {
		r, rerr := m.referenceLocked(fadeID)
		return current, false, r, rerr
	}
	r, serr := m.setCutPointLocked(fadeID, target)
	return target, true, r, serr
}

// Reference returns the current valid reference for a fade.
func (m *Model) Reference(fadeID FadeID) (FadeReference, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.referenceLocked(fadeID)
}

// Resolve validates a reference against the current revision and
// fingerprint. Any geometry change makes it fail with ErrStaleReference;
// the caller must drop the handle and request a fresh Reference.
func (m *Model) Resolve(ref FadeReference) (FadeCurve, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	f, ok := m.fades[ref.FadeID]
	if !ok {
		return FadeCurve{}, fmt.Errorf("%w: %s", ErrFadeNotFound, ref.FadeID)
	}
	currentFP := fingerprintOf(f, m.clips[f.Left], m.clips[f.Right])
	if ref.Revision != m.revision || ref.Fingerprint != currentFP {
		return FadeCurve{}, fmt.Errorf("%w: fade %s was rev %d/fp %x, model is rev %d/fp %x",
			ErrStaleReference, ref.FadeID, ref.Revision, ref.Fingerprint, m.revision, currentFP)
	}
	if buildErr := m.buildErrs[ref.FadeID]; buildErr != nil {
		return FadeCurve{}, buildErr
	}
	curve, ok := m.curves[ref.FadeID]
	if !ok {
		return FadeCurve{}, fmt.Errorf("%w: %s", ErrNothingToRebuild, ref.FadeID)
	}
	return cloneCurve(curve), nil
}

// Points returns a copy of the current automation points for one fade.
func (m *Model) Points(fadeID FadeID) []AutomationPoint {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return clonePoints(m.points[fadeID])
}

// AllPoints returns every current-generation automation point, sorted.
func (m *Model) AllPoints() []AutomationPoint {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var all []AutomationPoint
	for _, pts := range m.points {
		all = append(all, pts...)
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].FadeID != all[j].FadeID {
			return all[i].FadeID < all[j].FadeID
		}
		if all[i].Lane != all[j].Lane {
			return all[i].Lane < all[j].Lane
		}
		return all[i].Order < all[j].Order
	})
	return all
}

// LastReport returns the checks from the latest rebuild for a fade.
func (m *Model) LastReport(fadeID FadeID) (FadeReport, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if err := m.buildErrs[fadeID]; err != nil {
		return FadeReport{}, err
	}
	c, ok := m.curves[fadeID]
	if !ok {
		return FadeReport{}, fmt.Errorf("%w: %s", ErrFadeNotFound, fadeID)
	}
	return c.Report, nil
}

// RenderCurrent re-renders sample-accurate audio for the current geometry.
func (m *Model) RenderCurrent(fadeID FadeID) ([]float64, FadeReport, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	f, ok := m.fades[fadeID]
	if !ok {
		return nil, FadeReport{}, fmt.Errorf("%w: %s", ErrFadeNotFound, fadeID)
	}
	left, right := m.clips[f.Left], m.clips[f.Right]
	return RenderFade(f, left, right, m.sampleRate)
}

// ---------------------------------------------------------------------------
// internals
// ---------------------------------------------------------------------------

func (m *Model) moveTargetsLocked(id ClipID) ([]ClipID, error) {
	c, ok := m.clips[id]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrClipNotFound, id)
	}
	if c.Group == "" {
		return []ClipID{id}, nil
	}
	var ids []ClipID
	for cid, cand := range m.clips {
		if cand.Group == c.Group {
			ids = append(ids, cid)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids, nil
}

// moveByLocked applies a delta to one clip or its whole link group, then
// invalidates and rebuilds every fade. Callers hold m.mu.
func (m *Model) moveByLocked(id ClipID, delta int64) error {
	targets, err := m.moveTargetsLocked(id)
	if err != nil {
		return err
	}
	for _, cid := range targets {
		m.clips[cid].Start += delta
	}
	m.revision++
	m.recomputeAllLocked()
	return nil
}

// setCutPointLocked is the lock-held cut edit implementation.
func (m *Model) setCutPointLocked(fadeID FadeID, cut int64) (FadeReference, error) {
	f, ok := m.fades[fadeID]
	if !ok {
		return FadeReference{}, fmt.Errorf("%w: %s", ErrFadeNotFound, fadeID)
	}
	left, ok := m.clips[f.Left]
	if !ok {
		return FadeReference{}, fmt.Errorf("%w: %s", ErrClipNotFound, f.Left)
	}
	right, ok := m.clips[f.Right]
	if !ok {
		return FadeReference{}, fmt.Errorf("%w: %s", ErrClipNotFound, f.Right)
	}
	if cut <= left.Start {
		return FadeReference{}, fmt.Errorf("%w: cut %d must be after %s start %d",
			ErrInvalidDuration, cut, left.ID, left.Start)
	}

	// Roll the edit boundary while keeping the laid-out audio mapping
	// invariant on the right side: shifting Start and InOffset by the same
	// delta leaves sourceIndex(t) unchanged. The left mapping is unchanged
	// because only its used length is re-trimmed.
	right.InOffset += cut - right.Start
	right.Start = cut
	left.VisibleLen = cut - left.Start

	m.revision++
	m.recomputeAllLocked()
	return m.referenceLocked(fadeID)
}

// recomputeAllLocked drops every stored automation generation and rebuilds
// every fade at the current revision. Callers must hold m.mu (write).
func (m *Model) recomputeAllLocked() {
	m.points = make(map[FadeID][]AutomationPoint)
	m.curves = make(map[FadeID]*FadeCurve)
	m.buildErrs = make(map[FadeID]error)

	ids := make([]FadeID, 0, len(m.fades))
	for id := range m.fades {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, id := range ids {
		m.recomputeLocked(id)
	}
}

func (m *Model) recomputeLocked(id FadeID) {
	f := m.fades[id]
	left, right := m.clips[f.Left], m.clips[f.Right]

	_, report, err := RenderFade(f, left, right, m.sampleRate)
	if err != nil {
		m.buildErrs[id] = err
		return
	}

	fp := fingerprintOf(f, left, right)
	ref := FadeReference{
		FadeID:      id,
		Revision:    m.revision,
		CutSample:   report.Plan.Cut,
		Fingerprint: fp,
	}
	pts := automationPoints(&report.Plan, m.revision, f.Segments)
	m.points[id] = pts
	m.curves[id] = &FadeCurve{Ref: ref, Plan: report.Plan, Report: report, Points: pts}
}

func (m *Model) referenceLocked(id FadeID) (FadeReference, error) {
	if c, ok := m.curves[id]; ok {
		return c.Ref, nil
	}
	f, ok := m.fades[id]
	if !ok {
		return FadeReference{}, fmt.Errorf("%w: %s", ErrFadeNotFound, id)
	}
	ref := FadeReference{
		FadeID:      id,
		Revision:    m.revision,
		Fingerprint: fingerprintOf(f, m.clips[f.Left], m.clips[f.Right]),
	}
	if left := m.clips[f.Left]; left != nil && m.clips[f.Right] != nil {
		ref.CutSample = left.End()
	}
	if err := m.buildErrs[id]; err != nil {
		return ref, err
	}
	return ref, nil
}

func automationPoints(plan *FadePlan, rev Revision, segments int) []AutomationPoint {
	if segments <= 0 {
		segments = 16
	}
	points := make([]AutomationPoint, 0, 2*(segments+1))
	laneSpecs := []struct {
		lane Lane
		in   bool
	}{{LaneFadeOut, false}, {LaneFadeIn, true}}

	for _, spec := range laneSpecs {
		for k := 0; k <= segments; k++ {
			x := float64(k) / float64(segments)
			out, in := Gains(plan.Law, x)
			value := out
			if spec.in {
				value = in
			}
			sample := plan.RegionStart + int64(math.Round(x*float64(plan.DurationSamples-1)))
			points = append(points, AutomationPoint{
				FadeID:   plan.FadeID,
				Lane:     spec.lane,
				Order:    k,
				Sample:   sample,
				Value:    value,
				Revision: rev,
			})
		}
	}
	return points
}

func fingerprintOf(f *ClipFade, left, right *Clip) uint64 {
	h := fnv.New64a()
	fmt.Fprintf(h, "fade=%s|d=%d|law=%s|anchor=%s|pol=%d|seg=%d|ceil=%d",
		f.ID, f.DurationSamples, f.Law, f.Anchor, f.Polarity, f.Segments, math.Float64bits(f.Ceiling))
	writeClipFingerprint(h, "L", left)
	writeClipFingerprint(h, "R", right)
	return h.Sum64()
}

func writeClipFingerprint(h io.Writer, side string, c *Clip) {
	if c == nil {
		fmt.Fprintf(h, "|%s=nil", side)
		return
	}
	fmt.Fprintf(h, "|%s=%s,%d,%d,%d,%d", side, c.ID, c.Start, c.InOffset, c.VisibleLen, c.contentHash)
}

func clonePoints(src []AutomationPoint) []AutomationPoint {
	if src == nil {
		return nil
	}
	return append([]AutomationPoint(nil), src...)
}

func cloneCurve(c *FadeCurve) FadeCurve {
	out := *c
	out.Points = clonePoints(c.Points)
	out.Plan.Warnings = append([]string(nil), c.Plan.Warnings...)
	out.Report.Plan.Warnings = append([]string(nil), c.Report.Plan.Warnings...)
	out.Report.ZeroCrossings.Indices = append([]int(nil), c.Report.ZeroCrossings.Indices...)
	return out
}
