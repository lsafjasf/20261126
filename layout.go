// Package fourch interprets a four-channel stream whose container only says
// "4ch", and checks whether mappings from that stream to loudspeakers are
// invertible.
package fourch

import (
	"errors"
	"fmt"
	"math"
	"sort"
)

// Kind identifies a four-channel interpretation.
type Kind string

const (
	// KindQuadraphonic is four discrete speaker feeds: FL, FR, BL, BR.
	KindQuadraphonic Kind = "quadraphonic"
	// KindAmbisonicACNSN3D is first-order Ambisonics in ACN order and SN3D
	// normalization.
	KindAmbisonicACNSN3D Kind = "ambisonic-acn-sn3d"
)

// RoleKind describes what a channel represents.
type RoleKind string

const (
	RoleDiscrete RoleKind = "discrete-speaker"
	RoleOmni     RoleKind = "omnidirectional"
	RoleAxis     RoleKind = "first-order-axis"
)

// Vec3 is a direction in the AmbiX coordinate convention: +X is front,
// +Y is left, and +Z is up.
type Vec3 struct {
	X float64
	Y float64
	Z float64
}

// Direction returns the unit vector at azimuth and elevation. Azimuth is in
// degrees, counter-clockwise from +X (front) toward +Y (left). Elevation is in
// degrees above the horizontal plane.
func Direction(azimuthDeg, elevationDeg float64) Vec3 {
	az := azimuthDeg * math.Pi / 180
	el := elevationDeg * math.Pi / 180
	c := math.Cos(el)
	return Vec3{
		X: c * math.Cos(az),
		Y: c * math.Sin(az),
		Z: math.Sin(el),
	}
}

// ChannelRole ties a stream channel to a physical or mathematical role.
type ChannelRole struct {
	Index  int
	Name   string
	Kind   RoleKind
	Vector Vec3
}

// Convention documents the coordinate convention used by a candidate.
type Convention struct {
	Name        string
	Description string
}

// AmbiXConvention is the convention used by this package.
var AmbiXConvention = Convention{
	Name:        "AmbiX",
	Description: "+X front, +Y left, +Z up; azimuth is counter-clockwise from +X",
}

// Candidate is one possible explanation for a four-channel stream.
type Candidate struct {
	Kind          Kind
	Label         string
	Channels      [4]ChannelRole
	Normalization string
	Convention    Convention
}

// DefaultCandidates returns the two interpretations that a bare "4ch" label
// supports: discrete quadraphonic feeds and first-order ACN/SN3D Ambisonics.
// Neither is preferred without evidence.
func DefaultCandidates() []Candidate {
	return []Candidate{QuadraphonicCandidate(), AmbisonicCandidate()}
}

// QuadraphonicCandidate returns the conventional horizontal quad layout.
func QuadraphonicCandidate() Candidate {
	positions := [...]struct {
		name string
		az   float64
	}{
		{"FL", 45},
		{"FR", -45},
		{"BL", 135},
		{"BR", -135},
	}
	var roles [4]ChannelRole
	for i, p := range positions {
		roles[i] = ChannelRole{
			Index:  i,
			Name:   p.name,
			Kind:   RoleDiscrete,
			Vector: Direction(p.az, 0),
		}
	}
	return Candidate{
		Kind:          KindQuadraphonic,
		Label:         "quadraphonic FL/FR/BL/BR",
		Channels:      roles,
		Normalization: "discrete speaker feeds (no spherical-harmonic normalization)",
		Convention:    AmbiXConvention,
	}
}

