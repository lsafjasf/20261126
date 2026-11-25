package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"handlefix"
)

func main() {
	inPath := flag.String("in", "testdata/project.json", "project JSON path")
	tracePath := flag.String("trace", "", "optional trim EdgeEdit JSON path")
	reportPath := flag.String("report", "", "optional report JSON output path")
	exportPath := flag.String("export", "", "optional fixed/exported project JSON path")
	flag.Parse()

	if err := run(*inPath, *tracePath, *reportPath, *exportPath); err != nil {
		fmt.Fprintln(os.Stderr, "handlefix:", err)
		os.Exit(1)
	}
}

func run(inPath, tracePath, reportPath, exportPath string) error {
	project, err := handlefix.LoadProject(inPath)
	if err != nil {
		return err
	}

	var edits []handlefix.EdgeEdit
	if tracePath != "" {
		edits, err = handlefix.LoadEdgeEdits(tracePath)
		if err != nil {
			return err
		}
	}

	report, err := handlefix.AuditTrace(project, edits)
	if err != nil {
		return err
	}
	if reportPath != "" {
		if err := handlefix.WriteJSON(reportPath, report); err != nil {
			return err
		}
	}
	if err := writeIndented(report); err != nil {
		return err
	}

	if exportPath == "" {
		return nil
	}

	source := project
	if tracePath != "" {
		source, err = handlefix.ReverseEdits(project, edits)
		if err != nil {
			return err
		}
	}
	exported, exportReport, err := handlefix.PrepareExport(source)
	if err != nil {
		return err
	}
	if err := handlefix.WriteJSON(exportPath, exported); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "wrote %s (%d transitions, %d hard cuts)\n",
		exportPath, exportReport.Total, exportReport.HardCutCount)
	return nil
}

func writeIndented(value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	_, err = os.Stdout.Write(data)
	return err
}
