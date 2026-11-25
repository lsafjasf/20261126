package main

import (
	"os"

	"ssastyle"
)

func main() {
	os.Exit(ssastyle.RunCLI(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}
