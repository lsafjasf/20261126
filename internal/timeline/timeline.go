package timeline

import (
	"fmt"
	"sort"
	"strings"
)

const (
	AnchorMedia  = "media"
	AnchorClip   = "clip"
	AnchorGlobal = "global"
)

const (
	PolicyFloor   = "floor"
	PolicyNearest = "nearest"
	PolicyCeil    = "ceil"
)

type Rate int64

func (r Rate) Rational() Rat {
	return RatInt(int64(r))
}

func (r Rate) String() string {
	return fmt.Sprintf("%d", int64(r))
}

func (r Rate) MarshalJSON() ([]byte, error) {
	return []byte(`"` + r.String() + `"`), nil
}

func (r *Rate) UnmarshalJSON(data []byte) error {
	text := strings.Trim(string(data), `"`)
	var value int64
	if _, err := fmt.Sscan(text, &value); err != nil || value <= 0 {
		return fmt.Errorf("invalid frame rate %q", text)
	}
	*r = Rate(value)
	return nil
}

type Marker struct {
	ID                string `json:"id"`
	Label             string `json:"label"`
	Role              string `json:"role"`
	AnchorKind        string `json:"anchor_kind"`
	AnchorFrame       int64  `json:"anchor_frame"`
	AnchorRate        Rate   `json:"anchor_rate"`
	GlobalTimeSeconds Rat    `json:"global_time_seconds"`
	RolePolicy        string `json:"role_policy"`
	MediaOffsetFrames int64  `json:"media_offset_frames,omitempty"`
	ClipOffsetFrames  int64  `json:"clip_offset_frames,omitempty"`
	NearestEvenTie    bool   `json:"nearest_even_tie,omitempty"`
}

func (m Marker) Validate() error {
	switch m.AnchorKind {
	case AnchorMedia, AnchorClip, AnchorGlobal:
	default:
		return fmt.Errorf("marker %s has invalid anchor kind %q", m.ID, m.AnchorKind)
	}
	switch m.RolePolicy {
	case PolicyFloor, PolicyNearest, PolicyCeil:
	default:
		return fmt.Errorf("marker %s has invalid role policy %q", m.ID, m.RolePolicy)
	}
	if m.AnchorRate <= 0 {
		return fmt.Errorf("marker %s has non-positive anchor rate", m.ID)
	}
	anchorCoordinate := markerAnchorCoordinate(m)
	if !anchorCoordinate.IsInt() || anchorCoordinate.Floor() != m.AnchorFrame {
		return fmt.Errorf("marker %s global time does not resolve to anchor frame %d", m.ID, m.AnchorFrame)
	}
	return nil
}

type Timeline struct {
	ProjectName    string
	SequenceRate   Rate
	DurationFrames int64
	Rates          []Rate
	Markers        []Marker
}

func (t Timeline) Validate() error {
	if t.SequenceRate <= 0 || t.DurationFrames < 0 {
		return fmt.Errorf("invalid sequence rate or duration")
	}
	if len(t.Rates) == 0 {
		return fmt.Errorf("at least one target rate is required")
	}
	seen := map[Rate]struct{}{}
	for _, rate := range t.Rates {
		if rate <= 0 {
			return fmt.Errorf("target rates must be positive")
		}
		if _, ok := seen[rate]; ok {
			return fmt.Errorf("duplicate target rate %d", rate)
		}
		seen[rate] = struct{}{}
	}
	for _, marker := range t.Markers {
		if err := marker.Validate(); err != nil {
			return err
		}
		if _, ok := seen[marker.AnchorRate]; !ok {
			return fmt.Errorf("marker %s anchor rate %d is not in target rates", marker.ID, marker.AnchorRate)
		}
	}
	return nil
}

func (t Timeline) GlobalTime(marker Marker) Rat {
	return marker.GlobalTimeSeconds
}

func ExactFrameAt(globalTime Rat, targetRate Rate) Rat {
	return globalTime.Mul(targetRate.Rational())
}

func AnchorCoordinateAt(globalTime Rat, anchorRate Rate) Rat {
	return globalTime.Mul(anchorRate.Rational())
}

func Snap(value Rat, policy string) (int64, error) {
	switch policy {
	case PolicyFloor:
		return value.Floor(), nil
	case PolicyCeil:
		return value.Ceil(), nil
	case PolicyNearest:
		return value.NearestEven(), nil
	default:
		return 0, fmt.Errorf("unknown snap policy %q", policy)
	}
}

type Candidate struct {
	Frame               int64    `json:"frame"`
	Relation            string   `json:"relation"`
	Tags                []string `json:"tags"`
	DeltaFrames         Rat      `json:"delta_frames"`
	AbsFrames           Rat      `json:"abs_frames"`
	DeltaSeconds        Rat      `json:"delta_seconds"`
	DeltaMillisDisplay  float64  `json:"delta_millis_display_only"`
	DistanceRank        int      `json:"distance_rank"`
	Recommended         bool     `json:"recommended"`
	AnchorCoordinate    Rat      `json:"anchor_coordinate"`
	AnchorDeltaFrames   Rat      `json:"anchor_delta_frames"`
	ReverseSnap         SnapSet  `json:"reverse_snap"`
	SafeReversePolicies []string `json:"safe_reverse_policies,omitempty"`
	RolePreferred       *bool    `json:"role_preferred,omitempty"`
}

type SnapSet struct {
	Floor   int64 `json:"floor"`
	Nearest int64 `json:"nearest"`
	Ceil    int64 `json:"ceil"`
}

type Placement struct {
	Rate       Rate        `json:"rate"`
	ExactFrame Rat         `json:"exact_frame"`
	Exact      bool        `json:"exact"`
	Candidates []Candidate `json:"candidates"`
}

