package opusalign

import "fmt"

type durationResolution struct {
	toc      TOC
	tocOK    bool
	tocValue int64
	declared int64
	value    int64
	known    bool
	illegal  []InvalidDuration
	problems []Issue
}

func BuildTimeline(input TimelineInput) (Report, error) {
	if input.PreSkip < 0 {
		return Report{}, fmt.Errorf("pre_skip must be non-negative, got %d", input.PreSkip)
	}

	report := Report{
		SampleRate: SampleRate,
		PreSkip:    input.PreSkip,
		Intervals:  make([]PacketInterval, 0, len(input.Packets)),
	}

	seenIDs := make(map[string]int)
	seenRanges := make(map[SampleRange]int)
	var lastEnd *int64
	var lastSequence *int64
	lastSequenceIndex := -1
	rawCursor := int64(0)
	cursorValid := true

	for packetIndex, packet := range input.Packets {
		idDuplicateMarker := -1
		received := packet.Received == nil || *packet.Received
		resolution := resolveDuration(packet)
		interval := PacketInterval{
			Index:                   packetIndex,
			ID:                      packet.ID,
			Sequence:                packet.Sequence,
			Received:                received,
			EndOfStream:             packet.EndOfStream,
			DurationKnown:           resolution.known,
			DurationSamples:         resolution.value,
			TOCDurationSamples:      resolution.tocValue,
			DeclaredDurationSamples: resolution.declared,
			Issues:                  append([]Issue(nil), resolution.problems...),
			TextStatus:              TextEmpty,
		}
		for issueIndex := range interval.Issues {
			interval.Issues[issueIndex].Packet = packetIndex
		}
		for _, invalid := range resolution.illegal {
			invalid.PacketIndex = packetIndex
			report.InvalidDurations = append(report.InvalidDurations, invalid)
		}

		if packet.ID != "" {
			if first, found := seenIDs[packet.ID]; found {
				interval.Issues = append(interval.Issues, Issue{
					Code: IssueDuplicate, Severity: "error", Packet: packetIndex,
					Message: fmt.Sprintf("packet id %q was first seen at index %d", packet.ID, first),
				})
				report.Duplicates = append(report.Duplicates, DuplicateRange{
					PacketIndex: packetIndex, FirstIndex: first, Reason: "packet_id",
				})
				idDuplicateMarker = len(report.Duplicates) - 1
			} else {
				seenIDs[packet.ID] = packetIndex
			}
		}

		rawEnd, hasAnchor := rawAnchor(packet, input.PreSkip)
		if hasAnchor {
			adjustedEnd := rawEnd - int64(input.PreSkip)
			interval.AnchorSample = &adjustedEnd
		}

		positioned := false
		if resolution.known {
			if hasAnchor {
				positioned = true
			} else if cursorValid {
				rawEnd = rawCursor + resolution.value
				positioned = true
			} else {
				interval.Issues = append(interval.Issues, Issue{
					Code: IssueUnknownDuration, Severity: "error", Packet: packetIndex,
					Message: "packet has no timestamp anchor and cumulative position is broken",
				})
			}
		}

		if positioned {
			rawStart := rawEnd - resolution.value
			if packet.EndOfStream && hasAnchor && lastEnd != nil {
				previousRawEnd := *lastEnd + int64(input.PreSkip)
				expectedRawEnd := previousRawEnd + resolution.value
				switch {
				case rawEnd >= previousRawEnd && rawEnd < expectedRawEnd:
					rawStart = previousRawEnd
					interval.EndTrimSamples = expectedRawEnd - rawEnd
				case rawEnd < previousRawEnd:
					interval.Issues = append(interval.Issues, Issue{
						Code: IssueIllegalDuration, Severity: "error", Packet: packetIndex,
						Message: "end-of-stream granule precedes the previous packet end",
					})
					report.InvalidDurations = append(report.InvalidDurations, InvalidDuration{
						PacketIndex: packetIndex, Value: rawEnd,
						Reason: "end-of-stream granule precedes previous packet end",
					})
				}
			}
			interval.Raw = SampleRange{Start: rawStart, End: rawEnd}
			interval.Playback = SampleRange{
				Start: rawStart - int64(input.PreSkip),
				End:   rawEnd - int64(input.PreSkip),
			}
			interval.Audible = audibleRange(interval.Playback)
			if idDuplicateMarker >= 0 {
				report.Duplicates[idDuplicateMarker].Range = interval.Playback
			}
			rawCursor = rawEnd
			cursorValid = true

			if first, found := seenRanges[interval.Playback]; found {
				interval.Issues = append(interval.Issues, Issue{
					Code: IssueDuplicate, Severity: "error", Packet: packetIndex,
					Message: fmt.Sprintf("same sample interval as packet %d", first),
				})
				report.Duplicates = append(report.Duplicates, DuplicateRange{
					Range: interval.Playback, PacketIndex: packetIndex,
					FirstIndex: first, Reason: "same_interval",
				})
			} else {
				seenRanges[interval.Playback] = packetIndex
			}

			if lastEnd != nil {
				delta := interval.Playback.Start - *lastEnd
				switch {
				case delta > 0:
					missing := MissingRange{
						Range:             SampleRange{Start: *lastEnd, End: interval.Playback.Start},
						Samples:           delta,
						AfterPacketIndex:  packetIndex - 1,
						BeforePacketIndex: packetIndex,
						Kind:              "implicit_padding",
					}
					if lastSequence != nil && packet.Sequence != nil {
						missing.FromSequence = increment64(*lastSequence)
						missing.ToSequence = decrement64(*packet.Sequence)
					}
					report.MissingRanges = append(report.MissingRanges, missing)
					interval.Issues = append(interval.Issues, Issue{
						Code: IssueMissing, Severity: "warning", Packet: packetIndex,
						Message: fmt.Sprintf("%d sample timestamp gap before packet", delta),
					})
				case delta < 0:
					code := IssueOverlap
					if hasDuplicateIdentity(packet, lastSequence) {
						code = IssueDuplicate
					}
					interval.Issues = append(interval.Issues, Issue{
						Code: code, Severity: "error", Packet: packetIndex,
						Message: fmt.Sprintf("packet starts %d samples before previous end", -delta),
					})
				}
			}
			endCopy := interval.Playback.End
			lastEnd = &endCopy
		} else {
			cursorValid = false
			lastEnd = nil
		}

		if packet.Sequence != nil {
			if lastSequence != nil {
				switch {
				case *packet.Sequence == *lastSequence:
					if !hasIssue(interval.Issues, IssueDuplicate) {
						interval.Issues = append(interval.Issues, Issue{
							Code: IssueDuplicate, Severity: "error", Packet: packetIndex,
							Message: fmt.Sprintf("duplicate sequence %d", *packet.Sequence),
						})
					}
					report.Duplicates = append(report.Duplicates, DuplicateRange{
						Range: interval.Playback, PacketIndex: packetIndex,
						FirstIndex: lastSequenceIndex, Reason: "sequence",
					})
				case *packet.Sequence > *lastSequence+1:
					from := *lastSequence + 1
					to := *packet.Sequence - 1
					interval.Issues = append(interval.Issues, Issue{
						Code: IssueSequenceGap, Severity: "warning", Packet: packetIndex,
						Message: fmt.Sprintf("missing sequence %d..%d", from, to),
					})
					recordSequenceGap(&report, packetIndex, from, to)
				case *packet.Sequence < *lastSequence:
					interval.Issues = append(interval.Issues, Issue{
						Code: IssueOverlap, Severity: "error", Packet: packetIndex,
						Message: "sequence number moved backwards",
					})
				}
			}
			seqCopy := *packet.Sequence
			lastSequence = &seqCopy
			lastSequenceIndex = packetIndex
		}

		applyPlaybackText(&interval, packet, received)
		if !received {
			interval.Issues = append(interval.Issues, Issue{
				Code: IssueMissing, Severity: "warning", Packet: packetIndex,
				Message: "packet was not received during ordinary playback",
			})
		}

		report.Intervals = append(report.Intervals, interval)
	}

	if len(input.Cues) > 0 {
		report.Subtitles = SyncCuesToReport(input.Cues, report)
	} else {
		report.Subtitles = PacketCues(report.Intervals)
	}
	finalizeStats(&report)
	return report, nil
}

