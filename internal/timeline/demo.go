package timeline

import "fmt"

func DemoTimeline() Timeline {
	rates := []Rate{24, 25, 30}
	markers := make([]Marker, 0, 149)
	markers = append(markers, mediaSubtitles()...)
	markers = append(markers, clipMarkers()...)
	markers = append(markers, chapterMarkers()...)
	markers = append(markers, boundaryMarkers()...)
	return Timeline{
		ProjectName:    "mixed-24-25-30-demo",
		SequenceRate:   25,
		DurationFrames: 3600,
		Rates:          rates,
		Markers:        markers,
	}
}

func mediaSubtitles() []Marker {
	rates := []Rate{24, 25, 30}
	offsets := map[Rate]int64{24: 0, 25: 1200, 30: 2880}
	markers := []Marker{}
	for _, rate := range rates {
		for index := 1; index <= 18; index++ {
			base := int64(index*20 - 19)
			inFrame := base
			outFrame := base + 12
			inID := fmt.Sprintf("sub%d-%03d-in", rate, index)
			outID := fmt.Sprintf("sub%d-%03d-out", rate, index)
			inLabel := fmt.Sprintf("%dfps 字幕 %d 入点", rate, index)
			outLabel := fmt.Sprintf("%dfps 字幕 %d 出点", rate, index)
			markers = append(markers,
				mediaMarker(inID, inLabel, "subtitle_in", inFrame, rate, offsets[rate], PolicyFloor),
				mediaMarker(outID, outLabel, "subtitle_out", outFrame, rate, offsets[rate], PolicyCeil),
			)
		}
	}
	return markers
}

func mediaMarker(id, label, role string, frame int64, rate Rate, offsetFrames int64, policy string) Marker {
	globalFrame := RatInt(offsetFrames + frame)
	return Marker{
		ID:                id,
		Label:             label,
		Role:              role,
		AnchorKind:        AnchorMedia,
		AnchorFrame:       frame,
		AnchorRate:        rate,
		GlobalTimeSeconds: globalFrame.Quo(rate.Rational()),
		MediaOffsetFrames: offsetFrames,
		RolePolicy:        policy,
	}
}

func clipMarkers() []Marker {
	rates := []Rate{24, 25, 30}
	offsets := map[Rate]int64{24: 0, 25: 1200, 30: 2880}
	frames := []int64{6, 23, 40, 57, 74, 91, 108, 125}
	markers := make([]Marker, 0, 24)
	for _, rate := range rates {
		for index, frame := range frames {
			id := fmt.Sprintf("clip-c%d-%03d", rate, index+1)
			label := fmt.Sprintf("片段锚定标记 c%d #%d", rate, index+1)
			globalFrame := RatInt(offsets[rate] + frame)
			markers = append(markers, Marker{
				ID:                id,
				Label:             label,
				Role:              "marker",
				AnchorKind:        AnchorClip,
				AnchorFrame:       frame,
				AnchorRate:        rate,
				GlobalTimeSeconds: globalFrame.Quo(rate.Rational()),
				ClipOffsetFrames:  offsets[rate],
				RolePolicy:        PolicyNearest,
			})
		}
	}
	return markers
}

func chapterMarkers() []Marker {
	markers := make([]Marker, 0, 15)
	for index := 1; index <= 15; index++ {
		frame := int64((index - 1) * 250)
		markers = append(markers, Marker{
			ID:                fmt.Sprintf("chapter-%02d", index),
			Label:             fmt.Sprintf("第 %d 章", index),
			Role:              "chapter",
			AnchorKind:        AnchorGlobal,
			AnchorFrame:       frame,
			AnchorRate:        25,
			GlobalTimeSeconds: RatInt(frame).Quo(Rate(25).Rational()),
			RolePolicy:        PolicyNearest,
		})
	}
	return markers
}

func boundaryMarkers() []Marker {
	return []Marker{
		{
			ID:                "case-24-to-25-half-even",
			Label:             "24fps 第12帧 = 25fps 第12.5帧（半帧，偶数候选）",
			Role:              "chapter",
			AnchorKind:        AnchorMedia,
			AnchorFrame:       12,
			AnchorRate:        24,
			GlobalTimeSeconds: NewRat(1, 2),
			MediaOffsetFrames: 0,
			NearestEvenTie:    true,
			RolePolicy:        PolicyNearest,
		},
		{
			ID:                "case-30-to-25-half",
			Label:             "30fps 第3帧 = 25fps 第2.5帧（半帧）",
			Role:              "chapter",
			AnchorKind:        AnchorMedia,
			AnchorFrame:       3,
			AnchorRate:        30,
			GlobalTimeSeconds: NewRat(961, 10),
			MediaOffsetFrames: 2880,
			NearestEvenTie:    true,
			RolePolicy:        PolicyNearest,
		},
	}
}
