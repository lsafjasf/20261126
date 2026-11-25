package timeline

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"testing"
)

var propertyRates = []Rate{
	{Num: 30000, Den: 1001},
	{Num: 60000, Den: 1001},
	{Num: 24000, Den: 1001},
	{Num: 25, Den: 1},
	{Num: 24, Den: 1},
	{Num: 30, Den: 1},
	{Num: 50, Den: 1},
}

var propertySpeeds = []Rate{
	{Num: 1, Den: 1},
	{Num: 1, Den: 2},
	{Num: 2, Den: 1},
	{Num: 3, Den: 2},
	{Num: 2, Den: 3},
	{Num: 1000, Den: 1001},
	{Num: 1001, Den: 1000},
	{Num: 24, Den: 25},
	{Num: 25, Den: 24},
}

// legacyFrameToTimecode models the original exporter: its special case for an
// exact ten-minute boundary adds one frame before formatting. The timeline
// duration is written as a raw frame count, so the total stays correct while
// each ten-minute cut moves by one frame.
func legacyFrameToTimecode(rate Rate, frame int64) (string, error) {
	group := rate.framesPer10Minutes()
	if frame > 0 && frame%group == 0 {
		frame++
	}
	return FrameToTimecode(rate, frame)
}

func legacyMarshal(t Timeline) ([]byte, error) {
	return marshalWith(t, legacyFrameToTimecode)
}

func compareTimelines(want, got Timeline) error {
	if want.Rate != got.Rate {
		return fmt.Errorf("rate = %s, want %s", got.Rate, want.Rate)
	}
	if want.Duration != got.Duration {
		return fmt.Errorf("duration = %d, want %d", got.Duration, want.Duration)
	}
	if len(got.Tracks) != len(want.Tracks) {
		return fmt.Errorf("track count = %d, want %d", len(got.Tracks), len(want.Tracks))
	}

	for ti := range want.Tracks {
		wantTrack := want.Tracks[ti]
		gotTrack := got.Tracks[ti]
		if gotTrack.ID != wantTrack.ID {
			return fmt.Errorf("track %d id = %q, want %q", ti, gotTrack.ID, wantTrack.ID)
		}
		if len(gotTrack.Clips) != len(wantTrack.Clips) {
			return fmt.Errorf(
				"track %q clip count = %d, want %d",
				wantTrack.ID, len(gotTrack.Clips), len(wantTrack.Clips),
			)
		}
		for ci := range wantTrack.Clips {
			if err := compareClip(wantTrack.ID, ci, wantTrack.Clips[ci], gotTrack.Clips[ci]); err != nil {
				return err
			}
		}
		for ci := 1; ci < len(wantTrack.Clips); ci++ {
			wantGap := wantTrack.Clips[ci].SeqIn - wantTrack.Clips[ci-1].SeqOut
			gotGap := gotTrack.Clips[ci].SeqIn - gotTrack.Clips[ci-1].SeqOut
			if gotGap != wantGap {
				return fmt.Errorf(
					"track %q gap before clip %d = %d, want %d",
					wantTrack.ID, ci, gotGap, wantGap,
				)
			}
		}
		wantTail := want.Duration - trackEnd(wantTrack)
		gotTail := got.Duration - trackEnd(gotTrack)
		if gotTail != wantTail {
			return fmt.Errorf(
				"track %q trailing gap = %d, want %d",
				wantTrack.ID, gotTail, wantTail,
			)
		}
	}
	return nil
}

