// medref keeps a catalog of media references whose paths may mix drive
// letters, Unicode names, relative paths and case variants. It preserves
// every original spelling and can rebase the catalog after the project
// moves, listing missing and unmapped references separately.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"medref/internal/medref"
)

const defaultManifest = "medref.json"

func main() {
	if len(os.Args) < 2 {
		usage(os.Stderr)
		os.Exit(2)
	}
	cmd := os.Args[1]
	args := os.Args[2:]
	var err error
	switch cmd {
	case "init":
		err = cmdInit(args)
	case "add":
		err = cmdAdd(args)
	case "map":
		err = cmdMap(args)
	case "status":
		err = cmdStatus(args)
	case "recheck":
		err = cmdRecheck(args)
	case "relocate":
		err = cmdRelocate(args)
	case "help", "-h", "--help":
		usage(os.Stdout)
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", cmd)
		usage(os.Stderr)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func usage(w io.Writer) {
	fmt.Fprint(w, `medref - normalize media references and relocate them after a move

Usage:
  medref init     -root DIR [-platform linux|windows|darwin] [-m FILE]
  medref add      [-m FILE] [-from FILE | PATH ...]
  medref map      -volume VOL -target DIR [-m FILE]
  medref status   [-m FILE] [-json]
  medref recheck  [-m FILE] [-json]
  medref relocate -root NEWDIR [-m FILE] [-json]

Original reference strings are never rewritten; normalization only feeds
the canonical key and merge decisions.
`)
}

func cmdInit(args []string) error {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	root := fs.String("root", ".", "project root directory")
	platName := fs.String("platform", "linux", "case semantics: linux|windows|darwin")
	m := fs.String("m", defaultManifest, "manifest file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	plat, ok := medref.ParsePlatform(*platName)
	if !ok {
		return fmt.Errorf("unsupported platform %q", *platName)
	}
	store, err := medref.NewStore(*root, plat)
	if err != nil {
		return err
	}
	if err := store.Save(*m); err != nil {
		return err
	}
	fmt.Printf("initialized %s for project %s (%s)\n", *m, store.Root(), *platName)
	return nil
}

func cmdAdd(args []string) error {
	fs := flag.NewFlagSet("add", flag.ContinueOnError)
	m := fs.String("m", defaultManifest, "manifest file")
	from := fs.String("from", "", "read references (one per line) from FILE (- for stdin)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	raws, err := collectRaws(fs.Args(), *from)
	if err != nil {
		return err
	}
	if len(raws) == 0 {
		return fmt.Errorf("add needs at least one PATH or -from FILE")
	}
	store, err := medref.LoadStore(*m)
	if err != nil {
		return err
	}
	created, merged, dupes := 0, 0, 0
	for _, raw := range raws {
		res, err := store.Add(raw)
		if err != nil {
			return err
		}
		switch {
		case res.Created:
			created++
		case res.Duplicate:
			dupes++
		default:
			merged++
		}
	}
	if err := store.Save(*m); err != nil {
		return err
	}
	fmt.Printf("added: %d new, %d merged, %d duplicates; %d total entries\n",
		created, merged, dupes, len(store.Manifest().Entries))
	return nil
}
