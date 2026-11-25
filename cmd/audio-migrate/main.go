package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"audiotracks"
)

const usageText = `Usage: audio-migrate [-check] [-out FILE] EDIT_TABLE.json

Migrate legacy audio tracks to canonical dialogue/music/effects/dub tracks.
The entire project is rejected if any track or default is invalid.

  -check       validate and migrate without writing output
  -out FILE    atomically write the canonical project to FILE
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("audio-migrate", flag.ContinueOnError)
	flags.SetOutput(stderr)
	outPath := flags.String("out", "", "write canonical project to file")
	checkOnly := flags.Bool("check", false, "validate without output")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 1 {
		fmt.Fprint(stderr, usageText)
		return 2
	}

	input, err := os.ReadFile(flags.Arg(0))
	if err != nil {
		fmt.Fprintf(stderr, "read %s: %v\n", flags.Arg(0), err)
		return 2
	}
	project, err := audiotracks.MigrateJSON(input)
	if err != nil {
		printError(stderr, err)
		return 1
	}
	if *checkOnly {
		return 0
	}

	if *outPath != "" {
		if err := audiotracks.SaveProjectJSONAtomic(*outPath, project); err != nil {
			printError(stderr, err)
			return 2
		}
		return 0
	}

	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(project); err != nil {
		fmt.Fprintf(stderr, "write stdout: %v\n", err)
		return 2
	}
	return 0
}

func printError(writer io.Writer, err error) {
	var validation *audiotracks.ValidationError
	if !errors.As(err, &validation) {
		fmt.Fprintln(writer, err)
		return
	}
	fmt.Fprintf(writer, "%v\n", err)
	for _, problem := range validation.Problems {
		fmt.Fprintf(writer, "  [%s] track=%q language=%q: %s\n",
			problem.Code, problem.TrackID, problem.Language, problem.Message)
	}
}
