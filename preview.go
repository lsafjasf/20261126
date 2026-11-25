package smptett

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// PreviewOptions controls preview time sampling and output.
type PreviewOptions struct {
	Times     []time.Duration
	FPS       float64
	MaxFrames int
	Strict    bool
	Input     string
}

// PreviewReport is persisted as manifest.json next to the frame images.
type PreviewReport struct {
	SchemaVersion int                    `json:"schema_version"`
	Input         string                 `json:"input,omitempty"`
	Canvas        CanvasReport           `json:"canvas"`
	Timeline      TimelineReport         `json:"timeline"`
	Layers        map[string]LayerReport `json:"layers"`
	Frames        []FrameReport          `json:"frames"`
	Visibility    VisibilityReport       `json:"visibility_checks"`
	Warnings      []string               `json:"warnings,omitempty"`
}

type CanvasReport struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

type TimelineReport struct {
	StartTime  string  `json:"start_time"`
	EndTime    string  `json:"end_time"`
	DurationMS int64   `json:"duration_ms"`
	FrameRate  float64 `json:"frame_rate"`
	TickRate   float64 `json:"tick_rate"`
}

type LayerReport struct {
	Name                string           `json:"name"`
	ResourceCount       int              `json:"resource_count"`
	IntrinsicallyHidden int              `json:"intrinsically_hidden"`
	Resources           []ResourceReport `json:"resources"`
}

type ResourceReport struct {
	ID              string   `json:"id"`
	Kind            string   `json:"kind"`
	BeginTime       string   `json:"begin_time"`
	EndTime         string   `json:"end_time"`
	BeginMS         int64    `json:"begin_ms"`
	EndMS           int64    `json:"end_ms"`
	Region          string   `json:"region,omitempty"`
	Rect            Rect     `json:"rect"`
	Text            string   `json:"text,omitempty"`
	MediaType       string   `json:"media_type,omitempty"`
	Source          string   `json:"source,omitempty"`
	SourceBytes     int      `json:"source_bytes,omitempty"`
	Opacity         float64  `json:"opacity"`
	IntrinsicHidden bool     `json:"intrinsic_hidden"`
	HiddenReasons   []string `json:"hidden_reasons,omitempty"`
	ParseWarning    string   `json:"parse_warning,omitempty"`
}

type FrameReport struct {
	Index            int                         `json:"index"`
	Time             string                      `json:"time"`
	TimeMS           int64                       `json:"time_ms"`
	File             string                      `json:"file"`
	FileSHA256       string                      `json:"file_sha256"`
	Layers           map[string]LayerFrameReport `json:"layers"`
	VisibleResources []string                    `json:"visible_resources,omitempty"`
	ActiveResources  []string                    `json:"active_resources,omitempty"`
	Checks           FrameChecks                 `json:"checks"`
}

type LayerFrameReport struct {
	File                 string   `json:"file"`
	SHA256               string   `json:"sha256"`
	NonTransparentPixels int      `json:"non_transparent_pixels"`
	VisibleResources     []string `json:"visible_resources,omitempty"`
}

type FrameChecks struct {
	HiddenAnnotationVisibleIDs []string `json:"hidden_annotation_visible_ids,omitempty"`
	Passed                     bool     `json:"passed"`
}

type VisibilityReport struct {
	BackgroundImages  LayerVisibilitySummary  `json:"background_images"`
	ForegroundText    LayerVisibilitySummary  `json:"foreground_text"`
	Annotations       LayerVisibilitySummary  `json:"annotations"`
	HiddenAnnotations HiddenAnnotationSummary `json:"hidden_annotations"`
	AllChecksPassed   bool                    `json:"all_checks_passed"`
}

type LayerVisibilitySummary struct {
	Total        int      `json:"total"`
	Hidden       int      `json:"intrinsically_hidden"`
	EverVisible  []string `json:"ever_visible,omitempty"`
	NeverVisible []string `json:"never_visible,omitempty"`
}

