package safety

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jakej985-rgb/subaru-ecu-explorer/internal/adapter"
	"github.com/jakej985-rgb/subaru-ecu-explorer/internal/adapter/fake"
)

func gatedSession(t *testing.T) (*GatedSession, *fake.Transport) {
	t.Helper()
	tr := fake.NewTransport()
	info := adapter.Info{ID: "dev1", Kind: adapter.KindELM327}
	s := fake.NewSession(info, adapter.Capabilities{ReadOnly: true, Transport: "fake"}, tr)
	return Wrap(s, nil), tr
}

func TestProhibitedOperationsWriteNothing(t *testing.T) {
	prohibited := []Operation{
		OpWrite, OpClearDTC, OpActuatorTest, OpProgramming,
		OpFlash, OpSecurityAccess, OpImmobilizer, OpPinProbe,
		Operation("arbitrary-command"), // unknown ops are denied too
	}
	for _, op := range prohibited {
		g, tr := gatedSession(t)
		resp, err := g.Transact(context.Background(), op, []byte("payload"))
		if !errors.Is(err, ErrProhibitedOperation) {
			t.Fatalf("op %s: expected ErrProhibitedOperation, got %v", op, err)
		}
		if resp != nil {
			t.Fatalf("op %s: expected nil response, got %v", op, resp)
		}
		if got := tr.Written(); len(got) != 0 {
			t.Fatalf("op %s: prohibited op wrote %d bytes to transport", op, len(got))
		}
	}
}

func TestAllowedOperationsTransact(t *testing.T) {
	for _, op := range []Operation{OpIdentify, OpReadData, OpReadDTC, OpReadMeasurement} {
		g, tr := gatedSession(t)
		tr.Feed([]byte("OK\r"))
		resp, err := g.Transact(context.Background(), op, []byte("01 00\r"))
		if err != nil {
			t.Fatalf("op %s: %v", op, err)
		}
		if !strings.Contains(string(resp), "OK") {
			t.Fatalf("op %s: unexpected response %q", op, resp)
		}
		if got := string(tr.Written()); got != "01 00\r" {
			t.Fatalf("op %s: unexpected request bytes %q", op, got)
		}
	}
}

func TestNonReadOnlySessionRejected(t *testing.T) {
	tr := fake.NewTransport()
	s := fake.NewSession(adapter.Info{ID: "dev", Kind: adapter.KindELM327},
		adapter.Capabilities{ReadOnly: false, Transport: "fake"}, tr)
	g := Wrap(s, nil)
	if _, err := g.Transact(context.Background(), OpIdentify, []byte("x")); !errors.Is(err, ErrReadOnlyRequired) {
		t.Fatalf("expected ErrReadOnlyRequired, got %v", err)
	}
	if len(tr.Written()) != 0 {
		t.Fatal("rejected session must not write")
	}
}

func TestHealthGate(t *testing.T) {
	g, _ := gatedSession(t)
	// Degraded health is not in the default allowlist of states.
	inner := g.Inner().(*fake.Session)
	inner.SetHealth(adapter.Health{State: adapter.HealthDegraded})
	if _, err := g.Transact(context.Background(), OpIdentify, []byte("x")); !errors.Is(err, ErrHealthGate) {
		t.Fatalf("expected ErrHealthGate, got %v", err)
	}
}

func TestRateLimit(t *testing.T) {
	g, tr := gatedSession(t)
	policy := &Policy{MinInterval: 100 * time.Millisecond, ResponseTimeout: 10 * time.Millisecond}
	g = Wrap(g.Inner(), policy)

	if _, err := g.Transact(context.Background(), OpIdentify, []byte("a")); err != nil {
		t.Fatal(err)
	}
	if _, err := g.Transact(context.Background(), OpIdentify, []byte("b")); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("expected ErrRateLimited, got %v", err)
	}
	if got := tr.Written(); string(got) != "a" {
		t.Fatalf("rate-limited op wrote bytes: %q", got)
	}
	time.Sleep(110 * time.Millisecond)
	if _, err := g.Transact(context.Background(), OpIdentify, []byte("c")); err != nil {
		t.Fatalf("expected success after interval, got %v", err)
	}
}

