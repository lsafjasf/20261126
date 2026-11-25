package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
)

// aafdiag rebuilds the object reference graph of an AAF object dump,
// reports missing nodes and inexpressible properties, and emits a minimal
// re-importable project, or fails with explicit reasons.
//
// Usage:
//
//	aafdiag -in broken.json [-out minimal.json] [-report report.json]
//
// Exit code is 0 when a minimal project was emitted (warnings allowed),
// 1 when fatal problems prevented it, 2 on usage or IO errors.
func main() {
	in := flag.String("in", "", "input AAF object dump (JSON)")
	out := flag.String("out", "", "output path for the minimal project (JSON)")
	rep := flag.String("report", "", "output path for the diagnostic report (JSON)")
	flag.Parse()
	if *in == "" {
		fmt.Fprintln(os.Stderr, "error: -in is required")
		flag.Usage()
		os.Exit(2)
	}
	if code := run(*in, *out, *rep); code != 0 {
		os.Exit(code)
	}
}

func run(inPath, outPath, reportPath string) int {
	data, err := os.ReadFile(inPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: read input: %v\n", err)
		return 2
	}
	proj, err := LoadProject(data)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 2
	}
	graph := BuildGraph(proj)
	report := Diagnose(proj, graph, inPath)
	var minimal *Project
	if report.OK {
		substituted := SubstituteFillers(proj, graph)
		if len(substituted) > 0 {
			graph = BuildGraph(proj) // edges changed; rebuild before emit
		}
		minimal, err = EmitMinimal(proj, graph)
		if err != nil {
			report.OK = false
			report.add(Diag{Severity: SevFatal, Code: "EMIT_FAILED", Message: err.Error()})
		}
	}
	if report.OK && minimal != nil {
		if err := selfCheck(minimal); err != nil {
			report.OK = false
			report.add(Diag{Severity: SevFatal, Code: "SELF_CHECK_FAILED",
				Message: "emitted project failed re-import validation: " + err.Error()})
		}
	}
	if report.OK && outPath != "" {
		encoded, err := MarshalProject(minimal)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: marshal minimal project: %v\n", err)
			return 2
		}
		if err := os.WriteFile(outPath, encoded, 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "error: write output: %v\n", err)
			return 2
		}
	}
	report.sortDiags()
	if reportPath != "" {
		encoded, _ := json.MarshalIndent(report, "", "  ")
		if err := os.WriteFile(reportPath, encoded, 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "error: write report: %v\n", err)
			return 2
		}
	}
	printSummary(report, outPath)
	if !report.OK {
		return 1
	}
	return 0
}

// selfCheck proves the emitted project is re-importable: it must round-trip
// through the loader and produce a graph with no dangling references.
func selfCheck(minimal *Project) error {
	encoded, err := MarshalProject(minimal)
	if err != nil {
		return err
	}
	reloaded, err := LoadProject(encoded)
	if err != nil {
		return err
	}
	reg := BuildGraph(reloaded)
	if missing := reg.MissingEdges(); len(missing) > 0 {
		return fmt.Errorf("%d dangling references remain, first: %s.%s -> %s",
			len(missing), missing[0].FromCls, missing[0].Prop, missing[0].ToID)
	}
	return nil
}

func printSummary(r *Report, outPath string) {
	for _, d := range r.Diags {
		loc := d.ObjectID
		if d.Property != "" {
			loc += "." + d.Property
		}
		if loc != "" {
			fmt.Fprintf(os.Stderr, "[%s] %s: %s (%s)\n", d.Severity, d.Code, d.Message, loc)
		} else {
			fmt.Fprintf(os.Stderr, "[%s] %s: %s\n", d.Severity, d.Code, d.Message)
		}
	}
	if r.OK {
		fmt.Fprintf(os.Stderr, "ok: minimal project emitted to %s (%d warnings)\n",
			outPath, r.Warnings)
	} else {
		fmt.Fprintf(os.Stderr, "FAILED: %d fatal problem(s); no minimal project emitted\n",
			r.Fatals)
	}
}
