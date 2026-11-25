// Command edlrelink re-anchors an offline editor's change list onto the
// current online project. Media is identified by the pair (clip ID, media
// hash); file names are never trusted as identity. Every offline op is
// classified as applied, pending review or dead.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"edlrelink/internal/edl"
)

const usage = `edlrelink - relocate offline EDL edits by clip ID and media hash

Usage:
  edlrelink check  -offline old_library.json -online online_library.json \
                   -changelist offline_changelist.json [-media-root DIR] \
                   [-out-dir DIR]
  edlrelink apply  -offline old_library.json -online online_library.json \
                   -changelist offline_changelist.json [-media-root DIR] \
                   [-out-dir DIR]

check  Classify every op into applied / pending / dead and write the report.
apply  Same classification, plus an importable online change list containing
       only applied ops. Pending and dead ops are never auto-applied.

Flags:
  -media-root  Verify each online manifest hash against files under DIR.
  -out-dir     Where to write outputs (default: current directory).
  -json        Also print the JSON report to stdout.
`

type commonFlags struct {
	offline    string
	online     string
	changelist string
	mediaRoot  string
	outDir     string
	printJSON  bool
}

func parseFlags(args []string) (commonFlags, error) {
	fs := flag.NewFlagSet(args[0], flag.ContinueOnError)
	var c commonFlags
	fs.StringVar(&c.offline, "offline", "", "old/offline media library JSON")
	fs.StringVar(&c.online, "online", "", "current online media library JSON")
	fs.StringVar(&c.changelist, "changelist", "", "offline change list JSON")
	fs.StringVar(&c.mediaRoot, "media-root", "", "root dir for on-disk hash verification")
	fs.StringVar(&c.outDir, "out-dir", ".", "output directory")
	fs.BoolVar(&c.printJSON, "json", false, "print JSON report to stdout")
	if err := fs.Parse(args[1:]); err != nil {
		return c, err
	}
	if c.offline == "" || c.online == "" || c.changelist == "" {
		return c, fmt.Errorf("all of -offline, -online and -changelist are required")
	}
	return c, nil
}

func run(c commonFlags, alsoApply bool) error {
	offline, err := edl.LoadLibrary(c.offline)
	if err != nil {
		return err
	}
	online, err := edl.LoadLibrary(c.online)
	if err != nil {
		return err
	}
	cl, err := edl.LoadChangeList(c.changelist)
	if err != nil {
		return err
	}
	if c.mediaRoot != "" {
		if err := edl.VerifyLibrary(online, c.mediaRoot); err != nil {
			return fmt.Errorf("online media verification: %w", err)
		}
	}

	rep := edl.Resolve(offline, online, cl)
	if err := os.MkdirAll(c.outDir, 0o755); err != nil {
		return err
	}
	reportPath := filepath.Join(c.outDir, "relocation-report.json")
	if err := edl.WriteJSON(reportPath, rep); err != nil {
		return err
	}

	textPath := filepath.Join(c.outDir, "relocation-report.txt")
	tf, err := os.Create(textPath)
	if err != nil {
		return err
	}
	edl.RenderText(tf, rep)
	if err := tf.Close(); err != nil {
		return err
	}

	if alsoApply {
		applied, err := edl.BuildAppliedChangeList(rep)
		if err != nil {
			return err
		}
		if err := edl.WriteJSON(
			filepath.Join(c.outDir, "applied-changelist.json"), applied); err != nil {
			return err
		}
	}

	edl.RenderText(os.Stdout, rep)
	if c.printJSON {
		fmt.Println("--- JSON report ---")
		data, _ := os.ReadFile(reportPath)
		os.Stdout.Write(data)
	}
	fmt.Printf("wrote %s\nwrote %s\n", reportPath, textPath)
	if alsoApply {
		fmt.Printf("wrote %s\n", filepath.Join(c.outDir, "applied-changelist.json"))
	}
	return nil
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	switch os.Args[1] {
	case "-h", "--help", "help":
		fmt.Print(usage)
		return
	}

	var alsoApply bool
	switch os.Args[1] {
	case "check":
		alsoApply = false
	case "apply":
		alsoApply = true
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n%s", os.Args[1], usage)
		os.Exit(2)
	}

	c, err := parseFlags(append([]string{os.Args[1]}, os.Args[2:]...))
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(2)
	}
	if err := run(c, alsoApply); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
