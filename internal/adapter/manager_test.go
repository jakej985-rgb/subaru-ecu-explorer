package adapter_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jakej985-rgb/subaru-ecu-explorer/internal/adapter"
	"github.com/jakej985-rgb/subaru-ecu-explorer/internal/adapter/fake"
)

func cand(kind adapter.Kind, id string) adapter.Candidate {
	return adapter.Candidate{Info: adapter.Info{ID: id, Kind: kind, Path: "/dev/" + id}}
}

func TestManagerDiscoverAcrossDrivers(t *testing.T) {
	m := adapter.NewManager()
	m.RegisterDriver(fake.NewDriver(adapter.KindELM327, cand(adapter.KindELM327, "elm-a"), cand(adapter.KindELM327, "elm-b")))
	m.RegisterDriver(fake.NewDriver(adapter.KindArduino, cand(adapter.KindArduino, "mega-1")))

	cands, err := m.Discover(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(cands) != 3 {
		t.Fatalf("expected 3 candidates, got %d", len(cands))
	}
	kinds := map[adapter.Kind]int{}
	for _, c := range cands {
		kinds[c.Info.Kind]++
	}
	if kinds[adapter.KindELM327] != 2 || kinds[adapter.KindArduino] != 1 {
		t.Fatalf("unexpected kind distribution: %v", kinds)
	}
}

func TestManagerExclusiveOwnership(t *testing.T) {
	m := adapter.NewManager()
	d := fake.NewDriver(adapter.KindELM327, cand(adapter.KindELM327, "elm-a"))
	m.RegisterDriver(d)

	s1, err := m.Connect(context.Background(), cand(adapter.KindELM327, "elm-a"))
	if err != nil {
		t.Fatal(err)
	}
	if s1 == nil {
		t.Fatal("expected session")
	}

	// Second connect to the same physical device must fail.
	if _, err := m.Connect(context.Background(), cand(adapter.KindELM327, "elm-a")); !errors.Is(err, adapter.ErrBusy) {
		t.Fatalf("expected adapter.ErrBusy, got %v", err)
	}

	if err := m.Disconnect("elm-a"); err != nil {
		t.Fatal(err)
	}
	// After disconnect, reconnect works.
	if _, err := m.Connect(context.Background(), cand(adapter.KindELM327, "elm-a")); err != nil {
		t.Fatalf("reconnect after disconnect failed: %v", err)
	}
}

func TestManagerUnknownKindAndCandidate(t *testing.T) {
	m := adapter.NewManager()
	if _, err := m.Connect(context.Background(), cand(adapter.Kind("subaru-ssm"), "x")); !errors.Is(err, adapter.ErrUnsupported) {
		t.Fatalf("expected adapter.ErrUnsupported, got %v", err)
	}
	m.RegisterDriver(fake.NewDriver(adapter.KindELM327, cand(adapter.KindELM327, "elm-a")))
	if _, err := m.Connect(context.Background(), cand(adapter.KindELM327, "missing")); !errors.Is(err, adapter.ErrNotFound) {
		t.Fatalf("expected adapter.ErrNotFound, got %v", err)
	}
}

func TestManagerHealthAndSessionLifecycle(t *testing.T) {
	m := adapter.NewManager()
	m.RegisterDriver(fake.NewDriver(adapter.KindArduino, cand(adapter.KindArduino, "mega-1")))

	if h := m.Health("mega-1"); h.State != adapter.HealthClosed {
		t.Fatalf("expected closed health for unknown session, got %s", h.State)
	}
	s, err := m.Connect(context.Background(), cand(adapter.KindArduino, "mega-1"))
	if err != nil {
		t.Fatal(err)
	}
	if s.Info().Kind != adapter.KindArduino {
		t.Fatalf("expected kind %s, got %s", adapter.KindArduino, s.Info().Kind)
	}
	if !s.Capabilities().ReadOnly {
		t.Fatal("fake session should declare read-only capability")
	}
	if h := m.Health("mega-1"); h.State != adapter.HealthReady {
		t.Fatalf("expected ready health, got %s", h.State)
	}
	if err := m.Disconnect("mega-1"); err != nil {
		t.Fatal(err)
	}
	if m.Session("mega-1") != nil {
		t.Fatal("session should be released after disconnect")
	}
	if h := m.Health("mega-1"); h.State != adapter.HealthClosed {
		t.Fatalf("expected closed health after disconnect, got %s", h.State)
	}
	// Disconnect is idempotent.
	if err := m.Disconnect("mega-1"); err != nil {
		t.Fatal(err)
	}
}

func TestManagerDiscoverErrorPropagation(t *testing.T) {
	m := adapter.NewManager()
	d := fake.NewDriver(adapter.KindELM327)
	d.SetDiscoverError(errors.New("usb subsystem down"))
	m.RegisterDriver(d)
	m.RegisterDriver(fake.NewDriver(adapter.KindArduino, cand(adapter.KindArduino, "mega-1")))

	cands, err := m.Discover(context.Background())
	if err == nil {
		t.Fatal("expected discover error")
	}
	// Healthy driver still contributes candidates.
	if len(cands) != 1 {
		t.Fatalf("expected 1 candidate from healthy driver, got %d", len(cands))
	}
}

func TestManagerConnectCancellation(t *testing.T) {
	m := adapter.NewManager()
	d := fake.NewDriver(adapter.KindELM327, cand(adapter.KindELM327, "elm-a"))
	m.RegisterDriver(d)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := m.Connect(ctx, cand(adapter.KindELM327, "elm-a")); err == nil {
		t.Fatal("expected cancelled context to fail connect")
	}
	if len(m.ConnectedIDs()) != 0 {
		t.Fatal("cancelled connect must not leave a session")
	}
}
