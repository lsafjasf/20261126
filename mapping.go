package fourch

import (
	"fmt"
	"strings"
)

// Speaker describes the direction of one discrete loudspeaker in AmbiX
// coordinates. Name is the role name used by discrete mappings (for example
// "FL") and is only a label for Ambisonic decoding.
type Speaker struct {
	Name         string
	AzimuthDeg   float64
	ElevationDeg float64
}

// CanonicalQuadSpeakers returns loudspeakers at the conventional quadraphonic
// positions represented by the candidate channel roles.
func CanonicalQuadSpeakers() []Speaker {
	return []Speaker{
		{Name: "FL", AzimuthDeg: 45},
		{Name: "FR", AzimuthDeg: -45},
		{Name: "BL", AzimuthDeg: 135},
		{Name: "BR", AzimuthDeg: -135},
	}
}

// Mapping is a linear map from four encoded channels to discrete speaker
// feeds. Rows follow Speakers order; columns follow the source candidate's
// channel order.
type Mapping struct {
	Source   Candidate
	Speakers []Speaker
	Matrix   Matrix
}

// Decode applies the mapping to one four-channel sample.
func (m Mapping) Decode(channels [4]float64) ([]float64, error) {
	return m.Matrix.Apply(channels[:])
}

// InverseReport describes what can be recovered from the speaker feeds.
//
// Injective means the map has full column rank, so a left inverse exists.
// Invertible is stricter: it requires a square matrix as well. For a 4x4 map,
// Invertible and Injective are equivalent.
type InverseReport struct {
	Rows           int
	Cols           int
	Rank           int
	Square         bool
	FullColumnRank bool
	Injective      bool
	Invertible     bool
	Inverse        Matrix
	Nullspace      Matrix
	Reason         string
}

// Invertibility computes the rank and a nullspace basis for the mapping. A
// non-positive tolerance selects a scale-aware default.
func (m Mapping) Invertibility(tolerance float64) (InverseReport, error) {
	rows, cols, err := m.Matrix.Dimensions()
	if err != nil {
		return InverseReport{}, err
	}
	rank, err := m.Matrix.Rank(tolerance)
	if err != nil {
		return InverseReport{}, err
	}
	nullspace, err := m.Matrix.Nullspace(tolerance)
	if err != nil {
		return InverseReport{}, err
	}
	report := InverseReport{
		Rows:           rows,
		Cols:           cols,
		Rank:           rank,
		Square:         rows == cols,
		FullColumnRank: rank == cols,
		Injective:      rank == cols,
		Nullspace:      nullspace,
	}
	if report.Square && report.FullColumnRank {
		inverse, err := m.Matrix.Inverse(tolerance)
		if err != nil {
			return InverseReport{}, fmt.Errorf("full-rank inverse failed: %w", err)
		}
		report.Inverse = inverse
		report.Invertible = true
		report.Reason = "square and full column rank; all four encoded channels are recoverable"
		return report, nil
	}
	switch {
	case !report.Square && report.FullColumnRank:
		report.Reason = "full column rank but non-square; the map is injective (left-invertible), not a square inverse"
	case report.Square:
		report.Reason = fmt.Sprintf("square but rank %d of %d; the mapping loses directions in its nullspace", rank, cols)
	default:
		report.Reason = fmt.Sprintf("rank %d of %d; the mapping is not injective", rank, cols)
	}
	return report, nil
}

// MapToSpeakers builds a linear mapping for candidate. Quadraphonic candidates
// require exactly one speaker for each of FL, FR, BL, and BR. Ambisonic
// candidates use a basic first-order pressure-projection decoder
// [W,Y,Z,X] -> [1,y,z,x] per speaker.
func MapToSpeakers(candidate Candidate, speakers []Speaker) (Mapping, error) {
	switch candidate.Kind {
	case KindQuadraphonic:
		return quadraphonicToSpeakers(candidate, speakers)
	case KindAmbisonicACNSN3D:
		return ambisonicToSpeakers(candidate, speakers)
	default:
		return Mapping{}, fmt.Errorf("unknown candidate kind %q", candidate.Kind)
	}
}

