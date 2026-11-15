package safety

import (
	"context"
	"errors"
	"strings"
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
