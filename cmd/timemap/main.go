package main

import (
	"flag"
	"fmt"
	"os"

	"timemap/timeline"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "timemap:", err)
		os.Exit(1)
	}
}

func run() error {
	outDir := flag.String("out", "", "输出目录（留空则只打印摘要）")
	flag.Parse()

	p := timeline.DemoProject()
	r, err := timeline.NewResolver(p)
	if err != nil {
		return err
	}
	records, err := r.Frames()
	if err != nil {
		return err
	}
	flat, err := timeline.Flatten(p)
	if err != nil {
		return err
	}
	mismatches, err := timeline.VerifyFrames(r, flat)
	if err != nil {
		return err
	}
	markerMismatches, err := timeline.VerifyMarkers(r, flat)
	if err != nil {
		return err
	}
	drifts, err := timeline.CompareMarkers(p)
	if err != nil {
		return err
	}

	printFrameTable(records)
	printMarkers(flat)
	printDrift(drifts)
	printVerify(mismatches, markerMismatches)
	printCycleDemo()

	if *outDir != "" {
		return writeOutputs(*outDir, records)
	}
	return nil
}

func printFrameTable(records []timeline.FrameRecord) {
	fmt.Println("== 逐帧父子时间映射（展开前解析） ==")
	fmt.Println("root kind   src  frame internal external combined markers")
	for _, rec := range records {
		fmt.Printf("%4d %-6s %-4s %5d %8s %8s %8s %v\n",
			rec.RootFrame, rec.Kind, rec.SourceID, rec.SourceFrame,
			rec.InternalSpeed, rec.ExternalSpeed, rec.CombinedSpeed, rec.Markers)
	}
	fmt.Println()
}

func printMarkers(ft *timeline.FlatTimeline) {
	fmt.Println("== 展开后标记根帧（逐级 floor） ==")
	for _, m := range ft.Markers {
		fmt.Printf("  %-16s rootFrame=%d origin=%s\n", m.ID, m.RootFrame, m.Origin)
	}
	fmt.Println()
}

func printDrift(drifts []timeline.MarkerDrift) {
	fmt.Println("== 朴素连乘展开的标记漂移 ==")
	if len(drifts) == 0 {
		fmt.Println("  当前工程未产生漂移（需更深嵌套或不同速率才会触发）")
	}
	for _, d := range drifts {
		fmt.Printf("  %s: correct=%d naive=%d drift=%d\n",
			d.MarkerID, d.CorrectRoot, d.NaiveRoot, d.DriftFrames)
	}
	fmt.Println()
}

func printVerify(mismatches []timeline.FrameMismatch, markerMismatches []timeline.MarkerMismatch) {
	fmt.Println("== 展开前后同一画面索引 / 标记覆盖校验 ==")
	if len(mismatches) == 0 {
		fmt.Println("  画面索引 OK：全部根帧的源帧/内部速度/外部速度完全一致")
	}
	for _, m := range mismatches {
		fmt.Println("  MISMATCH:", m)
	}
	if len(markerMismatches) == 0 {
		fmt.Println("  标记覆盖 OK：展开后标记根帧均在解析器命中集合内")
	}
	for _, m := range markerMismatches {
		fmt.Println("  MARKER MISMATCH:", m)
	}
	fmt.Println()
}

func printCycleDemo() {
	fmt.Println("== 循环引用检测（独立分支） ==")
	_, err := timeline.NewResolver(timeline.CyclicProject())
	if err != nil {
		fmt.Println("  已拒绝构建:", err)
	} else {
		fmt.Println("  异常：循环工程未被拦截")
	}
	fmt.Println()
}

func writeOutputs(dir string, records []timeline.FrameRecord) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	csvBytes, err := timeline.ExportCSV(records)
	if err != nil {
		return err
	}
	if err := os.WriteFile(dir+"/frames.csv", csvBytes, 0o644); err != nil {
		return err
	}
	jsonBytes, err := timeline.ExportJSON(records)
	if err != nil {
		return err
	}
	if err := os.WriteFile(dir+"/frames.json", jsonBytes, 0o644); err != nil {
		return err
	}
	fmt.Println("已导出", dir+"/frames.csv", "与", dir+"/frames.json")
	return nil
}