func compareClip(trackID string, index int, want, got Clip) error {
	prefix := fmt.Sprintf("track %q clip %d", trackID, index)
	if got.SeqIn != want.SeqIn {
		return fmt.Errorf("%s sequence in = %d, want %d", prefix, got.SeqIn, want.SeqIn)
	}
	if got.SeqOut != want.SeqOut {
		return fmt.Errorf("%s sequence out = %d, want %d", prefix, got.SeqOut, want.SeqOut)
	}
	if got.SrcIn != want.SrcIn {
		return fmt.Errorf("%s source in = %d, want %d", prefix, got.SrcIn, want.SrcIn)
	}
	if got.SrcOut != want.SrcOut {
		return fmt.Errorf("%s source out = %d, want %d", prefix, got.SrcOut, want.SrcOut)
	}
	if got.Speed != want.Speed {
		return fmt.Errorf("%s speed = %s, want %s", prefix, got.Speed, want.Speed)
	}

	for _, clip := range []struct {
		name string
		clip Clip
	}{{"want", want}, {"got", got}} {
		if src, ok := clip.clip.SourceAt(clip.clip.SeqIn); !ok || src != clip.clip.SrcIn {
			return fmt.Errorf("%s %s source at sequence in = %d, want %d", prefix, clip.name, src, clip.clip.SrcIn)
		}
		if src, ok := clip.clip.SourceAt(clip.clip.SeqOut); !ok || src != clip.clip.SrcOut {
			return fmt.Errorf("%s %s source at sequence out = %d, want %d", prefix, clip.name, src, clip.clip.SrcOut)
		}
		if seq, ok := clip.clip.SequenceAt(clip.clip.SrcIn); !ok || seq != clip.clip.SeqIn {
			return fmt.Errorf("%s %s sequence at source in = %d, want %d", prefix, clip.name, seq, clip.clip.SeqIn)
		}
		if seq, ok := clip.clip.SequenceAt(clip.clip.SrcOut); !ok || seq != clip.clip.SeqOut {
			return fmt.Errorf("%s %s sequence at source out = %d, want %d", prefix, clip.name, seq, clip.clip.SeqOut)
		}
	}

	seqMid := want.SeqIn + want.SequenceDuration()/2
	wantSrc, wantOK := want.SourceAt(seqMid)
	gotSrc, gotOK := got.SourceAt(seqMid)
	if wantOK != gotOK || gotSrc != wantSrc {
		return fmt.Errorf("%s source at sequence frame %d = %d, want %d", prefix, seqMid, gotSrc, wantSrc)
	}
	srcMid := want.SrcIn + want.SourceDuration()/2
	wantSeq, wantOK := want.SequenceAt(srcMid)
	gotSeq, gotOK := got.SequenceAt(srcMid)
	if wantOK != gotOK || gotSeq != wantSeq {
		return fmt.Errorf("%s sequence at source frame %d = %d, want %d", prefix, srcMid, gotSeq, wantSeq)
	}
	return nil
}

func trackEnd(track Track) int64 {
	var end int64
	for _, clip := range track.Clips {
		if clip.SeqOut > end {
			end = clip.SeqOut
		}
	}
	return end
}

func timelineEnd(t Timeline) int64 {
	var end int64
	for _, track := range t.Tracks {
		if trackEnd(track) > end {
			end = trackEnd(track)
		}
	}
	return end
}

func timelineJSON(t Timeline) string {
	data, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return fmt.Sprintf("<json error: %v>", err)
	}
	return string(data)
}

func randomTimeline(rng *rand.Rand, rate Rate) Timeline {
	fps := rate.nominalFPS()
	hotAt := int64(1+rng.Intn(3)) * rate.framesPer10Minutes()
	timeline := Timeline{Rate: rate}

	trackCount := 1 + rng.Intn(3)
	for ti := 0; ti < trackCount; ti++ {
		track := Track{ID: fmt.Sprintf("V%d", ti+1)}
		clipCount := rng.Intn(4)
		cursor := int64(0)
		if rng.Intn(2) == 0 {
			cursor = rng.Int63n(fps + 1)
		}
		for ci := 0; ci < clipCount; ci++ {
			speed := propertySpeeds[rng.Intn(len(propertySpeeds))]
			den := speed.Den / gcd(speed.Num, speed.Den)
			seqLen := den * int64(1+rng.Intn(5))
			seqIn := cursor
			if rng.Intn(5) == 0 && cursor > fps/2 {
				seqIn -= rng.Int63n(fps/2 + 1)
				if seqIn < 0 {
					seqIn = 0
				}
			}
			seqOut := seqIn + seqLen
			srcIn := rng.Int63n(fps*600 + 1)
			srcLen := mulDiv(seqLen, speed.Num, speed.Den)
			track.Clips = append(track.Clips, Clip{
				SeqIn:  seqIn,
				SeqOut: seqOut,
				SrcIn:  srcIn,
				SrcOut: srcIn + srcLen,
				Speed:  speed,
			})
			cursor = seqOut
			if rng.Intn(2) == 0 {
				cursor += rng.Int63n(fps/2 + 1)
			}
		}
		timeline.Tracks = append(timeline.Tracks, track)
	}

	// Every generated timeline contains a cut exactly at a ten-minute boundary.
	boundaryLen := int64(1 + rng.Intn(int(fps)))
	boundaryIn := hotAt - boundaryLen
	if boundaryIn < 0 {
		boundaryIn = 0
		boundaryLen = hotAt
	}
	boundarySrcIn := rng.Int63n(fps*600 + 1)
	timeline.Tracks[0].Clips = append(timeline.Tracks[0].Clips, Clip{
		SeqIn:  boundaryIn,
		SeqOut: hotAt,
		SrcIn:  boundarySrcIn,
		SrcOut: boundarySrcIn + boundaryLen,
		Speed:  Rate{Num: 1, Den: 1},
	})

	timeline.Sort()
	timeline.Duration = timelineEnd(timeline) + 1 + rng.Int63n(fps+1)
	return timeline
}