type HiddenAnnotationSummary struct {
	Total       int      `json:"total"`
	EverVisible []string `json:"ever_visible,omitempty"`
	Passed      bool     `json:"passed"`
}

// PreviewTimes derives deterministic sample times from cue boundaries.
func PreviewTimes(doc *Document, opts PreviewOptions) ([]time.Duration, error) {
	if opts.MaxFrames <= 0 {
		opts.MaxFrames = 200
	}
	duration := doc.Duration
	if duration < 0 {
		duration = 0
	}
	times := append([]time.Duration(nil), opts.Times...)
	if len(times) == 0 && opts.FPS > 0 {
		stepFloat := float64(time.Second) / opts.FPS
		if stepFloat > float64(^uint64(0)>>1) {
			return nil, fmt.Errorf("fps %v produces a time interval too large", opts.FPS)
		}
		step := time.Duration(stepFloat)
		if step <= 0 {
			return nil, fmt.Errorf("fps %v produces a sub-nanosecond interval", opts.FPS)
		}
		total := int(duration / step)
		if duration%step != 0 {
			total++
		}
		if total < 1 {
			total = 1
		}
		if total > opts.MaxFrames {
			for i := 0; i < opts.MaxFrames; i++ {
				if opts.MaxFrames == 1 {
					times = append(times, 0)
					break
				}
				t := time.Duration(int64(duration-time.Nanosecond) * int64(i) / int64(opts.MaxFrames-1))
				times = append(times, t)
			}
		} else {
			for i := 0; i < total; i++ {
				times = append(times, time.Duration(int64(step)*int64(i)))
			}
		}
		if len(times) == 0 {
			times = append(times, 0)
		}
		if duration > 0 && times[len(times)-1] < duration-time.Nanosecond {
			times = append(times, duration-time.Nanosecond)
		}
	}
	if len(times) == 0 {
		set := map[time.Duration]bool{0: true}
		if duration > 0 {
			set[duration-time.Nanosecond] = true
		}
		for _, res := range doc.Resources {
			if res.Begin >= 0 && res.Begin < duration {
				set[res.Begin] = true
			}
			if res.End > res.Begin && res.End <= duration {
				set[res.End-time.Nanosecond] = true
			}
		}
		for t := range set {
			times = append(times, t)
		}
	}

	sort.Slice(times, func(i, j int) bool { return times[i] < times[j] })
	times = dedupeTimes(times)
	if len(times) > opts.MaxFrames {
		times = evenlyLimit(times, opts.MaxFrames)
	}
	for _, t := range times {
		if t < 0 {
			return nil, fmt.Errorf("preview time %s is negative", t)
		}
	}
	return times, nil
}

func dedupeTimes(times []time.Duration) []time.Duration {
	if len(times) < 2 {
		return times
	}
	out := times[:1]
	for _, t := range times[1:] {
		if t != out[len(out)-1] {
			out = append(out, t)
		}
	}
	return out
}

func evenlyLimit(times []time.Duration, limit int) []time.Duration {
	if limit <= 0 || len(times) <= limit {
		return times
	}
	if limit == 1 {
		return []time.Duration{times[0]}
	}
	out := make([]time.Duration, 0, limit)
	for i := 0; i < limit; i++ {
		index := int(float64(i) * float64(len(times)-1) / float64(limit-1))
		out = append(out, times[index])
	}
	return dedupeTimes(out)
}

