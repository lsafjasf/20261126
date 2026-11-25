// Command timelinesnap manages compact version snapshots of collaborative
// editing timelines: reachable-only captures, content-aware diffs, rollback
// and bounded on-disk history.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"

	"timelinesnap"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "timelinesnap:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		usage()
		return errors.New("missing command")
	}
	cmd, rest := args[0], args[1:]
	switch cmd {
	case "snapshot":
		return cmdSnapshot(rest)
	case "list":
		return cmdList(rest)
	case "show":
		return cmdShow(rest)
	case "diff":
		return cmdDiff(rest)
	case "restore":
		return cmdRestore(rest)
	case "revert":
		return cmdRevert(rest)
	case "gc":
		return cmdGC(rest)
	case "help", "-h", "--help":
		usage()
		return nil
	default:
		return fmt.Errorf("unknown command %q", cmd)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `timelinesnap - reachable-only timeline snapshots

usage:
  timelinesnap snapshot -project project.json [-store .timelinesnap] [-m msg] [-a author]
  timelinesnap list     [-store .timelinesnap]
  timelinesnap show     -rev N [-store .timelinesnap]
  timelinesnap diff     -old N -new N [-store .timelinesnap]
  timelinesnap restore  -rev N [-store .timelinesnap] [-out project.json]
  timelinesnap revert   -rev N [-store .timelinesnap] [-m msg]
  timelinesnap gc       [-store .timelinesnap] [-keep N]
`)
}

func openStore(path string) (*timelinesnap.FileStore, error) {
	return timelinesnap.OpenFileStore(path)
}

func loadProject(path string) (*timelinesnap.Project, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var p timelinesnap.Project
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

func printJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func cmdSnapshot(args []string) error {
	fs := flag.NewFlagSet("snapshot", flag.ContinueOnError)
	storePath := fs.String("store", ".timelinesnap", "snapshot store directory")
	projectPath := fs.String("project", "", "project JSON file")
	message := fs.String("m", "", "commit message")
	author := fs.String("a", "", "commit author")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *projectPath == "" {
		return errors.New("-project is required")
	}
	project, err := loadProject(*projectPath)
	if err != nil {
		return err
	}
	store, err := openStore(*storePath)
	if err != nil {
		return err
	}
	defer store.Close()

	result, err := timelinesnap.SnapshotProject(context.Background(), store, project, *message, *author)
	if err != nil {
		return err
	}
	if !result.Created {
		fmt.Println("no content changes; head unchanged at revision", result.Commit.Revision)
	} else {
		fmt.Println("committed revision", result.Commit.Revision, "snapshot", result.Commit.SnapshotID[:12])
	}
	fmt.Printf("reachable=%d dropped=%d dangling=%d\n",
		result.Stats.ReachableResources,
		result.Stats.DroppedResources,
		len(result.Stats.DanglingRefs))
	return nil
}

func cmdList(args []string) error {
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	storePath := fs.String("store", ".timelinesnap", "snapshot store directory")
	if err := fs.Parse(args); err != nil {
		return err
	}
	store, err := openStore(*storePath)
	if err != nil {
		return err
	}
	defer store.Close()
	commits, err := store.List(context.Background())
	if err != nil {
		return err
	}
	for _, c := range commits {
		extra := ""
		if c.RevertOf > 0 {
			extra = fmt.Sprintf(" revert-of=%d", c.RevertOf)
		}
		fmt.Printf("rev=%d snap=%s author=%q%s %s\n",
			c.Revision, c.SnapshotID[:12], c.Author, extra, c.Message)
	}
	if len(commits) == 0 {
		fmt.Println("(no snapshots)")
	}
	return nil
}

func cmdShow(args []string) error {
	fs := flag.NewFlagSet("show", flag.ContinueOnError)
	storePath := fs.String("store", ".timelinesnap", "snapshot store directory")
	rev := fs.Int("rev", 0, "commit revision")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *rev == 0 {
		return errors.New("-rev is required")
	}
	store, err := openStore(*storePath)
	if err != nil {
		return err
	}
	defer store.Close()
	project, err := timelinesnap.RestoreCommit(context.Background(), store, *rev)
	if err != nil {
		return err
	}
	return printJSON(project)
}

func cmdDiff(args []string) error {
	fs := flag.NewFlagSet("diff", flag.ContinueOnError)
	storePath := fs.String("store", ".timelinesnap", "snapshot store directory")
	oldRev := fs.Int("old", 0, "older revision")
	newRev := fs.Int("new", 0, "newer revision")
	quiet := fs.Bool("check", false, "exit 1 on content changes, 0 when order-only")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *oldRev == 0 || *newRev == 0 {
		return errors.New("-old and -new are required")
	}
	store, err := openStore(*storePath)
	if err != nil {
		return err
	}
	defer store.Close()
	report, err := timelinesnap.CompareRevisions(context.Background(), store, *oldRev, *newRev)
	if err != nil {
		return err
	}
	if err := printJSON(report); err != nil {
		return err
	}
	if *quiet && report.HasContentChanges() {
		os.Exit(1)
	}
	return nil
}

func cmdRestore(args []string) error {
	fs := flag.NewFlagSet("restore", flag.ContinueOnError)
	storePath := fs.String("store", ".timelinesnap", "snapshot store directory")
	rev := fs.Int("rev", 0, "commit revision")
	out := fs.String("out", "", "output project JSON (default stdout)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *rev == 0 {
		return errors.New("-rev is required")
	}
	store, err := openStore(*storePath)
	if err != nil {
		return err
	}
	defer store.Close()
	project, err := timelinesnap.RestoreCommit(context.Background(), store, *rev)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(project, "", "  ")
	if err != nil {
		return err
	}
	if *out == "" {
		_, err = os.Stdout.Write(append(data, '\n'))
		return err
	}
	return os.WriteFile(*out, append(data, '\n'), 0o644)
}

func cmdRevert(args []string) error {
	fs := flag.NewFlagSet("revert", flag.ContinueOnError)
	storePath := fs.String("store", ".timelinesnap", "snapshot store directory")
	rev := fs.Int("rev", 0, "revision to restore")
	message := fs.String("m", "", "commit message")
	author := fs.String("a", "", "commit author")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *rev == 0 {
		return errors.New("-rev is required")
	}
	store, err := openStore(*storePath)
	if err != nil {
		return err
	}
	defer store.Close()
	commit, err := store.Revert(context.Background(), *rev, *message, *author)
	if err != nil {
		return err
	}
	fmt.Println("new revision", commit.Revision, "restores snapshot of revision", strconv.Itoa(commit.RevertOf))
	return nil
}

func cmdGC(args []string) error {
	fs := flag.NewFlagSet("gc", flag.ContinueOnError)
	storePath := fs.String("store", ".timelinesnap", "snapshot store directory")
	keep := fs.Int("keep", 0, "maximum commits to retain (0 = keep all, only orphan blobs removed)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	store, err := openStore(*storePath)
	if err != nil {
		return err
	}
	defer store.Close()
	result, err := store.GC(context.Background(), timelinesnap.RetentionPolicy{MaxCommits: *keep})
	if err != nil {
		return err
	}
	fmt.Printf("removed commits=%d blobs=%d\n", result.RemovedCommits, result.RemovedBlobs)
	return nil
}
