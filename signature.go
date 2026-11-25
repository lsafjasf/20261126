package channelmap

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
)

// SignatureSet describes known multi-channel test features. It is the bridge
// from observable samples to speaker-role names; raw audio alone has no
// universal center/surround label.
type SignatureSet struct {
	Name        string                   `json:"name,omitempty"`
	Description string                   `json:"description,omitempty"`
	SampleRate  int                      `json:"sample_rate,omitempty"`
	Roles       map[string]RoleSignature `json:"roles"`
}

// RoleSignature is a small, explainable feature contract for one role.
type RoleSignature struct {
	FrequencyHz          float64 `json:"frequency_hz,omitempty"`
	FrequencyToleranceHz float64 `json:"frequency_tolerance_hz,omitempty"`
	MinFrequencyHz       float64 `json:"min_frequency_hz,omitempty"`
	MaxFrequencyHz       float64 `json:"max_frequency_hz,omitempty"`
	MinRMS               float64 `json:"min_rms,omitempty"`
	MaxZeroCrossingRate  float64 `json:"max_zero_crossing_rate,omitempty"`
	MinLowBandRatio      float64 `json:"min_low_band_ratio,omitempty"`
	// Template is an optional aligned characteristic sample snippet. Matching
	// uses Pearson correlation and does not require an audio codec package.
	Template []float64 `json:"template,omitempty"`
	// TemplateStart is the sample offset in each wire to compare with Template.
	TemplateStart int `json:"template_start,omitempty"`
}

func LoadSignatures(r io.Reader) (*SignatureSet, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	var set SignatureSet
	if err := json.Unmarshal(data, &set); err != nil {
		return nil, fmt.Errorf("signatures: invalid JSON: %w", err)
	}
	if len(set.Roles) == 0 {
		return nil, fmt.Errorf("signatures: roles must not be empty")
	}
	for role, sig := range set.Roles {
		if role == "" {
			return nil, fmt.Errorf("signatures: empty role name")
		}
		if sig.FrequencyHz < 0 || sig.MinFrequencyHz < 0 || sig.MaxFrequencyHz < 0 {
			return nil, fmt.Errorf("signatures[%s]: frequencies must be non-negative", role)
		}
		if sig.MaxFrequencyHz > 0 && sig.MinFrequencyHz > sig.MaxFrequencyHz {
			return nil, fmt.Errorf("signatures[%s]: min_frequency_hz exceeds max_frequency_hz", role)
		}
	}
	return &set, nil
}

// DefaultCalibrationSignatures matches the test tones emitted by
// cmd/wavchannelgen. Frequencies are a test convention, not a universal audio
// standard, so reports identify matches using this profile name.
func DefaultCalibrationSignatures() SignatureSet {
	return SignatureSet{
		Name:        "wavchannel-calibration-v1",
		Description: "Built-in distinct-tone calibration profile used by the bundled generator and tests.",
		Roles: map[string]RoleSignature{
			"FL": {FrequencyHz: 320, FrequencyToleranceHz: 18, MinRMS: 0.02},
			"FR": {FrequencyHz: 400, FrequencyToleranceHz: 18, MinRMS: 0.02},
			"FC": {FrequencyHz: 1000, FrequencyToleranceHz: 24, MinRMS: 0.02},
			"LFE": {
				FrequencyHz: 70, FrequencyToleranceHz: 14,
				MaxFrequencyHz: 120, MinLowBandRatio: 0.70, MinRMS: 0.02,
			},
			"BL": {FrequencyHz: 520, FrequencyToleranceHz: 20, MinRMS: 0.02},
			"BR": {FrequencyHz: 640, FrequencyToleranceHz: 22, MinRMS: 0.02},
		},
	}
}

type roleScore struct {
	Role       string
	WireIndex  int
	Cost       float64
	Evidence   []string
	Violations []string
}

