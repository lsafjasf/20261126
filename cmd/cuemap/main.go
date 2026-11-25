package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"cuemap"
)

func main() {
	text := flag.String("text", "", "cue text or ruby markup to typeset")
	id := flag.String("id", "cli-cue", "cue identifier")
	modeName := flag.String("mode", "horizontal", "horizontal or vertical")
	dirName := flag.String("dir", "auto", "auto, ltr, or rtl")
	fixturePath := flag.String("fixtures", "", "optional testdata/cues.json path")
	fixtureName := flag.String("name", "", "fixture name to select")
	flag.Parse()

	var fixture *cuemap.Fixture
	if *fixturePath != "" {
		file, err := cuemap.LoadFixtures(*fixturePath)
		if err != nil {
			fatal(err)
		}
		for i := range file.Fixtures {
			if *fixtureName == "" || file.Fixtures[i].Name == *fixtureName {
				fixture = &file.Fixtures[i]
				break
			}
		}
		if fixture == nil {
			fatal(fmt.Errorf("fixture %q not found", *fixtureName))
		}
	}

	var cue cuemap.Cue
	var options cuemap.Options
	if fixture != nil {
		cue = fixture.Cue
		options = fixture.Options
	} else {
		if *text == "" {
			fatal(fmt.Errorf("either -fixtures or -text is required"))
		}
		cue = cuemap.ParseCue(*id, *text)
		options = cuemap.Options{
			Mode:      parseMode(*modeName),
			Direction: parseDirection(*dirName),
		}
	}

	result := cuemap.Typeset(cue, options)
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(result); err != nil {
		fatal(err)
	}
}

func parseMode(s string) cuemap.WritingMode {
	if s == "vertical" || s == "v" {
		return cuemap.ModeVertical
	}
	return cuemap.ModeHorizontal
}

func parseDirection(s string) cuemap.Direction {
	switch s {
	case "ltr", "left-to-right":
		return cuemap.DirectionLTR
	case "rtl", "right-to-left":
		return cuemap.DirectionRTL
	default:
		return cuemap.DirectionAuto
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "cuemap:", err)
	os.Exit(1)
}
