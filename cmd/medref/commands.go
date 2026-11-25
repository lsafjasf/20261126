package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"medref/internal/medref"
)

func cmdMap(args []string) error {
	fs := flag.NewFlagSet("map", flag.ContinueOnError)
	m := fs.String("m", defaultManifest, "manifest file")
	volume := fs.String("volume", "", "volume key, e.g. D: or a UNC share")
	target := fs.String("target", "", "local directory that backs the volume")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *volume == "" || *target == "" {
		return fmt.Errorf("map needs -volume and -target")
	}
	store, err := medref.LoadStore(*m)
	if err != nil {
		return err
	}
	store.SetRootMapping(*volume, *target)
	if err := store.Save(*m); err != nil {
		return err
	}
	fmt.Printf("mapped %s -> %s\n", *volume, *target)
	return nil
}

func cmdStatus(args []string) error {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	m := fs.String("m", defaultManifest, "manifest file")
	asJSON := fs.Bool("json", false, "emit JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	store, err := medref.LoadStore(*m)
	if err != nil {
		return err
	}
	missing := store.Recheck()
	if err := store.Save(*m); err != nil {
		return err
	}
	if *asJSON {
		return writeJSON(map[string]any{
			"project": store.Manifest().Project,
			"total":   len(store.Manifest().Entries),
			"missing": missing,
			"entries": store.Manifest().Entries,
		})
	}
	printStatus(store, missing)
	return nil
}

func cmdRecheck(args []string) error {
	fs := flag.NewFlagSet("recheck", flag.ContinueOnError)
	m := fs.String("m", defaultManifest, "manifest file")
	asJSON := fs.Bool("json", false, "emit JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	store, err := medref.LoadStore(*m)
	if err != nil {
		return err
	}
	missing := store.Recheck()
	if err := store.Save(*m); err != nil {
		return err
	}
	if *asJSON {
		return writeJSON(map[string]any{"missing": missing})
	}
	if len(missing) == 0 {
		fmt.Println("all references resolve")
		return nil
	}
	fmt.Println("missing:")
	for _, ref := range missing {
		fmt.Println("  " + ref)
	}
	return nil
}

func cmdRelocate(args []string) error {
	fs := flag.NewFlagSet("relocate", flag.ContinueOnError)
	m := fs.String("m", defaultManifest, "manifest file")
	root := fs.String("root", "", "new project root directory")
	asJSON := fs.Bool("json", false, "emit JSON report")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *root == "" {
		return fmt.Errorf("relocate needs -root NEWDIR")
	}
	store, err := medref.LoadStore(*m)
	if err != nil {
		return err
	}
	report, err := store.Relocate(*root)
	if err != nil {
		return err
	}
	if err := store.Save(*m); err != nil {
		return err
	}
	if *asJSON {
		return writeJSON(report)
	}
	printReport(report)
	return nil
}

func collectRaws(positional []string, from string) ([]string, error) {
	out := append([]string{}, positional...)
	if from == "" {
		return out, nil
	}
	var data []byte
	var err error
	if from == "-" {
		data, err = io.ReadAll(os.Stdin)
	} else {
		data, err = os.ReadFile(from)
	}
	if err != nil {
		return nil, err
	}
	start := 0
	for i, b := range data {
		if b == '\n' {
			if line := trimLine(data[start:i]); line != "" {
				out = append(out, line)
			}
			start = i + 1
		}
	}
	if line := trimLine(data[start:]); line != "" {
		out = append(out, line)
	}
	return out, nil
}

func trimLine(b []byte) string {
	s := string(b)
	for len(s) > 0 && (s[len(s)-1] == '\r' || s[len(s)-1] == ' ' || s[len(s)-1] == '\t') {
		s = s[:len(s)-1]
	}
	for len(s) > 0 && (s[0] == ' ' || s[0] == '\t') {
		s = s[1:]
	}
	return s
}

func writeJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
