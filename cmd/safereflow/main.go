package main

import (
	"os"

	safereflow "safereflow"
)

func main() {
	os.Exit(safereflow.RunCLI(os.Args[1:], os.Stdout, os.Stderr))
}
