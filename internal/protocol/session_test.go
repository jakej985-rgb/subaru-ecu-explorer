package protocol_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jakej985-rgb/subaru-ecu-explorer/internal/adapter"
	"github.com/jakej985-rgb/subaru-ecu-explorer/internal/adapter/fake"
	"github.com/jakej985-rgb/subaru-ecu-explorer/internal/protocol"
)

// lineCodec is a test codec: requests marshal to "svc:payload\n", replies
// are ">payload\n" lines.
func lineCodec() protocol.Codec {
	return protocol.CodecFunc{
		NameFunc: func() string { return "test-line" },
		MarshalFunc: func(req protocol.Request) ([]byte, error) {
			return []byte{'>', '>', req.Service, req.Payload[0], '\n'}, nil
		},
		UnmarshalFn: func(buf []byte) (protocol.Message, int, error) {
			for i, c := range buf {
				if c == '\n' {
					if i == 0 {
						return protocol.Message{}, 0, errors.New("empty line")
					}
					return protocol.Message{
						Raw:     append([]byte(nil), buf[:i+1]...),
						Payload: append([]byte(nil), buf[:i]...),
					}, i + 1, nil
				}
			}
			return protocol.Message{}, 0, protocol.ErrIncomplete
		},
	}
}

func TestTransactRoundTrip(t *testing.T) {
	tr := fake.NewTransport()
	s := protocol.NewSession(lineCodec(), tr, protocol.Options{IdleTimeout: 20 * time.Millisecond})

	go func() {
		// Wait for the request bytes, then reply with two lines.
		time.Sleep(10 * time.Millisecond)
		tr.Feed([]byte("reply-one\nreply-two\n"))
	}()

	resp, err := s.Transact(context.Background(), protocol.Request{Service: 0x01, Payload: []byte{0xAA}})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Status != protocol.StatusOK {
		t.Fatalf("status %s", resp.Status)
	}
	if len(resp.Messages) != 3 { // 1 host + 2 ecu
		t.Fatalf("messages %d, want 3", len(resp.Messages))
	}
	if resp.Messages[0].Direction != protocol.DirectionHostToEcu {
		t.Fatal("first message should be host->ecu")
	}
	if resp.Messages[1].Direction != protocol.DirectionEcuToHost {
		t.Fatal("reply should be ecu->host")
	}
	if string(resp.Answers[0].Payload) != "reply-one" {
		t.Fatalf("answer payload %q", resp.Answers[0].Payload)
	}
	// Raw wire bytes preserved on every message.
	if len(resp.Messages[1].Raw) == 0 {
		t.Fatal("raw bytes not preserved")
	}
	// Request bytes were written to the transport.
	if len(tr.Written()) == 0 || tr.Written()[0] != '>' {
		t.Fatalf("unexpected written bytes %v", tr.Written())
	}
}

func TestTransactTimeout(t *testing.T) {
	tr := fake.NewTransport()
	s := protocol.NewSession(lineCodec(), tr, protocol.Options{
		ResponseTimeout: 100 * time.Millisecond,
		IdleTimeout:     20 * time.Millisecond,
	})
	// Never feed a reply.
	_, err := s.Transact(context.Background(), protocol.Request{Service: 0x01, Payload: []byte{1}})
	if !errors.Is(err, protocol.ErrTimeout) {
		t.Fatalf("expected ErrTimeout, got %v", err)
	}
}

func TestTransactRequestTimeoutOverrides(t *testing.T) {
	tr := fake.NewTransport()
	s := protocol.NewSession(lineCodec(), tr, protocol.Options{ResponseTimeout: 5 * time.Second})
	start := time.Now()
	_, err := s.Transact(context.Background(), protocol.Request{
		Service: 1, Payload: []byte{1}, Timeout: 80 * time.Millisecond,
	})
	if !errors.Is(err, protocol.ErrTimeout) {
		t.Fatalf("expected ErrTimeout, got %v", err)
	}
	if time.Since(start) > 2*time.Second {
		t.Fatal("request timeout not honored")
	}
}