// QuadraphonicToSpeakers routes each discrete feed to its named speaker. A
// conventional four-corner layout therefore yields a permutation (the identity
// for canonical order).
func QuadraphonicToSpeakers(speakers []Speaker) (Mapping, error) {
	return quadraphonicToSpeakers(QuadraphonicCandidate(), speakers)
}

func quadraphonicToSpeakers(candidate Candidate, speakers []Speaker) (Mapping, error) {
	if len(speakers) != 4 {
		return Mapping{}, fmt.Errorf("quadraphonic routing needs exactly 4 speakers, got %d", len(speakers))
	}
	if len(candidate.Channels) != 4 {
		return Mapping{}, fmt.Errorf("quadraphonic candidate needs exactly 4 channel roles")
	}
	byName := make(map[string]int, len(candidate.Channels))
	for i, role := range candidate.Channels {
		name := strings.ToUpper(strings.TrimSpace(role.Name))
		if name == "" {
			return Mapping{}, fmt.Errorf("quadraphonic channel %d has no role name", i)
		}
		if _, exists := byName[name]; exists {
			return Mapping{}, fmt.Errorf("quadraphonic role %q is duplicated", name)
		}
		byName[name] = i
	}
	channelForSpeaker := make([]int, len(speakers))
	used := make(map[string]bool, len(speakers))
	copied := make([]Speaker, len(speakers))
	for i, speaker := range speakers {
		if err := validateSpeaker(speaker, true); err != nil {
			return Mapping{}, fmt.Errorf("speaker %d: %w", i, err)
		}
		name := strings.ToUpper(strings.TrimSpace(speaker.Name))
		channel, ok := byName[name]
		if !ok {
			return Mapping{}, fmt.Errorf("speaker %q is not a quadraphonic role", speaker.Name)
		}
		if used[name] {
			return Mapping{}, fmt.Errorf("speaker role %q is duplicated", name)
		}
		used[name] = true
		channelForSpeaker[i] = channel
		speaker.Name = name
		copied[i] = speaker
	}
	for name := range byName {
		if !used[name] {
			return Mapping{}, fmt.Errorf("speaker role %q is missing", name)
		}
	}
	matrix, _ := NewMatrix(len(copied), 4)
	for i, channel := range channelForSpeaker {
		matrix[i][channel] = 1
	}
	return Mapping{Source: candidate, Speakers: copied, Matrix: matrix}, nil
}

// AmbisonicToSpeakers builds a basic FOA decoder for the supplied speaker
// directions. In ACN order the row for a speaker is [1, y, z, x].
func AmbisonicToSpeakers(speakers []Speaker) (Mapping, error) {
	return ambisonicToSpeakers(AmbisonicCandidate(), speakers)
}

func ambisonicToSpeakers(candidate Candidate, speakers []Speaker) (Mapping, error) {
	if len(speakers) == 0 {
		return Mapping{}, fmt.Errorf("ambisonic decoding needs at least one speaker")
	}
	if len(candidate.Channels) != 4 {
		return Mapping{}, fmt.Errorf("Ambisonic candidate needs exactly 4 channel roles")
	}
	copied := make([]Speaker, len(speakers))
	matrix, _ := NewMatrix(len(speakers), 4)
	for i, speaker := range speakers {
		if err := validateSpeaker(speaker, false); err != nil {
			return Mapping{}, fmt.Errorf("speaker %d: %w", i, err)
		}
		if strings.TrimSpace(speaker.Name) == "" {
			speaker.Name = fmt.Sprintf("S%d", i)
		}
		copied[i] = speaker
		d := Direction(speaker.AzimuthDeg, speaker.ElevationDeg)
		response, err := ambisonicResponse(candidate, d)
		if err != nil {
			return Mapping{}, fmt.Errorf("speaker %d: %w", i, err)
		}
		matrix[i] = response[:]
	}
	return Mapping{Source: candidate, Speakers: copied, Matrix: matrix}, nil
}

func validateSpeaker(speaker Speaker, requireName bool) error {
	if requireName && strings.TrimSpace(speaker.Name) == "" {
		return fmt.Errorf("speaker name is empty")
	}
	if !isFinite(speaker.AzimuthDeg) || !isFinite(speaker.ElevationDeg) {
		return fmt.Errorf("speaker direction must be finite")
	}
	return nil
}
