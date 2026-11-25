package analyze

import (
	"fmt"
	"time"

	"assclip/ass"
	"assclip/layout"
)

const epsilon = 0.01

// Analyze converts every Dialogue cue and reports clipping/safe-area hazards.
func Analyze(script *ass.Script, cfg Config) Report {
	report := Report{
		Script: ScriptReport{
			PlayResX:      cfg.Canvas.ScriptWidth,
			PlayResY:      cfg.Canvas.ScriptHeight,
			WrapStyle:     script.Info.WrapStyle,
			ScaledBorders: script.Info.ScaledBorderAndShadow,
			StyleCount:    len(script.StyleList),
			CueCount:      len(script.Events),
			ParseWarnings: append([]string(nil), script.Warnings...),
		},
		Target:   cfg.Canvas,
		Frame:    cfg.Canvas.Frame(),
		Warnings: append([]string(nil), script.Warnings...),
	}

	for i, event := range script.Events {
		cue := analyzeCue(script, event, i+1, cfg)
		report.Cues = append(report.Cues, cue)
		report.Summary.Cues++
		if cue.Clip != nil {
			report.Summary.CuesWithClip++
		}
		hasError := false
		for _, conflict := range cue.Conflicts {
			report.Summary.Conflicts++
			switch conflict.Severity {
			case "error":
				report.Summary.Errors++
				hasError = true
			case "warning":
				report.Summary.Warnings++
			}
		}
		if hasError {
			report.Summary.CuesWithErrors++
		}
	}
	return report
}

func analyzeCue(script *ass.Script, event ass.Event, index int, cfg Config) CueReport {
	style, styleWarning := resolveStyle(script, event.Style)
	override, overrideWarnings := ass.ParseOverride(event.Text)
	insets := layout.ResolveInsets(style, event)
	pixelInsets := cfg.Canvas.MapInsets(insets)
	alignment := style.Alignment
	if override.HasAlignment {
		alignment = override.Alignment
	}
	if alignment < 1 || alignment > 9 {
		alignment = 2
	}
	safe := cfg.Canvas.SafeArea(insets, alignment)

	cue := CueReport{
		Index:         index,
		Line:          event.Line,
		Start:         formatTime(event.Start),
		End:           formatTime(event.End),
		Style:         event.Style,
		Alignment:     alignment,
		Text:          override.VisibleText,
		MarginsScript: insets,
		MarginsPixels: pixelInsets,
		SafeArea:      safe,
		Conflicts:     []Conflict{},
	}
	if styleWarning != "" {
		cue.Warnings = append(cue.Warnings, styleWarning)
	}
	cue.Warnings = append(cue.Warnings, overrideWarnings...)

	textBox, hasText := estimateTextBounds(style, override, cfg.Canvas, insets)
	if hasText {
		cue.EstimatedText = &textBox
	}

	if len(override.Clips) > 0 {
		clip := override.Clips[len(override.Clips)-1]
		cue.Clip = makeClipReport(clip, cfg.Canvas)
		if len(override.Clips) > 1 {
			cue.Warnings = append(cue.Warnings,
				fmt.Sprintf("%d clip tags; reporting the last effective clip", len(override.Clips)))
		}
		if cue.Clip.FrameOverflow.Any() {
			o := cue.Clip.FrameOverflow
			addConflict(&cue, Conflict{
				Code: "clip_outside_frame", Severity: "warning",
				Message:  "clip bounds extend beyond the output frame",
				Overflow: &o,
			})
		}
	}

	if hasText {
		addTextConflicts(&cue, textBox, cfg.Canvas.Frame(), safe.Rect)
		addSubjectConflicts(&cue, textBox, cfg.Subjects)
	}
	return cue
}

