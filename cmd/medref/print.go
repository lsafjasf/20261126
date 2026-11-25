package main

import (
	"fmt"

	"medref/internal/medref"
)

func printStatus(store *medref.Store, missing []string) {
	m := store.Manifest()
	fmt.Printf("project: %s\n", m.Project)
	fmt.Printf("entries: %d (missing %d)\n", len(m.Entries), len(missing))
	for _, e := range m.Entries {
		flag := "ok     "
		if e.Missing {
			flag = "MISSING"
		}
		fmt.Printf("  [%s] %s\n", flag, e.Aliases[0])
		for _, alias := range e.Aliases[1:] {
			fmt.Printf("           alias: %s\n", alias)
		}
	}
}

func printReport(r *medref.RelocateReport) {
	fmt.Printf("relocated %s -> %s\n", r.OldRoot, r.NewRoot)
	fmt.Printf("refs: %d, updated: %d, missing: %d, unmapped: %d\n",
		r.TotalRefs, len(r.Updated), len(r.Missing), len(r.Unmapped))
	for _, ref := range r.Missing {
		fmt.Println("  missing:  " + ref)
	}
	for _, ref := range r.Unmapped {
		fmt.Println("  unmapped: " + ref)
	}
}
