// mediarelink re-associates media that went missing during a project
// migration. Matching uses duration, codec, frame hash and audio
// fingerprint only; file names never decide identity. Low-confidence
// matches are blocked, never auto-applied.
//
// Usage:
//
//	mediarelink check -project P.json -media-root DIR [-out-dir DIR]
//	mediarelink apply -project P.json -media-root DIR [-out-dir DIR] [-allow-partial]
//
// check only reports. apply writes project.relinked.json plus the report
// and refuses to write anything while any asset is blocked, unless
// -allow-partial is given. Exit codes: 0 ok, 1 error, 2 blocked.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"mediarelink/internal/match"
	"mediarelink/internal/media"
	"mediarelink/internal/project"
	"mediarelink/internal/report"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) == 0 {
		usage()
		return 1
	}
	cmd, rest := args[0], args[1:]
	switch cmd {
	case "check":
		return runCheck(rest, false)
	case "apply":
		return runCheck(rest, true)
	default:
		usage()
		return 1
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: mediarelink <check|apply> -project P.json -media-root DIR [-out-dir DIR] [-allow-partial]")
}

func runCheck(args []string, apply bool) int {
	fs := flag.NewFlagSet("mediarelink", flag.ContinueOnError)
	projectPath := fs.String("project", "", "migrated project JSON")
	mediaRoot := fs.String("media-root", "", "root of the post-migration media tree")
	outDir := fs.String("out-dir", "relink-out", "output directory")
	allowPartial := fs.Bool("allow-partial", false, "apply confident relinks even if others stay blocked")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if *projectPath == "" || *mediaRoot == "" {
		usage()
		return 1
	}

	proj, err := project.Load(*projectPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "load project:", err)
		return 1
	}
	probes, skipped, err := media.Scan(*mediaRoot)
	if err != nil {
		fmt.Fprintln(os.Stderr, "scan media:", err)
		return 1
	}

	present, missing := proj.Partition(*mediaRoot)
	plans := match.Plan(missing, probes)

	effects := proj.EffectsByMedia()
	for i := range plans {
		plans[i].Effects = effects[plans[i].MediaID]
	}

	rep := report.Report{
		Project:      proj.Name,
		MediaRoot:    *mediaRoot,
		Summary:      match.Summarize(plans, len(present)),
		Present:      present,
		Assets:       plans,
		SkippedFiles: skipped,
	}

	var relinkErr *project.RelinkError
	if apply {
		originalIDs := make([]string, 0, len(proj.Media))
		for _, a := range proj.Media {
			originalIDs = append(originalIDs, a.ID)
		}
		rerr, err := proj.Apply(plans, *allowPartial)
		if err != nil {
			fmt.Fprintln(os.Stderr, "apply:", err)
			return 1
		}
		if rerr != nil {
			relinkErr = rerr
		} else {
			outProject := filepath.Join(*outDir, "project.relinked.json")
			if err := os.MkdirAll(*outDir, 0o755); err != nil {
				fmt.Fprintln(os.Stderr, err)
				return 1
			}
			if err := proj.Save(outProject); err != nil {
				fmt.Fprintln(os.Stderr, "save project:", err)
				return 1
			}
			reloaded, err := project.Load(outProject)
			if err != nil {
				fmt.Fprintln(os.Stderr, "reload project:", err)
				return 1
			}
			v := project.Verify(originalIDs, reloaded, plans)
			rep.Verification = &v
			if !v.OK() {
				if _, _, werr := report.WriteAll(rep, *outDir); werr != nil {
					fmt.Fprintln(os.Stderr, "write report:", werr)
				}
				fmt.Fprintln(os.Stderr, "traceability verification failed, see report")
				return 1
			}
			fmt.Println("wrote", outProject)
		}
	}

	jsonPath, txtPath, err := report.WriteAll(rep, *outDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "write report:", err)
		return 1
	}
	fmt.Printf("assets: %d total, %d present, %d missing -> %d relinked, %d blocked\n",
		rep.Summary.Total, rep.Summary.OK, rep.Summary.Missing,
		rep.Summary.Relinked, rep.Summary.Blocked)
	fmt.Println("report:", jsonPath, "and", txtPath)

	if relinkErr != nil {
		fmt.Fprintln(os.Stderr, "BLOCKED:", relinkErr.Error())
		fmt.Fprintln(os.Stderr, "nothing written; resolve the blocked assets or rerun with -allow-partial")
		return 2
	}
	return 0
}
