package adapter_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/jakej985-rgb/subaru-ecu-explorer/internal/adapter"
	"github.com/jakej985-rgb/subaru-ecu-explorer/internal/adapter/fake"
)

// gatedDriver blocks inside Open and/or Close until released, so tests can
// deterministically interleave Connect/Disconnect calls.
type gatedDriver struct {
	openEntered, openRelease   chan struct{}
	closeEntered, closeRelease chan struct{}
	opens                      int32
}

func (d *gatedDriver) Kind() adapter.Kind { return adapter.KindELM327 }

func (d *gatedDriver) Discover(context.Context) ([]adapter.Candidate, error) { return nil, nil }

func (d *gatedDriver) Open(_ context.Context, c adapter.Candidate) (adapter.Session, error) {
	atomic.AddInt32(&d.opens, 1)
	if d.openEntered != nil {
		d.openEntered <- struct{}{}
		<-d.openRelease
	}
	s := fake.NewSession(c.Info, adapter.Capabilities{ReadOnly: true, Transport: "fake"}, fake.NewTransport())
	if d.closeEntered != nil {
		return &slowCloseSession{Session: s, entered: d.closeEntered, release: d.closeRelease}, nil
	}
	return s, nil
}

type slowCloseSession struct {
	*fake.Session
	entered, release chan struct{}
}

func (s *slowCloseSession) Close() error {
	s.entered <- struct{}{}
	<-s.release
	return s.Session.Close()
}

func TestManagerConnectReservesDuringOpen(t *testing.T) {
	d := &gatedDriver{openEntered: make(chan struct{}), openRelease: make(chan struct{})}
	m := adapter.NewManager()
	m.RegisterDriver(d)

	done := make(chan error, 1)
	go func() {
		_, err := m.Connect(context.Background(), cand(adapter.KindELM327, "elm-a"))
		done <- err
	}()
	<-d.openEntered // first Connect is inside Open

	if _, err := m.Connect(context.Background(), cand(adapter.KindELM327, "elm-a")); !errors.Is(err, adapter.ErrBusy) {
		t.Fatalf("expected ErrBusy while opening, got %v", err)
	}
	if m.Session("elm-a") != nil || len(m.ConnectedIDs()) != 0 {
		t.Fatal("opening reservation must not be exposed as an active session")
	}
	close(d.openRelease)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if n := atomic.LoadInt32(&d.opens); n != 1 {
		t.Fatalf("expected exactly 1 Open, got %d", n)
	}
}

func TestManagerDisconnectHoldsReservationUntilClosed(t *testing.T) {
	d := &gatedDriver{closeEntered: make(chan struct{}), closeRelease: make(chan struct{})}
	m := adapter.NewManager()
	m.RegisterDriver(d)
	if _, err := m.Connect(context.Background(), cand(adapter.KindELM327, "elm-a")); err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() { done <- m.Disconnect("elm-a") }()
	<-d.closeEntered // Disconnect is inside Close

	if _, err := m.Connect(context.Background(), cand(adapter.KindELM327, "elm-a")); !errors.Is(err, adapter.ErrBusy) {
		t.Fatalf("expected ErrBusy while closing, got %v", err)
	}
	close(d.closeRelease)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	d.closeEntered = nil // reconnected session closes without gating
	if _, err := m.Connect(context.Background(), cand(adapter.KindELM327, "elm-a")); err != nil {
		t.Fatalf("reconnect after close failed: %v", err)
	}
}

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