func legacyCounterexample(t Timeline) bool {
	if err := t.Validate(); err != nil {
		return false
	}
	data, err := legacyMarshal(t)
	if err != nil {
		return false
	}
	got, err := Unmarshal(data)
	if err != nil {
		return false
	}
	if got.Duration != t.Duration {
		return false
	}
	return compareTimelines(t, got) != nil
}

// shrinkTimeline repeatedly applies structure-preserving reductions while the
// supplied predicate still reports the same observable failure.
func shrinkTimeline(t Timeline, fails func(Timeline) bool) Timeline {
	if !fails(t) {
		return t
	}
	for pass := 0; pass < 1000; pass++ {
		next, changed := shrinkStep(t, fails)
		if !changed {
			return t
		}
		t = next
	}
	return t
}

func shrinkStep(t Timeline, fails func(Timeline) bool) (Timeline, bool) {
	for ti := range t.Tracks {
		candidate := t.Clone()
		candidate.Tracks = append(candidate.Tracks[:ti], candidate.Tracks[ti+1:]...)
		if len(candidate.Tracks) == 0 {
			continue
		}
		candidate = withMinimalDuration(candidate)
		if fails(candidate) {
			return candidate, true
		}
	}

	for ti := range t.Tracks {
		for ci := range t.Tracks[ti].Clips {
			candidate := t.Clone()
			clips := candidate.Tracks[ti].Clips
			candidate.Tracks[ti].Clips = append(clips[:ci:ci], clips[ci+1:]...)
			candidate = withMinimalDuration(candidate)
			if fails(candidate) {
				return candidate, true
			}
		}
	}

	if len(t.Tracks) > 1 {
		for ti := range t.Tracks {
			if len(t.Tracks[ti].Clips) != 0 {
				continue
			}
			candidate := t.Clone()
			candidate.Tracks = append(candidate.Tracks[:ti], candidate.Tracks[ti+1:]...)
			candidate = withMinimalDuration(candidate)
			if fails(candidate) {
				return candidate, true
			}
		}
	}

	for ti := range t.Tracks {
		for ci := range t.Tracks[ti].Clips {
			clip := t.Tracks[ti].Clips[ci]
			if clip.Speed == (Rate{Num: 1, Den: 1}) {
				continue
			}
			candidate := t.Clone()
			shaped := &candidate.Tracks[ti].Clips[ci]
			shaped.Speed = Rate{Num: 1, Den: 1}
			shaped.SrcOut = shaped.SrcIn + shaped.SequenceDuration()
			candidate = withMinimalDuration(candidate)
			if fails(candidate) {
				return candidate, true
			}
		}
	}

	for ti := range t.Tracks {
		for ci := range t.Tracks[ti].Clips {
			clip := t.Tracks[ti].Clips[ci]
			if clip.Speed != (Rate{Num: 1, Den: 1}) {
				continue
			}
			if clip.SrcIn != clip.SeqIn {
				candidate := t.Clone()
				shaped := &candidate.Tracks[ti].Clips[ci]
				shaped.SrcIn = shaped.SeqIn
				shaped.SrcOut = shaped.SeqIn + shaped.SequenceDuration()
				candidate = withMinimalDuration(candidate)
				if fails(candidate) {
					return candidate, true
				}
			}
			if clip.SrcIn != 0 {
				candidate := t.Clone()
				shaped := &candidate.Tracks[ti].Clips[ci]
				shaped.SrcIn = 0
				shaped.SrcOut = shaped.SequenceDuration()
				candidate = withMinimalDuration(candidate)
				if fails(candidate) {
					return candidate, true
				}
			}
		}
	}

	if min := minimumSequenceIn(t); min > 0 {
		candidate := t.Clone()
		for ti := range candidate.Tracks {
			for ci := range candidate.Tracks[ti].Clips {
				candidate.Tracks[ti].Clips[ci].SeqIn -= min
				candidate.Tracks[ti].Clips[ci].SeqOut -= min
			}
		}
		candidate = withMinimalDuration(candidate)
		if fails(candidate) {
			return candidate, true
		}
	}

	if t.Duration != timelineEnd(t)+1 {
		candidate := withMinimalDuration(t.Clone())
		if fails(candidate) {
			return candidate, true
		}
	}

	for ti := range t.Tracks {
		for ci := range t.Tracks[ti].Clips {
			clip := t.Tracks[ti].Clips[ci]
			if clip.Speed != (Rate{Num: 1, Den: 1}) || clip.SequenceDuration() <= 1 {
				continue
			}
			oldLen := clip.SequenceDuration()
			best := oldLen
			lo, hi := int64(1), oldLen-1
			for lo <= hi {
				mid := lo + (hi-lo)/2
				candidate := t.Clone()
				shaped := &candidate.Tracks[ti].Clips[ci]
				shaped.SeqOut = shaped.SeqIn + mid
				shaped.SrcOut = shaped.SrcIn + mid
				candidate = withMinimalDuration(candidate)
				if fails(candidate) {
					best = mid
					hi = mid - 1
				} else {
					lo = mid + 1
				}
			}
			if best != oldLen {
				candidate := t.Clone()
				shaped := &candidate.Tracks[ti].Clips[ci]
				shaped.SeqOut = shaped.SeqIn + best
				shaped.SrcOut = shaped.SrcIn + best
				candidate = withMinimalDuration(candidate)
				if fails(candidate) {
					return candidate, true
				}
			}
		}
	}

	for ti := range t.Tracks {
		for ci := range t.Tracks[ti].Clips {
			clip := t.Tracks[ti].Clips[ci]
			if clip.SequenceDuration() <= 1 {
				continue
			}
			candidate := t.Clone()
			shaped := &candidate.Tracks[ti].Clips[ci]
			shaped.SeqIn = shaped.SeqOut - 1
			shaped.SrcOut = shaped.SrcIn + 1
			candidate = withMinimalDuration(candidate)
			if fails(candidate) {
				return candidate, true
			}
		}
	}

	if len(t.Tracks) == 1 && t.Tracks[0].ID != "V1" {
		candidate := t.Clone()
		candidate.Tracks[0].ID = "V1"
		candidate = withMinimalDuration(candidate)
		if fails(candidate) {
			return candidate, true
		}
	}
	return t, false
}