func BuildPlacement(marker Marker, targetRate Rate) Placement {
	exact := ExactFrameAt(marker.GlobalTimeSeconds, targetRate)
	anchorExact := markerAnchorCoordinate(marker)
	frames := candidateFrames(exact)
	candidates := make([]Candidate, 0, len(frames))
	recommended := targetNearestFrame(exact, marker)
	roleFrame := preferredFrame(exact, marker.RolePolicy)
	for _, frame := range frames {
		frameRat := RatInt(frame)
		delta := frameRat.Sub(exact)
		frameTime := frameRat.Quo(targetRate.Rational())
		deltaSeconds := frameTime.Sub(marker.GlobalTimeSeconds)
		anchorCoordinate := frameTime.Mul(marker.AnchorRate.Rational())
		if marker.AnchorKind == AnchorMedia {
			anchorCoordinate = anchorCoordinate.Sub(RatInt(marker.MediaOffsetFrames))
		}
		if marker.AnchorKind == AnchorClip {
			anchorCoordinate = anchorCoordinate.Sub(RatInt(marker.ClipOffsetFrames))
		}
		anchorDelta := anchorCoordinate.Sub(anchorExact)
		reverse := SnapSet{
			Floor:   anchorCoordinate.Floor(),
			Nearest: anchorCoordinate.NearestEven(),
			Ceil:    anchorCoordinate.Ceil(),
		}
		safe := safeReversePolicies(reverse, marker.AnchorFrame)
		relation, tags := relationAndTags(frame, exact, marker)
		rolePreferred := frame == roleFrame && (relation == "exact" || relation == PolicyFloor || relation == PolicyCeil)
		candidates = append(candidates, Candidate{
			Frame:               frame,
			Relation:            relation,
			Tags:                tags,
			DeltaFrames:         delta,
			AbsFrames:           delta.Abs(),
			DeltaSeconds:        deltaSeconds,
			DeltaMillisDisplay:  deltaSeconds.Float64Value() * 1000,
			AnchorCoordinate:    anchorCoordinate,
			AnchorDeltaFrames:   anchorDelta,
			ReverseSnap:         reverse,
			SafeReversePolicies: safe,
			RolePreferred:       optionalBoolPointer(true, rolePreferred),
		})
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		cmp := candidates[i].AbsFrames.Cmp(candidates[j].AbsFrames)
		if cmp != 0 {
			return cmp < 0
		}
		if candidates[i].Frame != candidates[j].Frame {
			return candidates[i].Frame < candidates[j].Frame
		}
		return false
	})
	for index := range candidates {
		candidates[index].DistanceRank = index + 1
		candidates[index].Recommended = candidates[index].Frame == recommended
		if candidates[index].Relation == "before_floor" || candidates[index].Relation == "after_ceil" {
			candidates[index].RolePreferred = nil
		}
	}
	return Placement{Rate: targetRate, ExactFrame: exact, Exact: exact.IsInt(), Candidates: candidates}
}

func markerAnchorCoordinate(marker Marker) Rat {
	coordinate := AnchorCoordinateAt(marker.GlobalTimeSeconds, marker.AnchorRate)
	if marker.AnchorKind == AnchorMedia {
		return coordinate.Sub(RatInt(marker.MediaOffsetFrames))
	}
	if marker.AnchorKind == AnchorClip {
		return coordinate.Sub(RatInt(marker.ClipOffsetFrames))
	}
	return coordinate
}

func candidateFrames(exact Rat) []int64 {
	if exact.IsInt() {
		frame := exact.Floor()
		return []int64{frame, frame - 1, frame + 1}
	}
	floor, ceil := exact.Floor(), exact.Ceil()
	return []int64{floor, ceil, floor - 1, ceil + 1}
}

func relationAndTags(frame int64, exact Rat, marker Marker) (string, []string) {
	if exact.IsInt() {
		if frame == exact.Floor() {
			return "exact", []string{"exact", PolicyNearest, PolicyFloor, PolicyCeil}
		}
		if frame < exact.Floor() {
			return "before_floor", []string{"before_floor"}
		}
		return "after_ceil", []string{"after_ceil"}
	}
	floor, ceil := exact.Floor(), exact.Ceil()
	tags := []string{}
	relation := ""
	if frame == floor {
		relation = PolicyFloor
		tags = append(tags, PolicyFloor)
	}
	if frame == ceil {
		relation = PolicyCeil
		tags = append(tags, PolicyCeil)
	}
	if frame == targetNearestFrame(exact, marker) {
		tags = append(tags, PolicyNearest)
	}
	if frame < floor {
		relation = "before_floor"
		tags = []string{"before_floor"}
	}
	if frame > ceil {
		relation = "after_ceil"
		tags = []string{"after_ceil"}
	}
	return relation, tags
}

func targetNearestFrame(exact Rat, marker Marker) int64 {
	if marker.NearestEvenTie || marker.AnchorKind != AnchorMedia {
		return exact.NearestEven()
	}
	return exact.NearestUp()
}

func preferredFrame(exact Rat, policy string) int64 {
	frame, err := Snap(exact, policy)
	if err != nil {
		panic(err)
	}
	return frame
}

func safeReversePolicies(reverse SnapSet, originalFrame int64) []string {
	policies := make([]string, 0, 3)
	if reverse.Floor == originalFrame {
		policies = append(policies, PolicyFloor)
	}
	if reverse.Nearest == originalFrame {
		policies = append(policies, PolicyNearest)
	}
	if reverse.Ceil == originalFrame {
		policies = append(policies, PolicyCeil)
	}
	if len(policies) == 0 {
		return nil
	}
	return policies
}

func optionalBoolPointer(value bool, include bool) *bool {
	if !include {
		return nil
	}
	return &value
}
