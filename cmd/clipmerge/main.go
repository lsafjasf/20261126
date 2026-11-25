// Command clipmerge evaluates property-level clip edit merges.
//
// Usage:
//
//	clipmerge run -request req.json [-out report.json]
//	clipmerge serve [-addr 127.0.0.1:8080]
//
// The merge is a pure function of the request: no clocks, no randomness,
// no last-write-wins. Identical requests always produce identical reports.
package main

import (
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"

	"clipmerge"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "run":
		err = cmdRun(os.Args[2:])
	case "serve":
		err = cmdServe(os.Args[2:])
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "clipmerge:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `usage:
  clipmerge run -request req.json [-out report.json]
  clipmerge serve [-addr 127.0.0.1:8080]`)
}

func cmdRun(args []string) error {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	reqPath := fs.String("request", "", "path to the merge request JSON (or - for stdin)")
	outPath := fs.String("out", "", "path to write the report (default stdout)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *reqPath == "" {
		return fmt.Errorf("-request is required")
	}

	var in io.Reader
	if *reqPath == "-" {
		in = os.Stdin
	} else {
		f, err := os.Open(*reqPath)
		if err != nil {
			return err
		}
		defer f.Close()
		in = f
	}
	req, err := clipmerge.DecodeRequest(in)
	if err != nil {
		return err
	}
	rep, err := clipmerge.Merge(req)
	if err != nil {
		return err
	}
	data := rep.Marshal()
	if *outPath == "" {
		_, err = os.Stdout.Write(data)
		return err
	}
	return os.WriteFile(*outPath, data, 0o644)
}

func cmdServe(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	addr := fs.String("addr", "127.0.0.1:8080", "listen address")
	if err := fs.Parse(args); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "clipmerge: listening on %s\n", *addr)
	return http.ListenAndServe(*addr, clipmerge.NewHandler())
}
