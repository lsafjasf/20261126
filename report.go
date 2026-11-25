package channelmap

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"strings"
)

const ReportSchema = "wavchannel.report/v1"

type Status string

const (
	StatusConsistent   Status = "consistent"
	StatusConflict     Status = "conflict"
	StatusUndecidable  Status = "undecidable"
	StatusInvalidInput Status = "invalid_input"
)

type Confidence string

const (
	ConfidenceHigh   Confidence = "high"
	ConfidenceMedium Confidence = "medium"
	ConfidenceLow    Confidence = "low"
	ConfidenceNone   Confidence = "none"
)

type Options struct {
	Path                  string
	Profile               string
	Signatures            *SignatureSet
	UseCalibrationProfile bool
	IncludeSampleSHA256   bool
}

type Report struct {
	Schema          string            `json:"schema"`
	Status          Status            `json:"status"`
	Confidence      Confidence        `json:"confidence"`
	Summary         string            `json:"summary"`
	Input           *InputInfo        `json:"input,omitempty"`
	Codec           CodecReport       `json:"codec"`
	Mask            MaskReport        `json:"mask"`
	DeclaredMapping []MappingEntry    `json:"declared_mapping"`
	Features        []Features        `json:"features,omitempty"`
	InferredMapping []InferredEntry   `json:"inferred_mapping"`
	Candidates      []Candidate       `json:"candidates,omitempty"`
	Conflicts       []Conflict        `json:"conflicts"`
	Undecidable     []UndecidableItem `json:"undecidable"`
	Evidence        []string          `json:"evidence"`
	Repair          *Repair           `json:"repair,omitempty"`
}

type InputInfo struct {
	Path         string `json:"path,omitempty"`
	Bytes        int    `json:"bytes"`
	SHA256       string `json:"sha256,omitempty"`
	ChunkCount   int    `json:"chunk_count"`
	FrameCount   int    `json:"frame_count"`
	ChannelCount int    `json:"channel_count"`
	SampleRate   int    `json:"sample_rate"`
	DurationMS   int64  `json:"duration_ms"`
}

type CodecReport struct {
	Format
	DeclarationStatus string   `json:"declaration_status"`
	Issues            []string `json:"issues,omitempty"`
}

type MaskReport struct {
	Value          string   `json:"value"`
	Present        bool     `json:"present"`
	PopCount       int      `json:"popcount"`
	StandardLayout string   `json:"standard_layout,omitempty"`
	OrderRule      string   `json:"order_rule"`
	Source         string   `json:"source"`
	Warnings       []string `json:"warnings,omitempty"`
}

type MappingEntry struct {
	WireIndex int    `json:"wire_index"`
	Bit       uint32 `json:"bit,omitempty"`
	Role      string `json:"role"`
	Label     string `json:"label,omitempty"`
	Source    string `json:"source"`
}

type InferredEntry struct {
	WireIndex    int             `json:"wire_index"`
	DeclaredRole string          `json:"declared_role"`
	InferredRole string          `json:"inferred_role,omitempty"`
	Status       string          `json:"status"`
	Confidence   Confidence      `json:"confidence"`
	Score        float64         `json:"score,omitempty"`
	Evidence     []string        `json:"evidence,omitempty"`
	Alternatives []RoleCandidate `json:"alternatives,omitempty"`
}

type RoleCandidate struct {
	Role  string  `json:"role"`
	Cost  float64 `json:"cost"`
	Score float64 `json:"score"`
}

type Candidate struct {
	RolesByWire []string        `json:"roles_by_wire"`
	TotalCost   float64         `json:"total_cost"`
	AverageCost float64         `json:"average_cost"`
	Confidence  Confidence      `json:"confidence"`
	Assignments []CandidateRole `json:"assignments"`
}

type CandidateRole struct {
	WireIndex  int      `json:"wire_index"`
	Role       string   `json:"role"`
	Cost       float64  `json:"cost"`
	Evidence   []string `json:"evidence,omitempty"`
	Violations []string `json:"violations,omitempty"`
}

