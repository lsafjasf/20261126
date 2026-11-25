// Command cea708demo runs a deterministic CEA-708 scenario: a program
// switching between service 1 and service 2, window latching, clear rules and
// rebuild from a recovery point after a network outage.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	cea708 "cea708"
)

type report struct {
	Frames   []cea708.Frame       `json:"frames"`
	Clears   []cea708.ClearRecord `json:"clears"`
	Rules    []RuleStep           `json:"clear_rules"`
	Recovery RecoveryInfo         `json:"recovery"`
}

func main() {
	format := flag.String("format", "text", "output format: text|json")
	out := flag.String("out", "", "write output to file instead of stdout")
	flag.Parse()

	items := switchScript()
	frames, clears, recovery, err := buildRecovery(items)
	if err != nil {
		fmt.Fprintf(os.Stderr, "demo failed: %v\n", err)
		os.Exit(1)
	}
	steps := clearRuleSteps()

	var data []byte
	switch *format {
	case "json":
		data, err = json.MarshalIndent(report{
			Frames: frames, Clears: clears, Rules: steps, Recovery: recovery,
		}, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "json encode: %v\n", err)
			os.Exit(1)
		}
	default:
		fmt.Println("CEA-708 service switch / window latch / clear-rules demo")
		printFrameTimeline(frames, clears)
		printRuleSteps(steps)
		printRecovery(recovery)
		if !recovery.StateEqual {
			os.Exit(2)
		}
		return
	}

	if *out != "" {
		if err := os.WriteFile(*out, data, 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "write %s: %v\n", *out, err)
			os.Exit(1)
		}
		return
	}
	fmt.Println(string(data))
	if !recovery.StateEqual {
		os.Exit(2)
	}
}
