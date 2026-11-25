package main

import (
	"fmt"
	"strings"

	cea708 "cea708"
)

// renderBox crops a frame to its occupied rows and draws an ASCII caption box.
func renderBox(f cea708.Frame) string {
	rows := f.NonEmptyLines()
	if len(rows) == 0 {
		rows = []string{"(blank screen)"}
	}
	width := 0
	for _, r := range rows {
		if w := len([]rune(r)); w > width {
			width = w
		}
	}
	var b strings.Builder
	border := "+" + strings.Repeat("-", width+2) + "+"
	b.WriteString(border + "\n")
	for _, r := range rows {
		pad := width - len([]rune(r))
		fmt.Fprintf(&b, "| %s%s |\n", r, strings.Repeat(" ", pad))
	}
	b.WriteString(border)
	return b.String()
}

func printFrameTimeline(frames []cea708.Frame, clears []cea708.ClearRecord) {
	clearByFrame := map[int64]cea708.ClearRecord{}
	for _, c := range clears {
		clearByFrame[c.FrameIndex] = c
	}

	fmt.Println("== service switch timeline ==")
	for _, f := range frames {
		tag := fmt.Sprintf("#%d service=%d %s seq=%d", f.Index, f.Service, f.Kind, f.StreamSeq)
		if c, ok := clearByFrame[f.Index]; ok {
			tag += fmt.Sprintf("  [CLEAR %s %d->%d]", c.Reason, c.From, c.To)
		}
		fmt.Println(tag)
		for _, line := range strings.Split(renderBox(f), "\n") {
			fmt.Println("   " + line)
		}
	}
}

func printRuleSteps(steps []RuleStep) {
	fmt.Println("\n== clear-screen rules on one service ==")
	for _, s := range steps {
		fmt.Printf("%d. %s - %s\n", s.Step, s.Rule, s.Explanation)
		for _, line := range strings.Split(renderBox(s.Frame), "\n") {
			fmt.Println("   " + line)
		}
	}
}

func printRecovery(info RecoveryInfo) {
	fmt.Println("\n== network interruption / recovery point rebuild ==")
	fmt.Printf("checkpoint seq=%d boundary=%d token=%d bytes\n",
		info.CheckpointSeq, info.Boundary, len(info.Token))
	if info.OutageError != "" {
		fmt.Printf("resume while down -> %s\n", info.OutageError)
	}
	fmt.Printf("after reconnect: snapshot=%v replayed=%d items (seq %d..%d)\n",
		info.Rebuild.UsedSnapshot, info.Rebuild.Applied,
		info.Rebuild.StartSeq, info.Rebuild.FinalSeq)
	fmt.Printf("rebuilt screen identical to uninterrupted decode: %v\n", info.StateEqual)
}