func TestPreconditionFailureBlocksIO(t *testing.T) {
	g, tr := gatedSession(t)
	policy := &Policy{
		ResponseTimeout: 10 * time.Millisecond,
		Preconditions: []Precondition{
			func(ctx context.Context, s adapter.Session) error {
				return errors.New("voltage unstable")
			},
		},
	}
	g = Wrap(g.Inner(), policy)
	if _, err := g.Transact(context.Background(), OpIdentify, []byte("x")); !errors.Is(err, ErrPrecondition) {
		t.Fatalf("expected ErrPrecondition, got %v", err)
	}
	if len(tr.Written()) != 0 {
		t.Fatal("precondition failure must not write")
	}
}

func TestContextCancellation(t *testing.T) {
	g, tr := gatedSession(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := g.Transact(ctx, OpIdentify, []byte("x")); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	if len(tr.Written()) != 0 {
		t.Fatal("cancelled op must not write")
	}
}

func TestAuthorizeDoesNotPerformIO(t *testing.T) {
	g, tr := gatedSession(t)
	if err := g.Authorize(context.Background(), OpIdentify); err != nil {
		t.Fatal(err)
	}
	if err := g.Authorize(context.Background(), OpFlash); !errors.Is(err, ErrProhibitedOperation) {
		t.Fatalf("expected prohibited, got %v", err)
	}
	if len(tr.Written()) != 0 {
		t.Fatal("Authorize must not write")
	}
}

// echoSession replies to each request with the request bytes, simulating
// a device that answers every request.
// It also records any Write issued while a previous exchange is still
// awaiting its response, which deterministically detects overlap.
type echoSession struct {
	*fake.Session
	tr *fake.Transport

	mu       sync.Mutex
	inflight int
	overlaps int
}

func (e *echoSession) Write(p []byte) (int, error) {
	e.mu.Lock()
	e.inflight++
	if e.inflight > 1 {
		e.overlaps++
	}
	e.mu.Unlock()
	n, err := e.Session.Write(p)
	if err == nil {
		req := append([]byte(nil), p...)
		// Delay the reply to widen the window for overlapping callers.
		time.AfterFunc(5*time.Millisecond, func() { e.tr.Feed(req) })
	}
	return n, err
}

func (e *echoSession) Read(p []byte) (int, error) {
	n, err := e.Session.Read(p)
	if n > 0 {
		e.mu.Lock()
		e.inflight--
		e.mu.Unlock()
	}
	return n, err
}

func TestConcurrentTransactionsKeepResponseOwnership(t *testing.T) {
	tr := fake.NewTransport()
	s := fake.NewSession(adapter.Info{ID: "dev", Kind: adapter.KindELM327},
		adapter.Capabilities{ReadOnly: true, Transport: "fake"}, tr)
	echo := &echoSession{Session: s, tr: tr}
	g := Wrap(echo, &Policy{
		ResponseTimeout:     time.Second,
		ResponseIdleTimeout: 10 * time.Millisecond,
	})

	reqs := []string{"AAAA", "BBBB", "CCCC", "DDDD", "EEEE", "FFFF"}
	var wg sync.WaitGroup
	errs := make(chan error, len(reqs))
	for _, r := range reqs {
		wg.Add(1)
		go func(req string) {
			defer wg.Done()
			resp, err := g.Transact(context.Background(), OpReadData, []byte(req))
			if err != nil {
				errs <- err
				return
			}
			if string(resp) != req {
				errs <- errors.New("request " + req + " got response " + string(resp))
			}
		}(r)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
	if echo.overlaps != 0 {
		t.Fatalf("%d requests were written while another exchange was in flight", echo.overlaps)
	}
}

func TestOversizedResponseDoesNotLeakIntoNext(t *testing.T) {
	tr := fake.NewTransport()
	s := fake.NewSession(adapter.Info{ID: "dev", Kind: adapter.KindELM327},
		adapter.Capabilities{ReadOnly: true, Transport: "fake"}, tr)
	g := Wrap(s, &Policy{
		MaxResponseBytes:    8,
		ResponseTimeout:     500 * time.Millisecond,
		ResponseIdleTimeout: 10 * time.Millisecond,
	})

	tr.Feed([]byte("0123456789ABCDEFGHIJ"))
	if _, err := g.Transact(context.Background(), OpReadData, []byte("a")); !errors.Is(err, ErrResponseTooLarge) {
		t.Fatalf("expected ErrResponseTooLarge, got %v", err)
	}

	tr.Feed([]byte("OK"))
	resp, err := g.Transact(context.Background(), OpReadData, []byte("b"))
	if err != nil {
		t.Fatal(err)
	}
	if string(resp) != "OK" {
		t.Fatalf("stale bytes leaked into next response: %q", resp)
	}
}

func TestCancellationDuringBlockedRead(t *testing.T) {
	tr := fake.NewTransport()
	s := fake.NewSession(adapter.Info{ID: "dev", Kind: adapter.KindELM327},
		adapter.Capabilities{ReadOnly: true, Transport: "fake"}, tr)
	g := Wrap(s, &Policy{ResponseTimeout: 5 * time.Second})

	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(20*time.Millisecond, cancel)
	start := time.Now()
	_, err := g.Transact(ctx, OpReadData, []byte("x"))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	if el := time.Since(start); el > time.Second {
		t.Fatalf("cancellation did not interrupt read; took %s", el)
	}
}

func TestQueuedCallerHonorsCancellation(t *testing.T) {
	tr := fake.NewTransport()
	s := fake.NewSession(adapter.Info{ID: "dev", Kind: adapter.KindELM327},
		adapter.Capabilities{ReadOnly: true, Transport: "fake"}, tr)
	g := Wrap(s, &Policy{ResponseTimeout: 2 * time.Second})

	// A occupies the session waiting on a silent device.
	go func() { _, _ = g.Transact(context.Background(), OpReadData, []byte("a")) }()
	time.Sleep(20 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := g.Transact(ctx, OpReadData, []byte("b")); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected context.DeadlineExceeded, got %v", err)
	}
	if el := time.Since(start); el > 500*time.Millisecond {
		t.Fatalf("queued caller waited for in-flight exchange; took %s", el)
	}
}

func TestCancelledPartialResponseDoesNotLeakIntoNext(t *testing.T) {
	tr := fake.NewTransport()
	s := fake.NewSession(adapter.Info{ID: "dev", Kind: adapter.KindELM327},
		adapter.Capabilities{ReadOnly: true, Transport: "fake"}, tr)
	g := Wrap(s, &Policy{
		ResponseTimeout:     2 * time.Second,
		ResponseIdleTimeout: 200 * time.Millisecond,
	})

	// A receives a partial reply, then is cancelled mid-response.
	tr.Feed([]byte("AB"))
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(30*time.Millisecond, cancel)
	if _, err := g.Transact(ctx, OpReadData, []byte("a")); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}

	// The rest of A's reply arrives late; B must not see it.
	tr.Feed([]byte("CD"))
	time.AfterFunc(300*time.Millisecond, func() { tr.Feed([]byte("OK")) })
	resp, err := g.Transact(context.Background(), OpReadData, []byte("b"))
	if err != nil {
		t.Fatal(err)
	}
	if string(resp) != "OK" {
		t.Fatalf("stale bytes leaked into next response: %q", resp)
	}
}

func TestResyncWithShortResponseTimeout(t *testing.T) {
	tr := fake.NewTransport()
	s := fake.NewSession(adapter.Info{ID: "dev", Kind: adapter.KindELM327},
		adapter.Capabilities{ReadOnly: true, Transport: "fake"}, tr)
	// ResponseTimeout below the default 100ms idle timeout.
	g := Wrap(s, &Policy{ResponseTimeout: 50 * time.Millisecond})

	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(10*time.Millisecond, cancel)
	if _, err := g.Transact(ctx, OpReadData, []byte("a")); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}

	// The device is silent; recovery must succeed and the request be sent.
	if _, err := g.Transact(context.Background(), OpReadData, []byte("b")); err != nil {
		t.Fatalf("expected recovery, got %v", err)
	}
	if got := string(tr.Written()); got != "ab" {
		t.Fatalf("expected retry to write request, wrote %q", got)
	}
}

func TestContextDeadlineExpiryIsError(t *testing.T) {
	g, _ := gatedSession(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := g.Transact(ctx, OpReadData, []byte("x")); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected context.DeadlineExceeded, got %v", err)
	}
}

func TestPreconditionCancellingContextBlocksWrite(t *testing.T) {
	tr := fake.NewTransport()
	s := fake.NewSession(adapter.Info{ID: "dev", Kind: adapter.KindELM327},
		adapter.Capabilities{ReadOnly: true, Transport: "fake"}, tr)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	g := Wrap(s, &Policy{
		Preconditions: []Precondition{
			func(context.Context, adapter.Session) error {
				cancel()
				return nil
			},
		},
	})
	if _, err := g.Transact(ctx, OpReadData, []byte("x")); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	if len(tr.Written()) != 0 {
		t.Fatal("cancelled-in-precondition op must not write")
	}
}