func withMinimalDuration(t Timeline) Timeline {
	t.Sort()
	t.Duration = timelineEnd(t) + 1
	return t
}

func minimumSequenceIn(t Timeline) int64 {
	min := int64(-1)
	for _, track := range t.Tracks {
		for _, clip := range track.Clips {
			if min < 0 || clip.SeqIn < min {
				min = clip.SeqIn
			}
		}
	}
	return min
}

func TestRoundTripProperty(t *testing.T) {
	rng := rand.New(rand.NewSource(0x20261126))
	const cases = 400
	for i := 0; i < cases; i++ {
		rate := propertyRates[rng.Intn(len(propertyRates))]
		want := randomTimeline(rng, rate)
		if err := want.Validate(); err != nil {
			t.Fatalf("case %d generated invalid timeline: %v\n%s", i, err, timelineJSON(want))
		}
		data, err := Marshal(want)
		if err != nil {
			t.Fatalf("case %d Marshal: %v\n%s", i, err, timelineJSON(want))
		}
		got, err := Unmarshal(data)
		if err != nil {
			t.Fatalf("case %d Unmarshal: %v\n%s\nencoded:\n%s", i, err, timelineJSON(want), data)
		}
		if err := compareTimelines(want, got); err != nil {
			t.Fatalf("case %d round trip mismatch: %v\ninput:\n%s\nencoded:\n%s", i, err, timelineJSON(want), data)
		}
	}
}

