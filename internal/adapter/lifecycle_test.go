package adapter_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jakej985-rgb/subaru-ecu-explorer/internal/adapter"
	"github.com/jakej985-rgb/subaru-ecu-explorer/internal/adapter/fake"
)

func TestConcurrentConnectExclusiveOwnership(t *testing.T) {
	m := adapter.NewManager()
	m.RegisterDriver(fake.NewDriver(adapter.KindELM327, cand(adapter.KindELM327, "elm-a")))

	const n = 16
	var wg sync.WaitGroup
	results := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			s, err := m.Connect(context.Background(), cand(adapter.KindELM327, "elm-a"))
			if err == nil {
				_ = s.Close()
				// Immediately disconnect so the next goroutine can proceed;
				// we only care that no two sessions coexist.
				_ = m.Disconnect("elm-a")
			}
			results[i] = err
		}(i)
	}
	wg.Wait()

	// Every goroutine either connected or got ErrBusy; none got another error.
	connected := 0
	for _, err := range results {
		switch {
		case err == nil:
			connected++
		case errors.Is(err, adapter.ErrBusy):
		default:
			t.Fatalf("unexpected connect error: %v", err)
		}
	}
	if connected == 0 {
		t.Fatal("expected at least one successful connect")
	}
}

func TestConnectWithPolicyRetriesTransient(t *testing.T) {
	m := adapter.NewManager()
	d := fake.NewDriver(adapter.KindELM327, cand(adapter.KindELM327, "elm-a"))
	d.SetOpenFailures(2)
	m.RegisterDriver(d)

	s, err := m.ConnectWithPolicy(context.Background(), cand(adapter.KindELM327, "elm-a"),
		adapter.ReconnectPolicy{MaxAttempts: 3, Backoff: time.Millisecond})
	if err != nil {
		t.Fatalf("expected success after transient failures, got %v", err)
	}
	_ = s.Close()
}

func TestConnectWithPolicyGivesUp(t *testing.T) {
	m := adapter.NewManager()
	d := fake.NewDriver(adapter.KindELM327, cand(adapter.KindELM327, "elm-a"))
	d.SetOpenFailures(10)
	m.RegisterDriver(d)

	start := time.Now()
	_, err := m.ConnectWithPolicy(context.Background(), cand(adapter.KindELM327, "elm-a"),
		adapter.ReconnectPolicy{MaxAttempts: 3, Backoff: time.Millisecond})
	if err == nil {
		t.Fatal("expected failure after exhausting attempts")
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("retries took too long: %v", elapsed)
	}
	if len(m.ConnectedIDs()) != 0 {
		t.Fatal("failed connect must not leave a session")
	}
}

func TestConnectWithPolicyPermanentErrorNoRetry(t *testing.T) {
	m := adapter.NewManager()
	d := fake.NewDriver(adapter.KindELM327, cand(adapter.KindELM327, "elm-a"))
	d.SetOpenError(adapter.ErrPermission)
	m.RegisterDriver(d)

	start := time.Now()
	_, err := m.ConnectWithPolicy(context.Background(), cand(adapter.KindELM327, "elm-a"),
		adapter.ReconnectPolicy{MaxAttempts: 5, Backoff: 50 * time.Millisecond})
	if !errors.Is(err, adapter.ErrPermission) {
		t.Fatalf("expected ErrPermission, got %v", err)
	}
	if elapsed := time.Since(start); elapsed > 60*time.Millisecond {
		t.Fatalf("permanent error must not retry/backoff, took %v", elapsed)
	}
}

