package main

import (
	"encoding/csv"
	"fmt"
	"os"
	"strconv"
	"strings"

	"framealign"
)

// demoEvents 覆盖四种源帧率 × 切点/字幕/标记，且刻意包含
// 精确命中、半帧（.5）与一般分数帧三种情形。
func demoEvents() []framealign.Event {
	mk := func(kind framealign.EventKind, label string, fps framealign.FrameRate, frame int64) framealign.Event {
		return framealign.Event{Kind: kind, Label: label, SourceFPS: fps, SourceFrame: frame}
	}
	return []framealign.Event{
		// 25→25：恒精确，基线对照。
		mk(framealign.KindCut, "cut-25-exact", framealign.FPS25, 250),
		// 30→25：每 6 帧精确一次；n=6k 精确，其余分数。
		mk(framealign.KindCut, "cut-30-exact", framealign.FPS30, 720),
		mk(framealign.KindCut, "cut-30-frac", framealign.FPS30, 721),
		// 30→25 的半帧：n=3 时 2.5 帧。
		mk(framealign.KindMarker, "marker-30-half", framealign.FPS30, 3),
		// 24→25：每 24 帧精确一次。
		mk(framealign.KindCut, "cut-24-exact", framealign.FPS24, 480),
		mk(framealign.KindSubtitleIn, "sub-in-24-frac", framealign.FPS24, 481),
		mk(framealign.KindSubtitleOut, "sub-out-24-frac", framealign.FPS24, 529),
		// 24→25 的半帧：n=12 时 12.5 帧。
		mk(framealign.KindMarker, "marker-24-half", framealign.FPS24, 12),
		// 29.97→25：每 1200 帧精确一次，绝大多数为分数帧。
		mk(framealign.KindCut, "cut-2997-exact", framealign.FPS2997, 1200),
		mk(framealign.KindSubtitleIn, "sub-in-2997-frac", framealign.FPS2997, 12345),
		mk(framealign.KindMarker, "marker-2997-frac", framealign.FPS2997, 107892), // 00:01:00;00 附近
	}
}

// loadEvents 解析事件 CSV：kind,label,src_fps,position。
// position 含 ":" 或 ";" 时按时间码解析，否则按整数帧号。
func loadEvents(path string) ([]framealign.Event, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	rows, err := csv.NewReader(f).ReadAll()
	if err != nil {
		return nil, fmt.Errorf("解析 %s: %w", path, err)
	}
	var events []framealign.Event
	for i, row := range rows {
		if len(row) == 0 || strings.HasPrefix(strings.TrimSpace(row[0]), "#") {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(row[0]), "kind") {
			continue // 表头
		}
		if len(row) != 4 {
			return nil, fmt.Errorf("%s 第 %d 行：应为 kind,label,src_fps,position", path, i+1)
		}
		kind := framealign.EventKind(strings.TrimSpace(row[0]))
		switch kind {
		case framealign.KindCut, framealign.KindSubtitleIn, framealign.KindSubtitleOut, framealign.KindMarker:
		default:
			return nil, fmt.Errorf("%s 第 %d 行：未知事件类型 %q", path, i+1, kind)
		}
		fps, err := framealign.FindRate(strings.TrimSpace(row[2]))
		if err != nil {
			return nil, fmt.Errorf("%s 第 %d 行: %w", path, i+1, err)
		}
		pos := strings.TrimSpace(row[3])
		var frame int64
		if strings.ContainsAny(pos, ":;") {
			frame, err = framealign.ParseTimecode(fps, pos)
		} else {
			frame, err = strconv.ParseInt(pos, 10, 64)
		}
		if err != nil {
			return nil, fmt.Errorf("%s 第 %d 行: %w", path, i+1, err)
		}
		events = append(events, framealign.Event{
			Kind: kind, Label: strings.TrimSpace(row[1]),
			SourceFPS: fps, SourceFrame: frame,
		})
	}
	if len(events) == 0 {
		return nil, fmt.Errorf("%s 不含任何事件", path)
	}
	return events, nil
}
