package bwf

import (
	"encoding/binary"
	"fmt"
	"math"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func Analyze(path, projectPath string) (*Report, error) {
	f, err := Open(path)
	if err != nil {
		return nil, err
	}
	r := &Report{
		File:        path,
		GeneratedAt: time.Now().UTC(),
		Container:   f.Container,
	}
	r.Semantics = append(r.Semantics,
		"RIFF/WAVE 的 chunk 顺序本身不证明时间正确性；只有可解析尺寸、帧边界和元数据内容能提供证据。",
		"bext.TimeReference 是首样本的绝对样本序号（按 fmt 采样率计数），不是文件时长；它与 data 帧数相减没有直接语义。",
		"iXML 的 START_TIME 是时间码标签，必须结合 TIMECODE_RATE、DF/NDF 和 TIMECODE_SAMPLE_RATE 才能映射到样本。",
	)
	for _, ch := range f.Container.Chunks {
		if ch.Truncated {
			r.add("chunk_truncated", "error", fmt.Sprintf("chunk %q at %d declares more payload than the file contains", ch.ID, ch.Offset))
		}
		if ch.Placeholder && ch.ID == "data" {
			r.add("data_size_placeholder", "warning", "data chunk still has a 32-bit size placeholder and no usable RF64 override")
		}
	}
	if f.Container.Form != "WAVE" {
		r.add("not_wave", "error", fmt.Sprintf("RIFF form is %q, not WAVE", f.Container.Form))
	}
	if f.Container.Kind == "RF64" {
		if f.Chunk("ds64") == nil {
			r.add("rf64_without_ds64", "error", "RF64 file has no ds64 chunk")
		}
	}

	fmtp, fmtErr := f.ParseFmt()
	if fmtErr != nil {
		r.add("fmt_error", "error", fmtErr.Error())
	} else {
		r.Fmt = fmtp
		r.checkFmt(fmtp)
	}
	r.Audio = f.audio(fmtp, r)

	bext, bextErr := f.ParseBEXT()
	if bextErr == nil {
		r.BEXT = bext
		r.checkBEXT(bext)
	} else if f.Chunk("bext") != nil {
		r.add("bext_error", "error", bextErr.Error())
	}
	ix, ixErr := f.ParseIXML()
	if ixErr == nil {
		r.IXML = ix
		r.checkIXML(ix, fmtp)
	} else if f.Chunk("iXML") != nil {
		r.add("ixml_error", "error", ixErr.Error())
	}
	if projectPath != "" {
		p, err := ParseProject(projectPath)
		if err != nil {
			return nil, fmt.Errorf("project metadata: %w", err)
		}
		r.Project = p
		r.checkProject(p, fmtp)
	}
	r.collectStarts(fmtp)
	r.collectComparisons()
	r.buildConclusions()
	return r, nil
}

func (r *Report) add(code, severity, message string) {
	r.Findings = append(r.Findings, Finding{Code: code, Severity: severity, Message: message})
}

func (r *Report) checkFmt(f *Fmt) {
	if f.Channels == 0 {
		r.add("fmt_zero_channels", "error", "fmt declares zero channels")
	}
	if f.SampleRate == 0 {
		r.add("fmt_zero_rate", "error", "fmt declares zero sample rate")
	}
	if f.BlockAlign == 0 {
		r.add("fmt_zero_block_align", "error", "fmt declares zero block alignment")
	}
	if f.SampleRate > 0 && f.BlockAlign > 0 {
		want := uint32(f.SampleRate) * uint32(f.BlockAlign)
		if f.ByteRate != want {
			r.add("fmt_byte_rate", "warning", fmt.Sprintf("byteRate=%d, expected sampleRate*blockAlign=%d", f.ByteRate, want))
		}
	}
	if f.Channels > 0 && f.BitsPerSample > 0 && f.BlockAlign > 0 {
		want := (uint32(f.Channels)*uint32(f.BitsPerSample) + 7) / 8
		if uint32(f.BlockAlign) < want {
			r.add("fmt_block_align_short", "error", fmt.Sprintf("blockAlign=%d is smaller than channels*bits/8=%d", f.BlockAlign, want))
		} else if uint32(f.BlockAlign) > want {
			r.add("fmt_block_align_padded", "info", fmt.Sprintf("blockAlign=%d exceeds channels*bits/8=%d; this can be legitimate sample padding", f.BlockAlign, want))
		}
	}
}

func (r *Report) checkBEXT(b *BEXT) {
	if b.Short {
		r.add("bext_short", "warning", fmt.Sprintf("bext payload is %d bytes; fixed fields need at least 348", b.PayloadSize))
	}
	if b.Version > 2 {
		r.add("bext_version_unknown", "info", fmt.Sprintf("bext version=%d is newer than the parser's known versions", b.Version))
	}
	if b.OriginationDate == "" || b.OriginationTime == "" {
		r.add("bext_origination_empty", "info", "bext origination date/time is incomplete")
	}
}

func (r *Report) checkIXML(x *IXML, fmtp *Fmt) {
	if !strings.EqualFold(x.Root, "BWFXML") {
		r.add("ixml_root", "warning", fmt.Sprintf("iXML root is %q, expected BWFXML", x.Root))
	}
	names := []string{"START_TIME", "START_TIMECODE", "BWF_TIME_REFERENCE", "TIMECODE_RATE", "TIMECODE_FLAG", "TIMECODE_SAMPLE_RATE"}
	for _, name := range names {
		vals := x.Values(name)
		if len(vals) > 1 {
			unique := make(map[string]bool)
			for _, v := range vals {
				unique[v] = true
			}
			if len(unique) > 1 {
				r.add("ixml_duplicate_conflict", "warning", fmt.Sprintf("iXML has conflicting %s values: %q", name, vals))
			}
		}
	}
	if x.TimecodeRate != "" {
		if _, err := ParseRate(x.TimecodeRate); err != nil {
			r.add("ixml_bad_rate", "warning", err.Error())
		}
	}
	if (x.TimecodeFlag != "" && strings.EqualFold(x.TimecodeFlag, "DF")) && !strings.Contains(strings.ToUpper(x.TimecodeFlag), "NDF") {
		if x.StartTime != nil && !x.StartTime.DropFrame && !strings.Contains(x.TimecodeRate, "/") {
			r.add("ixml_df_flag_mismatch", "warning", "TIMECODE_FLAG says DF but START_TIME has no semicolon/DF marker")
		}
	}
	if strings.EqualFold(strings.TrimSpace(x.TimecodeFlag), "NDF") && x.StartTime != nil && x.StartTime.DropFrame {
		r.add("ixml_ndf_flag_mismatch", "warning", "TIMECODE_FLAG says NDF but START_TIME uses a semicolon/drop-frame marker")
	}
	if _, ok := x.firstValue("START_TIME", "START_TIMECODE"); ok && x.StartTime == nil {
		r.add("ixml_start_unmapped", "warning", "iXML START_TIME could not be mapped to samples; check TIMECODE_RATE")
	}
	if fmtp == nil {
		return
	}
	if x.TimecodeSampleRate != 0 && x.TimecodeSampleRate != fmtp.SampleRate {
		r.add("ixml_tc_rate_diff", "info", fmt.Sprintf("iXML TIMECODE_SAMPLE_RATE=%d differs from fmt sampleRate=%d", x.TimecodeSampleRate, fmtp.SampleRate))
	}
}

func (r *Report) checkProject(p *ProjectMeta, fmtp *Fmt) {
	if fmtp == nil {
		return
	}
	if p.SampleRate != 0 && p.SampleRate != fmtp.SampleRate {
		r.add("project_sample_rate_diff", "warning", fmt.Sprintf("project sampleRate=%d differs from fmt sampleRate=%d", p.SampleRate, fmtp.SampleRate))
	}
	if p.DurationSamples != nil && int64(r.Audio.Frames) != *p.DurationSamples {
		delta := *p.DurationSamples - int64(r.Audio.Frames)
		sev := "warning"
		if abs64(delta) <= 1000 {
			sev = "info"
		}
		r.add("project_duration_diff", sev, fmt.Sprintf("project durationSamples=%d, data represents %d frames (delta=%+d samples)", *p.DurationSamples, r.Audio.Frames, delta))
	}
	if p.DurationSeconds != nil && fmtp.SampleRate > 0 {
		want := float64(r.Audio.Frames) / float64(fmtp.SampleRate)
		if math.Abs(*p.DurationSeconds-want) > 0.5/float64(fmtp.SampleRate) {
			r.add("project_seconds_diff", "warning", fmt.Sprintf("project durationSeconds=%.9f, audio duration=%.9f", *p.DurationSeconds, want))
		}
	}
	if p.ExpectedFileName != "" && !strings.EqualFold(filepath.Base(p.ExpectedFileName), filepath.Base(r.File)) {
		r.add("project_filename_diff", "info", fmt.Sprintf("project expects filename %q", p.ExpectedFileName))
	}
}

func (f *File) audio(fmtp *Fmt, r *Report) Audio {
	var a Audio
	for _, ch := range f.Container.Chunks {
		if ch.ID != "data" {
			continue
		}
		a.DataChunks = append(a.DataChunks, fmt.Sprintf("data@%d", ch.Offset))
		a.DataBytes += ch.Size
	}
	if len(a.DataChunks) == 0 {
		r.add("data_missing", "error", "no data chunk")
		return a
	}
	if len(a.DataChunks) > 1 {
		r.add("data_multiple", "warning", fmt.Sprintf("file has %d data chunks; frame total was summed", len(a.DataChunks)))
	}
	if fmtp == nil || fmtp.SampleRate == 0 {
		return a
	}
	frames := uint64(0)
	if fmtp.Compressed {
		if ch := f.Chunk("fact"); ch != nil && ch.Size >= 4 {
			p, err := f.Payload(ch, 4)
			if err == nil {
				frames = uint64(binary.LittleEndian.Uint32(p[:4]))
			}
		}
	}
	if frames == 0 && fmtp.BlockAlign > 0 {
		frames = a.DataBytes / uint64(fmtp.BlockAlign)
		a.PartialBytes = a.DataBytes % uint64(fmtp.BlockAlign)
	}
	a.Frames = frames
	a.DurationSeconds = float64(frames) / float64(fmtp.SampleRate)
	if a.PartialBytes > 0 {
		r.add("data_partial_frame", "warning", fmt.Sprintf("data payload has %d trailing bytes beyond a whole frame", a.PartialBytes))
	}
	if fmtp.Compressed && f.Chunk("fact") == nil {
		r.add("compressed_without_fact", "warning", "compressed fmt has no fact chunk; frame count is byte-derived only")
	}
	return a
}

func abs64(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}

func sortedFindings(r *Report) {
	sort.SliceStable(r.Findings, func(i, j int) bool {
		severity := map[string]int{"error": 0, "warning": 1, "info": 2}
		if severity[r.Findings[i].Severity] != severity[r.Findings[j].Severity] {
			return severity[r.Findings[i].Severity] < severity[r.Findings[j].Severity]
		}
		return r.Findings[i].Code < r.Findings[j].Code
	})
}

func (r *Report) collectStarts(fmtp *Fmt) {
	add := func(id, kind string, start int64, sampleRate uint32, rate, confidence, evidence string) {
		r.Starts = append(r.Starts, StartSource{
			ID: id, Kind: kind, Start: start, SampleRate: sampleRate,
			Rate: rate, Confidence: confidence, Evidence: evidence,
		})
	}
	if r.BEXT != nil {
		sr := uint32(0)
		if fmtp != nil {
			sr = fmtp.SampleRate
		}
		if r.BEXT.TimeReference > math.MaxInt64 {
			r.add("bext_time_reference_overflow", "error", "bext TimeReference exceeds the signed sample range supported by this report")
		} else {
			add("bext.timeReference", "absolute sample", int64(r.BEXT.TimeReference), sr, "",
				"normative for BWF first sample",
				fmt.Sprintf("low=%d high=%d, version=%d", r.BEXT.TimeReferenceLow, r.BEXT.TimeReferenceHigh, r.BEXT.Version))
		}
	}
	if r.IXML != nil {
		sr := uint32(0)
		if fmtp != nil {
			sr = fmtp.SampleRate
		}
		if r.IXML.DirectTimeReference != nil {
			if *r.IXML.DirectTimeReference > math.MaxInt64 {
				r.add("ixml_time_reference_overflow", "error", "iXML BWF_TIME_REFERENCE exceeds int64")
			} else {
				add("iXML.BWF_TIME_REFERENCE", "absolute sample", int64(*r.IXML.DirectTimeReference), sr, "",
					"embedded duplicate", "direct sample reference in iXML")
			}
		}
		if r.IXML.StartTime != nil {
			rate, err := ParseRate(r.IXML.TimecodeRate)
			if err != nil {
				r.add("ixml_start_rate_missing", "warning", fmt.Sprintf("cannot map iXML START_TIME: %v", err))
			} else {
				mappedRate := sr
				if r.IXML.TimecodeSampleRate != 0 {
					mappedRate = r.IXML.TimecodeSampleRate
				}
				pos, err := TimecodeSample(*r.IXML.StartTime, rate, mappedRate)
				if err != nil {
					r.add("ixml_start_map_error", "warning", err.Error())
				} else {
					add("iXML.START_TIME", "timecode-derived", pos, mappedRate, rate.Text,
						"timecode precision only",
						fmt.Sprintf("%s at %s fps, mapped at %d Hz", r.IXML.StartTime.Text, rate.Text, mappedRate))
				}
			}
		}
		if r.IXML.DirectTimeReference != nil && r.IXML.StartTime != nil {
			r.compareIXMLInternals(fmtp)
		}
	}
	if r.Project != nil {
		baseRate := uint32(0)
		if fmtp != nil {
			baseRate = fmtp.SampleRate
		}
		projRate := r.Project.SampleRate
		if projRate == 0 {
			projRate = baseRate
		}
		if r.Project.StartSample != nil {
			if *r.Project.StartSample < 0 {
				r.add("project_negative_start", "error", "project startSample is negative")
			} else {
				add("project.startSample", "project sample", *r.Project.StartSample, projRate, "",
					"sidecar metadata", "explicit startSample in project JSON")
			}
		}
		if r.Project.TimeReference != nil {
			if *r.Project.TimeReference > math.MaxInt64 {
				r.add("project_time_reference_overflow", "error", "project timeReference exceeds int64")
			} else {
				add("project.timeReference", "project sample", int64(*r.Project.TimeReference), projRate, "",
					"sidecar metadata", "explicit timeReference in project JSON")
			}
		}
		if r.Project.StartTimecode != "" {
			rateText := r.Project.TimecodeRate
			if rateText == "" && r.IXML != nil {
				rateText = r.IXML.TimecodeRate
			}
			rate, err := ParseRate(rateText)
			if err != nil {
				r.add("project_timecode_unmapped", "warning", fmt.Sprintf("cannot map project startTimecode: %v", err))
			} else {
				tc, err := ParseTimecode(r.Project.StartTimecode, rateText)
				if err != nil {
					r.add("project_timecode_bad", "warning", err.Error())
				} else {
					if r.Project.DropFrame != nil {
						tc.DropFrame = *r.Project.DropFrame
					}
					mappedRate := projRate
					if mappedRate == 0 && r.IXML != nil {
						mappedRate = r.IXML.TimecodeSampleRate
					}
					pos, err := TimecodeSample(tc, rate, mappedRate)
					if err != nil {
						r.add("project_timecode_map_error", "warning", err.Error())
					} else {
						add("project.startTimecode", "timecode-derived", pos, mappedRate, rate.Text,
							"sidecar timecode", fmt.Sprintf("%s at %s fps", tc.Text, rate.Text))
					}
				}
			}
		}
	}
	sort.SliceStable(r.Starts, func(i, j int) bool { return r.Starts[i].ID < r.Starts[j].ID })
}

func (r *Report) compareIXMLInternals(fmtp *Fmt) {
	sr := uint32(0)
	if fmtp != nil {
		sr = fmtp.SampleRate
	}
	if r.IXML.TimecodeSampleRate != 0 {
		sr = r.IXML.TimecodeSampleRate
	}
	rate, err := ParseRate(r.IXML.TimecodeRate)
	if err != nil {
		return
	}
	tcPos, err := TimecodeSample(*r.IXML.StartTime, rate, sr)
	if err != nil {
		return
	}
	delta := int64(*r.IXML.DirectTimeReference) - tcPos
	frame := FrameSamples(rate, sr)
	if delta == 0 || (frame > 0 && math.Abs(float64(delta)) <= frame) {
		r.add("ixml_start_within_frame", "info", fmt.Sprintf("iXML direct reference and START_TIME differ by %+d samples, within one %s fps frame (%.3f samples)", delta, rate.Text, frame))
	} else {
		r.add("ixml_start_conflict", "warning", fmt.Sprintf("iXML direct reference and START_TIME differ by %+d samples, more than one %s fps frame", delta, rate.Text))
	}
}

func (r *Report) collectComparisons() {
	rate, haveRate := r.primaryRate()
	for i := 0; i < len(r.Starts); i++ {
		for j := i + 1; j < len(r.Starts); j++ {
			a, b := r.Starts[i], r.Starts[j]
			if a.SampleRate == 0 || b.SampleRate == 0 {
				continue
			}
			if a.SampleRate != b.SampleRate {
				r.add("start_rate_mismatch", "warning", fmt.Sprintf("%s is at %d Hz but %s is at %d Hz; their raw sample counts are not directly comparable", a.ID, a.SampleRate, b.ID, b.SampleRate))
				continue
			}
			delta := a.Start - b.Start
			frame := 0.0
			if haveRate {
				frame = FrameSamples(rate, a.SampleRate)
			}
			within := frame > 0 && math.Abs(float64(delta)) <= frame
			meaning := "different sample positions"
			switch {
			case delta == 0:
				meaning = "agreement at sample level"
			case within:
				meaning = "sub-frame or one-frame difference; could be timecode quantization, trim offset, or stale metadata"
			default:
				meaning = "different timecode-visible origin or different time base"
			}
			comp := Comparison{
				A: a.ID, B: b.ID, DeltaSamples: delta,
				DeltaSeconds:   float64(delta) / float64(a.SampleRate),
				WithinOneFrame: within, Meaning: meaning,
			}
			if frame > 0 {
				comp.FrameSamples = uint64(math.Ceil(frame))
			}
			r.Comparisons = append(r.Comparisons, comp)
		}
	}
}

func (r *Report) primaryRate() (Rate, bool) {
	if r.IXML != nil {
		if rate, err := ParseRate(r.IXML.TimecodeRate); err == nil {
			return rate, true
		}
	}
	if r.Project != nil {
		if rate, err := ParseRate(r.Project.TimecodeRate); err == nil {
			return rate, true
		}
	}
	return Rate{}, false
}

func (r *Report) buildConclusions() {
	sortedFindings(r)
	r.Semantics = append(r.Semantics,
		"时间参考与音频时长是两个坐标：前者定位首样本，后者只描述 data 中可播放的帧数。若把两者相减得到“几百样本差”，这是比较对象错误，不能作为损坏证据。",
	)
	rate, haveRate := r.primaryRate()
	if haveRate && r.Fmt != nil && r.Fmt.SampleRate > 0 {
		frame := FrameSamples(rate, r.Fmt.SampleRate)
		r.Semantics = append(r.Semantics, fmt.Sprintf(
			"%s fps 下一帧约 %.3f 个 %d Hz 样本；几百样本通常不足一帧，但足以破坏逐样本同步。",
			rate.Text, frame, r.Fmt.SampleRate))
	}
	if r.BEXT != nil && r.Fmt != nil && r.Fmt.SampleRate > 0 {
		r.Semantics = append(r.Semantics, "bext.TimeReference="+sampleClock(int64(r.BEXT.TimeReference), r.Fmt.SampleRate)+
			"（按文件采样率换算；文件本身没有记录时区或绝对时钟）。")
	}
	r.addSharedOffsetHypothesis()
	if r.Project != nil && r.Project.DurationSamples != nil && r.Fmt != nil {
		delta := *r.Project.DurationSamples - int64(r.Audio.Frames)
		if abs64(delta) > 0 && abs64(delta) <= 2000 {
			r.Hypotheses = append(r.Hypotheses, Hypothesis{
				Rank:       len(r.Hypotheses) + 1,
				Confidence: "medium",
				Statement:  fmt.Sprintf("工程声明的时长比 data 帧数多/少 %+d 个样本；更像尾部裁剪、补齐或时长元数据未同步，而不是 TimeReference 与时长直接矛盾。", delta),
				Evidence: []string{
					fmt.Sprintf("data frames=%d", r.Audio.Frames),
					fmt.Sprintf("project durationSamples=%d", *r.Project.DurationSamples),
				},
			})
		}
	}
	if len(r.Starts) == 0 {
		r.Hypotheses = append(r.Hypotheses, Hypothesis{
			Rank: 1, Confidence: "none",
			Statement: "没有可比较的样本级起点声明；无法做时间参考交叉验证。",
		})
	} else if len(r.Starts) == 1 {
		r.Hypotheses = append(r.Hypotheses, Hypothesis{
			Rank: len(r.Hypotheses) + 1, Confidence: "low",
			Statement: fmt.Sprintf("只有 %s 提供起点；没有第二来源可交叉验证。", r.Starts[0].ID),
			Evidence:  []string{startEvidence(r.Starts[0])},
		})
	} else {
		r.addStartHypotheses(rate, haveRate)
	}
	r.addUndecidables(haveRate)
}

func (r *Report) addSharedOffsetHypothesis() bool {
	if r.Project == nil || r.Project.DurationSamples == nil || r.Fmt == nil || r.Fmt.SampleRate == 0 {
		return false
	}
	durationDelta := *r.Project.DurationSamples - int64(r.Audio.Frames)
	if durationDelta == 0 || abs64(durationDelta) > 2000 {
		return false
	}
	for _, c := range r.Comparisons {
		if abs64(c.DeltaSamples) != abs64(durationDelta) {
			continue
		}
		if !strings.Contains(c.A, "bext") && !strings.Contains(c.B, "bext") {
			continue
		}
		r.Hypotheses = append(r.Hypotheses, Hypothesis{
			Rank:       len(r.Hypotheses) + 1,
			Confidence: "medium",
			Statement: fmt.Sprintf(
				"同一个 %d 样本（%.3f ms）偏移同时出现在起点元数据（%s vs %s）和工程时长 vs data 帧数中；最像一次裁剪/补齐或共享取整造成的版本漂移，而不是两处独立损坏。哪一侧是采集真相仍不可判定。",
				abs64(durationDelta), float64(abs64(durationDelta))/float64(r.Fmt.SampleRate)*1000, c.A, c.B),
			Evidence: []string{
				fmt.Sprintf("%s - %s = %+d samples", c.A, c.B, c.DeltaSamples),
				fmt.Sprintf("project durationSamples - data frames = %+d samples", durationDelta),
			},
		})
		return true
	}
	return false
}

func (r *Report) addStartHypotheses(rate Rate, haveRate bool) {
	if len(r.Comparisons) == 0 {
		r.Hypotheses = append(r.Hypotheses, Hypothesis{
			Rank: len(r.Hypotheses) + 1, Confidence: "low",
			Statement: "存在多个起点声明，但没有同采样率的可比较对；无法判断它们是否指向同一位置。",
			Evidence:  startEvidenceList(r.Starts),
		})
		return
	}
	allExact := true
	allWithin := true
	frame := 0.0
	if haveRate && r.Fmt != nil {
		frame = FrameSamples(rate, r.Fmt.SampleRate)
	}
	var exactPairs []Comparison
	for _, c := range r.Comparisons {
		if c.DeltaSamples != 0 {
			allExact = false
		}
		if c.DeltaSamples != 0 && (!c.WithinOneFrame || frame == 0) {
			allWithin = false
		}
		if c.DeltaSamples == 0 {
			exactPairs = append(exactPairs, c)
		}
	}
	switch {
	case allExact:
		r.Hypotheses = append(r.Hypotheses, Hypothesis{
			Rank: len(r.Hypotheses) + 1, Confidence: "high",
			Statement: "所有可比较的起点声明在样本级完全一致；该起点可高置信视为三方共同声明。",
			Evidence:  startEvidenceList(r.Starts),
		})
	case allWithin:
		r.addWithinFrameHypothesis(rate, frame)
	case len(exactPairs) > 0:
		r.addExactClusterHypothesis()
	default:
		r.Hypotheses = append(r.Hypotheses, Hypothesis{
			Rank: len(r.Hypotheses) + 1, Confidence: "low",
			Statement: "起点声明之间没有多数一致，且差异超过一个时间码帧；无法确定哪个来源反映真实录制起点。",
			Evidence:  startEvidenceList(r.Starts),
		})
	}
}

func (r *Report) addWithinFrameHypothesis(rate Rate, frame float64) {
	groups := exactStartGroups(r.Starts)
	best := groups[0]
	for _, g := range groups[1:] {
		if len(g.domains) > len(best.domains) {
			best = g
		}
	}
	allDomains := startDomains(r.Starts)
	if len(best.domains) >= 2 && len(best.domains) < len(allDomains) {
		outside := make([]string, 0)
		for _, s := range r.Starts {
			if s.Start != best.samples {
				outside = append(outside, s.ID)
			}
		}
		r.Hypotheses = append(r.Hypotheses, Hypothesis{
			Rank: len(r.Hypotheses) + 1, Confidence: "medium",
			Statement: fmt.Sprintf("%s 在样本级一致；%s 早/晚 %+d 样本，但仍落在同一 %s fps 时间码帧内。较可能是 iXML/时间码取整或剪辑偏移后的陈旧值；bext 是样本级规范源，不能据此自动改写。", strings.Join(best.ids, ", "), strings.Join(outside, ", "), outsideDelta(r.Starts, best.samples), rate.Text),
			Evidence:  startEvidenceList(r.Starts),
		})
		return
	}
	r.Hypotheses = append(r.Hypotheses, Hypothesis{
		Rank: len(r.Hypotheses) + 1, Confidence: "medium",
		Statement: fmt.Sprintf("所有起点都落在同一 %s fps 时间码帧内（≤%.3f 样本）；最可能是同一时间码起点的不同表达，差异来自整帧时间码、裁剪偏移、取整或后写元数据。bext 仍是唯一样本级规范源，不能据此改写音频。", rate.Text, frame),
		Evidence:  startEvidenceList(r.Starts),
	})
}

func (r *Report) addExactClusterHypothesis() {
	groups := exactStartGroups(r.Starts)
	sort.SliceStable(groups, func(i, j int) bool { return len(groups[i].domains) > len(groups[j].domains) })
	best := groups[0]
	outside := make([]string, 0)
	for _, s := range r.Starts {
		if s.Start != best.samples {
			outside = append(outside, s.ID)
		}
	}
	r.Hypotheses = append(r.Hypotheses, Hypothesis{
		Rank: len(r.Hypotheses) + 1, Confidence: "medium",
		Statement: fmt.Sprintf("%s 在样本级一致；%s 是孤立值，较可能陈旧、属于另一版本或另一时间基，但不能自动覆盖。", strings.Join(best.ids, ", "), strings.Join(outside, ", ")),
		Evidence:  startEvidenceList(r.Starts),
	})
}

type startGroup struct {
	samples int64
	ids     []string
	domains map[string]bool
}

func exactStartGroups(starts []StartSource) []startGroup {
	var groups []startGroup
	for _, s := range starts {
		found := -1
		for i := range groups {
			if groups[i].samples == s.Start {
				found = i
				break
			}
		}
		if found < 0 {
			groups = append(groups, startGroup{samples: s.Start, domains: make(map[string]bool)})
			found = len(groups) - 1
		}
		groups[found].ids = append(groups[found].ids, s.ID)
		groups[found].domains[startDomain(s.ID)] = true
	}
	return groups
}

func startDomain(id string) string {
	if i := strings.IndexByte(id, '.'); i >= 0 {
		return id[:i]
	}
	return id
}

func startDomains(starts []StartSource) []string {
	seen := make(map[string]bool)
	var out []string
	for _, s := range starts {
		d := startDomain(s.ID)
		if !seen[d] {
			seen[d] = true
			out = append(out, d)
		}
	}
	return out
}

func outsideDelta(starts []StartSource, samples int64) int64 {
	for _, s := range starts {
		if s.Start != samples {
			return s.Start - samples
		}
	}
	return 0
}

func (r *Report) addUndecidables(haveRate bool) {
	if len(r.Starts) > 1 {
		r.Undecidable = append(r.Undecidable,
			"没有原始录音文件、媒体哈希或编辑历史时，无法确定多个起点声明中哪一个是采集时的真实首样本。",
		)
	}
	nonzero := false
	for _, c := range r.Comparisons {
		if c.DeltaSamples != 0 {
			nonzero = true
			break
		}
	}
	if nonzero {
		r.Undecidable = append(r.Undecidable,
			"几百个样本的差异究竟是故意裁剪、采集/重采样延迟、时间码取整还是陈旧元数据，仅靠容器无法判定。",
		)
	}
	if r.BEXT != nil {
		r.Undecidable = append(r.Undecidable,
			"BWF 文件未声明绝对时钟和时区；TimeReference 的“午夜”参考点在不同工作流中也可能被当作工程原点。",
		)
	}
	if r.IXML != nil && r.IXML.StartTime != nil {
		r.Undecidable = append(r.Undecidable,
			"iXML START_TIME 可能表示日时间码、工程时间码或素材相对起点；缺少项目 schema/惯例时不能唯一解释。",
		)
	}
	if haveRate && r.IXML != nil && r.IXML.StartTime != nil {
		// The TC label is inherently quantized; only bext carries a sample-level claim.
		r.Undecidable = append(r.Undecidable,
			"iXML START_TIME 最多定位到时间码帧边界，帧内样本位置不能由该字段单独恢复。",
		)
	}
	r.Undecidable = append(r.Undecidable,
		"data 字节数决定可播放帧数，但不能证明这些帧尚未经过头部/尾部裁剪或填充；需要原始媒体或校验记录。",
	)
}

func startEvidence(s StartSource) string {
	return fmt.Sprintf("%s: %d samples @ %d Hz (%s), %s", s.ID, s.Start, s.SampleRate, s.Rate, s.Evidence)
}

func startEvidenceList(starts []StartSource) []string {
	out := make([]string, 0, len(starts))
	for _, s := range starts {
		out = append(out, startEvidence(s))
	}
	return out
}

func sampleClock(samples int64, rate uint32) string {
	if rate == 0 {
		return fmt.Sprintf("%d samples (unknown rate)", samples)
	}
	neg := samples < 0
	if neg {
		samples = -samples
	}
	daySamples := int64(rate) * 86400
	days := samples / daySamples
	rem := samples % daySamples
	secs := rem / int64(rate)
	hours := secs / 3600
	secs -= hours * 3600
	minutes := secs / 60
	secs -= minutes * 60
	frac := (rem % int64(rate)) * 1_000_000 / int64(rate)
	prefix := ""
	if neg {
		prefix = "-"
	}
	if days > 0 {
		return fmt.Sprintf("%s%d days + %02d:%02d:%02d.%06d (%d samples)", prefix, days, hours, minutes, secs, frac, samples)
	}
	return fmt.Sprintf("%s%02d:%02d:%02d.%06d (%d samples)", prefix, hours, minutes, secs, frac, samples)
}