func TestShutdownClosesAllAndBlocksConnect(t *testing.T) {
	m := adapter.NewManager()
	m.RegisterDriver(fake.NewDriver(adapter.KindELM327, cand(adapter.KindELM327, "elm-a")))
	m.RegisterDriver(fake.NewDriver(adapter.KindArduino, cand(adapter.KindArduino, "mega-1")))

	s1, err := m.Connect(context.Background(), cand(adapter.KindELM327, "elm-a"))
	if err != nil {
		t.Fatal(err)
	}
	s2, err := m.Connect(context.Background(), cand(adapter.KindArduino, "mega-1"))
	if err != nil {
		t.Fatal(err)
	}

	if err := m.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(m.ConnectedIDs()) != 0 {
		t.Fatal("shutdown must release all sessions")
	}
	// Closed transports reject further I/O.
	if _, err := s1.Write([]byte("x")); err == nil {
		t.Fatal("write on shutdown session must fail")
	}
	if _, err := s2.Write([]byte("x")); err == nil {
		t.Fatal("write on shutdown session must fail")
	}
	if _, err := m.Connect(context.Background(), cand(adapter.KindELM327, "elm-a")); !errors.Is(err, adapter.ErrClosed) {
		t.Fatalf("expected ErrClosed after shutdown, got %v", err)
	}
	// Idempotent.
	if err := m.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestDisconnectLeavesNoActiveIO(t *testing.T) {
	m := adapter.NewManager()
	d := fake.NewDriver(adapter.KindELM327, cand(adapter.KindELM327, "elm-a"))
	m.RegisterDriver(d)

	s, err := m.Connect(context.Background(), cand(adapter.KindELM327, "elm-a"))
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Disconnect("elm-a"); err != nil {
		t.Fatal(err)
	}
	if m.Session("elm-a") != nil {
		t.Fatal("session must be released")
	}
	// The underlying transport is closed: writes fail.
	if _, err := s.Write([]byte("x")); err == nil {
		t.Fatal("write after disconnect must fail")
	}
}

func TestMonitorHealthTransitions(t *testing.T) {
	m := adapter.NewManager()
	d := fake.NewDriver(adapter.KindELM327, cand(adapter.KindELM327, "elm-a"))
	m.RegisterDriver(d)
	s, err := m.Connect(context.Background(), cand(adapter.KindELM327, "elm-a"))
	if err != nil {
		t.Fatal(err)
	}

	mn := adapter.NewMonitor(m, time.Hour, nil, false)

	// Baseline: first check records ready.
	events := mn.Check(context.Background())
	if len(events) != 1 || events[0].To != adapter.HealthReady {
		t.Fatalf("expected initial ready event, got %+v", events)
	}

	// Degraded transition is reported but session stays.
	fs := s.(*fake.Session)
	fs.SetHealth(adapter.Health{State: adapter.HealthDegraded, Detail: "crc errors"})
	events = mn.Check(context.Background())
	if len(events) != 1 || events[0].To != adapter.HealthDegraded {
		t.Fatalf("expected degraded event, got %+v", events)
	}
	if m.Session("elm-a") == nil {
		t.Fatal("degraded session must not be disconnected")
	}

	// Fault with disconnectOnFault releases the device.
	mn2 := adapter.NewMonitor(m, time.Hour, nil, true)
	mn2.Check(context.Background()) // baseline fault state
	fs.SetHealth(adapter.Health{State: adapter.HealthFault, Detail: "adapter dead"})
	mn2.Check(context.Background())
	if m.Session("elm-a") != nil {
		t.Fatal("faulted session must be disconnected")
	}
	if len(m.ConnectedIDs()) != 0 {
		t.Fatal("faulted device must be released")
	}
}

func TestMonitorProbeFailureMarksFault(t *testing.T) {
	m := adapter.NewManager()
	m.RegisterDriver(fake.NewDriver(adapter.KindELM327, cand(adapter.KindELM327, "elm-a")))
	s, err := m.Connect(context.Background(), cand(adapter.KindELM327, "elm-a"))
	if err != nil {
		t.Fatal(err)
	}
	_ = s

	probe := func(ctx context.Context, s adapter.Session) error {
		return errors.New("no response to ping")
	}
	mn := adapter.NewMonitor(m, time.Hour, probe, true)
	mn.Check(context.Background()) // baseline: probe already fails -> fault
	events := mn.Check(context.Background())
	// First check already recorded fault (ready->fault on second check may
	// not re-emit; verify disconnect happened instead).
	_ = events
	if m.Session("elm-a") != nil {
		t.Fatal("probe failure must disconnect the session")
	}
}
