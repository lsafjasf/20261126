package vttlayout

import (
	"fmt"
	"math"
	"sort"
)

type interval struct {
	start, end Milliseconds
}

func requiredDuration(c *Cue, reading ReadingModel) Milliseconds {
	runes := len([]rune(c.Text))
	byReading := Milliseconds(math.Ceil(float64(runes) / reading.MaxCharsPerSec * 1000))
	d := c.Duration()
	if byReading > d {
		d = byReading
	}
	if d < reading.MinDwellMS {
		d = reading.MinDwellMS
	}
	return d
}

func occupiedIntervals(doc *Document, target *Cue) []interval {
	var ivs []interval
	for _, c := range doc.Cues {
		if c.Order == target.Order || c.EndMS <= c.StartMS {
			continue
		}
		ivs = append(ivs, interval{c.StartMS, c.EndMS})
	}
	sort.Slice(ivs, func(i, j int) bool { return ivs[i].start < ivs[j].start })
	return ivs
}

// findSlotAfter finds the earliest free slot of length dur at or after
// earliest, avoiding all occupied intervals.
func findSlotAfter(ivs []interval, earliest, dur, gap Milliseconds) (Milliseconds, Milliseconds) {
	s := earliest
	for loop := 0; loop < len(ivs)+2; loop++ {
		advanced := false
		for _, iv := range ivs {
			if iv.end <= s {
				continue
			}
			if iv.start >= s+dur {
				continue
			}
			s = iv.end + gap
			advanced = true
		}
		if !advanced {
			break
		}
	}
	return s, s + dur
}

// findSlotBefore finds the latest free slot of length dur ending at or before
// latestEnd.
func findSlotBefore(ivs []interval, latestEnd, dur, gap Milliseconds) (Milliseconds, Milliseconds) {
	e := latestEnd
	for loop := 0; loop < len(ivs)+2; loop++ {
		advanced := false
		// Walk from the latest interval backwards.
		for i := len(ivs) - 1; i >= 0; i-- {
			iv := ivs[i]
			if iv.start >= e {
				continue
			}
			if iv.end <= e-dur {
				continue
			}
			e = iv.start - gap
			if e < dur {
				e = dur
			}
			advanced = true
		}
		if !advanced {
			break
		}
	}
	return e - dur, e
}

func timeCandidate(doc *Document, cv Canvas, opts Options, combo *Combo, target *Cue, reading ReadingModel, after bool) *Candidate {
	dur := requiredDuration(target, reading)
	ivs := occupiedIntervals(doc, target)
	gap := reading.GapMS

	var ns, ne Milliseconds
	var kind CandidateKind
	var title string
	if after {
		ns, ne = findSlotAfter(ivs, combo.EndMS+gap, dur, gap)
		kind = CandidateExtendTime
	} else {
		ns, ne = findSlotBefore(ivs, combo.StartMS-gap, dur, gap)
		kind = CandidateShiftTime
	}

	shift := ns - target.StartMS
	extension := ne - target.EndMS
	if extension < 0 {
		extension = 0
	}

	d2 := withMovedCue(doc, target, ns, ne)
	resolves := verifyWindow(d2, cv, opts, combo.StartMS, combo.EndMS)

	label := cueLabel(target)
	if after {
		switch {
		case extension > 0:
			title = fmt.Sprintf("延长并错峰：%s 移至 %s 之后，结束时间延长 %dms",
				label, FormatTimestamp(ns), extension)
		default:
			title = fmt.Sprintf("错峰后置：%s 移至 %s", label, FormatTimestamp(ns))
			kind = CandidateShiftTime
		}
	} else {
		title = fmt.Sprintf("错峰前置：%s 移至 %s–%s",
			label, FormatTimestamp(ns), FormatTimestamp(ne))
	}

	detail := fmt.Sprintf(
		"按 %.0f 字/秒与最短停留 %dms 估算需要 %dms；新时段 %s → %s（平移 %+dms，相对原结尾延长 %dms）。",
		reading.MaxCharsPerSec, reading.MinDwellMS, dur,
		FormatTimestamp(ns), FormatTimestamp(ne), shift, extension)

	return &Candidate{
		ComboID: combo.ID, Kind: kind, TargetOrder: target.Order,
		TargetLabel: label, Title: title, Detail: detail, Resolves: resolves,
		ShiftMS: shift, ExtensionMS: extension, NewStartMS: ns, NewEndMS: ne,
		Proposed: []ProposedCue{{
			ReplacesOrder: target.Order, StartMS: ns, EndMS: ne, Text: target.Text,
		}},
	}
}

func canvasCandidate(cv Canvas, opts Options, combo *Combo) *Candidate {
	extra := math.Ceil(combo.OverflowPx + 4)
	if extra < 1 {
		extra = 1
	}
	cv2 := cv
	cv2.Height += extra

	// Geometry only: the overflow bound is computed from the safe rect, so
	// extending canvas height directly adds available height.
	resolves := combo.TotalHeightPx <= cv2.SafeRect().H+0.5

	return &Candidate{
		ComboID: combo.ID, Kind: CandidateExtendCanvas,
		Title: fmt.Sprintf("延长画布：安全区高度增加 %.0fpx（竖屏总高 %.0f→%.0f）",
			extra, cv.Height, cv2.Height),
		Detail: fmt.Sprintf(
			"保持宽度与字号，把可用画布向下延长 %.0fpx 即可容纳该组合；适用于播放器允许更高画布/更小底部留白的场景。",
			extra),
		Resolves:      resolves,
		ExtraHeightPx: extra,
	}
}

func fontCandidate(doc *Document, cv Canvas, opts Options, combo *Combo) *Candidate {
	safe := cv.SafeRect()
	gap := opts.gap(cv)

	// Search a smaller font where the combo's peak height fits.
	for f := math.Floor(cv.FontPx); f >= cv.FontPx*0.55; f-- {
		cv2 := cv
		cv2.FontPx = f
		p := Layout(doc, cv2, opts)
		combos := DetectCombos(doc, cv2, p)
		hit := false
		for _, cb := range combos {
			if cb.StartMS < combo.EndMS && cb.EndMS > combo.StartMS {
				hit = true
				break
			}
		}
		if !hit {
			return &Candidate{
				ComboID: combo.ID, Kind: CandidateReduceFont,
				Title: fmt.Sprintf("缩小字号：%.0fpx → %.0fpx", cv.FontPx, f),
				Detail: fmt.Sprintf(
					"在 %.0fpx 字号下重新折行与堆叠后，该组合可进入 %.0fpx 安全高度（间距 %.0fpx）。",
					f, safe.H, gap),
				Resolves:  true,
				NewFontPx: f,
			}
		}
	}
	return nil
}