func TestGeneratorCoverage(t *testing.T) {
	rng := rand.New(rand.NewSource(0xc0ffee))
	var gaps, overlaps, variableSpeed, multiTrack, tenMinuteCuts int
	for i := 0; i < 120; i++ {
		rate := propertyRates[rng.Intn(len(propertyRates))]
		timeline := randomTimeline(rng, rate)
		if timeline.Duration > timelineEnd(timeline) {
			gaps++
		}
		if len(timeline.Tracks) > 1 {
			multiTrack++
		}
		boundary := rate.framesPer10Minutes()
		for _, track := range timeline.Tracks {
			for ci, clip := range track.Clips {
				if clip.Speed != (Rate{Num: 1, Den: 1}) {
					variableSpeed++
				}
				if clip.SeqOut%boundary == 0 {
					tenMinuteCuts++
				}
				if ci == 0 {
					continue
				}
				prev := track.Clips[ci-1]
				switch {
				case clip.SeqIn > prev.SeqOut:
					gaps++
				case clip.SeqIn < prev.SeqOut:
					overlaps++
				}
			}
		}
	}
	if gaps == 0 || overlaps == 0 || variableSpeed == 0 || multiTrack == 0 || tenMinuteCuts == 0 {
		t.Fatalf(
			"generator coverage missing: gaps=%d overlaps=%d variableSpeed=%d multiTrack=%d tenMinuteCuts=%d",
			gaps, overlaps, variableSpeed, multiTrack, tenMinuteCuts,
		)
	}
}

func TestLegacyEveryTenMinutesAddsOneFrame(t *testing.T) {
	rate := mustRate(t, 30000, 1001)
	group := rate.framesPer10Minutes()
	want := Timeline{
		Rate:     rate,
		Duration: group + 1,
		Tracks: []Track{{
			ID: "V1",
			Clips: []Clip{{
				SeqIn:  group - 1,
				SeqOut: group,
				SrcIn:  group - 1,
				SrcOut: group,
				Speed:  Rate{Num: 1, Den: 1},
			}},
		}},
	}
	if err := want.Validate(); err != nil {
		t.Fatalf("counterexample is invalid: %v", err)
	}

	legacyData, err := legacyMarshal(want)
	if err != nil {
		t.Fatalf("legacy marshal: %v", err)
	}
	if !bytes.Contains(legacyData, []byte(`"seq_out": "00:10:00;01"`)) {
		t.Fatalf("legacy exporter did not reproduce the shifted cut:\n%s", legacyData)
	}
	legacyGot, err := Unmarshal(legacyData)
	if err != nil {
		t.Fatalf("legacy round trip failed before mismatch: %v", err)
	}
	if legacyGot.Duration != want.Duration {
		t.Fatalf("legacy total duration = %d, want %d", legacyGot.Duration, want.Duration)
	}
	if got := legacyGot.Tracks[0].Clips[0].SeqOut; got != group+1 {
		t.Fatalf("legacy sequence out = %d, want %d", got, group+1)
	}
	if err := compareTimelines(want, legacyGot); err == nil {
		t.Fatal("legacy round trip unexpectedly matched every clip boundary")
	}

	fixedData, err := Marshal(want)
	if err != nil {
		t.Fatalf("fixed marshal: %v", err)
	}
	if !bytes.Contains(fixedData, []byte(`"seq_out": "00:10:00;00"`)) {
		t.Fatalf("fixed exporter changed the cut:\n%s", fixedData)
	}
	fixedGot, err := Unmarshal(fixedData)
	if err != nil {
		t.Fatalf("fixed round trip: %v", err)
	}
	if err := compareTimelines(want, fixedGot); err != nil {
		t.Fatalf("fixed round trip mismatch: %v", err)
	}
}

