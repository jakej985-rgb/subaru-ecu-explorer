package serialenum

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func mkroot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "serial", "by-id"), 0o755); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestEnumerateByIDAndTTY(t *testing.T) {
	root := mkroot(t)
	byID := filepath.Join(root, "serial", "by-id")
	// Stable by-id symlink -> /dev/ttyUSB0-style target.
	target := filepath.Join(root, "ttyUSB0")
	if err := os.WriteFile(target, nil, 0o666); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../../ttyUSB0", filepath.Join(byID, "usb-FTDI_FT232R-if00-port0")); err != nil {
		t.Fatal(err)
	}
	// A second, different adapter.
	if err := os.WriteFile(filepath.Join(root, "ttyACM0"), nil, 0o666); err != nil {
		t.Fatal(err)
	}

	e := New(root)
	cands, err := e.Enumerate(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	// ttyUSB0 is an alias of the by-id link and must be collapsed into it.
	if len(cands) != 2 {
		t.Fatalf("expected 2 candidates (1 by-id + ttyACM0), got %d: %+v", len(cands), cands)
	}
	for _, c := range cands {
		if c.Info.ID == "serial/ttyUSB0" {
			t.Fatal("ttyUSB0 alias of a by-id device must not be listed separately")
		}
	}

	byIDCand := cands[0]
	if byIDCand.Info.ID != "serial/by-id/usb-FTDI_FT232R-if00-port0" {
		t.Fatalf("unexpected by-id candidate: %+v", byIDCand)
	}
	if byIDCand.Metadata["stable"] != "true" {
		t.Fatal("by-id candidate must be marked stable")
	}
	for _, c := range cands[1:] {
		if c.Metadata["stable"] != "false" {
			t.Fatalf("tty candidate %s must be marked unstable", c.Info.ID)
		}
	}
}

func TestEnumerateHotPlug(t *testing.T) {
	root := mkroot(t)
	e := New(root)
	before, err := e.Enumerate(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(before) != 0 {
		t.Fatalf("expected no candidates initially, got %d", len(before))
	}

	// Simulate hot-plug.
	if err := os.WriteFile(filepath.Join(root, "ttyUSB0"), nil, 0o666); err != nil {
		t.Fatal(err)
	}
	after, err := e.Enumerate(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != 1 || after[0].Info.ID != "serial/ttyUSB0" {
		t.Fatalf("expected ttyUSB0 after hot-plug, got %+v", after)
	}

	// Simulate unplug.
	if err := os.Remove(filepath.Join(root, "ttyUSB0")); err != nil {
		t.Fatal(err)
	}
	gone, err := e.Enumerate(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(gone) != 0 {
		t.Fatalf("expected no candidates after unplug, got %+v", gone)
	}
}

func TestEnumerateAmbiguousDevices(t *testing.T) {
	root := mkroot(t)
	// Two identical-looking adapters: distinct by-id names, same tty style.
	for _, name := range []string{"usb-AdapterA-if00", "usb-AdapterB-if00"} {
		if err := os.Symlink("../../ttyUSB0", filepath.Join(root, "serial", "by-id", name)); err != nil {
			t.Fatal(err)
		}
	}
	cands, err := New(root).Enumerate(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(cands) != 2 {
		t.Fatalf("expected 2 ambiguous candidates, got %d", len(cands))
	}
	// Both remain visible; the user selects explicitly by ID.
	if cands[0].Info.ID == cands[1].Info.ID {
		t.Fatal("ambiguous devices must keep distinct IDs")
	}
}

func TestEnumerateMissingRoot(t *testing.T) {
	e := New(filepath.Join(t.TempDir(), "does-not-exist"))
	cands, err := e.Enumerate(context.Background())
	// Missing root yields no candidates and a joined error, not a panic.
	if len(cands) != 0 {
		t.Fatalf("expected no candidates, got %+v", cands)
	}
	if err == nil {
		t.Fatal("expected error for missing root")
	}
}