func scoreRoleWire(role string, sig RoleSignature, f Features, samples []float64) roleScore {
	rs := roleScore{Role: role, WireIndex: f.WireIndex, Cost: 0}
	addPenalty := func(amount float64, violation string) {
		rs.Cost = math.Min(100, rs.Cost+amount)
		rs.Violations = append(rs.Violations, violation)
	}

	if !f.Active {
		addPenalty(100, "channel is silent or below activity threshold")
		return rs
	}
	rs.Evidence = append(rs.Evidence, fmt.Sprintf("RMS=%.4f peak=%.4f", f.RMS, f.Peak))

	if sig.MinRMS > 0 {
		if f.RMS < sig.MinRMS {
			addPenalty(40, fmt.Sprintf("RMS %.4f below required %.4f", f.RMS, sig.MinRMS))
		} else {
			rs.Evidence = append(rs.Evidence, fmt.Sprintf("RMS %.4f meets minimum %.4f", f.RMS, sig.MinRMS))
		}
	}
	if sig.FrequencyHz > 0 {
		tol := sig.FrequencyToleranceHz
		if tol <= 0 {
			tol = math.Max(12, sig.FrequencyHz*0.02)
		}
		diff := math.Abs(f.DominantFrequencyHz - sig.FrequencyHz)
		if diff <= tol {
			rs.Evidence = append(rs.Evidence, fmt.Sprintf("dominant %.2f Hz matches %s signature %.1f±%.1f Hz", f.DominantFrequencyHz, role, sig.FrequencyHz, tol))
			rs.Cost += 0
		} else {
			scaled := math.Min(60, 15+diff/math.Max(tol, 1)*12)
			addPenalty(scaled, fmt.Sprintf("dominant %.2f Hz differs from %s signature %.1f±%.1f Hz by %.2f Hz", f.DominantFrequencyHz, role, sig.FrequencyHz, tol, diff))
		}
	}
	if sig.MinFrequencyHz > 0 && f.DominantFrequencyHz < sig.MinFrequencyHz {
		addPenalty(35, fmt.Sprintf("dominant %.2f Hz below %.1f Hz", f.DominantFrequencyHz, sig.MinFrequencyHz))
	}
	if sig.MaxFrequencyHz > 0 && f.DominantFrequencyHz > sig.MaxFrequencyHz {
		addPenalty(45, fmt.Sprintf("dominant %.2f Hz above max %.1f Hz", f.DominantFrequencyHz, sig.MaxFrequencyHz))
	}
	if sig.MinLowBandRatio > 0 {
		if f.LowBandEnergyRatio >= sig.MinLowBandRatio {
			rs.Evidence = append(rs.Evidence, fmt.Sprintf("low-band energy %.2f≥%.2f", f.LowBandEnergyRatio, sig.MinLowBandRatio))
		} else {
			addPenalty(35, fmt.Sprintf("low-band energy %.2f below %.2f", f.LowBandEnergyRatio, sig.MinLowBandRatio))
		}
	}
	if sig.MaxZeroCrossingRate > 0 && f.ZeroCrossingRate > sig.MaxZeroCrossingRate {
		addPenalty(20, fmt.Sprintf("zero-crossing rate %.1f above %.1f", f.ZeroCrossingRate, sig.MaxZeroCrossingRate))
	}
	if len(sig.Template) >= 16 && len(samples) > 0 {
		start := sig.TemplateStart
		if start < 0 {
			start = 0
		}
		var segment []float64
		if start < len(samples) {
			segment = samples[start:]
			if len(segment) > len(sig.Template) {
				segment = segment[:len(sig.Template)]
			}
		} else {
			segment = nil
		}
		corr := Correlation(sig.Template, segment)
		switch {
		case corr >= 0.92:
			rs.Evidence = append(rs.Evidence, fmt.Sprintf("sample-template correlation %.4f≥0.92", corr))
		case corr >= 0.75:
			addPenalty(18, fmt.Sprintf("sample-template correlation %.4f is usable but below 0.92", corr))
		default:
			addPenalty(70, fmt.Sprintf("sample-template correlation %.4f below 0.75", corr))
		}
	}
	if len(rs.Violations) == 0 && rs.Cost == 0 {
		rs.Cost = 0.5
	}
	return rs
}

func lookupSignature(set *SignatureSet, role string) (RoleSignature, string, bool) {
	if set == nil {
		return RoleSignature{}, "", false
	}
	if sig, ok := set.Roles[role]; ok {
		return sig, set.Name, true
	}
	for _, alias := range roleAliases(role) {
		if sig, ok := set.Roles[alias]; ok {
			return sig, set.Name, true
		}
	}
	return RoleSignature{}, "", false
}

func roleAliases(role string) []string {
	switch role {
	case "BL":
		return []string{"SL", "SURROUND_LEFT", "Ls"}
	case "SL":
		return []string{"BL", "SURROUND_LEFT", "Ls"}
	case "BR":
		return []string{"SR", "SURROUND_RIGHT", "Rs"}
	case "SR":
		return []string{"BR", "SURROUND_RIGHT", "Rs"}
	case "FC":
		return []string{"CENTER", "C"}
	default:
		return []string{RoleGroup(role)}
	}
}

func RoleGroup(role string) string {
	switch role {
	case "FL", "FLC", "TFL":
		return "FRONT_LEFT"
	case "FR", "FRC", "TFR":
		return "FRONT_RIGHT"
	case "FC", "TFC", "TC", "TBC", "BC":
		return "CENTER"
	case "LFE":
		return "LFE"
	case "BL", "SL", "TBL":
		return "SURROUND_LEFT"
	case "BR", "SR", "TBR":
		return "SURROUND_RIGHT"
	default:
		return "UNKNOWN"
	}
}

// RoleClass collapses left/right symmetry when only the speaker family is decidable.
func RoleClass(role string) string {
	switch RoleGroup(role) {
	case "CENTER":
		return "CENTER"
	case "LFE":
		return "LFE"
	case "SURROUND_LEFT", "SURROUND_RIGHT":
		return "SURROUND"
	case "FRONT_LEFT", "FRONT_RIGHT":
		return "FRONT"
	default:
		return "UNKNOWN"
	}
}
