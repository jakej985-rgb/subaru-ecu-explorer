// Command validate-definitions loads every JSON definition under a directory
// (default: data/definitions), validates each document, registers it, and
// runs cross-document reference checks. It exits non-zero if anything fails.
package main

import (
	"fmt"
	"os"

	"github.com/jakej985-rgb/subaru-ecu-explorer/internal/vehicle"
)

func main() {
	root := "data/definitions"
	if len(os.Args) > 1 {
		root = os.Args[1]
	}

	r := vehicle.NewRegistry()
	report := vehicle.LoadDefinitions(root, r)

	failed := 0
	for _, res := range report.Results {
		status := "ok"
		if res.Err != nil {
			status = "FAIL"
			failed++
		}
		fmt.Printf("%-4s %-14s %s", status, res.Kind, res.Path)
		if res.Err != nil {
			fmt.Printf("  (%v)", res.Err)
		}
		fmt.Println()
	}

	// Run reference checks even after load failures so independent problems
	// surface in one pass; unresolved refs may stem from files that failed above.
	if err := vehicle.ValidateRegistry(r); err != nil {
		fmt.Printf("reference check FAIL: %v\n", err)
		if !report.OK() {
			fmt.Println("  (some unresolved references may be caused by files that failed to load)")
		}
		failed++
	} else {
		fmt.Printf("reference check ok (%d profiles, %d engines, %d transmissions, %d drivetrains, %d trims, %d configurations, %d hardware, %d catalogs)\n",
			len(r.Profiles), len(r.Engines), len(r.Transmissions), len(r.Drivetrains), len(r.Trims), len(r.Configurations), len(r.Hardware), len(r.Catalogs))
	}

	for _, cat := range r.Catalogs {
		for _, c := range cat.Conflicts() {
			fmt.Printf("WARN catalog %s: %s\n", cat.ID, c)
		}
	}

	if failed > 0 {
		fmt.Printf("%d problem(s) found\n", failed)
		os.Exit(1)
	}
	fmt.Println("all definitions valid")
}