type Conflict struct {
	Code          string     `json:"code"`
	Severity      string     `json:"severity"`
	Message       string     `json:"message"`
	WireIndices   []int      `json:"wire_indices,omitempty"`
	DeclaredRoles []string   `json:"declared_roles,omitempty"`
	ObservedRoles []string   `json:"observed_roles,omitempty"`
	Confidence    Confidence `json:"confidence"`
	Evidence      []string   `json:"evidence,omitempty"`
}

type UndecidableItem struct {
	Code        string   `json:"code"`
	Message     string   `json:"message"`
	WireIndices []int    `json:"wire_indices,omitempty"`
	Roles       []string `json:"roles,omitempty"`
	Reason      string   `json:"reason"`
	Candidates  []string `json:"candidates,omitempty"`
}

type Repair struct {
	SourceIndicesForMaskOrder []int    `json:"source_indices_for_mask_order"`
	WireToSpeaker             []string `json:"wire_to_speaker"`
	Description               string   `json:"description"`
}

func AnalyzeReader(r io.Reader, opts Options) (*Report, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	return AnalyzeBytes(data, opts)
}

func AnalyzeBytes(data []byte, opts Options) (*Report, error) {
	file, err := Parse(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	if opts.Path == "" {
		opts.Path = ""
	}
	return Analyze(file, data, opts), nil
}

// Analyze combines mask order, codec declaration, and sample evidence.
func Analyze(file *File, raw []byte, opts Options) *Report {
	r := &Report{
		Schema:          ReportSchema,
		Status:          StatusConsistent,
		Confidence:      ConfidenceNone,
		Codec:           CodecReport{Format: file.Format, DeclarationStatus: "declared"},
		Conflicts:       []Conflict{},
		Undecidable:     []UndecidableItem{},
		Evidence:        []string{},
		InferredMapping: []InferredEntry{},
	}
	if opts.Path != "" || len(raw) > 0 {
		info := &InputInfo{Path: opts.Path, Bytes: len(raw), ChunkCount: len(file.Chunks), FrameCount: file.Frames, ChannelCount: file.Format.Channels, SampleRate: file.Format.SampleRate}
		if file.Format.SampleRate > 0 {
			info.DurationMS = int64(float64(file.Frames) / float64(file.Format.SampleRate) * 1000)
		}
		if opts.IncludeSampleSHA256 && len(raw) > 0 {
			sum := sha256.Sum256(raw)
			info.SHA256 = hex.EncodeToString(sum[:])
		}
		r.Input = info
	}

	buildMaskAndDeclared(r, file)
	checkCodec(r, file)
	if len(file.Samples) > 0 {
		r.Features = ExtractFeatures(file.Samples, file.Format.SampleRate)
	}

	sig := resolveSignatures(r, opts)
	inferMapping(r, file, sig)
	finalizeStatus(r)
	return r
}

func resolveSignatures(r *Report, opts Options) *SignatureSet {
	if opts.Signatures != nil {
		r.Evidence = append(r.Evidence, "using caller-supplied role signature set")
		return opts.Signatures
	}
	defaults := DefaultCalibrationSignatures()
	if opts.UseCalibrationProfile || opts.Profile == "calibration-v1" {
		r.Evidence = append(r.Evidence, "using built-in calibration profile wavchannel-calibration-v1")
		return &defaults
	}
	if opts.Profile != "auto" {
		return nil
	}

	roles := make([]string, 0, len(r.DeclaredMapping))
	for _, m := range r.DeclaredMapping {
		roles = append(roles, m.Role)
	}
	problem := newMatchProblem(roles, &defaults, r.Features, nil)
	candidates := problem.enumerate(1)
	if len(candidates) > 0 && candidates[0].AverageCost <= 12 &&
		(candidates[0].Confidence == ConfidenceHigh || candidates[0].Confidence == ConfidenceMedium) {
		r.Evidence = append(r.Evidence, "auto-detected bundled calibration tone features")
		return &defaults
	}
	r.Evidence = append(r.Evidence, "no confident built-in calibration profile match; using generic feature evidence and undecidable role identities")
	return nil
}

func buildMaskAndDeclared(r *Report, f *File) {
	ch := f.Format.Channels
	mr := MaskReport{OrderRule: "WAV interleaved order follows dwChannelMask bits from least to most significant set bit"}
	var speakers []Speaker

	switch {
	case f.Format.HasChannelMask && f.Format.ChannelMask != 0:
		mr.Present = true
		mr.Value = fmt.Sprintf("0x%08X", f.Format.ChannelMask)
		mr.PopCount = MaskPopCount(f.Format.ChannelMask)
		mr.Source = "WAVEFORMATEXTENSIBLE.dwChannelMask"
		speakers = MaskRoles(f.Format.ChannelMask)
	case f.Format.HasChannelMask:
		mr.Present = true
		mr.Value = "0x00000000"
		mr.PopCount = 0
		mr.Source = "dwChannelMask is zero"
		mr.Warnings = append(mr.Warnings, "zero channel mask does not declare speaker positions")
		r.Undecidable = append(r.Undecidable, UndecidableItem{Code: "ZERO_CHANNEL_MASK", Message: "dwChannelMask=0 does not assign speaker positions", Reason: "zero can denote direct/undefined ordering rather than a speaker layout"})
	default:
		mr.Value = "absent"
		mr.Source = "plain WAVEFORMATEX has no channel mask"
		if mask, ok := DefaultMaskForChannels(ch); ok {
			mr.Value = fmt.Sprintf("0x%08X", mask)
			mr.PopCount = MaskPopCount(mask)
			mr.Source = "low-confidence conventional default for channel count"
			speakers = MaskRoles(mask)
			mr.Warnings = append(mr.Warnings, "mask is not explicitly declared; conventional layout cannot prove actual arrangement")
			r.Undecidable = append(r.Undecidable, UndecidableItem{Code: "MASK_NOT_DECLARED", Message: "WAV has no explicit dwChannelMask", Reason: "a plain fmt chunk only gives channel count, not speaker order", Candidates: []string{mr.Value}})
		}
	}

	r.Mask = mr
	if f.Format.ChannelMask == Mask5Point1 || (!mr.Present && ch == 6) {
		r.Mask.StandardLayout = "5.1 (FL, FR, FC, LFE, BL, BR)"
	} else if f.Format.ChannelMask == Mask5Point1Side {
		r.Mask.StandardLayout = "5.1 side-surround (FL, FR, FC, LFE, SL, SR)"
	}

	for i := 0; i < ch; i++ {
		entry := MappingEntry{WireIndex: i, Role: fmt.Sprintf("CHANNEL_%d", i), Source: "undeclared"}
		if i < len(speakers) {
			sp := speakers[i]
			entry.Bit, entry.Role, entry.Label = sp.Bit, sp.Role, sp.Description
			if mr.Present && f.Format.ChannelMask != 0 {
				entry.Source = "explicit-mask-bit-order"
			} else {
				entry.Source = "conventional-default"
			}
		}
		r.DeclaredMapping = append(r.DeclaredMapping, entry)
		r.InferredMapping = append(r.InferredMapping, InferredEntry{WireIndex: i, DeclaredRole: entry.Role, Status: "undecidable", Confidence: ConfidenceNone})
	}

	if mr.PopCount > 0 && mr.PopCount != ch {
		r.Conflicts = append(r.Conflicts, Conflict{
			Code: "MASK_CHANNEL_COUNT_MISMATCH", Severity: "error", Confidence: ConfidenceHigh,
			Message:  fmt.Sprintf("dwChannelMask names %d positions but fmt declares %d channels", mr.PopCount, ch),
			Evidence: []string{fmt.Sprintf("mask=%s popcount=%d nChannels=%d", mr.Value, mr.PopCount, ch)},
		})
	}
	var unknownBits []int
	for _, sp := range speakers {
		if strings.HasPrefix(sp.Role, "UNKNOWN_BIT_") {
			unknownBits = append(unknownBits, sp.BitIndex)
		}
	}
	if len(unknownBits) > 0 {
		r.Mask.Warnings = append(r.Mask.Warnings, fmt.Sprintf("unknown speaker bits: %v", unknownBits))
		r.Undecidable = append(r.Undecidable, UndecidableItem{Code: "UNKNOWN_MASK_BITS", Message: "channel mask contains unknown speaker bits", Reason: "one or more bits are not in the known WAV speaker-position table", Candidates: []string{fmt.Sprintf("%v", unknownBits)}})
	}
}

func checkCodec(r *Report, f *File) {
	cr := &r.Codec
	if f.Format.FormatTag == FormatExtensible && f.Format.SubFormat == nil {
		cr.Issues = append(cr.Issues, "extensible format tag without subformat GUID")
	}
	if f.Format.FormatTag != FormatExtensible && f.Format.HasChannelMask {
		cr.Issues = append(cr.Issues, "channel mask present outside WAVE_FORMAT_EXTENSIBLE")
	}
	expectedAlign := f.Format.Channels * (f.Format.BitsPerSample / 8)
	uncompressed := f.Format.Codec.SampleKind == "pcm-integer" || f.Format.Codec.SampleKind == "ieee-float"
	if uncompressed && f.Format.BitsPerSample > 0 && f.Format.BitsPerSample%8 == 0 && f.Format.BlockAlign != expectedAlign {
		cr.Issues = append(cr.Issues, fmt.Sprintf("nBlockAlign=%d, expected %d for uncompressed %d-bit × %d channels", f.Format.BlockAlign, expectedAlign, f.Format.BitsPerSample, f.Format.Channels))
	}
	if f.Format.AvgBytesPerSecond > 0 && f.Format.BlockAlign > 0 {
		expectedRate := uint64(f.Format.SampleRate) * uint64(f.Format.BlockAlign)
		if uint64(f.Format.AvgBytesPerSecond) != expectedRate {
			cr.Issues = append(cr.Issues, fmt.Sprintf("nAvgBytesPerSec=%d, expected %d", f.Format.AvgBytesPerSecond, expectedRate))
		}
	}
	if !f.Format.Codec.Decodable {
		r.Undecidable = append(r.Undecidable, UndecidableItem{
			Code:    "CONTENT_DECODE_UNAVAILABLE",
			Message: fmt.Sprintf("codec %s cannot be sample-decoded by the built-in PCM/float reader", f.Format.Codec.ID),
			Reason:  "compressed or legacy codec payload requires a codec implementation; mask/codec metadata can still be checked",
		})
	}
	for _, issue := range cr.Issues {
		r.Evidence = append(r.Evidence, "codec issue: "+issue)
	}
	if len(cr.Issues) == 0 {
		cr.DeclarationStatus = "consistent"
	} else {
		cr.DeclarationStatus = "issues"
		for _, issue := range cr.Issues {
			severity := "error"
			if strings.HasPrefix(issue, "nAvgBytesPerSec=") {
				severity = "warning"
			}
			r.Conflicts = append(r.Conflicts, Conflict{
				Code: "CODEC_DECLARATION_MISMATCH", Severity: severity, Confidence: ConfidenceHigh,
				Message:  issue,
				Evidence: []string{fmt.Sprintf("format_tag=0x%04x codec=%s", f.Format.FormatTag, f.Format.Codec.ID)},
			})
		}
	}
}

func finalizeStatus(r *Report) {
	hasHighConflict := false
	for _, c := range r.Conflicts {
		if c.Severity == "error" || c.Confidence == ConfidenceHigh || c.Confidence == ConfidenceMedium {
			hasHighConflict = true
		}
		r.Confidence = maxConfidence(r.Confidence, c.Confidence)
	}
	if hasHighConflict {
		r.Status = StatusConflict
	} else if len(r.Undecidable) > 0 || r.Confidence == ConfidenceLow {
		r.Status = StatusUndecidable
	} else {
		r.Status = StatusConsistent
	}

	switch {
	case hasHighConflict:
		if r.Summary == "" {
			r.Summary = fmt.Sprintf("检测到 %d 个可解释冲突；详见 conflicts 与 mapping。", len(r.Conflicts))
		}
	case len(r.Undecidable) > 0:
		r.Status = StatusUndecidable
		if r.Summary == "" {
			r.Summary = "元数据可解析，但部分声道身份不可由现有证据判定。"
		}
	default:
		r.Status = StatusConsistent
		if r.Summary == "" {
			r.Summary = "掩码、codec 声明与样本证据一致。"
		}
	}
}

func maxConfidence(a, b Confidence) Confidence {
	rank := map[Confidence]int{ConfidenceNone: 0, ConfidenceLow: 1, ConfidenceMedium: 2, ConfidenceHigh: 3}
	if rank[b] > rank[a] {
		return b
	}
	return a
}