func resolveStyle(script *ass.Script, name string) (ass.Style, string) {
	if style, ok := script.StyleByName(name); ok {
		return style, ""
	}
	if len(script.StyleList) > 0 {
		return script.StyleList[0],
			fmt.Sprintf("style %q not found; using %q", name, script.StyleList[0].Name)
	}
	return ass.Style{
		Name: "Default", FontSize: 48, ScaleX: 100, ScaleY: 100,
		Alignment: 2, MarginL: 10, MarginR: 10, MarginV: 10,
	}, "no styles found; using built-in Default"
}

func makeClipReport(clip ass.Clip, canvas layout.Canvas) *ClipReport {
	bounds := canvas.MapBounds(clip.Bounds)
	return &ClipReport{
		Mode:          clip.Mode,
		Source:        clip.Source,
		IsVector:      clip.IsVector,
		PointCount:    len(clip.Points),
		ScriptBounds:  clip.Bounds,
		Bounds:        bounds,
		ClampedBounds: layout.Clamp(bounds, canvas.Frame()),
		FrameOverflow: layout.OutsideOverflow(bounds, canvas.Frame()),
	}
}

func addTextConflicts(cue *CueReport, text, frame, safe layout.Rect) {
	if overflow := layout.OutsideOverflow(text, frame); overflow.Any() {
		o := overflow
		addConflict(cue, Conflict{
			Code: "text_outside_frame", Severity: "error",
			Message:  "estimated text extends beyond the output frame",
			Overflow: &o,
		})
	}
	if overflow := layout.OutsideOverflow(text, safe); overflow.Any() {
		o := overflow
		addConflict(cue, Conflict{
			Code: "text_outside_safe_area", Severity: "warning",
			Message:  "estimated text crosses a margin-derived safe-area edge",
			Overflow: &o,
		})
	}
	if cue.Clip == nil {
		return
	}

	if cue.Clip.Mode == ass.ClipInverse {
		if overlap, ok := text.Intersect(cue.Clip.Bounds); ok {
			addConflict(cue, Conflict{
				Code: "text_hidden_by_inverse_clip", Severity: "error",
				Message: "estimated text intersects the excluded region of an inverse clip",
				Area:    overlap.Area(),
			})
		}
		return
	}
	if o := layout.OutsideOverflow(text, cue.Clip.Bounds); o.Any() {
		addConflict(cue, Conflict{
			Code: "text_cropped_by_clip", Severity: "error",
			Message:  "the clip bounds do not cover the estimated text box",
			Overflow: &o,
		})
	}
}

func addSubjectConflicts(cue *CueReport, text layout.Rect, subjects []layout.Rect) {
	for i, subject := range subjects {
		overlap, ok := text.Intersect(subject)
		if !ok || overlap.Area() <= epsilon {
			continue
		}
		visibleArea := overlap.Area()
		if cue.Clip != nil {
			clipOverlap, clipped := overlap.Intersect(cue.Clip.Bounds)
			switch cue.Clip.Mode {
			case ass.ClipNormal:
				if !clipped {
					visibleArea = 0
				} else {
					visibleArea = clipOverlap.Area()
				}
			case ass.ClipInverse:
				if clipped {
					visibleArea -= clipOverlap.Area()
				}
			}
		}
		if visibleArea <= epsilon {
			continue
		}
		addConflict(cue, Conflict{
			Code: "text_visible_over_subject", Severity: "error",
			Message: fmt.Sprintf("estimated visible text overlaps protected subject zone %d", i+1),
			Area:    visibleArea,
			Zone:    fmt.Sprintf("subject_%d", i+1),
		})
	}
}

func addConflict(cue *CueReport, conflict Conflict) {
	cue.Conflicts = append(cue.Conflicts, conflict)
}

func formatTime(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	totalCS := int64(d / (10 * time.Millisecond))
	cs := totalCS % 100
	totalSeconds := totalCS / 100
	s := totalSeconds % 60
	m := (totalSeconds / 60) % 60
	h := totalSeconds / 3600
	return fmt.Sprintf("%d:%02d:%02d.%02d", h, m, s, cs)
}