func resolveDuration(packet Packet) durationResolution {
	result := durationResolution{declared: packet.DurationSamples}
	if packet.DurationSamples != 0 {
		if !IsLegalDurationSamples(packet.DurationSamples) {
			result.illegal = append(result.illegal, InvalidDuration{
				Value: packet.DurationSamples, Reason: "duration is not a positive 2.5 ms multiple <= 120 ms",
			})
			result.problems = append(result.problems, Issue{
				Code: IssueIllegalDuration, Severity: "error",
				Message: fmt.Sprintf("illegal declared duration %d", packet.DurationSamples),
			})
			result.value = packet.DurationSamples
			result.known = true
		} else {
			result.value = packet.DurationSamples
			result.known = true
		}
	}

	var toc TOC
	var err error
	if len(packet.Data) > 0 {
		var duration int
		duration, toc, err = PacketDurationSamples(packet.Data)
		if err == nil {
			result.tocOK = true
			result.tocValue = int64(duration)
			result.toc = toc
			result.value = result.tocValue
			result.known = true
		}
	} else if packet.TOC != nil {
		toc = ParseTOCByte(*packet.TOC)
		if toc.DurationKnown && IsLegalDurationSamples(int64(toc.DurationSamples)) {
			result.tocOK = true
			result.tocValue = int64(toc.DurationSamples)
			result.toc = toc
			result.value = result.tocValue
			result.known = true
		} else if toc.Code == 3 {
			err = fmt.Errorf("code 3 TOC requires the full packet to read frame count")
		} else {
			err = fmt.Errorf("illegal TOC duration %d", toc.DurationSamples)
		}
	}

	if err != nil {
		result.problems = append(result.problems, Issue{
			Code: IssueIllegalDuration, Severity: "error", Message: err.Error(),
		})
		result.illegal = append(result.illegal, InvalidDuration{Reason: err.Error()})
	}
	if result.tocOK && packet.DurationSamples != 0 && packet.DurationSamples != result.tocValue {
		message := fmt.Sprintf("TOC duration %d disagrees with packet duration %d", result.tocValue, packet.DurationSamples)
		result.problems = append(result.problems, Issue{
			Code: IssueTOCMismatch, Severity: "error", Message: message,
		})
		result.illegal = append(result.illegal, InvalidDuration{
			Value: packet.DurationSamples, Reason: message,
		})
		result.value = result.tocValue
	}
	if !result.known {
		result.problems = append(result.problems, Issue{
			Code: IssueUnknownDuration, Severity: "error",
			Message: "no legal TOC or packet duration available",
		})
	}
	return result
}