// GeneratePreview writes layer PNGs, composite frames, and manifest.json.
func GeneratePreview(doc *Document, outDir string, opts PreviewOptions) (*PreviewReport, error) {
	times, err := PreviewTimes(doc, opts)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return nil, fmt.Errorf("create output directory: %w", err)
	}
	for _, kind := range ResourceKinds {
		if err := os.MkdirAll(filepath.Join(outDir, "layers", string(kind)), 0o755); err != nil {
			return nil, fmt.Errorf("create layer directory: %w", err)
		}
	}
	if err := os.MkdirAll(filepath.Join(outDir, "frames"), 0o755); err != nil {
		return nil, fmt.Errorf("create frames directory: %w", err)
	}

	report := &PreviewReport{
		SchemaVersion: 1,
		Input:         opts.Input,
		Canvas:        CanvasReport{Width: doc.Width, Height: doc.Height},
		Timeline: TimelineReport{
			StartTime:  formatTimestamp(0),
			EndTime:    formatTimestamp(doc.Duration),
			DurationMS: doc.Duration.Milliseconds(),
			FrameRate:  doc.FrameRate,
			TickRate:   doc.TickRate,
		},
		Layers:   make(map[string]LayerReport),
		Warnings: append([]string(nil), doc.Warnings...),
	}
	for _, kind := range ResourceKinds {
		report.Layers[string(kind)] = layerReport(doc, kind)
	}

	everVisible := make(map[ResourceKind]map[string]bool)
	for _, kind := range ResourceKinds {
		everVisible[kind] = make(map[string]bool)
	}
	hiddenAnnotations := make(map[string]bool)
	for _, res := range doc.Resources {
		if res.Kind == KindAnnotation && res.intrinsicallyHidden() {
			hiddenAnnotations[res.ID] = true
		}
	}

	for index, t := range times {
		rendered := doc.RenderFrame(t)
		stamp := fmt.Sprintf("%03d-%012dns", index, t.Nanoseconds())
		frameReport := FrameReport{
			Index:  index,
			Time:   formatTimestamp(t),
			TimeMS: t.Milliseconds(),
			Layers: make(map[string]LayerFrameReport),
		}
		for _, kind := range ResourceKinds {
			rel := filepath.ToSlash(filepath.Join("layers", string(kind), stamp+".png"))
			digest, err := writePNG(filepath.Join(outDir, filepath.FromSlash(rel)), rendered.Layers[kind])
			if err != nil {
				return nil, err
			}
			frameReport.Layers[string(kind)] = LayerFrameReport{
				File:                 rel,
				SHA256:               digest,
				NonTransparentPixels: rendered.LayerPixelCount[kind],
				VisibleResources:     append([]string(nil), rendered.VisibleByLayer[kind]...),
			}
			for _, id := range rendered.VisibleByLayer[kind] {
				everVisible[kind][id] = true
			}
		}
		frameReport.File = filepath.ToSlash(filepath.Join("frames", stamp+".png"))
		digest, err := writePNG(filepath.Join(outDir, "frames", stamp+".png"), rendered.Composite)
		if err != nil {
			return nil, err
		}
		frameReport.FileSHA256 = digest
		for id, vis := range rendered.Visibility {
			if vis.Active {
				frameReport.ActiveResources = append(frameReport.ActiveResources, id)
			}
			if vis.Visible {
				frameReport.VisibleResources = append(frameReport.VisibleResources, id)
				if hiddenAnnotations[id] {
					frameReport.Checks.HiddenAnnotationVisibleIDs = append(frameReport.Checks.HiddenAnnotationVisibleIDs, id)
				}
			}
		}
		sort.Strings(frameReport.ActiveResources)
		sort.Strings(frameReport.VisibleResources)
		sort.Strings(frameReport.Checks.HiddenAnnotationVisibleIDs)
		frameReport.Checks.Passed = len(frameReport.Checks.HiddenAnnotationVisibleIDs) == 0
		report.Frames = append(report.Frames, frameReport)
	}

	report.Visibility = buildVisibilityReport(doc, report.Frames, everVisible, hiddenAnnotations)
	if opts.Strict && !report.Visibility.AllChecksPassed {
		return nil, fmt.Errorf("visibility checks failed: a hidden annotation became visible")
	}
	if err := writeManifest(filepath.Join(outDir, "manifest.json"), report); err != nil {
		return nil, err
	}
	return report, nil
}