func TestMinimalCounterexampleArtifact(t *testing.T) {
	path := filepath.Join("testdata", "minimal-counterexample.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read counterexample: %v", err)
	}
	want, err := Unmarshal(data)
	if err != nil {
		t.Fatalf("parse counterexample: %v", err)
	}
	group := want.Rate.framesPer10Minutes()
	if want.Duration != group+1 || len(want.Tracks) != 1 || len(want.Tracks[0].Clips) != 1 {
		t.Fatalf("counterexample is no longer minimal:\n%s", timelineJSON(want))
	}
	clip := want.Tracks[0].Clips[0]
	if clip.SeqIn != group-1 || clip.SeqOut != group || clip.SrcIn != group-1 || clip.SrcOut != group {
		t.Fatalf("unexpected minimal clip: %+v", clip)
	}

	legacyData, err := legacyMarshal(want)
	if err != nil {
		t.Fatalf("legacy marshal: %v", err)
	}
	legacyGot, err := Unmarshal(legacyData)
	if err != nil {
		t.Fatalf("legacy counterexample no longer imports: %v", err)
	}
	if legacyGot.Duration != want.Duration {
		t.Fatalf("legacy duration = %d, want %d", legacyGot.Duration, want.Duration)
	}
	if err := compareTimelines(want, legacyGot); err == nil {
		t.Fatal("legacy counterexample stopped failing")
	}

	fixedData, err := Marshal(want)
	if err != nil {
		t.Fatalf("fixed marshal: %v", err)
	}
	fixedGot, err := Unmarshal(fixedData)
	if err != nil {
		t.Fatalf("fixed import: %v", err)
	}
	if err := compareTimelines(want, fixedGot); err != nil {
		t.Fatalf("fixed counterexample still fails: %v", err)
	}
}

func TestShrinkerFindsMinimalCounterexample(t *testing.T) {
	rate := mustRate(t, 30000, 1001)
	group := rate.framesPer10Minutes()
	large := Timeline{
		Rate: rate,
		Tracks: []Track{
			{ID: "V1", Clips: []Clip{
				{SeqIn: 0, SeqOut: 3000, SrcIn: 10000, SrcOut: 13000, Speed: Rate{Num: 1, Den: 1}},
				{SeqIn: 2000, SeqOut: 8000, SrcIn: 500, SrcOut: 6500, Speed: Rate{Num: 1, Den: 1}},
				{SeqIn: group - 1, SeqOut: group, SrcIn: group - 1, SrcOut: group, Speed: Rate{Num: 1, Den: 1}},
			}},
			{ID: "V2", Clips: []Clip{
				{SeqIn: group / 2, SeqOut: group/2 + 100, SrcIn: 0, SrcOut: 200, Speed: Rate{Num: 2, Den: 1}},
			}},
			{ID: "A1", Clips: []Clip{
				{SeqIn: 0, SeqOut: 5000, SrcIn: 0, SrcOut: 5000, Speed: Rate{Num: 1, Den: 1}},
			}},
		},
	}
	large.Sort()
	large.Duration = timelineEnd(large) + 500
	if err := large.Validate(); err != nil {
		t.Fatalf("large counterexample is invalid: %v", err)
	}
	if !legacyCounterexample(large) {
		t.Fatal("large fixture does not reproduce the legacy failure")
	}

	minimal := shrinkTimeline(large, legacyCounterexample)
	if err := minimal.Validate(); err != nil {
		t.Fatalf("shrunk timeline is invalid: %v", err)
	}
	if len(minimal.Tracks) != 1 || len(minimal.Tracks[0].Clips) != 1 {
		t.Fatalf("shrinker did not minimize structure:\n%s", timelineJSON(minimal))
	}
	clip := minimal.Tracks[0].Clips[0]
	if clip.SeqIn != group-1 || clip.SeqOut != group ||
		clip.SrcIn != group-1 || clip.SrcOut != group ||
		clip.Speed != (Rate{Num: 1, Den: 1}) {
		t.Fatalf("unexpected shrunk clip: %+v", clip)
	}
	if minimal.Duration != group+1 {
		t.Fatalf("shrunk duration = %d, want %d", minimal.Duration, group+1)
	}
}