func rawAnchor(packet Packet, preSkip int) (int64, bool) {
	switch {
	case packet.GranulePosition != nil:
		return *packet.GranulePosition, true
	case packet.RawEndSample != nil:
		return *packet.RawEndSample, true
	case packet.PlaybackEndSample != nil:
		return *packet.PlaybackEndSample + int64(preSkip), true
	default:
		return 0, false
	}
}

func audibleRange(playback SampleRange) SampleRange {
	start := max64(playback.Start, 0)
	end := max64(playback.End, 0)
	if end < start {
		end = start
	}
	return SampleRange{Start: start, End: end}
}

func applyPlaybackText(interval *PacketInterval, packet Packet, received bool) {
	switch {
	case !received:
		interval.Text = ""
		if packet.GeneratorText != "" {
			interval.TextStatus = TextGeneratorSuppressed
			interval.Issues = append(interval.Issues, Issue{
				Code: IssueGeneratorText, Severity: "info", Packet: interval.Index,
				Message: "generator transcript was not used to repair lost playback packet",
			})
			return
		}
		interval.TextStatus = TextLost
	case packet.Text != "":
		interval.Text = packet.Text
		interval.TextStatus = TextObserved
		if packet.GeneratorText != "" {
			interval.Issues = append(interval.Issues, Issue{
				Code: IssueGeneratorText, Severity: "info", Packet: interval.Index,
				Message: "generator transcript retained separately and not copied into playback text",
			})
		}
	case packet.GeneratorText != "":
		interval.TextStatus = TextGeneratorSuppressed
		interval.Issues = append(interval.Issues, Issue{
			Code: IssueGeneratorText, Severity: "info", Packet: interval.Index,
			Message: "generator transcript cannot replace absent observed playback text",
		})
	default:
		interval.TextStatus = TextEmpty
	}
}

func recordSequenceGap(report *Report, packetIndex int, from, to int64) {
	report.SequenceGaps = append(report.SequenceGaps, SequenceGap{
		AfterPacketIndex:  packetIndex - 1,
		BeforePacketIndex: packetIndex,
		FromSequence:      from,
		ToSequence:        to,
	})
	for i := range report.MissingRanges {
		if report.MissingRanges[i].BeforePacketIndex == packetIndex {
			fromCopy := from
			toCopy := to
			report.MissingRanges[i].FromSequence = &fromCopy
			report.MissingRanges[i].ToSequence = &toCopy
			return
		}
	}
}

func hasDuplicateIdentity(packet Packet, lastSequence *int64) bool {
	return packet.Sequence != nil && lastSequence != nil && *packet.Sequence == *lastSequence
}

func hasIssue(issues []Issue, code string) bool {
	for _, issue := range issues {
		if issue.Code == code {
			return true
		}
	}
	return false
}

func finalizeStats(report *Report) {
	report.Stats.PacketCount = len(report.Intervals)
	report.Stats.DuplicateCount = len(report.Duplicates)
	report.Stats.MissingRangeCount = len(report.MissingRanges)
	report.Stats.SequenceGapCount = len(report.SequenceGaps)
	report.Stats.InvalidDurationCount = len(report.InvalidDurations)
	report.Stats.CueCount = len(report.Subtitles)
	for _, interval := range report.Intervals {
		if interval.Received {
			report.Stats.ReceivedCount++
		} else {
			report.Stats.LostCount++
			report.Stats.LostSamples += interval.DurationSamples
		}
	}
	for _, missing := range report.MissingRanges {
		if missing.Samples > 0 {
			report.Stats.MissingSamples += missing.Samples
		}
	}
}

func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func increment64(v int64) *int64 {
	v++
	return &v
}

func decrement64(v int64) *int64 {
	v--
	return &v
}