func layerReport(doc *Document, kind ResourceKind) LayerReport {
	report := LayerReport{Name: string(kind)}
	for _, res := range doc.Resources {
		if res.Kind != kind {
			continue
		}
		report.ResourceCount++
		if res.intrinsicallyHidden() {
			report.IntrinsicallyHidden++
		}
		report.Resources = append(report.Resources, ResourceReport{
			ID:              res.ID,
			Kind:            string(res.Kind),
			BeginTime:       formatTimestamp(res.Begin),
			EndTime:         formatTimestamp(res.End),
			BeginMS:         res.Begin.Milliseconds(),
			EndMS:           res.End.Milliseconds(),
			Region:          res.RegionID,
			Rect:            res.Rect,
			Text:            res.Text,
			MediaType:       res.MediaType,
			Source:          res.Source,
			SourceBytes:     res.SourceBytes,
			Opacity:         res.Style.Opacity,
			IntrinsicHidden: res.intrinsicallyHidden(),
			HiddenReasons:   append([]string(nil), res.HiddenReasons...),
			ParseWarning:    res.ParseWarning,
		})
	}
	return report
}

func buildVisibilityReport(doc *Document, frames []FrameReport, everVisible map[ResourceKind]map[string]bool, hiddenAnnotations map[string]bool) VisibilityReport {
	report := VisibilityReport{
		BackgroundImages: summaryForKind(doc, KindBackgroundImage, everVisible[KindBackgroundImage]),
		ForegroundText:   summaryForKind(doc, KindForegroundText, everVisible[KindForegroundText]),
		Annotations:      summaryForKind(doc, KindAnnotation, everVisible[KindAnnotation]),
		HiddenAnnotations: HiddenAnnotationSummary{
			Total: len(hiddenAnnotations),
		},
	}
	seen := make(map[string]bool)
	for _, frame := range frames {
		for _, id := range frame.Checks.HiddenAnnotationVisibleIDs {
			seen[id] = true
		}
	}
	for id := range hiddenAnnotations {
		if seen[id] {
			report.HiddenAnnotations.EverVisible = append(report.HiddenAnnotations.EverVisible, id)
		}
	}
	sort.Strings(report.HiddenAnnotations.EverVisible)
	report.HiddenAnnotations.Passed = len(report.HiddenAnnotations.EverVisible) == 0
	report.AllChecksPassed = report.HiddenAnnotations.Passed
	for _, frame := range frames {
		if !frame.Checks.Passed {
			report.AllChecksPassed = false
		}
	}
	return report
}

func summaryForKind(doc *Document, kind ResourceKind, ever map[string]bool) LayerVisibilitySummary {
	var summary LayerVisibilitySummary
	for _, res := range doc.Resources {
		if res.Kind != kind {
			continue
		}
		summary.Total++
		if res.intrinsicallyHidden() {
			summary.Hidden++
		}
		if ever[res.ID] {
			summary.EverVisible = append(summary.EverVisible, res.ID)
		} else {
			summary.NeverVisible = append(summary.NeverVisible, res.ID)
		}
	}
	sort.Strings(summary.EverVisible)
	sort.Strings(summary.NeverVisible)
	return summary
}

func writePNG(path string, img image.Image) (string, error) {
	var buf bytes.Buffer
	encoder := png.Encoder{CompressionLevel: png.BestSpeed}
	if err := encoder.Encode(&buf, img); err != nil {
		return "", fmt.Errorf("encode %s: %w", path, err)
	}
	sum := sha256.Sum256(buf.Bytes())
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		return "", fmt.Errorf("write %s: %w", path, err)
	}
	return hex.EncodeToString(sum[:]), nil
}

func writeManifest(path string, report *PreviewReport) error {
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("encode manifest: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write manifest: %w", err)
	}
	return nil
}
