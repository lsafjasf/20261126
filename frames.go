package vttlayout

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// WrittenFrame describes one frame file produced by WriteFrames.
type WrittenFrame struct {
	AtMS Milliseconds
	Mode FrameMode
	Path string
}

type framePair struct {
	at     Milliseconds
	before string
	after  string
}

// WriteFrames renders before/after SVG frames for the given times and an
// index.html contact sheet into dir. All files are written by Go.
func WriteFrames(dir string, doc *Document, plan *Plan, times []Milliseconds) ([]WrittenFrame, string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, "", err
	}
	if times == nil {
		times = KeyFrameTimes(doc)
	}
	var written []WrittenFrame
	var pairs []framePair

	for _, at := range times {
		stem := FormatTimestampShort(at)
		for _, mode := range []FrameMode{FrameBefore, FrameAfter} {
			svg := RenderSVGFrame(plan, at, mode)
			name := fmt.Sprintf("%s_%s.svg", mode, stem)
			path := filepath.Join(dir, name)
			if err := os.WriteFile(path, []byte(svg), 0o644); err != nil {
				return written, "", err
			}
			written = append(written, WrittenFrame{AtMS: at, Mode: mode, Path: path})
			if mode == FrameBefore {
				pairs = append(pairs, framePair{at: at, before: name})
			} else {
				for i := range pairs {
					if pairs[i].at == at {
						pairs[i].after = name
					}
				}
			}
		}
	}

	indexPath := filepath.Join(dir, "index.html")
	if err := os.WriteFile(indexPath, []byte(renderIndexHTML(plan, pairs)), 0o644); err != nil {
		return written, "", err
	}
	return written, indexPath, nil
}

func renderIndexHTML(plan *Plan, pairs []framePair) string {
	var b strings.Builder
	b.WriteString(`<!doctype html><html lang="zh"><head><meta charset="utf-8">`)
	b.WriteString(`<title>WebVTT region 重排预览帧</title><style>`)
	b.WriteString(`body{background:#0b0f17;color:#e5e7eb;font-family:sans-serif;margin:24px}`)
	b.WriteString(`.row{display:flex;gap:16px;flex-wrap:wrap;margin-bottom:28px;align-items:flex-start}`)
	b.WriteString(`figure{margin:0;background:#111827;border-radius:12px;padding:10px}`)
	b.WriteString(`img{height:520px;border-radius:8px;display:block}`)
	b.WriteString(`figcaption{margin-top:8px;font-family:monospace}`)
	b.WriteString(`.b{color:#f59e0b}.a{color:#10b981}</style></head><body>`)
	fmt.Fprintf(&b, "<h1>WebVTT region 预览 · %s · feasible=%v</h1>",
		plan.Canvas.Name, plan.Feasible)
	for _, p := range pairs {
		fmt.Fprintf(&b, `<div class="row"><figure><img src="%s"><figcaption class="b">before %s</figcaption></figure>`,
			p.before, FormatTimestamp(p.at))
		fmt.Fprintf(&b, `<figure><img src="%s"><figcaption class="a">after %s</figcaption></figure></div>`,
			p.after, FormatTimestamp(p.at))
	}
	b.WriteString(`</body></html>`)
	return b.String()
}