// AmbisonicCandidate returns first-order ACN/SN3D Ambisonics. The channel roles
// are deliberately in ACN order: W, Y, Z, X.
func AmbisonicCandidate() Candidate {
	roles := [4]ChannelRole{
		{Index: 0, Name: "ACN0/W", Kind: RoleOmni},
		{Index: 1, Name: "ACN1/Y", Kind: RoleAxis, Vector: Vec3{Y: 1}},
		{Index: 2, Name: "ACN2/Z", Kind: RoleAxis, Vector: Vec3{Z: 1}},
		{Index: 3, Name: "ACN3/X", Kind: RoleAxis, Vector: Vec3{X: 1}},
	}
	return Candidate{
		Kind:          KindAmbisonicACNSN3D,
		Label:         "first-order Ambisonics ACN/SN3D (W,Y,Z,X)",
		Channels:      roles,
		Normalization: "SN3D",
		Convention:    AmbiXConvention,
	}
}

// Predict returns the ideal unit-source channel vector for a source at the
// supplied direction. The result is not normalized: Ambisonic W equals one,
// while quadraphonic gains are amplitude-panning gains. Callers comparing
// recordings of unknown gain should normalize both vectors first.
func (c Candidate) Predict(azimuthDeg, elevationDeg float64) ([4]float64, error) {
	if !isFinite(azimuthDeg) || !isFinite(elevationDeg) {
		return [4]float64{}, errors.New("direction must be finite")
	}
	switch c.Kind {
	case KindAmbisonicACNSN3D:
		return ambisonicResponse(c, Direction(azimuthDeg, elevationDeg))
	case KindQuadraphonic:
		if len(c.Channels) != 4 {
			return [4]float64{}, errors.New("quadraphonic candidate needs four channels")
		}
		return quadGains(c.Channels, azimuthDeg), nil
	default:
		return [4]float64{}, fmt.Errorf("unknown candidate kind %q", c.Kind)
	}
}

func ambisonicResponse(c Candidate, d Vec3) ([4]float64, error) {
	if len(c.Channels) != 4 {
		return [4]float64{}, errors.New("invalid ACN channel roles")
	}
	out := [4]float64{1, 0, 0, 0}
	for i := 1; i < 4; i++ {
		axis := c.Channels[i].Vector
		if !isFinite(axis.X) || !isFinite(axis.Y) || !isFinite(axis.Z) ||
			axis.X*axis.X+axis.Y*axis.Y+axis.Z*axis.Z < 1e-24 {
			return [4]float64{}, fmt.Errorf("ACN channel %d has an invalid axis", i)
		}
		out[i] = d.X*axis.X + d.Y*axis.Y + d.Z*axis.Z
	}
	return out, nil
}

// quadGains uses two-dimensional VBAP between the two adjacent speakers that
// enclose the horizontal source direction. Elevation is intentionally ignored:
// a horizontal quad layout has no height channel.
func quadGains(roles [4]ChannelRole, azimuthDeg float64) [4]float64 {
	type indexed struct {
		index int
		az    float64
	}
	speakers := make([]indexed, 0, 4)
	for i, role := range roles {
		az := math.Atan2(role.Vector.Y, role.Vector.X) * 180 / math.Pi
		speakers = append(speakers, indexed{i, normalizeAngle(az)})
	}
	sort.Slice(speakers, func(i, j int) bool {
		return speakers[i].az < speakers[j].az
	})

	az := normalizeAngle(azimuthDeg)
	var gains [4]float64
	for i := range speakers {
		a := speakers[i]
		b := speakers[(i+1)%len(speakers)]
		span := normalizeAngle(b.az - a.az)
		if span == 0 {
			continue
		}
		rel := normalizeAngle(az - a.az)
		if rel <= span+1e-12 {
			den := math.Sin(span * math.Pi / 180)
			if math.Abs(den) < 1e-15 {
				continue
			}
			gains[a.index] = math.Sin((span-rel)*math.Pi/180) / den
			gains[b.index] = math.Sin(rel*math.Pi/180) / den
			return gains
		}
	}
	return gains
}

func normalizeAngle(deg float64) float64 {
	deg = math.Mod(deg, 360)
	if deg < 0 {
		deg += 360
	}
	return deg
}

func isFinite(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0)
}
