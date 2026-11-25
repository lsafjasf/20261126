// framealign 把多帧率素材的切点/字幕/标记映射到统一时间线，
// 对半帧落点给出多个可解释候选，并导出每种取整选择的误差表。
//
// 用法：
//
//	framealign [-timeline 25] [-events events.csv] [-out dir]
//
// events.csv 每行一个事件：kind,label,src_fps,position
// position 为源帧号（整数）或时间码（HH:MM:SS:FF / HH:MM:SS;FF）。
// 缺省使用内置演示事件集。
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"framealign"
)

func main() {
	timelineName := flag.String("timeline", "25", "时间线帧率（24/25/29.97/30）")
	eventsPath := flag.String("events", "", "事件 CSV；缺省用内置演示集")
	outDir := flag.String("out", "artifacts", "误差表输出目录")
	flag.Parse()

	tlRate, err := framealign.FindRate(*timelineName)
	if err != nil {
		fatal(err)
	}
	tl := framealign.NewTimeline(tlRate)

	var events []framealign.Event
	if *eventsPath == "" {
		events = demoEvents()
	} else {
		events, err = loadEvents(*eventsPath)
		if err != nil {
			fatal(err)
		}
	}

	rep := framealign.BuildReport(tl, events)
	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		fatal(err)
	}
	mustWrite(filepath.Join(*outDir, "candidates.csv"), rep.WriteCandidatesCSV)
	mustWrite(filepath.Join(*outDir, "summary.csv"), rep.WriteSummaryCSV)
	mustWrite(filepath.Join(*outDir, "report.md"), rep.WriteMarkdown)

	n, exact := len(rep.Alignments), 0
	for _, al := range rep.Alignments {
		if al.Exact {
			exact++
		}
	}
	fmt.Printf("时间线 %s fps：%d 个事件，%d 个精确命中，%d 个产生候选\n",
		tl.Rate.Name, n, exact, n-exact)
	fmt.Printf("误差表已写入 %s/{candidates.csv,summary.csv,report.md}\n", *outDir)
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "framealign:", err)
	os.Exit(1)
}

func mustWrite(path string, fn func(io.Writer) error) {
	f, err := os.Create(path)
	if err != nil {
		fatal(err)
	}
	defer f.Close()
	if err := fn(f); err != nil {
		fatal(err)
	}
}
