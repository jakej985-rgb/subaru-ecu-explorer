// Package serialenum enumerates USB/serial adapter candidates from the
// host filesystem. It performs no I/O on the devices themselves.
package serialenum

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jakej985-rgb/subaru-ecu-explorer/internal/adapter"
)

// Enumerator scans a device root (normally /dev) for serial adapter
// candidates. Root is configurable so tests can use fixture directories.
type Enumerator struct {
	// Root is the device root to scan; defaults to /dev.
	Root string
}

// New returns an Enumerator rooted at root ("" means /dev).
func New(root string) *Enumerator {
	if root == "" {
		root = "/dev"
	}
	return &Enumerator{Root: root}
}

var ttyPrefixes = []string{"ttyUSB", "ttyACM"}

// Enumerate returns serial candidates. Devices under
// <root>/serial/by-id with stable symlink names become stable candidates;
// bare ttyUSB*/ttyACM* nodes become candidates flagged stable=false because
// their names can change across replug. Read failures are joined into the
// returned error while successfully enumerated candidates are preserved.
func (e *Enumerator) Enumerate(ctx context.Context) ([]adapter.Candidate, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var out []adapter.Candidate
	var errs []error

	// covered holds resolved tty paths already represented by a by-id
	// candidate, so one physical device yields one candidate (and one
	// ownership ID in the Manager).
	covered := map[string]bool{}
	byIDDir := filepath.Join(e.Root, "serial", "by-id")
	entries, err := os.ReadDir(byIDDir)
	switch {
	case err == nil:
		for _, ent := range entries {
			if ent.IsDir() || ent.Type()&fs.ModeSymlink == 0 {
				continue
			}
			name := ent.Name()
			path := filepath.Join(byIDDir, name)
			meta := map[string]string{
				"stable": "true",
				"source": "by-id",
			}
			if target, err := filepath.EvalSymlinks(path); err == nil {
				covered[target] = true
				meta["target"] = target
			}
			out = append(out, adapter.Candidate{
				Info: adapter.Info{
					ID:   "serial/by-id/" + name,
					Path: path,
				},
				Metadata: meta,
			})
		}
	case errors.Is(err, fs.ErrNotExist):
		// Systems without /dev/serial/by-id are fine.
	default:
		errs = append(errs, fmt.Errorf("serial/by-id: %w", err))
	}

	rootEntries, err := os.ReadDir(e.Root)
	if err != nil {
		errs = append(errs, fmt.Errorf("%s: %w", e.Root, err))
	} else {
		for _, ent := range rootEntries {
			name := ent.Name()
			matched := false
			for _, p := range ttyPrefixes {
				if strings.HasPrefix(name, p) {
					matched = true
					break
				}
			}
			if !matched || ent.IsDir() {
				continue
			}
			ttyPath := filepath.Join(e.Root, name)
			if resolved, err := filepath.EvalSymlinks(ttyPath); err == nil && covered[resolved] {
				continue // already listed under its stable by-id name
			}
			out = append(out, adapter.Candidate{
				Info: adapter.Info{
					ID:   "serial/" + name,
					Path: ttyPath,
				},
				Metadata: map[string]string{
					"stable": "false",
					"source": "tty",
				},
			})
		}
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Info.ID < out[j].Info.ID })
	return out, errors.Join(errs...)
}