func TestTransactIdleTimeoutEndsExchange(t *testing.T) {
	tr := fake.NewTransport()
	s := protocol.NewSession(lineCodec(), tr, protocol.Options{
		ResponseTimeout: 2 * time.Second,
		IdleTimeout:     50 * time.Millisecond,
	})
	go func() {
		time.Sleep(10 * time.Millisecond)
		tr.Feed([]byte("one\n")) // single reply, then silence
	}()
	start := time.Now()
	resp, err := s.Transact(context.Background(), protocol.Request{Service: 1, Payload: []byte{1}})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Status != protocol.StatusOK {
		t.Fatalf("status %s", resp.Status)
	}
	if el := time.Since(start); el > time.Second {
		t.Fatalf("exchange took %s, idle timeout not honored", el)
	}
}

func TestTransactMalformedCodecError(t *testing.T) {
	bad := protocol.CodecFunc{
		NameFunc:    func() string { return "bad" },
		MarshalFunc: func(r protocol.Request) ([]byte, error) { return []byte("q\n"), nil },
		UnmarshalFn: func(buf []byte) (protocol.Message, int, error) {
			return protocol.Message{}, 0, protocol.ErrMalformed
		},
	}
	tr := fake.NewTransport()
	s := protocol.NewSession(bad, tr, protocol.Options{})
	go func() { time.Sleep(5 * time.Millisecond); tr.Feed([]byte("x\n")) }()
	_, err := s.Transact(context.Background(), protocol.Request{Service: 1, Payload: []byte{1}})
	if !errors.Is(err, protocol.ErrMalformed) {
		t.Fatalf("expected ErrMalformed, got %v", err)
	}
}

func TestTransactUnsupportedMarshal(t *testing.T) {
	uns := protocol.CodecFunc{
		NameFunc:    func() string { return "uns" },
		MarshalFunc: func(r protocol.Request) ([]byte, error) { return nil, protocol.ErrUnsupported },
		UnmarshalFn: func(buf []byte) (protocol.Message, int, error) {
			return protocol.Message{}, 0, protocol.ErrIncomplete
		},
	}
	tr := fake.NewTransport()
	s := protocol.NewSession(uns, tr, protocol.Options{})
	resp, err := s.Transact(context.Background(), protocol.Request{Service: 1})
	if !errors.Is(err, protocol.ErrUnsupported) {
		t.Fatalf("expected ErrUnsupported, got %v", err)
	}
	if resp.Status != protocol.StatusUnsupported {
		t.Fatalf("status %s", resp.Status)
	}
}

func TestTransactCompletePredicate(t *testing.T) {
	tr := fake.NewTransport()
	s := protocol.NewSession(lineCodec(), tr, protocol.Options{
		IdleTimeout: 500 * time.Millisecond, // would wait long without predicate
		Complete: func(msgs []protocol.Message) bool {
			return len(msgs) >= 1 // stop after first answer despite buffered extras
		},
	})
	go func() {
		time.Sleep(10 * time.Millisecond)
		tr.Feed([]byte("first\nsecond\nthird\n"))
	}()
	start := time.Now()
	resp, err := s.Transact(context.Background(), protocol.Request{Service: 1, Payload: []byte{1}})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Answers) != 1 {
		t.Fatalf("answers %d, want 1 (early completion)", len(resp.Answers))
	}
	if time.Since(start) > 300*time.Millisecond {
		t.Fatal("completion predicate not honored")
	}
}

func TestTransactMaxBytesCap(t *testing.T) {
	tr := fake.NewTransport()
	s := protocol.NewSession(lineCodec(), tr, protocol.Options{MaxBytes: 16})
	go func() {
		time.Sleep(5 * time.Millisecond)
		// Feed more than the cap in one burst.
		tr.Feed([]byte("0123456789abcdefEXTRA\n"))
	}()
	_, err := s.Transact(context.Background(), protocol.Request{Service: 1, Payload: []byte{1}})
	if err == nil || !errors.Is(err, protocol.ErrMalformed) {
		t.Fatalf("expected ErrMalformed on cap, got %v", err)
	}
}

func TestTransactContextCancel(t *testing.T) {
	tr := fake.NewTransport()
	s := protocol.NewSession(lineCodec(), tr, protocol.Options{ResponseTimeout: 10 * time.Second})
	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(20 * time.Millisecond); cancel() }()
	_, err := s.Transact(ctx, protocol.Request{Service: 1, Payload: []byte{1}})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestNegativeResponseError(t *testing.T) {
	e := &protocol.NegativeResponseError{Code: 0x7F, Detail: "service not supported"}
	if e.Error() == "" {
		t.Fatal("empty error string")
	}
}

