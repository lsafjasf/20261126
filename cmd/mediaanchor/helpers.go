package main

import (
	"flag"
	"fmt"

	"mediaanchor/anchor"
)

func newFlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	return fs
}

func loadProject(path string) (*anchor.Project, error) {
	if path == "" {
		return nil, fmt.Errorf("missing project path")
	}
	return anchor.Load(path)
}

func requireSinglePath(args []string) (string, error) {
	if len(args) != 1 {
		return "", fmt.Errorf("expected exactly one project path, got %d arguments", len(args))
	}
	return args[0], nil
}
