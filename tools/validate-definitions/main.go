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

	if report.OK() {
		if err := vehicle.ValidateRegistry(r); err != nil {
			fmt.Printf("reference check FAIL: %v\n", err)
			failed++
		} else {
			fmt.Printf("reference check ok (%d profiles, %d engines, %d transmissions, %d drivetrains, %d trims, %d configurations, %d hardware, %d catalogs)\n",
				len(r.Profiles), len(r.Engines), len(r.Transmissions), len(r.Drivetrains), len(r.Trims), len(r.Configurations), len(r.Hardware), len(r.Catalogs))
		}
	}

	if failed > 0 {
		fmt.Printf("%d problem(s) found\n", failed)
		os.Exit(1)
	}
	fmt.Println("all definitions valid")
}