func TestNoTransport(t *testing.T) {
	s := protocol.NewSession(lineCodec(), nil, protocol.Options{})
	_, err := s.Transact(context.Background(), protocol.Request{Service: 1})
	if !errors.Is(err, protocol.ErrNoTransport) {
		t.Fatalf("expected ErrNoTransport, got %v", err)
	}
}

func TestTransactMaxBytesAcrossCompleteFrames(t *testing.T) {
	tr := fake.NewTransport()
	s := protocol.NewSession(lineCodec(), tr, protocol.Options{
		MaxBytes:    4,
		IdleTimeout: 200 * time.Millisecond,
	})
	go func() {
		// Each frame decodes fully, so the parse buffer drains between
		// reads; only cumulative accounting catches the overflow.
		time.Sleep(5 * time.Millisecond)
		tr.Feed([]byte("a\n"))
		time.Sleep(10 * time.Millisecond)
		tr.Feed([]byte("b\n"))
		time.Sleep(10 * time.Millisecond)
		tr.Feed([]byte("c\n"))
	}()
	_, err := s.Transact(context.Background(), protocol.Request{Service: 1, Payload: []byte{1}})
	if !errors.Is(err, protocol.ErrMalformed) {
		t.Fatalf("expected ErrMalformed on cumulative cap, got %v", err)
	}
}

func TestTransactMaxBytesSingleOversizedRead(t *testing.T) {
	tr := fake.NewTransport()
	s := protocol.NewSession(lineCodec(), tr, protocol.Options{
		MaxBytes:    4,
		IdleTimeout: 50 * time.Millisecond,
	})
	go func() {
		time.Sleep(5 * time.Millisecond)
		tr.Feed([]byte("a\nb\nc\n")) // 6 bytes, cap 4
	}()
	_, err := s.Transact(context.Background(), protocol.Request{Service: 1, Payload: []byte{1}})
	if !errors.Is(err, protocol.ErrMalformed) {
		t.Fatalf("expected ErrMalformed, not silent truncation; got %v", err)
	}
}

func TestTransactPreCanceledContextDoesNotWrite(t *testing.T) {
	tr := fake.NewTransport()
	s := protocol.NewSession(lineCodec(), tr, protocol.Options{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := s.Transact(ctx, protocol.Request{Service: 1, Payload: []byte{1}})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	if n := len(tr.Written()); n != 0 {
		t.Fatalf("request written despite canceled context (%d bytes)", n)
	}
}

func TestTransactCancelInterruptsBlockedRead(t *testing.T) {
	tr := fake.NewTransport()
	s := protocol.NewSession(lineCodec(), tr, protocol.Options{ResponseTimeout: 10 * time.Second})
	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(20 * time.Millisecond); cancel() }()
	start := time.Now()
	_, err := s.Transact(ctx, protocol.Request{Service: 1, Payload: []byte{1}})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	if el := time.Since(start); el > 500*time.Millisecond {
		t.Fatalf("cancellation took %s; blocked read not interrupted", el)
	}
}

func TestTransactCancelDoesNotLeakIntoNextExchange(t *testing.T) {
	tr := fake.NewTransport()
	s := protocol.NewSession(lineCodec(), tr, protocol.Options{IdleTimeout: 20 * time.Millisecond})
	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(10 * time.Millisecond); cancel() }()
	_, _ = s.Transact(ctx, protocol.Request{Service: 1, Payload: []byte{1}})

	go func() { time.Sleep(10 * time.Millisecond); tr.Feed([]byte("ok\n")) }()
	resp, err := s.Transact(context.Background(), protocol.Request{Service: 1, Payload: []byte{1}})
	if err != nil {
		t.Fatalf("second exchange failed: %v", err)
	}
	if resp.Status != protocol.StatusOK {
		t.Fatalf("status %s", resp.Status)
	}
}

func TestAdapterTransportSatisfiesByteTransport(t *testing.T) {
	var tr adapter.Transport = fake.NewTransport()
	var _ protocol.ByteTransport = tr
}
